package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testforge/testforge/internal/domain"
)

func TestAPIKeyRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := SetupTestDB(t)
	defer testDB.Cleanup(t)

	db := sqlx.NewDb(testDB.DB, "postgres")
	tenantRepo := NewTenantRepository(db)
	apiKeyRepo := NewAPIKeyRepository(db)
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

		scopes, _ := json.Marshal([]string{"read", "write"})
		rateLimit := 300
		apiKey := &APIKey{
			ID:           uuid.New(),
			TenantID:     tenant.ID,
			Name:         "Test API Key",
			Scopes:       scopes,
			RateLimitRPM: &rateLimit,
		}
		rawKey := "tf_test_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		// Verify it was created
		fetched, err := apiKeyRepo.GetByID(ctx, apiKey.ID)
		require.NoError(t, err)
		assert.Equal(t, apiKey.ID, fetched.ID)
		assert.Equal(t, tenant.ID, fetched.TenantID)
		assert.Equal(t, "Test API Key", fetched.Name)
		assert.Equal(t, HashAPIKey(rawKey), fetched.KeyHash)
		assert.Equal(t, GetKeyPrefix(rawKey), fetched.KeyPrefix)
	})

	t.Run("GetByHash", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Hash Test Key",
			Scopes:   scopes,
		}
		rawKey := "tf_hash_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		fetched, err := apiKeyRepo.GetByHash(ctx, HashAPIKey(rawKey))
		require.NoError(t, err)
		assert.Equal(t, apiKey.ID, fetched.ID)
		assert.Equal(t, "Hash Test Key", fetched.Name)
	})

	t.Run("GetByHash_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		_, err := apiKeyRepo.GetByHash(ctx, "nonexistent-hash")
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyNotFound, err)
	})

	t.Run("GetByRawKey", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"*"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Raw Key Test",
			Scopes:   scopes,
		}
		rawKey := "tf_raw_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		fetched, err := apiKeyRepo.GetByRawKey(ctx, rawKey)
		require.NoError(t, err)
		assert.Equal(t, apiKey.ID, fetched.ID)
	})

	t.Run("ValidateAndGet", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read", "write"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Valid Key",
			Scopes:   scopes,
		}
		rawKey := "tf_valid_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		// Should validate successfully
		fetched, err := apiKeyRepo.ValidateAndGet(ctx, rawKey)
		require.NoError(t, err)
		assert.Equal(t, apiKey.ID, fetched.ID)
	})

	t.Run("ValidateAndGet_Expired", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		expiredTime := time.Now().Add(-time.Hour) // Expired 1 hour ago
		apiKey := &APIKey{
			ID:        uuid.New(),
			TenantID:  tenant.ID,
			Name:      "Expired Key",
			Scopes:    scopes,
			ExpiresAt: &expiredTime,
		}
		rawKey := "tf_expired_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		// Should fail validation
		_, err = apiKeyRepo.ValidateAndGet(ctx, rawKey)
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyExpired, err)
	})

	t.Run("ValidateAndGet_Revoked", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Revoked Key",
			Scopes:   scopes,
		}
		rawKey := "tf_revoked_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		// Revoke the key
		err = apiKeyRepo.Revoke(ctx, apiKey.ID, "Security concern")
		require.NoError(t, err)

		// Should fail validation
		_, err = apiKeyRepo.ValidateAndGet(ctx, rawKey)
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyRevoked, err)
	})

	t.Run("ListByTenant", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		// Create multiple API keys
		scopes, _ := json.Marshal([]string{"read"})
		for i := 0; i < 3; i++ {
			apiKey := &APIKey{
				ID:       uuid.New(),
				TenantID: tenant.ID,
				Name:     fmt.Sprintf("Key %d", i),
				Scopes:   scopes,
			}
			rawKey := "tf_list_" + uuid.New().String()
			err := apiKeyRepo.Create(ctx, apiKey, rawKey)
			require.NoError(t, err)
		}

		keys, err := apiKeyRepo.ListByTenant(ctx, tenant.ID)
		require.NoError(t, err)
		assert.Len(t, keys, 3)
	})

	t.Run("ListByTenant_ExcludesRevoked", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})

		// Create an active key
		activeKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Active Key",
			Scopes:   scopes,
		}
		err := apiKeyRepo.Create(ctx, activeKey, "tf_active_"+uuid.New().String())
		require.NoError(t, err)

		// Create and revoke a key
		revokedKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Revoked Key",
			Scopes:   scopes,
		}
		err = apiKeyRepo.Create(ctx, revokedKey, "tf_revoked_"+uuid.New().String())
		require.NoError(t, err)
		err = apiKeyRepo.Revoke(ctx, revokedKey.ID, "Test revocation")
		require.NoError(t, err)

		keys, err := apiKeyRepo.ListByTenant(ctx, tenant.ID)
		require.NoError(t, err)
		assert.Len(t, keys, 1)
		assert.Equal(t, "Active Key", keys[0].Name)
	})

	t.Run("Revoke", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Key To Revoke",
			Scopes:   scopes,
		}
		rawKey := "tf_revoke_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		err = apiKeyRepo.Revoke(ctx, apiKey.ID, "No longer needed")
		require.NoError(t, err)

		// Verify revocation
		fetched, err := apiKeyRepo.GetByID(ctx, apiKey.ID)
		require.NoError(t, err)
		assert.NotNil(t, fetched.RevokedAt)
		assert.Equal(t, "No longer needed", *fetched.RevokedReason)
	})

	t.Run("Revoke_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := apiKeyRepo.Revoke(ctx, uuid.New(), "Test")
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyNotFound, err)
	})

	t.Run("Revoke_AlreadyRevoked", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Double Revoke Key",
			Scopes:   scopes,
		}
		err := apiKeyRepo.Create(ctx, apiKey, "tf_double_"+uuid.New().String())
		require.NoError(t, err)

		// First revoke
		err = apiKeyRepo.Revoke(ctx, apiKey.ID, "First revoke")
		require.NoError(t, err)

		// Second revoke should fail
		err = apiKeyRepo.Revoke(ctx, apiKey.ID, "Second revoke")
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyNotFound, err)
	})

	t.Run("UpdateLastUsed", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Usage Tracking Key",
			Scopes:   scopes,
		}
		rawKey := "tf_usage_" + uuid.New().String()

		err := apiKeyRepo.Create(ctx, apiKey, rawKey)
		require.NoError(t, err)

		ip := net.ParseIP("192.168.1.1")
		err = apiKeyRepo.UpdateLastUsed(ctx, HashAPIKey(rawKey), ip)
		require.NoError(t, err)

		fetched, err := apiKeyRepo.GetByID(ctx, apiKey.ID)
		require.NoError(t, err)
		assert.NotNil(t, fetched.LastUsedAt)
		assert.Equal(t, "192.168.1.1", *fetched.LastUsedIP)
		assert.Equal(t, int64(1), fetched.UsageCount)

		// Update again
		err = apiKeyRepo.UpdateLastUsed(ctx, HashAPIKey(rawKey), ip)
		require.NoError(t, err)

		fetched, err = apiKeyRepo.GetByID(ctx, apiKey.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), fetched.UsageCount)
	})

	t.Run("Delete", func(t *testing.T) {
		testDB.TruncateTables(t)
		tenant := createTestTenant(t)

		scopes, _ := json.Marshal([]string{"read"})
		apiKey := &APIKey{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			Name:     "Key To Delete",
			Scopes:   scopes,
		}
		err := apiKeyRepo.Create(ctx, apiKey, "tf_delete_"+uuid.New().String())
		require.NoError(t, err)

		err = apiKeyRepo.Delete(ctx, apiKey.ID)
		require.NoError(t, err)

		_, err = apiKeyRepo.GetByID(ctx, apiKey.ID)
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyNotFound, err)
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		testDB.TruncateTables(t)

		err := apiKeyRepo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.Equal(t, ErrAPIKeyNotFound, err)
	})

	t.Run("HasScope", func(t *testing.T) {
		scopes, _ := json.Marshal([]string{"read", "write"})
		apiKey := &APIKey{Scopes: scopes}

		assert.True(t, apiKey.HasScope("read"))
		assert.True(t, apiKey.HasScope("write"))
		assert.False(t, apiKey.HasScope("admin"))
		assert.False(t, apiKey.HasScope("delete"))
	})

	t.Run("HasScope_Admin", func(t *testing.T) {
		scopes, _ := json.Marshal([]string{"admin"})
		apiKey := &APIKey{Scopes: scopes}

		// Admin scope grants access to everything
		assert.True(t, apiKey.HasScope("read"))
		assert.True(t, apiKey.HasScope("write"))
		assert.True(t, apiKey.HasScope("admin"))
		assert.True(t, apiKey.HasScope("anything"))
	})

	t.Run("HasScope_Wildcard", func(t *testing.T) {
		scopes, _ := json.Marshal([]string{"*"})
		apiKey := &APIKey{Scopes: scopes}

		assert.True(t, apiKey.HasScope("read"))
		assert.True(t, apiKey.HasScope("write"))
		assert.True(t, apiKey.HasScope("admin"))
	})

	t.Run("IsValid", func(t *testing.T) {
		// Valid key
		validKey := &APIKey{}
		assert.True(t, validKey.IsValid())

		// Revoked key
		revokedAt := time.Now()
		revokedKey := &APIKey{RevokedAt: &revokedAt}
		assert.False(t, revokedKey.IsValid())

		// Expired key
		expiredAt := time.Now().Add(-time.Hour)
		expiredKey := &APIKey{ExpiresAt: &expiredAt}
		assert.False(t, expiredKey.IsValid())

		// Future expiration (still valid)
		futureExpire := time.Now().Add(time.Hour)
		futureKey := &APIKey{ExpiresAt: &futureExpire}
		assert.True(t, futureKey.IsValid())
	})

	t.Run("HashAPIKey", func(t *testing.T) {
		key := "test-api-key-12345"
		hash1 := HashAPIKey(key)
		hash2 := HashAPIKey(key)

		// Same input should produce same hash
		assert.Equal(t, hash1, hash2)
		assert.Len(t, hash1, 64) // SHA-256 produces 64 hex characters

		// Different input should produce different hash
		differentHash := HashAPIKey("different-key")
		assert.NotEqual(t, hash1, differentHash)
	})

	t.Run("GetKeyPrefix", func(t *testing.T) {
		key := "tf_abc123_secretpart"
		prefix := GetKeyPrefix(key)
		assert.Equal(t, "tf_abc123_se", prefix)

		// Short key
		shortKey := "short"
		shortPrefix := GetKeyPrefix(shortKey)
		assert.Equal(t, "short", shortPrefix)
	})
}
