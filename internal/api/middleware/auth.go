package middleware

import (
	"context"
	"net/http"
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
	// In production, this would be a service that validates API keys
	// For now, we'll use a simple header check
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware() *AuthMiddleware {
	return &AuthMiddleware{}
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

		// For development, allow requests without API key
		// In production, this should be removed
		if apiKey == "" {
			// Check for tenant header for development
			tenantHeader := r.Header.Get("X-Tenant-ID")
			if tenantHeader != "" {
				tenantID, err := uuid.Parse(tenantHeader)
				if err == nil {
					ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// Allow unauthenticated access in development
			next.ServeHTTP(w, r)
			return
		}

		// TODO: Validate API key against database
		// For now, extract tenant ID from API key format: tf_<tenant_id>_<random>
		parts := strings.Split(apiKey, "_")
		if len(parts) >= 3 && parts[0] == "tf" {
			tenantID, err := uuid.Parse(parts[1])
			if err == nil {
				ctx := context.WithValue(r.Context(), ContextKeyTenantID, tenantID)
				ctx = context.WithValue(ctx, ContextKeyAPIKey, apiKey)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		http.Error(w, "Invalid API key", http.StatusUnauthorized)
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
