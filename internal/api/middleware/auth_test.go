package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestGetTenantID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		wantID   uuid.UUID
		wantOK   bool
	}{
		{
			name:   "valid tenant ID in context",
			ctx:    context.WithValue(context.Background(), ContextKeyTenantID, uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")),
			wantID: uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			wantOK: true,
		},
		{
			name:   "no tenant ID in context",
			ctx:    context.Background(),
			wantID: uuid.Nil,
			wantOK: false,
		},
		{
			name:   "wrong type in context",
			ctx:    context.WithValue(context.Background(), ContextKeyTenantID, "not-a-uuid"),
			wantID: uuid.Nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := GetTenantID(tt.ctx)
			if gotID != tt.wantID {
				t.Errorf("GetTenantID() gotID = %v, want %v", gotID, tt.wantID)
			}
			if gotOK != tt.wantOK {
				t.Errorf("GetTenantID() gotOK = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestAuthMiddleware_Handler(t *testing.T) {
	tenantID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	validAPIKey := "tf_" + tenantID.String() + "_abcdefghij123456"

	tests := []struct {
		name           string
		path           string
		apiKey         string
		authHeader     string
		devMode        bool
		skipDBLookup   bool
		tenantHeader   string
		wantStatus     int
		wantTenantInCtx bool
	}{
		{
			name:       "health endpoint bypasses auth",
			path:       "/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "ready endpoint bypasses auth",
			path:       "/ready",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing API key returns 401",
			path:       "/api/v1/projects",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:           "valid API key in X-API-Key header",
			path:           "/api/v1/projects",
			apiKey:         validAPIKey,
			skipDBLookup:   true,
			wantStatus:     http.StatusOK,
			wantTenantInCtx: true,
		},
		{
			name:           "valid API key in Authorization header",
			path:           "/api/v1/projects",
			authHeader:     "Bearer " + validAPIKey,
			skipDBLookup:   true,
			wantStatus:     http.StatusOK,
			wantTenantInCtx: true,
		},
		{
			name:       "invalid API key format - missing prefix",
			path:       "/api/v1/projects",
			apiKey:     "invalid_key_format",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid API key format - short random part",
			path:       "/api/v1/projects",
			apiKey:     "tf_" + tenantID.String() + "_short",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid tenant ID in API key",
			path:       "/api/v1/projects",
			apiKey:     "tf_invalid-uuid_abcdefghij123456",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:           "dev mode allows tenant header without API key",
			path:           "/api/v1/projects",
			devMode:        true,
			tenantHeader:   tenantID.String(),
			wantStatus:     http.StatusOK,
			wantTenantInCtx: true,
		},
		{
			name:           "dev mode allows unauthenticated access",
			path:           "/api/v1/projects",
			devMode:        true,
			wantStatus:     http.StatusOK,
			wantTenantInCtx: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create middleware
			opts := []AuthMiddlewareOption{
				WithDevMode(tt.devMode),
				WithSkipDBLookup(tt.skipDBLookup),
			}
			middleware := NewAuthMiddleware(opts...)

			// Create test handler that checks context
			var gotTenantID uuid.UUID
			var hasTenant bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotTenantID, hasTenant = GetTenantID(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			// Create request
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.tenantHeader != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantHeader)
			}

			// Execute
			rr := httptest.NewRecorder()
			middleware.Handler(handler).ServeHTTP(rr, req)

			// Check status
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			// Check tenant in context
			if tt.wantStatus == http.StatusOK && tt.wantTenantInCtx {
				if !hasTenant {
					t.Error("expected tenant ID in context")
				}
				if tt.apiKey != "" && gotTenantID != tenantID {
					t.Errorf("tenant ID = %v, want %v", gotTenantID, tenantID)
				}
			}
		})
	}
}

func TestHasScope(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		scope  string
		want   bool
	}{
		{
			name:   "has exact scope",
			scopes: []string{"read", "write"},
			scope:  "read",
			want:   true,
		},
		{
			name:   "wildcard scope matches all",
			scopes: []string{"*"},
			scope:  "read",
			want:   true,
		},
		{
			name:   "admin scope matches all",
			scopes: []string{"admin"},
			scope:  "read",
			want:   true,
		},
		{
			name:   "missing scope",
			scopes: []string{"read"},
			scope:  "write",
			want:   false,
		},
		{
			name:   "empty scopes",
			scopes: []string{},
			scope:  "read",
			want:   false,
		},
		{
			name:   "nil scopes",
			scopes: nil,
			scope:  "read",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.scopes != nil {
				ctx = context.WithValue(ctx, ContextKeyScopes, tt.scopes)
			}

			got := HasScope(ctx, tt.scope)
			if got != tt.want {
				t.Errorf("HasScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequireTenant(t *testing.T) {
	tenantID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	tests := []struct {
		name       string
		hasTenant  bool
		wantStatus int
	}{
		{
			name:       "with tenant passes",
			hasTenant:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "without tenant fails",
			hasTenant:  false,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.hasTenant {
				ctx := context.WithValue(req.Context(), ContextKeyTenantID, tenantID)
				req = req.WithContext(ctx)
			}

			rr := httptest.NewRecorder()
			RequireTenant(handler).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireScope(t *testing.T) {
	tests := []struct {
		name          string
		scopes        []string
		requiredScope string
		wantStatus    int
	}{
		{
			name:          "has required scope",
			scopes:        []string{"read", "write"},
			requiredScope: "write",
			wantStatus:    http.StatusOK,
		},
		{
			name:          "missing required scope",
			scopes:        []string{"read"},
			requiredScope: "write",
			wantStatus:    http.StatusForbidden,
		},
		{
			name:          "admin has all scopes",
			scopes:        []string{"admin"},
			requiredScope: "write",
			wantStatus:    http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			ctx := context.WithValue(req.Context(), ContextKeyScopes, tt.scopes)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			RequireScope(tt.requiredScope)(handler).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}
