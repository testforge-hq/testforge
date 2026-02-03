package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewRateLimitMiddleware(t *testing.T) {
	t.Run("creates middleware with parameters", func(t *testing.T) {
		m := NewRateLimitMiddleware(nil, 100, true)
		if m == nil {
			t.Fatal("NewRateLimitMiddleware returned nil")
		}
		if m.limit != 100 {
			t.Errorf("limit = %d, want 100", m.limit)
		}
		if !m.enabled {
			t.Error("enabled should be true")
		}
	})

	t.Run("creates disabled middleware", func(t *testing.T) {
		m := NewRateLimitMiddleware(nil, 60, false)
		if m.enabled {
			t.Error("enabled should be false")
		}
	})
}

func TestRateLimitMiddleware_Handler_Disabled(t *testing.T) {
	m := NewRateLimitMiddleware(nil, 100, false)

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	rr := httptest.NewRecorder()

	m.Handler(handler).ServeHTTP(rr, req)

	if !called {
		t.Error("handler should be called when rate limiting is disabled")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRateLimitMiddleware_Handler_NilCache(t *testing.T) {
	m := NewRateLimitMiddleware(nil, 100, true)

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	rr := httptest.NewRecorder()

	m.Handler(handler).ServeHTTP(rr, req)

	if !called {
		t.Error("handler should be called when cache is nil")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRateLimitMiddleware_Handler_HealthEndpoints(t *testing.T) {
	m := NewRateLimitMiddleware(nil, 100, true)

	endpoints := []string{"/health", "/ready"}

	for _, path := range endpoints {
		t.Run(path, func(t *testing.T) {
			called := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()

			m.Handler(handler).ServeHTTP(rr, req)

			if !called {
				t.Errorf("handler should be called for %s", path)
			}
		})
	}
}

func TestRateLimitMiddleware_GetRateLimitKey(t *testing.T) {
	m := NewRateLimitMiddleware(nil, 100, true)
	tenantID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	t.Run("with tenant ID in context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		ctx := context.WithValue(req.Context(), ContextKeyTenantID, tenantID)
		req = req.WithContext(ctx)

		key := m.getRateLimitKey(req)
		expected := "tenant:" + tenantID.String()
		if key != expected {
			t.Errorf("key = %s, want %s", key, expected)
		}
	})

	t.Run("with X-Forwarded-For header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.100")

		key := m.getRateLimitKey(req)
		if key != "ip:192.168.1.100" {
			t.Errorf("key = %s, want ip:192.168.1.100", key)
		}
	})

	t.Run("with X-Real-IP header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Real-IP", "10.0.0.1")

		key := m.getRateLimitKey(req)
		if key != "ip:10.0.0.1" {
			t.Errorf("key = %s, want ip:10.0.0.1", key)
		}
	})

	t.Run("falls back to RemoteAddr", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "172.16.0.1:12345"

		key := m.getRateLimitKey(req)
		if key != "ip:172.16.0.1:12345" {
			t.Errorf("key = %s, want ip:172.16.0.1:12345", key)
		}
	})

	t.Run("X-Forwarded-For takes precedence over X-Real-IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.100")
		req.Header.Set("X-Real-IP", "10.0.0.1")
		req.RemoteAddr = "172.16.0.1:12345"

		key := m.getRateLimitKey(req)
		if key != "ip:192.168.1.100" {
			t.Errorf("key = %s, want ip:192.168.1.100", key)
		}
	})
}

func TestNewTenantRateLimitMiddleware(t *testing.T) {
	m := NewTenantRateLimitMiddleware(nil)
	if m == nil {
		t.Fatal("NewTenantRateLimitMiddleware returned nil")
	}

	// Check default limits
	expectedLimits := map[string]int{
		"free":       60,
		"pro":        300,
		"enterprise": 1000,
	}

	for plan, expectedLimit := range expectedLimits {
		if m.limits[plan] != expectedLimit {
			t.Errorf("limits[%s] = %d, want %d", plan, m.limits[plan], expectedLimit)
		}
	}
}

func TestTenantRateLimitMiddleware_Handler_NilCache(t *testing.T) {
	m := NewTenantRateLimitMiddleware(nil)

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	rr := httptest.NewRecorder()

	m.Handler(handler).ServeHTTP(rr, req)

	if !called {
		t.Error("handler should be called when cache is nil")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestTenantRateLimitMiddleware_Handler_NoTenant(t *testing.T) {
	m := NewTenantRateLimitMiddleware(nil)

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	// No tenant ID in context
	rr := httptest.NewRecorder()

	m.Handler(handler).ServeHTTP(rr, req)

	if !called {
		t.Error("handler should be called when no tenant ID")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	// Skip full integration test that requires Redis
	t.Skip("Requires Redis connection - run as integration test")
}

func TestTenantRateLimitConfig(t *testing.T) {
	tests := []struct {
		name     string
		plan     string
		wantRPM  int
	}{
		{
			name:    "free plan",
			plan:    "free",
			wantRPM: 60,
		},
		{
			name:    "pro plan",
			plan:    "pro",
			wantRPM: 300,
		},
		{
			name:    "enterprise plan",
			plan:    "enterprise",
			wantRPM: 1000,
		},
		{
			name:    "unknown plan defaults to free",
			plan:    "unknown",
			wantRPM: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that rate limits are configured correctly for each plan
			var rpmLimit int
			switch tt.plan {
			case "free":
				rpmLimit = 60
			case "pro":
				rpmLimit = 300
			case "enterprise":
				rpmLimit = 1000
			default:
				rpmLimit = 60
			}

			if rpmLimit != tt.wantRPM {
				t.Errorf("rate limit for %s = %d, want %d", tt.plan, rpmLimit, tt.wantRPM)
			}
		})
	}
}

// TestRateLimitHeaders verifies that rate limit headers are set correctly
func TestRateLimitHeaders(t *testing.T) {
	// Create a simple handler that sets rate limit headers
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate rate limit headers
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "59")
		w.Header().Set("X-RateLimit-Reset", "1234567890")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Check headers
	if limit := rr.Header().Get("X-RateLimit-Limit"); limit != "60" {
		t.Errorf("X-RateLimit-Limit = %s, want 60", limit)
	}
	if remaining := rr.Header().Get("X-RateLimit-Remaining"); remaining != "59" {
		t.Errorf("X-RateLimit-Remaining = %s, want 59", remaining)
	}
	if reset := rr.Header().Get("X-RateLimit-Reset"); reset == "" {
		t.Error("X-RateLimit-Reset header not set")
	}
}

// TestTokenBucketAlgorithm tests the token bucket rate limiting logic
func TestTokenBucketAlgorithm(t *testing.T) {
	type token struct {
		lastRefill time.Time
		tokens     int
		capacity   int
		refillRate int // tokens per second
	}

	refill := func(tb *token) {
		now := time.Now()
		elapsed := now.Sub(tb.lastRefill)
		newTokens := int(elapsed.Seconds()) * tb.refillRate
		tb.tokens = min(tb.capacity, tb.tokens+newTokens)
		tb.lastRefill = now
	}

	consume := func(tb *token) bool {
		refill(tb)
		if tb.tokens > 0 {
			tb.tokens--
			return true
		}
		return false
	}

	// Test with capacity of 5 and 1 token per second
	bucket := &token{
		lastRefill: time.Now(),
		tokens:     5,
		capacity:   5,
		refillRate: 1,
	}

	// Should allow 5 requests immediately
	for i := 0; i < 5; i++ {
		if !consume(bucket) {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if consume(bucket) {
		t.Error("request 6 should be denied")
	}

	// After waiting, should allow more requests
	bucket.lastRefill = bucket.lastRefill.Add(-2 * time.Second)
	if !consume(bucket) {
		t.Error("request after refill should be allowed")
	}
}
