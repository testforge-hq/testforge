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

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		wantID string
		wantOK bool
	}{
		{
			name:   "valid user ID in context",
			ctx:    context.WithValue(context.Background(), ContextKeyUserID, "user-123"),
			wantID: "user-123",
			wantOK: true,
		},
		{
			name:   "no user ID in context",
			ctx:    context.Background(),
			wantID: "",
			wantOK: false,
		},
		{
			name:   "wrong type in context",
			ctx:    context.WithValue(context.Background(), ContextKeyUserID, 12345),
			wantID: "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := GetUserID(tt.ctx)
			if gotID != tt.wantID {
				t.Errorf("GetUserID() gotID = %v, want %v", gotID, tt.wantID)
			}
			if gotOK != tt.wantOK {
				t.Errorf("GetUserID() gotOK = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestGetAPIKeyID(t *testing.T) {
	testID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	tests := []struct {
		name   string
		ctx    context.Context
		wantID uuid.UUID
		wantOK bool
	}{
		{
			name:   "valid API key ID in context",
			ctx:    context.WithValue(context.Background(), ContextKeyAPIKeyID, testID),
			wantID: testID,
			wantOK: true,
		},
		{
			name:   "no API key ID in context",
			ctx:    context.Background(),
			wantID: uuid.Nil,
			wantOK: false,
		},
		{
			name:   "wrong type in context",
			ctx:    context.WithValue(context.Background(), ContextKeyAPIKeyID, "not-a-uuid"),
			wantID: uuid.Nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := GetAPIKeyID(tt.ctx)
			if gotID != tt.wantID {
				t.Errorf("GetAPIKeyID() gotID = %v, want %v", gotID, tt.wantID)
			}
			if gotOK != tt.wantOK {
				t.Errorf("GetAPIKeyID() gotOK = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestGetScopes(t *testing.T) {
	tests := []struct {
		name       string
		ctx        context.Context
		wantScopes []string
	}{
		{
			name:       "valid scopes in context",
			ctx:        context.WithValue(context.Background(), ContextKeyScopes, []string{"read", "write"}),
			wantScopes: []string{"read", "write"},
		},
		{
			name:       "no scopes in context",
			ctx:        context.Background(),
			wantScopes: nil,
		},
		{
			name:       "wrong type in context",
			ctx:        context.WithValue(context.Background(), ContextKeyScopes, "not-a-slice"),
			wantScopes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetScopes(tt.ctx)
			if len(got) != len(tt.wantScopes) {
				t.Errorf("GetScopes() = %v, want %v", got, tt.wantScopes)
			}
		})
	}
}

func TestRequireAnyScope(t *testing.T) {
	tests := []struct {
		name           string
		scopes         []string
		requiredScopes []string
		wantStatus     int
	}{
		{
			name:           "has one of required scopes",
			scopes:         []string{"read"},
			requiredScopes: []string{"read", "write"},
			wantStatus:     http.StatusOK,
		},
		{
			name:           "has another of required scopes",
			scopes:         []string{"write"},
			requiredScopes: []string{"read", "write"},
			wantStatus:     http.StatusOK,
		},
		{
			name:           "missing all required scopes",
			scopes:         []string{"execute"},
			requiredScopes: []string{"read", "write"},
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "admin has all scopes",
			scopes:         []string{"admin"},
			requiredScopes: []string{"read", "write"},
			wantStatus:     http.StatusOK,
		},
		{
			name:           "wildcard has all scopes",
			scopes:         []string{"*"},
			requiredScopes: []string{"read", "write"},
			wantStatus:     http.StatusOK,
		},
		{
			name:           "empty required scopes denies (nothing to match)",
			scopes:         []string{"read"},
			requiredScopes: []string{},
			wantStatus:     http.StatusForbidden,
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
			RequireAnyScope(tt.requiredScopes...)(handler).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuthMiddlewareOptions(t *testing.T) {
	t.Run("WithDevMode", func(t *testing.T) {
		m := NewAuthMiddleware(WithDevMode(true))
		if !m.devMode {
			t.Error("WithDevMode(true) should set devMode to true")
		}

		m2 := NewAuthMiddleware(WithDevMode(false))
		if m2.devMode {
			t.Error("WithDevMode(false) should set devMode to false")
		}
	})

	t.Run("WithSkipDBLookup", func(t *testing.T) {
		m := NewAuthMiddleware(WithSkipDBLookup(true))
		if !m.skipDBLookup {
			t.Error("WithSkipDBLookup(true) should set skipDBLookup to true")
		}
	})
}

func TestAuthError_Error(t *testing.T) {
	err := &AuthError{
		Code:    "TEST_ERROR",
		Message: "Test error message",
	}

	got := err.Error()
	// AuthError.Error() returns only the message
	want := "Test error message"
	if got != want {
		t.Errorf("AuthError.Error() = %q, want %q", got, want)
	}
}

func TestExtractAPIKey(t *testing.T) {
	m := NewAuthMiddleware()

	tests := []struct {
		name       string
		apiKey     string
		authHeader string
		wantKey    string
	}{
		{
			name:    "X-API-Key header",
			apiKey:  "tf_123_abcdef",
			wantKey: "tf_123_abcdef",
		},
		{
			name:       "Bearer token in Authorization header",
			authHeader: "Bearer tf_456_ghijkl",
			wantKey:    "tf_456_ghijkl",
		},
		{
			name:       "X-API-Key takes precedence over Authorization",
			apiKey:     "tf_123_abcdef",
			authHeader: "Bearer tf_456_ghijkl",
			wantKey:    "tf_123_abcdef",
		},
		{
			name:    "no API key",
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			got := m.extractAPIKey(req)
			if got != tt.wantKey {
				t.Errorf("extractAPIKey() = %q, want %q", got, tt.wantKey)
			}
		})
	}
}

func TestIsPublicPath(t *testing.T) {
	m := NewAuthMiddleware()

	tests := []struct {
		path   string
		public bool
	}{
		{"/health", true},
		{"/ready", true},
		{"/metrics", true},
		{"/api/v1/docs", true},
		{"/api/v1/docs/swagger.json", true},
		{"/swagger", true},
		{"/swagger/index.html", true},
		{"/api/v1/tenants", false},
		{"/api/v1/projects", false},
		{"/healthcheck", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := m.isPublicPath(tt.path)
			if got != tt.public {
				t.Errorf("isPublicPath(%q) = %v, want %v", tt.path, got, tt.public)
			}
		})
	}
}

func TestParseAPIKeyFormat(t *testing.T) {
	m := NewAuthMiddleware()
	validTenantID := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")

	tests := []struct {
		name       string
		apiKey     string
		wantTenant uuid.UUID
		wantErr    bool
	}{
		{
			name:       "valid format",
			apiKey:     "tf_123e4567-e89b-12d3-a456-426614174000_abcdefghij123456",
			wantTenant: validTenantID,
			wantErr:    false,
		},
		{
			name:    "missing tf prefix",
			apiKey:  "xx_123e4567-e89b-12d3-a456-426614174000_abcdefghij123456",
			wantErr: true,
		},
		{
			name:    "invalid tenant UUID",
			apiKey:  "tf_invalid-uuid_abcdefghij123456",
			wantErr: true,
		},
		{
			name:    "short random part",
			apiKey:  "tf_123e4567-e89b-12d3-a456-426614174000_short",
			wantErr: true,
		},
		{
			name:    "too few parts",
			apiKey:  "tf_123e4567-e89b-12d3-a456-426614174000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.parseAPIKeyFormat(tt.apiKey)
			if tt.wantErr {
				if err == nil {
					t.Error("parseAPIKeyFormat() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("parseAPIKeyFormat() unexpected error: %v", err)
				}
				if got != tt.wantTenant {
					t.Errorf("parseAPIKeyFormat() = %v, want %v", got, tt.wantTenant)
				}
			}
		})
	}
}
