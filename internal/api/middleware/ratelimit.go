package middleware

import (
	"net/http"
	"strconv"

	"github.com/testforge/testforge/internal/repository/redis"
)

// RateLimitMiddleware provides rate limiting functionality
type RateLimitMiddleware struct {
	cache   *redis.Cache
	limit   int
	enabled bool
}

// NewRateLimitMiddleware creates a new rate limit middleware
func NewRateLimitMiddleware(cache *redis.Cache, limit int, enabled bool) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		cache:   cache,
		limit:   limit,
		enabled: enabled,
	}
}

// Handler returns the middleware handler
func (m *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if rate limiting is disabled
		if !m.enabled || m.cache == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Skip for health checks
		if r.URL.Path == "/health" || r.URL.Path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		// Determine rate limit key
		key := m.getRateLimitKey(r)

		// Check rate limit
		allowed, count, err := m.cache.CheckRateLimit(r.Context(), key, m.limit)
		if err != nil {
			// On Redis error, allow the request but log
			next.ServeHTTP(w, r)
			return
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(m.limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(m.limit-count))

		if !allowed {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getRateLimitKey determines the key for rate limiting
func (m *RateLimitMiddleware) getRateLimitKey(r *http.Request) string {
	// Try to use tenant ID if available
	if tenantID, ok := GetTenantID(r.Context()); ok {
		return "tenant:" + tenantID.String()
	}

	// Fall back to IP address
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}

	return "ip:" + ip
}

// TenantRateLimitMiddleware provides tenant-specific rate limiting
type TenantRateLimitMiddleware struct {
	cache *redis.Cache
	// limits per plan type
	limits map[string]int
}

// NewTenantRateLimitMiddleware creates a tenant-specific rate limiter
func NewTenantRateLimitMiddleware(cache *redis.Cache) *TenantRateLimitMiddleware {
	return &TenantRateLimitMiddleware{
		cache: cache,
		limits: map[string]int{
			"free":       60,  // 60 requests/minute
			"pro":        300, // 300 requests/minute
			"enterprise": 1000, // 1000 requests/minute
		},
	}
}

// Handler returns the middleware handler with plan-based limits
func (m *TenantRateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.cache == nil {
			next.ServeHTTP(w, r)
			return
		}

		tenantID, ok := GetTenantID(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		// TODO: Get tenant plan from context or cache
		// For now, use default limit
		limit := m.limits["pro"]

		key := "tenant:" + tenantID.String()
		allowed, count, err := m.cache.CheckRateLimit(r.Context(), key, limit)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limit-count))

		if !allowed {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
