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

func TestTestRunRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := NewTenantRepository(db)
	projectRepo := NewProjectRepository(db)
	runRepo := NewTestRunRepository(db)
	ctx := context.Background()

	// Helper to create a tenant and project for tests
	createTestTenantAndProject := func(t *testing.T) (*domain.Tenant, *domain.Project) {
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

		return tenant, project
	}

	t.Run("Create", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")

		err := runRepo.Create(ctx, run)
		require.NoError(t, err)

		// Verify it was created
		fetched, err := runRepo.GetByID(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, run.ID, fetched.ID)
		assert.Equal(t, tenant.ID, fetched.TenantID)
		assert.Equal(t, project.ID, fetched.ProjectID)
		assert.Equal(t, "https://example.com", fetched.TargetURL)
		assert.Equal(t, domain.RunStatusPending, fetched.Status)
	})

	t.Run("GetByID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://test.com", "manual")
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)

		fetched, err := runRepo.GetByID(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, run.ID, fetched.ID)
		assert.Equal(t, "https://test.com", fetched.TargetURL)
		assert.Equal(t, "manual", fetched.TriggeredBy)
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := runRepo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByProjectID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		// Create multiple runs
		for i := 0; i < 5; i++ {
			run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
			err := runRepo.Create(ctx, run)
			require.NoError(t, err)
		}

		// List with pagination
		runs, total, err := runRepo.GetByProjectID(ctx, project.ID, 3, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, runs, 3)

		// Second page
		runs, total, err = runRepo.GetByProjectID(ctx, project.ID, 3, 3)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, runs, 2)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)

		// Update status through the workflow
		err = runRepo.UpdateStatus(ctx, run.ID, domain.RunStatusDiscovering)
		require.NoError(t, err)

		fetched, err := runRepo.GetByID(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.RunStatusDiscovering, fetched.Status)

		// Update to designing
		err = runRepo.UpdateStatus(ctx, run.ID, domain.RunStatusDesigning)
		require.NoError(t, err)

		fetched, err = runRepo.GetByID(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.RunStatusDesigning, fetched.Status)

		// Update to completed
		err = runRepo.UpdateStatus(ctx, run.ID, domain.RunStatusCompleted)
		require.NoError(t, err)

		fetched, err = runRepo.GetByID(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.RunStatusCompleted, fetched.Status)
	})

	t.Run("UpdateStatus_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := runRepo.UpdateStatus(ctx, uuid.New(), domain.RunStatusCompleted)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Update_WithSummary", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)

		// Update with summary
		run.Status = domain.RunStatusCompleted
		run.Summary = &domain.RunSummary{
			TotalTests: 10,
			Passed:     8,
			Failed:     1,
			Skipped:    1,
			PassRate:   80.0,
			Duration:   time.Minute,
		}
		run.ReportURL = "https://reports.example.com/run123"
		now := time.Now().UTC()
		run.CompletedAt = &now

		err = runRepo.Update(ctx, run)
		require.NoError(t, err)

		fetched, err := runRepo.GetByID(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.RunStatusCompleted, fetched.Status)
		assert.NotNil(t, fetched.Summary)
		assert.Equal(t, 10, fetched.Summary.TotalTests)
		assert.Equal(t, 8, fetched.Summary.Passed)
		assert.Equal(t, 80.0, fetched.Summary.PassRate)
		assert.Equal(t, "https://reports.example.com/run123", fetched.ReportURL)
	})

	t.Run("CountActiveByTenant", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		// Create some runs with different statuses
		statuses := []domain.RunStatus{
			domain.RunStatusPending,
			domain.RunStatusDiscovering,
			domain.RunStatusExecuting,
			domain.RunStatusCompleted,
			domain.RunStatusFailed,
		}

		for _, status := range statuses {
			run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
			err := runRepo.Create(ctx, run)
			require.NoError(t, err)
			err = runRepo.UpdateStatus(ctx, run.ID, status)
			require.NoError(t, err)
		}

		// Count active runs (pending, discovering, designing, automating, executing, healing, reporting)
		count, err := runRepo.CountActiveByTenant(ctx, tenant.ID)
		require.NoError(t, err)
		// pending, discovering, executing are active (3)
		assert.Equal(t, 3, count)
	})

	t.Run("Delete", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
		run.Status = domain.RunStatusCompleted // Only terminal runs can be deleted
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		err = runRepo.UpdateStatus(ctx, run.ID, domain.RunStatusCompleted)
		require.NoError(t, err)

		// Delete the run
		err = runRepo.Delete(ctx, run.ID)
		require.NoError(t, err)

		// Verify it's soft deleted
		_, err = runRepo.GetByID(ctx, run.ID)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByWorkflowID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, project := createTestTenantAndProject(t)

		run := domain.NewTestRun(tenant.ID, project.ID, "https://example.com", "api")
		workflowID := "testrun-" + run.ID.String()
		run.WorkflowID = workflowID
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)

		fetched, err := runRepo.GetByWorkflowID(ctx, workflowID)
		require.NoError(t, err)
		assert.Equal(t, run.ID, fetched.ID)
		assert.Equal(t, workflowID, fetched.WorkflowID)
	})

	t.Run("GetByWorkflowID_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := runRepo.GetByWorkflowID(ctx, "nonexistent-workflow")
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})
}
