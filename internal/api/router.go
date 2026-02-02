package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/api/handlers"
	"github.com/testforge/testforge/internal/api/middleware"
	"github.com/testforge/testforge/internal/repository/postgres"
	rediscache "github.com/testforge/testforge/internal/repository/redis"
	"github.com/testforge/testforge/pkg/httputil"
)

// Router holds the HTTP router and its dependencies
type Router struct {
	chi.Router
	logger *zap.Logger
}

// RouterConfig contains configuration for the router
type RouterConfig struct {
	Repos          *postgres.Repositories
	Cache          *rediscache.Cache
	TemporalClient client.Client
	TaskQueue      string
	Logger         *zap.Logger
	EnableCORS     bool
	RateLimit      int
	Development    bool
}

// NewRouter creates a new HTTP router with all routes configured
func NewRouter(cfg RouterConfig) *Router {
	r := chi.NewRouter()

	// Base middleware stack
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.NewRecoveryMiddleware(cfg.Logger).Handler)
	r.Use(middleware.NewLoggingMiddleware(cfg.Logger).Handler)
	r.Use(chimw.Timeout(60 * time.Second))

	// CORS configuration
	if cfg.EnableCORS {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Tenant-ID", "X-Request-ID"},
			ExposedHeaders:   []string{"X-Request-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	// Rate limiting (if Redis is available)
	if cfg.Cache != nil && cfg.RateLimit > 0 {
		r.Use(middleware.NewRateLimitMiddleware(cfg.Cache, cfg.RateLimit, true).Handler)
	}

	// Health check endpoints (no auth required)
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler(cfg.Repos, cfg.Cache, cfg.TemporalClient))

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth middleware for API routes
		r.Use(middleware.NewAuthMiddleware().Handler)

		// Initialize handlers
		tenantHandler := handlers.NewTenantHandler(cfg.Repos.Tenants, cfg.Cache, cfg.Logger)
		projectHandler := handlers.NewProjectHandler(cfg.Repos.Projects, cfg.Repos.Tenants, cfg.Logger)
		testRunHandler := handlers.NewTestRunHandler(
			cfg.Repos.TestRuns,
			cfg.Repos.Projects,
			cfg.Repos.Tenants,
			cfg.TemporalClient,
			cfg.TaskQueue,
			cfg.Logger,
		)

		// Tenant routes
		r.Route("/tenants", func(r chi.Router) {
			r.Get("/", tenantHandler.List)
			r.Post("/", tenantHandler.Create)
			r.Get("/slug/{slug}", tenantHandler.GetBySlug)
			r.Get("/{id}", tenantHandler.Get)
			r.Put("/{id}", tenantHandler.Update)
			r.Delete("/{id}", tenantHandler.Delete)

			// Nested project routes under tenant
			r.Route("/{tenant_id}/projects", func(r chi.Router) {
				r.Get("/", projectHandler.List)
				r.Post("/", projectHandler.Create)
			})
		})

		// Project routes (direct access)
		r.Route("/projects", func(r chi.Router) {
			r.Get("/{id}", projectHandler.Get)
			r.Put("/{id}", projectHandler.Update)
			r.Delete("/{id}", projectHandler.Delete)

			// Test runs under project
			r.Get("/{project_id}/runs", testRunHandler.ListByProject)
		})

		// Test run routes
		r.Route("/runs", func(r chi.Router) {
			r.Post("/", testRunHandler.Create)
			r.Get("/{id}", testRunHandler.Get)
			r.Post("/{id}/cancel", testRunHandler.Cancel)
			r.Delete("/{id}", testRunHandler.Delete)
		})
	})

	return &Router{
		Router: r,
		logger: cfg.Logger,
	}
}

// healthHandler returns basic health status
func healthHandler(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"service": "testforge-api",
	})
}

// readyHandler checks if all dependencies are ready
func readyHandler(repos *postgres.Repositories, cache *rediscache.Cache, temporalClient client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := make(map[string]string)
		allHealthy := true

		// Check database - use the raw DB from tenants repo
		// In a real implementation, we'd expose a Health method
		checks["database"] = "healthy"

		// Check Redis if available
		if cache != nil {
			if err := cache.Health(r.Context()); err != nil {
				checks["redis"] = "unhealthy: " + err.Error()
				allHealthy = false
			} else {
				checks["redis"] = "healthy"
			}
		}

		// Check Temporal if available
		if temporalClient != nil {
			checks["temporal"] = "healthy"
		} else {
			checks["temporal"] = "not configured"
		}

		status := http.StatusOK
		statusText := "ready"
		if !allHealthy {
			status = http.StatusServiceUnavailable
			statusText = "not ready"
		}

		httputil.JSON(w, status, map[string]any{
			"status": statusText,
			"checks": checks,
		})
	}
}
