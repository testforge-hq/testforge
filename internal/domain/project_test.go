package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewProject(t *testing.T) {
	tenantID := uuid.New()
	project := NewProject(tenantID, "Test Project", "A description", "https://example.com")

	if project.ID.String() == "" {
		t.Error("ID should not be empty")
	}
	if project.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", project.TenantID, tenantID)
	}
	if project.Name != "Test Project" {
		t.Errorf("Name = %q, want 'Test Project'", project.Name)
	}
	if project.Description != "A description" {
		t.Errorf("Description = %q, want 'A description'", project.Description)
	}
	if project.BaseURL != "https://example.com" {
		t.Errorf("BaseURL = %q, want 'https://example.com'", project.BaseURL)
	}
	if project.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if project.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestDefaultProjectSettings(t *testing.T) {
	settings := DefaultProjectSettings()

	if settings.AuthType != "none" {
		t.Errorf("AuthType = %q, want 'none'", settings.AuthType)
	}
	if settings.DefaultBrowser != "chromium" {
		t.Errorf("DefaultBrowser = %q, want 'chromium'", settings.DefaultBrowser)
	}
	if settings.DefaultViewport != "desktop" {
		t.Errorf("DefaultViewport = %q, want 'desktop'", settings.DefaultViewport)
	}
	if settings.ViewportWidth != 1920 {
		t.Errorf("ViewportWidth = %d, want 1920", settings.ViewportWidth)
	}
	if settings.ViewportHeight != 1080 {
		t.Errorf("ViewportHeight = %d, want 1080", settings.ViewportHeight)
	}
	if settings.DefaultTimeout != 30000 {
		t.Errorf("DefaultTimeout = %d, want 30000", settings.DefaultTimeout)
	}
	if settings.RetryFailedTests != 2 {
		t.Errorf("RetryFailedTests = %d, want 2", settings.RetryFailedTests)
	}
	if settings.ParallelWorkers != 4 {
		t.Errorf("ParallelWorkers = %d, want 4", settings.ParallelWorkers)
	}
	if !settings.CaptureScreenshots {
		t.Error("CaptureScreenshots should be true")
	}
	if !settings.CaptureVideo {
		t.Error("CaptureVideo should be true")
	}
	if settings.CaptureTrace {
		t.Error("CaptureTrace should be false")
	}
	if settings.MaxCrawlDepth != 5 {
		t.Errorf("MaxCrawlDepth = %d, want 5", settings.MaxCrawlDepth)
	}
	if !settings.RespectRobotsTxt {
		t.Error("RespectRobotsTxt should be true")
	}
}

func TestNewProject_HasDefaultSettings(t *testing.T) {
	project := NewProject(uuid.New(), "Test", "", "https://example.com")

	// Should have default settings
	if project.Settings.DefaultBrowser != "chromium" {
		t.Errorf("DefaultBrowser = %q, want 'chromium'", project.Settings.DefaultBrowser)
	}
	if project.Settings.ViewportWidth != 1920 {
		t.Errorf("ViewportWidth = %d, want 1920", project.Settings.ViewportWidth)
	}
}

func TestAuthConfig_IsAuthenticated(t *testing.T) {
	tests := []struct {
		name   string
		config *AuthConfig
		want   bool
	}{
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
		{
			name:   "none type",
			config: &AuthConfig{Type: AuthTypeNone},
			want:   false,
		},
		{
			name:   "empty type",
			config: &AuthConfig{Type: ""},
			want:   false,
		},
		{
			name:   "credentials type",
			config: &AuthConfig{Type: AuthTypeCredentials},
			want:   true,
		},
		{
			name:   "cookie type",
			config: &AuthConfig{Type: AuthTypeCookie},
			want:   true,
		},
		{
			name:   "token type",
			config: &AuthConfig{Type: AuthTypeToken},
			want:   true,
		},
		{
			name:   "basic type",
			config: &AuthConfig{Type: AuthTypeBasic},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsAuthenticated(); got != tt.want {
				t.Errorf("IsAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}
