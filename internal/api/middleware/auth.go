package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/testforge/testforge/internal/repository/postgres"
)

// Context keys
type contextKey string

const (
	ContextKeyTenantID contextKey = "tenant_id"
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyAPIKey   contextKey = "api_key"
	ContextKeyAPIKeyID contextKey = "api_key_id"
	ContextKeyScopes   contextKey = "scopes"
)

// Cache settings
const (
	apiKeyCacheTTL    = 5 * time.Minute
	apiKeyCachePrefix = "apikey:"
)

// GetTenantID extracts tenant ID from context
func GetTenantID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ContextKeyTenantID).(uuid.UUID)
	return id, ok
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(ContextKeyUserID).(string)
	return id, ok
}

// GetAPIKeyID extracts API key ID from context
func GetAPIKeyID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ContextKeyAPIKeyID).(uuid.UUID)
	return id, ok
}

// GetScopes extracts scopes from context
func GetScopes(ctx context.Context) []string {
	scopes, ok := ctx.Value(ContextKeyScopes).([]string)
	if !ok {
		return nil
	}
	return scopes
}

// HasScope checks if the current request has a specific scope
func HasScope(ctx context.Context, scope string) bool {
	scopes := GetScopes(ctx)
	for _, s := range scopes {
		if s == scope || s == "*" || s == "admin" {
			return true
		}
	}
	return false
}

// cachedAPIKey represents the cached API key data
type cachedAPIKey struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Scopes       []string  `json:"scopes"`
	RateLimitRPM *int      `json:"rate_limit_rpm,omitempty"`
	ExpiresAt    *int64    `json:"expires_at,omitempty"` // Unix timestamp
}

// AuthMiddleware handles API key authentication
type AuthMiddleware struct {
	devMode      bool
	apiKeyRepo   *postgres.APIKeyRepository
	redisClient  *redis.Client
	skipDBLookup bool // For testing/development without DB
}

// AuthMiddlewareOption is a functional option for AuthMiddleware
type AuthMiddlewareOption func(*AuthMiddleware)

// WithAPIKeyRepository sets the API key repository
func WithAPIKeyRepository(repo *postgres.APIKeyRepository) AuthMiddlewareOption {
	return func(m *AuthMiddleware) {
		m.apiKeyRepo = repo
	}
}

// WithRedisClient sets the Redis client for caching
func WithRedisClient(client *redis.Client) AuthMiddlewareOption {
	return func(m *AuthMiddleware) {
		m.redisClient = client
	}
}

// WithDevMode enables development mode
func WithDevMode(enabled bool) AuthMiddlewareOption {
	return func(m *AuthMiddleware) {
		m.devMode = enabled
	}
}

// WithSkipDBLookup skips database validation (for testing)
func WithSkipDBLookup(skip bool) AuthMiddlewareOption {
	return func(m *AuthMiddleware) {
		m.skipDBLookup = skip
	}
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(opts ...AuthMiddlewareOption) *AuthMiddleware {
	m := &AuthMiddleware{
		devMode: os.Getenv("TESTFORGE_DEV_MODE") == "true",
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// writeJSONError writes a JSON error response for auth failures
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

// Handler returns the middleware handler
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks and public endpoints
		if m.isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract API key from header
		apiKey := m.extractAPIKey(r)

		// Handle missing API key
		if apiKey == "" {
			if m.devMode {
				m.handleDevMode(w, r, next)
				return
			}
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key required")
			return
		}

		// Validate API key format: tf_<tenant_id>_<random>
		tenantID, err := m.parseAPIKeyFormat(apiKey)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "INVALID_API_KEY", err.Error())
			return
		}

		// Validate against database (with caching)
		cachedKey, err := m.validateAPIKey(r.Context(), apiKey)
		if err != nil {
			switch err {
			case postgres.ErrAPIKeyNotFound:
				writeJSONError(w, http.StatusUnauthorized, "INVALID_API_KEY", "API key not found")
			case postgres.ErrAPIKeyExpired:
				writeJSONError(w, http.StatusUnauthorized, "API_KEY_EXPIRED", "API key has expired")
			case postgres.ErrAPIKeyRevoked:
				writeJSONError(w, http.StatusUnauthorized, "API_KEY_REVOKED", "API key has been revoked")
			default:
				writeJSONError(w, http.StatusInternalServerError, "AUTH_ERROR", "Authentication failed")
			}
			return
		}

		// Verify tenant ID matches
		if cachedKey != nil && cachedKey.TenantID != tenantID {
			writeJSONError(w, http.StatusUnauthorized, "INVALID_API_KEY", "API key tenant mismatch")
			return
		}

		// Update usage asynchronously
		if m.apiKeyRepo != nil && cachedKey != nil {
			clientIP := m.getClientIP(r)
			m.apiKeyRepo.IncrementUsageAsync(postgres.HashAPIKey(apiKey), clientIP)
		}

		// Build context with authentication info
		ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, ContextKeyAPIKey, apiKey)

		if cachedKey != nil {
			ctx = context.WithValue(ctx, ContextKeyAPIKeyID, cachedKey.ID)
			ctx = context.WithValue(ctx, ContextKeyScopes, cachedKey.Scopes)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isPublicPath checks if the path should skip authentication
func (m *AuthMiddleware) isPublicPath(path string) bool {
	publicPaths := []string{
		"/health",
		"/ready",
		"/metrics",
		"/api/v1/docs",
		"/swagger",
	}
	for _, p := range publicPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// extractAPIKey extracts the API key from request headers
func (m *AuthMiddleware) extractAPIKey(r *http.Request) string {
	// Try X-API-Key header first
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return apiKey
	}

	// Try Authorization header with Bearer token
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return ""
}

// parseAPIKeyFormat validates the API key format and extracts tenant ID
func (m *AuthMiddleware) parseAPIKeyFormat(apiKey string) (uuid.UUID, error) {
	// Format: tf_<tenant_id>_<random>
	parts := strings.Split(apiKey, "_")
	if len(parts) < 3 || parts[0] != "tf" {
		return uuid.Nil, &AuthError{Code: "INVALID_FORMAT", Message: "Invalid API key format"}
	}

	// Validate random part length (at least 16 characters for security)
	if len(parts[2]) < 16 {
		return uuid.Nil, &AuthError{Code: "INVALID_FORMAT", Message: "Invalid API key format"}
	}

	// Parse tenant ID
	tenantID, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, &AuthError{Code: "INVALID_TENANT", Message: "Invalid tenant ID in API key"}
	}

	return tenantID, nil
}

// validateAPIKey validates the API key against database with caching
func (m *AuthMiddleware) validateAPIKey(ctx context.Context, apiKey string) (*cachedAPIKey, error) {
	// Skip DB lookup if configured (dev mode without DB)
	if m.skipDBLookup || m.apiKeyRepo == nil {
		return nil, nil
	}

	keyHash := postgres.HashAPIKey(apiKey)
	cacheKey := apiKeyCachePrefix + keyHash

	// Try cache first
	if m.redisClient != nil {
		cached, err := m.redisClient.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var ck cachedAPIKey
			if json.Unmarshal(cached, &ck) == nil {
				// Check expiration
				if ck.ExpiresAt != nil && time.Unix(*ck.ExpiresAt, 0).Before(time.Now()) {
					m.redisClient.Del(ctx, cacheKey)
					return nil, postgres.ErrAPIKeyExpired
				}
				return &ck, nil
			}
		}
	}

	// Query database
	dbKey, err := m.apiKeyRepo.ValidateAndGet(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	// Parse scopes
	var scopes []string
	json.Unmarshal(dbKey.Scopes, &scopes)

	// Build cached key
	ck := &cachedAPIKey{
		ID:           dbKey.ID,
		TenantID:     dbKey.TenantID,
		Scopes:       scopes,
		RateLimitRPM: dbKey.RateLimitRPM,
	}
	if dbKey.ExpiresAt != nil {
		ts := dbKey.ExpiresAt.Unix()
		ck.ExpiresAt = &ts
	}

	// Cache the result
	if m.redisClient != nil {
		if data, err := json.Marshal(ck); err == nil {
			m.redisClient.Set(ctx, cacheKey, data, apiKeyCacheTTL)
		}
	}

	return ck, nil
}

// handleDevMode handles requests in development mode
func (m *AuthMiddleware) handleDevMode(w http.ResponseWriter, r *http.Request, next http.Handler) {
	// In dev mode, allow requests with just tenant header
	tenantHeader := r.Header.Get("X-Tenant-ID")
	if tenantHeader != "" {
		tenantID, err := uuid.Parse(tenantHeader)
		if err == nil {
			ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
			ctx = context.WithValue(ctx, ContextKeyScopes, []string{"*"})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
	}
	// In dev mode, allow unauthenticated access (for testing)
	ctx := context.WithValue(r.Context(), ContextKeyScopes, []string{"*"})
	next.ServeHTTP(w, r.WithContext(ctx))
}

// getClientIP extracts the client IP from the request
func (m *AuthMiddleware) getClientIP(r *http.Request) net.IP {
	// Check X-Forwarded-For first (for proxied requests)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := net.ParseIP(strings.TrimSpace(ips[0]))
			if ip != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		ip := net.ParseIP(xri)
		if ip != nil {
			return ip
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return net.ParseIP(host)
	}

	return nil
}

// AuthError represents an authentication error
type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// RequireTenant middleware ensures tenant ID is present
func RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := GetTenantID(r.Context())
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "TENANT_REQUIRED", "Tenant ID required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope middleware ensures the request has a specific scope
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !HasScope(r.Context(), scope) {
				writeJSONError(w, http.StatusForbidden, "INSUFFICIENT_SCOPE",
					"This operation requires the '"+scope+"' scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyScope middleware ensures the request has at least one of the specified scopes
func RequireAnyScope(scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, scope := range scopes {
				if HasScope(r.Context(), scope) {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeJSONError(w, http.StatusForbidden, "INSUFFICIENT_SCOPE",
				"This operation requires one of these scopes: "+strings.Join(scopes, ", "))
		})
	}
}
