package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestResponseWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := newResponseWriter(rec)

		rw.WriteHeader(http.StatusCreated)

		if rw.statusCode != http.StatusCreated {
			t.Errorf("statusCode = %d, want %d", rw.statusCode, http.StatusCreated)
		}
	})

	t.Run("default status is 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := newResponseWriter(rec)

		if rw.statusCode != http.StatusOK {
			t.Errorf("default statusCode = %d, want %d", rw.statusCode, http.StatusOK)
		}
	})

	t.Run("tracks bytes written", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := newResponseWriter(rec)

		data := []byte("Hello, World!")
		n, err := rw.Write(data)

		if err != nil {
			t.Errorf("Write() error = %v", err)
		}
		if n != len(data) {
			t.Errorf("Write() returned %d, want %d", n, len(data))
		}
		if rw.written != int64(len(data)) {
			t.Errorf("written = %d, want %d", rw.written, len(data))
		}
	})

	t.Run("accumulates bytes across multiple writes", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := newResponseWriter(rec)

		rw.Write([]byte("Hello"))
		rw.Write([]byte("World"))

		if rw.written != 10 {
			t.Errorf("written = %d, want 10", rw.written)
		}
	})
}

func TestLoggingMiddleware_Handler(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("logs successful request", func(t *testing.T) {
		m := NewLoggingMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		req := httptest.NewRequest("GET", "/api/test", nil)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("sets request ID header", func(t *testing.T) {
		m := NewLoggingMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		requestID := rec.Header().Get("X-Request-ID")
		if requestID == "" {
			t.Error("X-Request-ID header should be set")
		}
	})

	t.Run("preserves provided request ID", func(t *testing.T) {
		m := NewLoggingMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Request-ID", "custom-request-id")
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		requestID := rec.Header().Get("X-Request-ID")
		if requestID != "custom-request-id" {
			t.Errorf("X-Request-ID = %q, want %q", requestID, "custom-request-id")
		}
	})

	t.Run("logs 4xx as warning", func(t *testing.T) {
		m := NewLoggingMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("logs 5xx as error", func(t *testing.T) {
		m := NewLoggingMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("includes tenant ID when present", func(t *testing.T) {
		m := NewLoggingMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		tenantID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
		ctx := context.WithValue(req.Context(), ContextKeyTenantID, tenantID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})
}

func TestNewLoggingMiddleware(t *testing.T) {
	logger := zap.NewNop()
	m := NewLoggingMiddleware(logger)

	if m == nil {
		t.Error("NewLoggingMiddleware() returned nil")
	}
	if m.logger != logger {
		t.Error("NewLoggingMiddleware() did not set logger")
	}
}

func TestRecoveryMiddleware_Handler(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("passes through normal requests", func(t *testing.T) {
		m := NewRecoveryMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if rec.Body.String() != "OK" {
			t.Errorf("body = %q, want %q", rec.Body.String(), "OK")
		}
	})

	t.Run("recovers from panic", func(t *testing.T) {
		m := NewRecoveryMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		// Should not panic
		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("recovers from panic with error value", func(t *testing.T) {
		m := NewRecoveryMiddleware(logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("runtime error: invalid memory address")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		m.Handler(handler).ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestNewRecoveryMiddleware(t *testing.T) {
	logger := zap.NewNop()
	m := NewRecoveryMiddleware(logger)

	if m == nil {
		t.Error("NewRecoveryMiddleware() returned nil")
	}
	if m.logger != logger {
		t.Error("NewRecoveryMiddleware() did not set logger")
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("generateRequestID() returned empty string")
	}
	if id2 == "" {
		t.Error("generateRequestID() returned empty string")
	}

	// Should contain a timestamp-like prefix
	if len(id1) < 14 {
		t.Errorf("generateRequestID() too short: %q", id1)
	}

	// Should contain a hyphen separating timestamp and random part
	if !strings.Contains(id1, "-") {
		t.Errorf("generateRequestID() should contain hyphen: %q", id1)
	}
}

func TestRandomString(t *testing.T) {
	tests := []struct {
		length int
	}{
		{1},
		{8},
		{16},
		{32},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			s := randomString(tt.length)
			if len(s) != tt.length {
				t.Errorf("randomString(%d) length = %d, want %d", tt.length, len(s), tt.length)
			}
		})
	}
}
