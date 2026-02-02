package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
)

// Context keys
type contextKey string

const (
	ContextKeyTenantID contextKey = "tenant_id"
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyAPIKey   contextKey = "api_key"
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

// AuthMiddleware handles API key authentication
// For Phase 1, this is a simple API key check. In production, integrate with OAuth/JWT.
type AuthMiddleware struct {
	devMode bool // Only allow unauthenticated access in explicit dev mode
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware() *AuthMiddleware {
	// Only enable dev mode if explicitly set via environment variable
	devMode := os.Getenv("TESTFORGE_DEV_MODE") == "true"
	return &AuthMiddleware{devMode: devMode}
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
		// Skip auth for health checks
		if r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		// Extract API key from header
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			// Try Authorization header with Bearer token
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				apiKey = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		// Handle missing API key
		if apiKey == "" {
			// In dev mode only, allow requests with just tenant header
			if m.devMode {
				tenantHeader := r.Header.Get("X-Tenant-ID")
				if tenantHeader != "" {
					tenantID, err := uuid.Parse(tenantHeader)
					if err == nil {
						ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					}
				}
				// In dev mode, allow unauthenticated access (for testing)
				next.ServeHTTP(w, r)
				return
			}

			// In production, require authentication
			writeJSONError(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key required")
			return
		}

		// Validate API key format: tf_<tenant_id>_<random>
		// The random part should be at least 16 characters for security
		parts := strings.Split(apiKey, "_")
		if len(parts) < 3 || parts[0] != "tf" || len(parts[2]) < 16 {
			writeJSONError(w, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key format")
			return
		}

		// Parse and validate tenant ID
		tenantID, err := uuid.Parse(parts[1])
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid tenant ID in API key")
			return
		}

		// TODO: In production, validate the API key against the database
		// For now, we trust the format validation above

		ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, ContextKeyAPIKey, apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireTenant middleware ensures tenant ID is present
func RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := GetTenantID(r.Context())
		if !ok {
			http.Error(w, "Tenant ID required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
