package postgres

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testforge/testforge/internal/domain"
)

func TestProjectRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := NewTenantRepository(db)
	projectRepo := NewProjectRepository(db)
	ctx := context.Background()

	// Helper to create a tenant for tests
	createTestTenant := func(t *testing.T) *domain.Tenant {
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
		return tenant
	}

	t.Run("Create", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "Test Project",
			Description: "A test project",
			BaseURL:     "https://example.com",
			Settings: domain.ProjectSettings{
				DefaultBrowser:   "chromium",
				DefaultTimeout:   30000,
				MaxCrawlDepth:    5,
				ParallelWorkers:  4,
				CaptureScreenshots: true,
			},
		}
		project.SetTimestamps()

		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		// Verify it was created
		fetched, err := projectRepo.GetByID(ctx, project.ID)
		require.NoError(t, err)
		assert.Equal(t, project.Name, fetched.Name)
		assert.Equal(t, project.Description, fetched.Description)
		assert.Equal(t, project.BaseURL, fetched.BaseURL)
		assert.Equal(t, project.TenantID, fetched.TenantID)
		assert.Equal(t, "chromium", fetched.Settings.DefaultBrowser)
	})

	t.Run("Create_InvalidTenant", func(t *testing.T) {
		testDB.TruncateTables(t)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    uuid.New(), // Non-existent tenant
			Name:        "Orphan Project",
			Description: "This should fail",
			BaseURL:     "https://example.com",
			Settings:    domain.ProjectSettings{},
		}
		project.SetTimestamps()

		err := projectRepo.Create(ctx, project)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "Get By ID Test",
			Description: "Testing GetByID",
			BaseURL:     "https://test.com",
			Settings:    domain.ProjectSettings{},
		}
		project.SetTimestamps()

		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		fetched, err := projectRepo.GetByID(ctx, project.ID)
		require.NoError(t, err)
		assert.Equal(t, project.ID, fetched.ID)
		assert.Equal(t, project.Name, fetched.Name)
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := projectRepo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetByTenantID", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		// Create multiple projects
		for i := 0; i < 5; i++ {
			project := &domain.Project{
				ID:          uuid.New(),
				TenantID:    tenant.ID,
				Name:        fmt.Sprintf("Project %d", i),
				Description: "Test",
				BaseURL:     "https://example.com",
				Settings:    domain.ProjectSettings{},
			}
			project.SetTimestamps()
			err := projectRepo.Create(ctx, project)
			require.NoError(t, err)
		}

		// List with pagination
		projects, total, err := projectRepo.GetByTenantID(ctx, tenant.ID, 3, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, projects, 3)

		// Second page
		projects, total, err = projectRepo.GetByTenantID(ctx, tenant.ID, 3, 3)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, projects, 2)
	})

	t.Run("GetByTenantID_Empty", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		projects, total, err := projectRepo.GetByTenantID(ctx, tenant.ID, 10, 0)
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Len(t, projects, 0)
	})

	t.Run("Update", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "Original Name",
			Description: "Original description",
			BaseURL:     "https://original.com",
			Settings:    domain.ProjectSettings{DefaultTimeout: 10000},
		}
		project.SetTimestamps()

		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		// Update the project
		project.Name = "Updated Name"
		project.Description = "Updated description"
		project.Settings.DefaultTimeout = 60000

		err = projectRepo.Update(ctx, project)
		require.NoError(t, err)

		// Verify the update
		fetched, err := projectRepo.GetByID(ctx, project.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", fetched.Name)
		assert.Equal(t, "Updated description", fetched.Description)
		assert.Equal(t, 60000, fetched.Settings.DefaultTimeout)
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "Nonexistent",
			Description: "This should fail",
			BaseURL:     "https://test.com",
			Settings:    domain.ProjectSettings{},
		}

		err := projectRepo.Update(ctx, project)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		project := &domain.Project{
			ID:          uuid.New(),
			TenantID:    tenant.ID,
			Name:        "To Be Deleted",
			Description: "Will be soft deleted",
			BaseURL:     "https://delete.com",
			Settings:    domain.ProjectSettings{},
		}
		project.SetTimestamps()

		err := projectRepo.Create(ctx, project)
		require.NoError(t, err)

		// Delete the project
		err = projectRepo.Delete(ctx, project.ID)
		require.NoError(t, err)

		// Verify it's soft deleted (not found)
		_, err = projectRepo.GetByID(ctx, project.ID)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := projectRepo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})
}
