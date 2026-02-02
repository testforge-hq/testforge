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

func TestTestCaseRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := NewTenantRepository(db)
	projectRepo := NewProjectRepository(db)
	runRepo := NewTestRunRepository(db)
	testCaseRepo := NewTestCaseRepository(db)
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

		steps := []domain.TestStep{
			{Order: 1, Type: "given", Action: "I am on the login page"},
			{Order: 2, Type: "when", Action: "I enter my credentials"},
			{Order: 3, Type: "then", Action: "I should see the dashboard"},
		}

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Login Test",
			"Verify user can login",
			"smoke",
			domain.PriorityHigh,
			steps,
		)

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)

		// Verify it was created
		fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.Equal(t, tc.ID, fetched.ID)
		assert.Equal(t, run.ID, fetched.TestRunID)
		assert.Equal(t, tenant.ID, fetched.TenantID)
		assert.Equal(t, "Login Test", fetched.Name)
		assert.Equal(t, "Verify user can login", fetched.Description)
		assert.Equal(t, "smoke", fetched.Category)
		assert.Equal(t, domain.PriorityHigh, fetched.Priority)
		assert.Equal(t, domain.TestCaseStatusPending, fetched.Status)
		assert.Len(t, fetched.Steps, 3)
	})

	t.Run("Create_InvalidTestRun", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, _ := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			uuid.New(), // Non-existent test run
			tenant.ID,
			"Invalid Test",
			"This should fail",
			"smoke",
			domain.PriorityMedium,
			[]domain.TestStep{},
		)

		err := testCaseRepo.Create(ctx, tc)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Get By ID Test",
			"Testing GetByID",
			"functional",
			domain.PriorityCritical,
			[]domain.TestStep{},
		)
		tc.Script = "test('example', async () => { /* script */ })"

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)

		fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.Equal(t, tc.ID, fetched.ID)
		assert.Equal(t, "Get By ID Test", fetched.Name)
		assert.Equal(t, tc.Script, fetched.Script)
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := testCaseRepo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByTestRunID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		// Create multiple test cases
		for i := 0; i < 5; i++ {
			tc := domain.NewTestCase(
				run.ID,
				tenant.ID,
				"Test Case",
				"Description",
				"regression",
				domain.PriorityMedium,
				nil,
			)
			err := testCaseRepo.Create(ctx, tc)
			require.NoError(t, err)
		}

		cases, err := testCaseRepo.GetByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, cases, 5)
	})

	t.Run("GetByTestRunID_Empty", func(t *testing.T) {
		testDB.TruncateTables(t)
		_, _, run := createTestRunWithDeps(t)

		cases, err := testCaseRepo.GetByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, cases, 0)
	})

	t.Run("CreateBatch", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		cases := make([]*domain.TestCase, 10)
		for i := 0; i < 10; i++ {
			cases[i] = domain.NewTestCase(
				run.ID,
				tenant.ID,
				"Batch Test Case",
				"Batch Description",
				"e2e",
				domain.PriorityLow,
				nil,
			)
		}

		err := testCaseRepo.CreateBatch(ctx, cases)
		require.NoError(t, err)

		fetched, err := testCaseRepo.GetByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, fetched, 10)
	})

	t.Run("CreateBatch_Empty", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := testCaseRepo.CreateBatch(ctx, []*domain.TestCase{})
		require.NoError(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Update Test",
			"Will be updated",
			"smoke",
			domain.PriorityHigh,
			[]domain.TestStep{
				{Order: 1, Type: "given", Action: "Original step"},
			},
		)

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)

		// Update the test case
		tc.Status = domain.TestCaseStatusPassed
		tc.Script = "updated script"
		tc.OriginalScript = "original script"
		tc.RetryCount = 2
		tc.Duration = 5 * time.Second
		tc.ExecutionResult = &domain.ExecutionResult{
			Passed:      true,
			StartedAt:   time.Now().Add(-5 * time.Second),
			CompletedAt: time.Now(),
		}
		tc.HealingHistory = []domain.HealingRecord{
			{
				Timestamp:        time.Now(),
				OriginalSelector: ".old-selector",
				HealedSelector:   ".new-selector",
				Strategy:         "attribute",
				Confidence:       0.95,
				Successful:       true,
			},
		}
		tc.Steps = []domain.TestStep{
			{Order: 1, Type: "given", Action: "Updated step"},
			{Order: 2, Type: "when", Action: "New step"},
		}

		err = testCaseRepo.Update(ctx, tc)
		require.NoError(t, err)

		// Verify the update
		fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TestCaseStatusPassed, fetched.Status)
		assert.Equal(t, "updated script", fetched.Script)
		assert.Equal(t, "original script", fetched.OriginalScript)
		assert.Equal(t, 2, fetched.RetryCount)
		assert.NotNil(t, fetched.ExecutionResult)
		assert.True(t, fetched.ExecutionResult.Passed)
		assert.Len(t, fetched.HealingHistory, 1)
		assert.Len(t, fetched.Steps, 2)
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := &domain.TestCase{
			ID:        uuid.New(),
			TestRunID: run.ID,
			TenantID:  tenant.ID,
			Name:      "Nonexistent",
			Status:    domain.TestCaseStatusPending,
		}

		err := testCaseRepo.Update(ctx, tc)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Status Update Test",
			"Testing status update",
			"functional",
			domain.PriorityMedium,
			[]domain.TestStep{},
		)

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)
		assert.Equal(t, domain.TestCaseStatusPending, tc.Status)

		// Update to running
		err = testCaseRepo.UpdateStatus(ctx, tc.ID, domain.TestCaseStatusRunning)
		require.NoError(t, err)

		fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TestCaseStatusRunning, fetched.Status)

		// Update to passed
		err = testCaseRepo.UpdateStatus(ctx, tc.ID, domain.TestCaseStatusPassed)
		require.NoError(t, err)

		fetched, err = testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TestCaseStatusPassed, fetched.Status)
	})

	t.Run("UpdateStatus_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := testCaseRepo.UpdateStatus(ctx, uuid.New(), domain.TestCaseStatusFailed)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Delete Test",
			"Will be deleted",
			"smoke",
			domain.PriorityLow,
			[]domain.TestStep{},
		)

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)

		// Delete the test case
		err = testCaseRepo.Delete(ctx, tc.ID)
		require.NoError(t, err)

		// Verify it's soft deleted (not found)
		_, err = testCaseRepo.GetByID(ctx, tc.ID)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := testCaseRepo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetFailedByTestRunID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		// Create test cases with different statuses
		statuses := []domain.TestCaseStatus{
			domain.TestCaseStatusPassed,
			domain.TestCaseStatusPassed,
			domain.TestCaseStatusFailed,
			domain.TestCaseStatusFailed,
			domain.TestCaseStatusSkipped,
		}

		priorities := []domain.Priority{
			domain.PriorityLow,
			domain.PriorityHigh,
			domain.PriorityCritical, // Failed - critical
			domain.PriorityMedium,   // Failed - medium
			domain.PriorityLow,
		}

		for i, status := range statuses {
			tc := domain.NewTestCase(
				run.ID,
				tenant.ID,
				"Test Case",
				"Description",
				"regression",
				priorities[i],
				nil,
			)
			err := testCaseRepo.Create(ctx, tc)
			require.NoError(t, err)
			err = testCaseRepo.UpdateStatus(ctx, tc.ID, status)
			require.NoError(t, err)
		}

		failed, err := testCaseRepo.GetFailedByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, failed, 2)

		// Should be ordered by priority DESC
		assert.Equal(t, domain.PriorityCritical, failed[0].Priority)
		assert.Equal(t, domain.PriorityMedium, failed[1].Priority)
	})

	t.Run("GetFailedByTestRunID_Empty", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		// Create only passing tests
		for i := 0; i < 3; i++ {
			tc := domain.NewTestCase(
				run.ID,
				tenant.ID,
				"Passing Test",
				"All pass",
				"smoke",
				domain.PriorityMedium,
				nil,
			)
			err := testCaseRepo.Create(ctx, tc)
			require.NoError(t, err)
			err = testCaseRepo.UpdateStatus(ctx, tc.ID, domain.TestCaseStatusPassed)
			require.NoError(t, err)
		}

		failed, err := testCaseRepo.GetFailedByTestRunID(ctx, run.ID)
		require.NoError(t, err)
		assert.Len(t, failed, 0)
	})

	t.Run("Create_WithExecutionResult", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Execution Result Test",
			"With result",
			"e2e",
			domain.PriorityHigh,
			[]domain.TestStep{},
		)

		startTime := time.Now().Add(-10 * time.Second)
		endTime := time.Now()
		tc.ExecutionResult = &domain.ExecutionResult{
			Passed:       false,
			ErrorMessage: "Element not found",
			ErrorStack:   "at line 42...",
			FailedStep:   intPtr(3),
			Screenshots: []domain.Screenshot{
				{Name: "failure.png", URL: "https://s3.example.com/failure.png", Step: 3, Timestamp: endTime},
			},
			VideoURL:    "https://s3.example.com/video.webm",
			TraceURL:    "https://s3.example.com/trace.zip",
			StartedAt:   startTime,
			CompletedAt: endTime,
		}
		tc.Status = domain.TestCaseStatusFailed

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)

		fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.NotNil(t, fetched.ExecutionResult)
		assert.False(t, fetched.ExecutionResult.Passed)
		assert.Equal(t, "Element not found", fetched.ExecutionResult.ErrorMessage)
		assert.Len(t, fetched.ExecutionResult.Screenshots, 1)
		assert.Equal(t, "https://s3.example.com/video.webm", fetched.ExecutionResult.VideoURL)
	})

	t.Run("Create_WithHealingHistory", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		tc := domain.NewTestCase(
			run.ID,
			tenant.ID,
			"Healing History Test",
			"With healing",
			"regression",
			domain.PriorityHigh,
			[]domain.TestStep{},
		)

		tc.HealingHistory = []domain.HealingRecord{
			{
				Timestamp:        time.Now().Add(-5 * time.Second),
				OriginalSelector: "#old-button",
				HealedSelector:   "button[data-testid='submit']",
				Strategy:         "attribute",
				Confidence:       0.92,
				Successful:       true,
			},
			{
				Timestamp:        time.Now(),
				OriginalSelector: ".form-input",
				HealedSelector:   "input[name='email']",
				Strategy:         "semantic",
				Confidence:       0.88,
				Successful:       true,
			},
		}
		tc.Status = domain.TestCaseStatusHealed
		tc.OriginalScript = "original script with old selectors"
		tc.Script = "healed script with new selectors"

		err := testCaseRepo.Create(ctx, tc)
		require.NoError(t, err)

		fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TestCaseStatusHealed, fetched.Status)
		assert.Len(t, fetched.HealingHistory, 2)
		assert.Equal(t, "attribute", fetched.HealingHistory[0].Strategy)
		assert.Equal(t, "semantic", fetched.HealingHistory[1].Strategy)
		assert.Equal(t, "original script with old selectors", fetched.OriginalScript)
	})

	t.Run("AllStatuses", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant, _, run := createTestRunWithDeps(t)

		statuses := []domain.TestCaseStatus{
			domain.TestCaseStatusPending,
			domain.TestCaseStatusRunning,
			domain.TestCaseStatusPassed,
			domain.TestCaseStatusFailed,
			domain.TestCaseStatusSkipped,
			domain.TestCaseStatusHealed,
			domain.TestCaseStatusFlaky,
		}

		for _, status := range statuses {
			tc := domain.NewTestCase(
				run.ID,
				tenant.ID,
				"Status: "+string(status),
				"Testing status",
				"functional",
				domain.PriorityMedium,
				nil,
			)
			err := testCaseRepo.Create(ctx, tc)
			require.NoError(t, err)
			err = testCaseRepo.UpdateStatus(ctx, tc.ID, status)
			require.NoError(t, err)

			fetched, err := testCaseRepo.GetByID(ctx, tc.ID)
			require.NoError(t, err)
			assert.Equal(t, status, fetched.Status, "Status mismatch for %s", status)
		}
	})
}

// Helper function for int pointer
func intPtr(i int) *int {
	return &i
}
