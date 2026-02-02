package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Project represents a testing project within a tenant
type Project struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	TenantID    uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	BaseURL     string          `json:"base_url" db:"base_url"`
	Settings    ProjectSettings `json:"settings" db:"settings"`
	Timestamps
}

// ProjectSettings contains project-specific configuration
type ProjectSettings struct {
	// Authentication
	AuthType     string            `json:"auth_type,omitempty"` // none, basic, bearer, cookie, custom
	AuthConfig   map[string]string `json:"auth_config,omitempty"`

	// Browser settings
	DefaultBrowser   string `json:"default_browser"`   // chromium, firefox, webkit
	DefaultViewport  string `json:"default_viewport"`  // desktop, tablet, mobile
	ViewportWidth    int    `json:"viewport_width"`
	ViewportHeight   int    `json:"viewport_height"`

	// Test behavior
	DefaultTimeout     int  `json:"default_timeout_ms"`
	RetryFailedTests   int  `json:"retry_failed_tests"`
	ParallelWorkers    int  `json:"parallel_workers"`
	CaptureScreenshots bool `json:"capture_screenshots"`
	CaptureVideo       bool `json:"capture_video"`
	CaptureTrace       bool `json:"capture_trace"`

	// Discovery settings
	MaxCrawlDepth     int      `json:"max_crawl_depth"`
	ExcludePatterns   []string `json:"exclude_patterns,omitempty"`
	IncludePatterns   []string `json:"include_patterns,omitempty"`
	RespectRobotsTxt  bool     `json:"respect_robots_txt"`

	// AI settings
	EnableAIDiscovery bool `json:"enable_ai_discovery"` // Use AI-powered multi-agent discovery by default
	EnableABA         bool `json:"enable_aba"`          // Enable Autonomous Business Analyst by default

	// Custom headers/cookies
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	CustomCookies []Cookie          `json:"custom_cookies,omitempty"`
}

// Cookie represents a browser cookie
type Cookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HTTPOnly bool   `json:"http_only,omitempty"`
}

// AuthType represents the type of authentication
type AuthType string

const (
	AuthTypeNone        AuthType = "none"
	AuthTypeCredentials AuthType = "credentials"
	AuthTypeCookie      AuthType = "cookie"
	AuthTypeToken       AuthType = "token"
	AuthTypeBasic       AuthType = "basic"
)

// AuthConfig contains authentication configuration for a project
type AuthConfig struct {
	Type        AuthType       `json:"type"`
	Credentials *Credentials   `json:"credentials,omitempty"`
	Cookies     []CookieConfig `json:"cookies,omitempty"`
	Headers     []HeaderConfig `json:"headers,omitempty"`
	BasicAuth   *BasicAuth     `json:"basic_auth,omitempty"`
}

// Credentials for form-based login
type Credentials struct {
	LoginURL         string `json:"login_url"`
	UsernameSelector string `json:"username_selector"`
	PasswordSelector string `json:"password_selector"`
	SubmitSelector   string `json:"submit_selector"`
	Username         string `json:"username"`          // encrypted in DB
	Password         string `json:"password"`          // encrypted in DB
	SuccessIndicator string `json:"success_indicator"` // selector or URL pattern to verify login success
	WaitAfterLogin   int    `json:"wait_after_login"`  // ms to wait after login
}

// CookieConfig for cookie-based auth
type CookieConfig struct {
	Name     string `json:"name"`
	Value    string `json:"value"` // encrypted in DB
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Secure   bool   `json:"secure"`
	HttpOnly bool   `json:"http_only"`
	SameSite string `json:"same_site,omitempty"` // Strict, Lax, None
}

// HeaderConfig for token/header-based auth
type HeaderConfig struct {
	Name  string `json:"name"`  // e.g., "Authorization"
	Value string `json:"value"` // e.g., "Bearer xxx" (encrypted in DB)
}

// BasicAuth for HTTP basic authentication
type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"` // encrypted in DB
}

// IsAuthenticated returns true if auth is configured
func (a *AuthConfig) IsAuthenticated() bool {
	return a != nil && a.Type != AuthTypeNone && a.Type != ""
}

// DefaultProjectSettings returns sensible defaults
func DefaultProjectSettings() ProjectSettings {
	return ProjectSettings{
		AuthType:           "none",
		DefaultBrowser:     "chromium",
		DefaultViewport:    "desktop",
		ViewportWidth:      1920,
		ViewportHeight:     1080,
		DefaultTimeout:     30000,
		RetryFailedTests:   2,
		ParallelWorkers:    4,
		CaptureScreenshots: true,
		CaptureVideo:       true,
		CaptureTrace:       false,
		MaxCrawlDepth:      5,
		RespectRobotsTxt:   true,
	}
}

// NewProject creates a new project with defaults
func NewProject(tenantID uuid.UUID, name, description, baseURL string) *Project {
	now := time.Now().UTC()
	return &Project{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		BaseURL:     baseURL,
		Settings:    DefaultProjectSettings(),
		Timestamps: Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// ProjectRepository defines data access for projects
type ProjectRepository interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, id uuid.UUID) (*Project, error)
	GetByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*Project, int, error)
	Update(ctx context.Context, project *Project) error
	Delete(ctx context.Context, id uuid.UUID) error
	ExistsByNameAndTenant(ctx context.Context, name string, tenantID uuid.UUID) (bool, error)
}

// CreateProjectInput for API requests
type CreateProjectInput struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	BaseURL     string           `json:"base_url"`
	Settings    *ProjectSettings `json:"settings,omitempty"`
}

// UpdateProjectInput for API requests
type UpdateProjectInput struct {
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	BaseURL     *string          `json:"base_url,omitempty"`
	Settings    *ProjectSettings `json:"settings,omitempty"`
}
