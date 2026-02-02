package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

var (
	// ErrAPIKeyNotFound is returned when an API key doesn't exist
	ErrAPIKeyNotFound = errors.New("api key not found")

	// ErrAPIKeyExpired is returned when an API key has expired
	ErrAPIKeyExpired = errors.New("api key expired")

	// ErrAPIKeyRevoked is returned when an API key has been revoked
	ErrAPIKeyRevoked = errors.New("api key revoked")

	// ErrInsufficientScope is returned when the key doesn't have required scope
	ErrInsufficientScope = errors.New("insufficient scope")
)

// APIKey represents an API key in the database
type APIKey struct {
	ID            uuid.UUID       `db:"id" json:"id"`
	TenantID      uuid.UUID       `db:"tenant_id" json:"tenant_id"`
	Name          string          `db:"name" json:"name"`
	KeyPrefix     string          `db:"key_prefix" json:"key_prefix"`
	KeyHash       string          `db:"key_hash" json:"-"` // Never expose hash
	Scopes        json.RawMessage `db:"scopes" json:"scopes"`
	RateLimitRPM  *int            `db:"rate_limit_rpm" json:"rate_limit_rpm,omitempty"`
	ExpiresAt     *time.Time      `db:"expires_at" json:"expires_at,omitempty"`
	RevokedAt     *time.Time      `db:"revoked_at" json:"revoked_at,omitempty"`
	RevokedReason *string         `db:"revoked_reason" json:"revoked_reason,omitempty"`
	LastUsedAt    *time.Time      `db:"last_used_at" json:"last_used_at,omitempty"`
	LastUsedIP    *string         `db:"last_used_ip" json:"last_used_ip,omitempty"`
	UsageCount    int64           `db:"usage_count" json:"usage_count"`
	Description   *string         `db:"description" json:"description,omitempty"`
	CreatedBy     *uuid.UUID      `db:"created_by" json:"created_by,omitempty"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

// HasScope checks if the API key has a specific scope
func (k *APIKey) HasScope(scope string) bool {
	var scopes []string
	if err := json.Unmarshal(k.Scopes, &scopes); err != nil {
		return false
	}

	for _, s := range scopes {
		if s == scope || s == "*" || s == "admin" {
			return true
		}
	}
	return false
}

// IsValid checks if the API key is valid (not expired or revoked)
func (k *APIKey) IsValid() bool {
	if k.RevokedAt != nil {
		return false
	}
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}

// APIKeyRepository handles API key database operations
type APIKeyRepository struct {
	db *sqlx.DB
}

// NewAPIKeyRepository creates a new API key repository
func NewAPIKeyRepository(db *sqlx.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

// HashAPIKey creates a SHA-256 hash of an API key
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// GetKeyPrefix extracts the prefix from an API key for display
func GetKeyPrefix(key string) string {
	if len(key) < 12 {
		return key
	}
	// Format: tf_<tenant>_<random> - show first 12 chars
	return key[:12]
}

// Create creates a new API key
func (r *APIKeyRepository) Create(ctx context.Context, key *APIKey, rawKey string) error {
	key.KeyHash = HashAPIKey(rawKey)
	key.KeyPrefix = GetKeyPrefix(rawKey)

	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}

	query := `
		INSERT INTO api_keys (
			id, tenant_id, name, key_prefix, key_hash, scopes,
			rate_limit_rpm, expires_at, description, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING created_at, updated_at`

	return r.db.QueryRowxContext(
		ctx, query,
		key.ID, key.TenantID, key.Name, key.KeyPrefix, key.KeyHash, key.Scopes,
		key.RateLimitRPM, key.ExpiresAt, key.Description, key.CreatedBy,
	).Scan(&key.CreatedAt, &key.UpdatedAt)
}

// GetByHash retrieves an API key by its hash
func (r *APIKeyRepository) GetByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	var key APIKey
	query := `
		SELECT id, tenant_id, name, key_prefix, key_hash, scopes,
		       rate_limit_rpm, expires_at, revoked_at, revoked_reason,
		       last_used_at, last_used_ip, usage_count, description,
		       created_by, created_at, updated_at
		FROM api_keys
		WHERE key_hash = $1`

	err := r.db.GetContext(ctx, &key, query, keyHash)
	if err == sql.ErrNoRows {
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying api key: %w", err)
	}

	return &key, nil
}

// GetByRawKey retrieves an API key by the raw key value
func (r *APIKeyRepository) GetByRawKey(ctx context.Context, rawKey string) (*APIKey, error) {
	return r.GetByHash(ctx, HashAPIKey(rawKey))
}

// ValidateAndGet validates an API key and returns it if valid
func (r *APIKeyRepository) ValidateAndGet(ctx context.Context, rawKey string) (*APIKey, error) {
	key, err := r.GetByRawKey(ctx, rawKey)
	if err != nil {
		return nil, err
	}

	if key.RevokedAt != nil {
		return nil, ErrAPIKeyRevoked
	}

	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return nil, ErrAPIKeyExpired
	}

	return key, nil
}

// GetByID retrieves an API key by its ID
func (r *APIKeyRepository) GetByID(ctx context.Context, id uuid.UUID) (*APIKey, error) {
	var key APIKey
	query := `
		SELECT id, tenant_id, name, key_prefix, key_hash, scopes,
		       rate_limit_rpm, expires_at, revoked_at, revoked_reason,
		       last_used_at, last_used_ip, usage_count, description,
		       created_by, created_at, updated_at
		FROM api_keys
		WHERE id = $1`

	err := r.db.GetContext(ctx, &key, query, id)
	if err == sql.ErrNoRows {
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying api key: %w", err)
	}

	return &key, nil
}

// ListByTenant retrieves all active API keys for a tenant
func (r *APIKeyRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]*APIKey, error) {
	var keys []*APIKey
	query := `
		SELECT id, tenant_id, name, key_prefix, scopes,
		       rate_limit_rpm, expires_at, last_used_at, usage_count,
		       description, created_at, updated_at
		FROM api_keys
		WHERE tenant_id = $1
		  AND revoked_at IS NULL
		ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &keys, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing api keys: %w", err)
	}

	return keys, nil
}

// Revoke revokes an API key
func (r *APIKeyRepository) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
	query := `
		UPDATE api_keys
		SET revoked_at = NOW(), revoked_reason = $2
		WHERE id = $1 AND revoked_at IS NULL`

	result, err := r.db.ExecContext(ctx, query, id, reason)
	if err != nil {
		return fmt.Errorf("revoking api key: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// UpdateLastUsed updates the last used timestamp and IP
func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, keyHash string, ip net.IP) error {
	var ipStr *string
	if ip != nil {
		s := ip.String()
		ipStr = &s
	}

	query := `
		UPDATE api_keys
		SET last_used_at = NOW(),
		    last_used_ip = $2,
		    usage_count = usage_count + 1
		WHERE key_hash = $1`

	_, err := r.db.ExecContext(ctx, query, keyHash, ipStr)
	return err
}

// IncrementUsageAsync queues a usage increment (fire-and-forget)
func (r *APIKeyRepository) IncrementUsageAsync(keyHash string, ip net.IP) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r.UpdateLastUsed(ctx, keyHash, ip)
	}()
}

// Delete permanently deletes an API key
func (r *APIKeyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM api_keys WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting api key: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrAPIKeyNotFound
	}

	return nil
}

// CleanupExpired removes keys that have been expired for more than the retention period
func (r *APIKeyRepository) CleanupExpired(ctx context.Context, retentionDays int) (int64, error) {
	query := `
		DELETE FROM api_keys
		WHERE (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '1 day' * $1)
		   OR (expires_at IS NOT NULL AND expires_at < NOW() - INTERVAL '1 day' * $1)`

	result, err := r.db.ExecContext(ctx, query, retentionDays)
	if err != nil {
		return 0, fmt.Errorf("cleaning up expired keys: %w", err)
	}

	return result.RowsAffected()
}
