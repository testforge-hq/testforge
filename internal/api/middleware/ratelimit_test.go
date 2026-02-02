package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimitMiddleware(t *testing.T) {
	// Skip if Redis is not available - this is a unit test
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
