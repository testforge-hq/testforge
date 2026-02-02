package scriptgen

import (
	"time"

	"github.com/testforge/testforge/internal/services/testdesign"
)

// GeneratorConfig configures the script generator
type GeneratorConfig struct {
	OutputDir        string
	TypeScriptStrict bool
	IncludeComments  bool
	GeneratePOM      bool // Page Object Model
	GenerateFixtures bool
	BaseURL          string
	TestRunID        string
	TenantID         string
	APIEndpoint      string
}

// DefaultGeneratorConfig returns sensible defaults
func DefaultGeneratorConfig() GeneratorConfig {
	return GeneratorConfig{
		OutputDir:        "generated",
		TypeScriptStrict: true,
		IncludeComments:  true,
		GeneratePOM:      true,
		GenerateFixtures: true,
		APIEndpoint:      "http://localhost:8081/api/v1",
	}
}

// GeneratedProject contains all generated files
type GeneratedProject struct {
	Files       map[string]string // filename -> content
	TestCount   int
	PageCount   int
	LinesOfCode int
	Metadata    ProjectMetadata
}

// ProjectMetadata contains generation metadata
type ProjectMetadata struct {
	GeneratedAt    time.Time `json:"generated_at"`
	SuiteID        string    `json:"suite_id"`
	SuiteName      string    `json:"suite_name"`
	TotalFeatures  int       `json:"total_features"`
	TotalScenarios int       `json:"total_scenarios"`
	TotalTests     int       `json:"total_tests"`
	TotalSteps     int       `json:"total_steps"`
	TokensUsed     int       `json:"tokens_used,omitempty"`
}

// PageObjectInfo contains information for generating a page object
type PageObjectInfo struct {
	Name        string
	Slug        string
	URL         string
	Title       string
	Description string
	Selectors   []SelectorInfo
	Actions     []ActionInfo
	Assertions  []AssertionInfo
}

// SelectorInfo describes a selector in a page object
type SelectorInfo struct {
	Name        string
	Description string
	Primary     string
	Fallbacks   []string
	Confidence  float64
	Type        string // "button", "input", "link", "form", etc.
}

// ActionInfo describes an action method in a page object
type ActionInfo struct {
	Name        string
	Description string
	Selector    string
	Action      string // "click", "fill", "select", etc.
	HasValue    bool
}

// AssertionInfo describes an assertion in a page object
type AssertionInfo struct {
	Name    string
	Type    string
	Target  string
	Value   string
	Message string
}

// TestFileInfo contains information for generating a test file
type TestFileInfo struct {
	Feature    testdesign.Feature
	Scenario   testdesign.Scenario
	TestCases  []testdesign.TestCase
	TestType   string // "smoke", "regression", "e2e", etc.
	PageImport string
}

// FixtureInfo contains information for generating fixtures
type FixtureInfo struct {
	Roles      []RoleInfo
	HasAuth    bool
	AuthMethod string
}

// RoleInfo describes a test role
type RoleInfo struct {
	Name        string
	StorageFile string
	Permissions []string
}

// GenerationResult contains the result of code generation
type GenerationResult struct {
	Success     bool
	FilesCount  int
	TestsCount  int
	LinesOfCode int
	Duration    time.Duration
	Errors      []string
	Warnings    []string
}
