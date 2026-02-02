package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// AdminContextKey is the context key for admin info
type AdminContextKey struct{}

// AdminInfo contains admin user information
type AdminInfo struct {
	UserID    uuid.UUID
	Email     string
	Name      string
	IsSuperAdmin bool
}

// Middleware provides admin authentication middleware
type Middleware struct {
	db     *sqlx.DB
	logger *zap.Logger
	superAdminEmails []string
}

// NewMiddleware creates new admin middleware
func NewMiddleware(db *sqlx.DB, superAdminEmails []string, logger *zap.Logger) *Middleware {
	return &Middleware{
		db:               db,
		logger:           logger,
		superAdminEmails: superAdminEmails,
	}
}

// RequireAdmin ensures the user is an admin
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get user from context (set by auth middleware)
		userID := r.Context().Value("user_id")
		if userID == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		uid, ok := userID.(uuid.UUID)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if user is admin
		var user struct {
			ID    uuid.UUID `db:"id"`
			Email string    `db:"email"`
			Name  string    `db:"name"`
		}

		err := m.db.GetContext(r.Context(), &user, `
			SELECT id, email, name FROM users WHERE id = $1`, uid)
		if err != nil {
			m.logger.Error("failed to get user", zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if user has admin role
		var isAdmin bool
		err = m.db.GetContext(r.Context(), &isAdmin, `
			SELECT EXISTS(
				SELECT 1 FROM tenant_memberships tm
				JOIN roles r ON r.id = tm.role_id
				WHERE tm.user_id = $1 AND r.name = 'admin'
			)`, uid)
		if err != nil {
			m.logger.Error("failed to check admin role", zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if super admin
		isSuperAdmin := m.isSuperAdmin(user.Email)

		if !isAdmin && !isSuperAdmin {
			m.logger.Warn("non-admin user attempted admin access",
				zap.String("email", user.Email),
			)
			http.Error(w, "Forbidden - Admin access required", http.StatusForbidden)
			return
		}

		// Add admin info to context
		adminInfo := &AdminInfo{
			UserID:       user.ID,
			Email:        user.Email,
			Name:         user.Name,
			IsSuperAdmin: isSuperAdmin,
		}

		ctx := context.WithValue(r.Context(), AdminContextKey{}, adminInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSuperAdmin ensures the user is a super admin
func (m *Middleware) RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adminInfo := GetAdminInfo(r.Context())
		if adminInfo == nil || !adminInfo.IsSuperAdmin {
			http.Error(w, "Forbidden - Super admin access required", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GetAdminInfo retrieves admin info from context
func GetAdminInfo(ctx context.Context) *AdminInfo {
	if info, ok := ctx.Value(AdminContextKey{}).(*AdminInfo); ok {
		return info
	}
	return nil
}

func (m *Middleware) isSuperAdmin(email string) bool {
	email = strings.ToLower(email)
	for _, adminEmail := range m.superAdminEmails {
		if strings.ToLower(adminEmail) == email {
			return true
		}
	}
	return false
}

// AuditMiddleware logs all admin actions
func (m *Middleware) AuditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adminInfo := GetAdminInfo(r.Context())
		if adminInfo != nil {
			m.logger.Info("admin action",
				zap.String("user_email", adminInfo.Email),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
			)
		}

		next.ServeHTTP(w, r)
	})
}
