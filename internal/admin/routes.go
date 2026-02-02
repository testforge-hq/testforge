package admin

import (
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Router sets up admin routes
type Router struct {
	handlers   *Handlers
	middleware *Middleware
}

// NewRouter creates a new admin router
func NewRouter(db *sqlx.DB, redis *redis.Client, superAdminEmails []string, logger *zap.Logger) *Router {
	dashboard := NewDashboardService(db, redis, logger)
	handlers := NewHandlers(dashboard, logger)
	middleware := NewMiddleware(db, superAdminEmails, logger)

	return &Router{
		handlers:   handlers,
		middleware: middleware,
	}
}

// RegisterRoutes registers admin routes on the given mux
func (r *Router) RegisterRoutes(mux *http.ServeMux) {
	// All admin routes require admin middleware
	adminRoutes := http.NewServeMux()

	// Overview & Dashboard
	adminRoutes.HandleFunc("GET /overview", r.handlers.GetOverview)
	adminRoutes.HandleFunc("GET /health", r.handlers.GetSystemHealth)

	// Tenant Management
	adminRoutes.HandleFunc("GET /tenants", r.handlers.ListTenants)
	adminRoutes.HandleFunc("GET /tenants/{id}", r.handlers.GetTenant)

	// User Management
	adminRoutes.HandleFunc("GET /users", r.handlers.ListUsers)

	// Test Runs
	adminRoutes.HandleFunc("GET /tests/running", r.handlers.GetRunningTests)

	// Billing & Costs
	adminRoutes.HandleFunc("GET /billing/overview", r.handlers.GetBillingOverview)
	adminRoutes.HandleFunc("GET /costs", r.handlers.GetCostAnalytics)

	// Activity
	adminRoutes.HandleFunc("GET /activity", r.handlers.GetRecentActivity)

	// Apply middleware chain
	handler := r.middleware.RequireAdmin(
		r.middleware.AuditMiddleware(adminRoutes),
	)

	// Mount at /admin
	mux.Handle("/admin/", http.StripPrefix("/admin", handler))
}

// APIResponse is a standard API response
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// Meta contains pagination metadata
type Meta struct {
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}
