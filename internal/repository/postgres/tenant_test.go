package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testforge/testforge/internal/domain"
)

func TestTenantRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	repo := NewTenantRepository(db)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:   uuid.New(),
			Name: "Test Tenant",
			Slug: "test-tenant",
			Plan: domain.PlanFree,
			Settings: domain.TenantSettings{
				MaxConcurrentRuns:   5,
				MaxTestCasesPerRun:  100,
				RetentionDays:       30,
				EnableSelfHealing:   true,
				EnableVisualTesting: false,
			},
		}
		tenant.SetTimestamps()

		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		// Verify it was created
		fetched, err := repo.GetByID(ctx, tenant.ID)
		require.NoError(t, err)
		assert.Equal(t, tenant.Name, fetched.Name)
		assert.Equal(t, tenant.Slug, fetched.Slug)
		assert.Equal(t, tenant.Plan, fetched.Plan)
		assert.Equal(t, tenant.Settings.MaxConcurrentRuns, fetched.Settings.MaxConcurrentRuns)
	})

	t.Run("Create_DuplicateSlug", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant1 := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Tenant 1",
			Slug:     "duplicate-slug",
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{},
		}
		tenant1.SetTimestamps()

		tenant2 := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Tenant 2",
			Slug:     "duplicate-slug", // Same slug
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{},
		}
		tenant2.SetTimestamps()

		err := repo.Create(ctx, tenant1)
		require.NoError(t, err)

		err = repo.Create(ctx, tenant2)
		require.Error(t, err)
		assert.True(t, domain.IsAlreadyExistsError(err))
	})

	t.Run("GetByID", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Get By ID Test",
			Slug:     "get-by-id-test",
			Plan:     domain.PlanPro,
			Settings: domain.TenantSettings{MaxConcurrentRuns: 10},
		}
		tenant.SetTimestamps()

		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		fetched, err := repo.GetByID(ctx, tenant.ID)
		require.NoError(t, err)
		assert.Equal(t, tenant.ID, fetched.ID)
		assert.Equal(t, tenant.Name, fetched.Name)
	})

	t.Run("GetByID_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("GetBySlug", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Get By Slug Test",
			Slug:     "get-by-slug-test",
			Plan:     domain.PlanEnterprise,
			Settings: domain.TenantSettings{},
		}
		tenant.SetTimestamps()

		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		fetched, err := repo.GetBySlug(ctx, tenant.Slug)
		require.NoError(t, err)
		assert.Equal(t, tenant.ID, fetched.ID)
		assert.Equal(t, tenant.Slug, fetched.Slug)
	})

	t.Run("GetBySlug_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := repo.GetBySlug(ctx, "nonexistent-slug")
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Update", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Original Name",
			Slug:     "update-test",
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{MaxConcurrentRuns: 1},
		}
		tenant.SetTimestamps()

		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		// Update the tenant
		tenant.Name = "Updated Name"
		tenant.Plan = domain.PlanPro
		tenant.Settings.MaxConcurrentRuns = 20

		err = repo.Update(ctx, tenant)
		require.NoError(t, err)

		// Verify the update
		fetched, err := repo.GetByID(ctx, tenant.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", fetched.Name)
		assert.Equal(t, domain.PlanPro, fetched.Plan)
		assert.Equal(t, 20, fetched.Settings.MaxConcurrentRuns)
	})

	t.Run("Update_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Nonexistent",
			Slug:     "nonexistent",
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{},
		}

		err := repo.Update(ctx, tenant)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "To Be Deleted",
			Slug:     "delete-test",
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{},
		}
		tenant.SetTimestamps()

		err := repo.Create(ctx, tenant)
		require.NoError(t, err)

		// Delete the tenant
		err = repo.Delete(ctx, tenant.ID)
		require.NoError(t, err)

		// Verify it's soft deleted (not found)
		_, err = repo.GetByID(ctx, tenant.ID)
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.True(t, domain.IsNotFoundError(err))
	})

	t.Run("List", func(t *testing.T) {
		testDB.TruncateTables(t)

		// Create multiple tenants
		for i := 0; i < 5; i++ {
			tenant := &domain.Tenant{
				ID:       uuid.New(),
				Name:     "Tenant",
				Slug:     uuid.New().String()[:8], // Unique slug
				Plan:     domain.PlanFree,
				Settings: domain.TenantSettings{},
			}
			tenant.SetTimestamps()
			err := repo.Create(ctx, tenant)
			require.NoError(t, err)
		}

		// List with pagination
		tenants, total, err := repo.List(ctx, 3, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, tenants, 3)

		// Second page
		tenants, total, err = repo.List(ctx, 3, 3)
		require.NoError(t, err)
		assert.Equal(t, 5, total)
		assert.Len(t, tenants, 2)
	})

	t.Run("ExistsBySlug", func(t *testing.T) {
		testDB.TruncateTables(t)

		tenant := &domain.Tenant{
			ID:       uuid.New(),
			Name:     "Exists Test",
			Slug:     "exists-test",
			Plan:     domain.PlanFree,
			Settings: domain.TenantSettings{},
		}
		tenant.SetTimestamps()

		// Before creation
		exists, err := repo.ExistsBySlug(ctx, tenant.Slug)
		require.NoError(t, err)
		assert.False(t, exists)

		// After creation
		err = repo.Create(ctx, tenant)
		require.NoError(t, err)

		exists, err = repo.ExistsBySlug(ctx, tenant.Slug)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}
