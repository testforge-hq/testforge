package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// DiscoveryConfig contains configuration for the discovery process
type DiscoveryConfig struct {
	MaxPages      int           `json:"max_pages"`
	MaxDuration   time.Duration `json:"max_duration"`
	MaxDepth      int           `json:"max_depth"`
	Concurrency   int           `json:"concurrency"`
	Goals         []string      `json:"goals"` // "forms", "auth", "crud", "navigation"
	ScreenshotDir string        `json:"screenshot_dir"`
	Headless      bool          `json:"headless"`
	Timeout       time.Duration `json:"timeout"`
}

// DefaultConfig returns default discovery configuration
func DefaultConfig() DiscoveryConfig {
	return DiscoveryConfig{
		MaxPages:    50,
		MaxDuration: 10 * time.Minute,
		MaxDepth:    3,
		Concurrency: 4,
		Goals:       []string{"forms", "auth", "crud", "navigation"},
		Headless:    true,
		Timeout:     30 * time.Second,
	}
}

// AppModel represents the discovered application structure
type AppModel struct {
	ID           string                `json:"id"`
	BaseURL      string                `json:"base_url"`
	Pages        map[string]*PageModel `json:"pages"`      // URL -> PageModel
	Components   []*Component          `json:"components"` // Reusable components
	Flows        []BusinessFlow        `json:"flows"`      // Detected user journeys
	SiteMap      []SiteMapEntry        `json:"sitemap"`
	TechStack    []string              `json:"tech_stack,omitempty"`
	DiscoveredAt time.Time             `json:"discovered_at"`
	Stats        DiscoveryStats        `json:"stats"`
}

// DiscoveryStats contains statistics about the discovery process
type DiscoveryStats struct {
	TotalPages      int           `json:"total_pages"`
	TotalForms      int           `json:"total_forms"`
	TotalButtons    int           `json:"total_buttons"`
	TotalLinks      int           `json:"total_links"`
	TotalInputs     int           `json:"total_inputs"`
	TotalScreenshots int          `json:"total_screenshots"`
	Duration        time.Duration `json:"duration"`
	MaxDepthReached int           `json:"max_depth_reached"`
}

// SiteMapEntry represents a page in the sitemap
type SiteMapEntry struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Depth    int    `json:"depth"`
	ParentURL string `json:"parent_url,omitempty"`
}

// PageModel represents a discovered page
type PageModel struct {
	URL           string          `json:"url"`
	Title         string          `json:"title"`
	Description   string          `json:"description,omitempty"`
	Depth         int             `json:"depth"`
	Forms         []FormModel     `json:"forms"`
	Buttons       []ButtonModel   `json:"buttons"`
	Links         []LinkModel     `json:"links"`
	Inputs        []InputModel    `json:"inputs"`
	Navigation    []NavItem       `json:"navigation"`
	Screenshots   []string        `json:"screenshots"` // S3 URIs
	DOMHash       string          `json:"dom_hash"`
	LoadTime      time.Duration   `json:"load_time"`
	HasAuth       bool            `json:"has_auth"`
	PageType      string          `json:"page_type"` // "landing", "form", "list", "detail", "auth", "error"
	CrawledAt     time.Time       `json:"crawled_at"`
}

// FormModel represents a form on a page
type FormModel struct {
	ID         string            `json:"id,omitempty"`
	Name       string            `json:"name,omitempty"`
	Action     string            `json:"action"`
	Method     string            `json:"method"`
	Fields     []FieldModel      `json:"fields"`
	Selectors  SelectorCandidates `json:"selectors"`
	SubmitText string            `json:"submit_text,omitempty"`
	FormType   string            `json:"form_type"` // "login", "signup", "search", "contact", "checkout", "generic"
}

// FieldModel represents a form field
type FieldModel struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Label       string            `json:"label,omitempty"`
	Placeholder string            `json:"placeholder,omitempty"`
	Required    bool              `json:"required"`
	Selectors   SelectorCandidates `json:"selectors"`
	Validation  string            `json:"validation,omitempty"` // "email", "phone", "password", etc.
}

// ButtonModel represents a button element
type ButtonModel struct {
	Text       string            `json:"text"`
	Type       string            `json:"type"` // "submit", "button", "reset"
	Selectors  SelectorCandidates `json:"selectors"`
	AriaLabel  string            `json:"aria_label,omitempty"`
	Disabled   bool              `json:"disabled"`
	OnClick    string            `json:"onclick,omitempty"`
}

// LinkModel represents a link element
type LinkModel struct {
	Text       string            `json:"text"`
	Href       string            `json:"href"`
	IsInternal bool              `json:"is_internal"`
	IsNavigation bool            `json:"is_navigation"`
	Selectors  SelectorCandidates `json:"selectors"`
}

// InputModel represents an input element outside of forms
type InputModel struct {
	Type        string            `json:"type"`
	Name        string            `json:"name,omitempty"`
	Placeholder string            `json:"placeholder,omitempty"`
	AriaLabel   string            `json:"aria_label,omitempty"`
	Selectors   SelectorCandidates `json:"selectors"`
}

// NavItem represents a navigation menu item
type NavItem struct {
	Text     string    `json:"text"`
	Href     string    `json:"href"`
	Children []NavItem `json:"children,omitempty"`
}

// SelectorCandidates stores multiple selector strategies for an element
type SelectorCandidates struct {
	TestID      string `json:"test_id,omitempty"`      // data-testid
	CypressID   string `json:"cypress_id,omitempty"`   // data-cy, data-test
	ID          string `json:"id,omitempty"`           // id attribute
	AriaLabel   string `json:"aria_label,omitempty"`   // aria-label
	Name        string `json:"name,omitempty"`         // name attribute
	CSS         string `json:"css"`                    // CSS selector (fallback)
	XPath       string `json:"xpath,omitempty"`        // XPath (last resort)
	TextContent string `json:"text_content,omitempty"` // For text-based selectors
}

// BestSelector returns the most reliable selector available
func (s SelectorCandidates) BestSelector() string {
	switch {
	case s.TestID != "":
		return `[data-testid="` + s.TestID + `"]`
	case s.CypressID != "":
		return `[data-cy="` + s.CypressID + `"]`
	case s.ID != "":
		return "#" + s.ID
	case s.AriaLabel != "":
		return `[aria-label="` + s.AriaLabel + `"]`
	case s.Name != "":
		return `[name="` + s.Name + `"]`
	default:
		return s.CSS
	}
}

// Component represents a reusable UI component
type Component struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"` // "header", "footer", "nav", "modal", "card", "form"
	Selectors   SelectorCandidates `json:"selectors"`
	FoundOnPages []string         `json:"found_on_pages"`
	DOMHash     string            `json:"dom_hash"`
}

// BusinessFlow represents a detected user journey
type BusinessFlow struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`        // "User Login", "Add to Cart"
	Description string     `json:"description"`
	Steps       []FlowStep `json:"steps"`       // Flow steps in order
	EntryPoint  string     `json:"entry_point"`
	Type        string     `json:"type"`        // "auth", "crud", "navigation", "checkout", "search"
	Priority    string     `json:"priority"`    // "critical", "high", "medium", "low"
	Confidence  float64    `json:"confidence"`  // How confident we are this is a real flow
}

// FlowStep represents a single step in a business flow
type FlowStep struct {
	Order    int    `json:"order"`
	Action   string `json:"action"`   // "navigate", "click", "fill", etc.
	URL      string `json:"url"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"`
}

// CrawlTask represents a page to be crawled
type CrawlTask struct {
	URL       string
	Depth     int
	ParentURL string
}

// CrawlResult represents the result of crawling a single page
type CrawlResult struct {
	Page       *PageModel
	NewLinks   []string
	Error      error
}

// HashDOM creates a hash of DOM content for change detection
func HashDOM(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:8])
}
