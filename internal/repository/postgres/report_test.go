package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testforge/testforge/internal/domain"
)

func TestReportRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := NewTenantRepository(db)
	projectRepo := NewProjectRepository(db)
	runRepo := NewTestRunRepository(db)
	reportRepo := NewReportRepository(db)
	ctx := context.Background()

	// Helper to create a tenant, project, and test run for tests
	createTestRunWithDeps := func(t *testing.T) (*domain.Tenant, *domain.Project, *domain.TestRun) {
		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Test Tenant",
			Slug:     uuid.New().String()[:8],
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{},
		}
		tenant.SetTimestamps()
		err := tenantRepo.Create(ctx, tenant)
		require.NoError(t, err)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "Test Project",
			Description: "A test project",
			BaseURL:     "https://example.com",
			Settings:    domain.ProjectSettings{},
		}
		project.SetTimestamps()
		err = projectRepo.Create(ctx, project)
		require.NoError(t, err)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
		err = runRepo.Create(ctx, run)
		require.NoError(t, err)

		return tenant, project, run
	}

	t.Run("Create", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		report := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFull,
			domain.ReportFormatHTML,
			"https://reports.example.com/report123.html",
			1024,
			domain.ReportSummary{
				TestRunID:  run.ID,
				TotalTests: 10,
				Passed:     8,
				Failed:     2,
				PassRate:   80.0,
			},
		)

		err := reportRepo.Create(ctx, report)
		require.NoError(t, err)

		// Verify it was created
		fetched, err := reportRepo.GetByID(ctx, report.ID)
		require.NoError(t, err)
		assert.Equal(t, report.ID, fetched.ID)
		assert.Equal(t, run.ID, fetched.TestRunID)
		assert.Equal(t, tenant.ID, fetched.TenantID)
		assert.Equal(t, domain.ReportTypeFull, fetched.Type)
		assert.Equal(t, domain.ReportFormatHTML, fetched.Format)
		assert.Equal(t, "https://reports.example.com/report123.html", fetched.URL)
		assert.Equal(t, int64(1024), fetched.Size)
		assert.Equal(t, 10, fetched.Summary.TotalTests)
		assert.Equal(t, 8, fetched.Summary.Passed)
	})

	t.Run("Create_InvalidTestRun", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, _ := createTestRunWithDeps(t)

		report := domain.NewReport(
			uuid.New(), // Non-existent test run
			tenant.ID,
			domain.ReportTypeSummary,
			domain.ReportFormatJSON,
			"https://reports.example.com/invalid.json",
			512,
			domain.ReportSummary{},
		)

		err := reportRepo.Create(ctx, report)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		report := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFailures,
			domain.ReportFormatPDF,
			"https://reports.example.com/failures.pdf",
			2048,
			domain.ReportSummary{
				TotalTests: 5,
				Failed:     5,
				PassRate:   0.0,
			},
		)

		err := reportRepo.Create(ctx, report)
		require.NoError(t, err)

		fetched, err := reportRepo.GetByID(ctx, report.ID)
		require.NoError(t, err)
		assert.Equal(t, report.ID, fetched.ID)
		assert.Equal(t, domain.ReportTypeFailures, fetched.Type)
		assert.Equal(t, domain.ReportFormatPDF, fetched.Format)
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := reportRepo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByTestRunID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		// Create multiple reports for the same test run
		formats := []domain.ReportFormat{
			domain.ReportFormatHTML,
			domain.ReportFormatPDF,
			domain.ReportFormatJSON,
		}

		for _, format := range formats {
			report := domain.NewReport(
				run.ID,
				tenant.ID,
				domain.ReportTypeFull,
				format,
				"https://reports.example.com/report."+string(format),
				1024,
				domain.ReportSummary{TotalTests: 10},
			)
			err := reportRepo.Create(ctx, report)
			require.NoError(t, err)
		}

		reports, err := reportRepo.GetByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, reports, 3)
	})

	t.Run("GetByTestRunID_Empty", func(t *testing.T) {
		testDB.TruncateTables(t)
		_, _, run := createTestRunWithDeps(t)

		reports, err := reportRepo.GetByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, reports, 0)
	})

	t.Run("GetLatestByProject", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project, _ := createTestRunWithDeps(t)

		// Create multiple test runs and reports
		for i := 0; i < 5; i++ {
			run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
			err := runRepo.Create(ctx, run)
			require.NoError(t, err)

			report := domain.NewReport(
				run.ID,
				tenant.ID,
				domain.ReportTypeFull,
				domain.ReportFormatHTML,
				"https://reports.example.com/report.html",
				1024,
				domain.ReportSummary{TotalTests: i + 1},
			)
			err = reportRepo.Create(ctx, report)
			require.NoError(t, err)
		}

		reports, err := reportRepo.GetLatestByProject(ctx, project.ID, 3)
		require.NoError(t, err)
		assert.Len(t, reports, 3)
		// Should be ordered by generated_at DESC
	})

	t.Run("Delete", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		report := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFull,
			domain.ReportFormatHTML,
			"https://reports.example.com/todelete.html",
			1024,
			domain.ReportSummary{},
		)

		err := reportRepo.Create(ctx, report)
		require.NoError(t, err)

		// Delete the report
		err = reportRepo.Delete(ctx, report.ID)
		require.NoError(t, err)

		// Verify it's soft deleted (not found)
		_, err = reportRepo.GetByID(ctx, report.ID)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := reportRepo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("DeleteExpired", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		// Create an expired report
		expiredReport := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFull,
			domain.ReportFormatHTML,
			"https://reports.example.com/expired.html",
			1024,
			domain.ReportSummary{},
		)
		expiredTime := time.Now().Add(-time.Hour) // Expired 1 hour ago
		expiredReport.ExpiresAt = &expiredTime

		err := reportRepo.Create(ctx, expiredReport)
		require.NoError(t, err)

		// Create a non-expired report
		validReport := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFull,
			domain.ReportFormatHTML,
			"https://reports.example.com/valid.html",
			1024,
			domain.ReportSummary{},
		)
		futureTime := time.Now().Add(time.Hour * 24) // Expires tomorrow
		validReport.ExpiresAt = &futureTime

		err = reportRepo.Create(ctx, validReport)
		require.NoError(t, err)

		// Delete expired reports
		count, err := reportRepo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		// Verify expired report is deleted
		_, err = reportRepo.GetByID(ctx, expiredReport.ID)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))

		// Verify valid report still exists
		fetched, err := reportRepo.GetByID(ctx, validReport.ID)
		require.NoError(t, err)
		assert.Equal(t, validReport.ID, fetched.ID)
	})

	t.Run("Create_WithExpiration", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		expiresAt := time.Now().Add(24 * time.Hour)
		report := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFull,
			domain.ReportFormatHTML,
			"https://reports.example.com/expiring.html",
			1024,
			domain.ReportSummary{},
		)
		report.ExpiresAt = &expiresAt

		err := reportRepo.Create(ctx, report)
		require.NoError(t, err)

		fetched, err := reportRepo.GetByID(ctx, report.ID)
		require.NoError(t, err)
		assert.NotNil(t, fetched.ExpiresAt)
		assert.WithinDuration(t, expiresAt, *fetched.ExpiresAt, time.Second)
	})

	t.Run("Create_WithFullSummary", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		summary := domain.ReportSummary{
			TestRunID:   run.ID,
			ProjectName: "Test Project",
			TargetURL:   "https://example.com",
			ExecutedAt:  time.Now(),
			Duration:    5 * time.Minute,
			TotalTests:  100,
			Passed:      85,
			Failed:      10,
			Skipped:     3,
			Healed:      2,
			PassRate:    85.0,
			ByCategory: map[string]domain.CategoryStats{
				"smoke": {Total: 20, Passed: 18, Failed: 2, PassRate: 90.0},
				"e2e":   {Total: 80, Passed: 67, Failed: 8, PassRate: 83.75},
			},
			ByPriority: map[domain.Priority]domain.PriorityStats{
				domain.PriorityCritical: {Total: 10, Passed: 10, Failed: 0, PassRate: 100.0},
				domain.PriorityHigh:     {Total: 30, Passed: 25, Failed: 5, PassRate: 83.33},
			},
		}

		report := domain.NewReport(
			run.ID,
			tenant.ID,
			domain.ReportTypeFull,
			domain.ReportFormatJSON,
			"https://reports.example.com/full.json",
			4096,
			summary,
		)

		err := reportRepo.Create(ctx, report)
		require.NoError(t, err)

		fetched, err := reportRepo.GetByID(ctx, report.ID)
		require.NoError(t, err)
		assert.Equal(t, 100, fetched.Summary.TotalTests)
		assert.Equal(t, 85, fetched.Summary.Passed)
		assert.Equal(t, 85.0, fetched.Summary.PassRate)
		assert.Len(t, fetched.Summary.ByCategory, 2)
		assert.Len(t, fetched.Summary.ByPriority, 2)
	})
}
