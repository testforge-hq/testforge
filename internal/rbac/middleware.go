package rbac

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"testforge/internal/api/middleware"
)

// ContextKey is a type for context keys
type ContextKey string

const (
	// ContextKeyUserPermissions holds the user's permissions in context
	ContextKeyUserPermissions ContextKey = "user_permissions"
	// ContextKeyUserRole holds the user's role in context
	ContextKeyUserRole ContextKey = "user_role"
)

// GetPermissionsFromContext retrieves permissions from context
func GetPermissionsFromContext(ctx context.Context) []string {
	perms, ok := ctx.Value(ContextKeyUserPermissions).([]string)
	if !ok {
		return nil
	}
	return perms
}

// Middleware provides RBAC middleware for HTTP handlers
type Middleware struct {
	enforcer *Enforcer
}

// NewMiddleware creates a new RBAC middleware
func NewMiddleware(enforcer *Enforcer) *Middleware {
	return &Middleware{enforcer: enforcer}
}

// LoadPermissions loads user permissions into the context
func (m *Middleware) LoadPermissions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get user ID and tenant ID from context (set by auth middleware)
		userID, hasUser := middleware.GetUserID(ctx)
		tenantID, hasTenant := middleware.GetTenantID(ctx)

		if hasUser && hasTenant {
			userUUID, err := uuid.Parse(userID)
			if err == nil {
				// Load permissions
				perms, err := m.enforcer.GetUserPermissions(ctx, userUUID, tenantID)
				if err == nil && perms != nil {
					ctx = context.WithValue(ctx, ContextKeyUserPermissions, perms)
				}
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission returns middleware that requires a specific permission
func (m *Middleware) RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Check if permissions are already in context
			perms := GetPermissionsFromContext(ctx)
			if perms != nil {
				if hasPermission(perms, permission) {
					next.ServeHTTP(w, r)
					return
				}
				writePermissionDenied(w, permission)
				return
			}

			// Fall back to checking via enforcer
			userID, hasUser := middleware.GetUserID(ctx)
			tenantID, hasTenant := middleware.GetTenantID(ctx)

			if !hasUser || !hasTenant {
				writePermissionDenied(w, permission)
				return
			}

			userUUID, err := uuid.Parse(userID)
			if err != nil {
				writePermissionDenied(w, permission)
				return
			}

			has, err := m.enforcer.CheckPermission(ctx, userUUID, tenantID, permission)
			if err != nil || !has {
				writePermissionDenied(w, permission)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission returns middleware that requires any of the specified permissions
func (m *Middleware) RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Check if permissions are already in context
			perms := GetPermissionsFromContext(ctx)
			if perms != nil {
				for _, required := range permissions {
					if hasPermission(perms, required) {
						next.ServeHTTP(w, r)
						return
					}
				}
				writePermissionDenied(w, permissions[0])
				return
			}

			// Fall back to checking via enforcer
			userID, hasUser := middleware.GetUserID(ctx)
			tenantID, hasTenant := middleware.GetTenantID(ctx)

			if !hasUser || !hasTenant {
				writePermissionDenied(w, permissions[0])
				return
			}

			userUUID, err := uuid.Parse(userID)
			if err != nil {
				writePermissionDenied(w, permissions[0])
				return
			}

			has, err := m.enforcer.CheckAnyPermission(ctx, userUUID, tenantID, permissions...)
			if err != nil || !has {
				writePermissionDenied(w, permissions[0])
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAllPermissions returns middleware that requires all specified permissions
func (m *Middleware) RequireAllPermissions(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			userID, hasUser := middleware.GetUserID(ctx)
			tenantID, hasTenant := middleware.GetTenantID(ctx)

			if !hasUser || !hasTenant {
				writePermissionDenied(w, permissions[0])
				return
			}

			userUUID, err := uuid.Parse(userID)
			if err != nil {
				writePermissionDenied(w, permissions[0])
				return
			}

			has, err := m.enforcer.CheckAllPermissions(ctx, userUUID, tenantID, permissions...)
			if err != nil || !has {
				writePermissionDenied(w, permissions[0])
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin returns middleware that requires admin permission
func (m *Middleware) RequireAdmin() func(http.Handler) http.Handler {
	return m.RequirePermission("*")
}

// hasPermission checks if a permission slice contains a permission
func hasPermission(perms []string, required string) bool {
	for _, p := range perms {
		if p == "*" || p == required {
			return true
		}
		// Check category wildcard
		if len(p) > 2 && p[len(p)-2:] == ":*" {
			category := p[:len(p)-2]
			if len(required) > len(category)+1 && required[:len(category)+1] == category+":" {
				return true
			}
		}
	}
	return false
}

// writePermissionDenied writes a permission denied response
func writePermissionDenied(w http.ResponseWriter, permission string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error": map[string]string{
			"code":    "PERMISSION_DENIED",
			"message": "You do not have permission to perform this action",
			"required": permission,
		},
	})
}
