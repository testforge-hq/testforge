package testdesign

import (
	"time"

	"github.com/testforge/testforge/internal/domain"
)

// TestType categorizes the type of test
type TestType string

const (
	TestTypeSmoke         TestType = "smoke"
	TestTypeRegression    TestType = "regression"
	TestTypeE2E           TestType = "e2e"
	TestTypeNegative      TestType = "negative"
	TestTypeBoundary      TestType = "boundary"
	TestTypeSecurity      TestType = "security"
	TestTypeAccessibility TestType = "accessibility"
	TestTypePerformance   TestType = "performance"
)

// Priority indicates test importance
type Priority string

const (
	PriorityCritical Priority = "critical"
	PriorityHigh     Priority = "high"
	PriorityMedium   Priority = "medium"
	PriorityLow      Priority = "low"
)

// TestCategory classifies the test domain
type TestCategory string

const (
	CategoryFunctional    TestCategory = "functional"
	CategoryUI            TestCategory = "ui"
	CategoryAPI           TestCategory = "api"
	CategoryIntegration   TestCategory = "integration"
	CategorySecurity      TestCategory = "security"
	CategoryAccessibility TestCategory = "accessibility"
	CategoryPerformance   TestCategory = "performance"
)

// Action represents a test step action
type Action string

const (
	ActionNavigate          Action = "navigate"
	ActionClick             Action = "click"
	ActionDoubleClick       Action = "double_click"
	ActionRightClick        Action = "right_click"
	ActionFill              Action = "fill"
	ActionClear             Action = "clear"
	ActionSelect            Action = "select"
	ActionCheck             Action = "check"
	ActionUncheck           Action = "uncheck"
	ActionHover             Action = "hover"
	ActionDrag              Action = "drag"
	ActionUpload            Action = "upload"
	ActionKeyPress          Action = "key_press"
	ActionScroll            Action = "scroll"
	ActionWait              Action = "wait"
	ActionAssert            Action = "assert"
	ActionScreenshot        Action = "screenshot"
	ActionExecuteJS         Action = "execute_js"
	ActionAPICall           Action = "api_call"
	ActionStoreValue        Action = "store_value"
	ActionCompareScreenshot Action = "compare_screenshot"
)

// AssertionType defines the type of assertion
type AssertionType string

const (
	AssertVisible         AssertionType = "visible"
	AssertHidden          AssertionType = "hidden"
	AssertEnabled         AssertionType = "enabled"
	AssertDisabled        AssertionType = "disabled"
	AssertTextEquals      AssertionType = "text_equals"
	AssertTextContains    AssertionType = "text_contains"
	AssertTextMatches     AssertionType = "text_matches"
	AssertValueEquals     AssertionType = "value_equals"
	AssertURLEquals       AssertionType = "url_equals"
	AssertURLContains     AssertionType = "url_contains"
	AssertURLMatches      AssertionType = "url_matches"
	AssertTitleEquals     AssertionType = "title_equals"
	AssertTitleContains   AssertionType = "title_contains"
	AssertElementCount    AssertionType = "element_count"
	AssertAttribute       AssertionType = "attribute"
	AssertCSSProperty     AssertionType = "css_property"
	AssertChecked         AssertionType = "checked"
	AssertFocused         AssertionType = "focused"
	AssertAPIResponse     AssertionType = "api_response"
	AssertConsoleNoErrors AssertionType = "console_no_errors"
	AssertPerformance     AssertionType = "performance"
	AssertAccessibility   AssertionType = "accessibility"
)

// TestSuite - Top level organization
type TestSuite struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	ProjectID    string            `json:"project_id"`
	Version      string            `json:"version"`
	Features     []Feature         `json:"features"`
	GlobalConfig TestConfig        `json:"global_config"`
	DataSets     []TestDataSet     `json:"data_sets"`
	Roles        []TestRole        `json:"roles"`
	Compliance   ComplianceMapping `json:"compliance"`
	Metadata     SuiteMetadata     `json:"metadata"`
	Stats        TestPlanStats     `json:"stats"`
}

// TestConfig holds global test configuration
type TestConfig struct {
	BaseURL         string            `json:"base_url"`
	DefaultTimeout  string            `json:"default_timeout"`
	RetryConfig     RetryConfig       `json:"retry_config"`
	BrowserSettings BrowserSettings   `json:"browser_settings"`
	Screenshots     ScreenshotConfig  `json:"screenshots"`
	Environment     string            `json:"environment"`
	Variables       map[string]string `json:"variables"`
}

// BrowserSettings for test execution
type BrowserSettings struct {
	Browser        string `json:"browser"`
	Headless       bool   `json:"headless"`
	ViewportWidth  int    `json:"viewport_width"`
	ViewportHeight int    `json:"viewport_height"`
	DeviceScale    int    `json:"device_scale"`
	UserAgent      string `json:"user_agent,omitempty"`
}

// ScreenshotConfig for screenshot capture
type ScreenshotConfig struct {
	OnFailure    bool   `json:"on_failure"`
	OnSuccess    bool   `json:"on_success"`
	FullPage     bool   `json:"full_page"`
	Format       string `json:"format"`
	Quality      int    `json:"quality"`
	StoragePath  string `json:"storage_path"`
}

// SuiteMetadata contains generation metadata
type SuiteMetadata struct {
	GeneratedAt    time.Time `json:"generated_at"`
	GeneratedBy    string    `json:"generated_by"`
	ModelUsed      string    `json:"model_used"`
	AppModelRef    string    `json:"app_model_ref"`
	TokensUsed     int       `json:"tokens_used"`
	InputTokens    int       `json:"input_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	EstimatedCost  float64   `json:"estimated_cost"`
	GenerationTime string    `json:"generation_time"`
	Chunks         int       `json:"chunks"`
}

// Feature - Logical grouping of related scenarios
type Feature struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Tags          []string   `json:"tags"`
	Scenarios     []Scenario `json:"scenarios"`
	BaseURL       string     `json:"base_url,omitempty"`
	SetupSteps    []TestStep `json:"setup_steps,omitempty"`
	TeardownSteps []TestStep `json:"teardown_steps,omitempty"`
	Priority      Priority   `json:"priority"`
}

// Scenario - A specific user story or use case
type Scenario struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	UserStory   string     `json:"user_story"`
	TestCases   []TestCase `json:"test_cases"`
	Tags        []string   `json:"tags"`
	Priority    Priority   `json:"priority"`
}

// TestCase - Individual test with full enterprise metadata
type TestCase struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// Classification
	Type     TestType     `json:"type"`
	Priority Priority     `json:"priority"`
	Category TestCategory `json:"category"`
	Tags     []string     `json:"tags"`

	// BDD
	Given string `json:"given"`
	When  string `json:"when"`
	Then  string `json:"then"`

	// Execution
	TargetURL string     `json:"target_url"`
	Steps     []TestStep `json:"steps"`

	// Multi-role
	ApplicableRoles []string `json:"applicable_roles"`
	RequiredRole    string   `json:"required_role"`

	// Data
	TestData     map[string]string `json:"test_data"`
	DataVariants []DataVariant     `json:"data_variants"`

	// Dependencies
	DependsOn     []string   `json:"depends_on"`
	SetupSteps    []TestStep `json:"setup_steps"`
	TeardownSteps []TestStep `json:"teardown_steps"`

	// Execution control
	Parallelizable bool `json:"parallelizable"`
	Idempotent     bool `json:"idempotent"`
	Destructive    bool `json:"destructive"`

	// Retry
	RetryConfig RetryConfig `json:"retry_config"`

	// Compliance
	ComplianceRefs []string `json:"compliance_refs"`

	// Expected results
	ExpectedResults []ExpectedResult `json:"expected_results"`

	// Timing
	EstimatedDuration string `json:"estimated_duration"`
	Timeout           string `json:"timeout"`
}

// DataVariant for data-driven testing
type DataVariant struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Values          map[string]string `json:"values"`
	ExpectedOutcome string            `json:"expected_outcome"`
}

// RetryConfig for test retry strategy
type RetryConfig struct {
	MaxAttempts       int     `json:"max_attempts"`
	Delay             string  `json:"delay"`
	BackoffMultiplier float64 `json:"backoff_multiplier"`
}

// ExpectedResult defines expected test outcomes
type ExpectedResult struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Assertion   Assertion `json:"assertion"`
}

// TestStep - Enhanced with enterprise features
type TestStep struct {
	Order       int    `json:"order"`
	Action      Action `json:"action"`
	Description string `json:"description"`

	// Target
	Target    string              `json:"target,omitempty"`
	Selectors *SelectorCandidates `json:"selectors,omitempty"`

	// Value
	Value     string `json:"value,omitempty"`
	ValueFrom string `json:"value_from,omitempty"`

	// Assertions
	Assertions []Assertion `json:"assertions,omitempty"`

	// Waits
	WaitFor     string `json:"wait_for,omitempty"`
	WaitTimeout string `json:"wait_timeout,omitempty"`

	// Screenshots
	Screenshot     bool   `json:"screenshot"`
	ScreenshotName string `json:"screenshot_name,omitempty"`

	// Accessibility check at this step
	A11yCheck bool `json:"a11y_check"`

	// Performance marker
	PerfMarker string `json:"perf_marker,omitempty"`

	// Continue on failure?
	ContinueOnFailure bool `json:"continue_on_failure"`

	// Conditional execution
	Condition string `json:"condition,omitempty"`
}

// SelectorCandidates stores multiple selector strategies
type SelectorCandidates struct {
	Primary      string   `json:"primary"`
	Fallbacks    []string `json:"fallbacks"`
	Description  string   `json:"description"`
	Confidence   float64  `json:"confidence"`
	LastVerified string   `json:"last_verified,omitempty"`
}

// Assertion defines a test assertion
type Assertion struct {
	Type     AssertionType `json:"type"`
	Target   string        `json:"target,omitempty"`
	Value    string        `json:"value,omitempty"`
	Operator string        `json:"operator,omitempty"`
	Message  string        `json:"message"`
	Severity string        `json:"severity"`
	SoftFail bool          `json:"soft_fail"`
}

// TestRole defines a test execution role
type TestRole struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Permissions []string          `json:"permissions"`
	AuthConfig  *AuthConfigRef    `json:"auth_config,omitempty"`
	TestData    map[string]string `json:"test_data"`
	Constraints []string          `json:"constraints"`
}

// AuthConfigRef references authentication configuration
type AuthConfigRef struct {
	Type          string             `json:"type"`
	CredentialKey string             `json:"credential_key,omitempty"`
	Config        *domain.AuthConfig `json:"config,omitempty"`
}

// TestDataSet contains environment-specific test data
type TestDataSet struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Environment string                  `json:"environment"`
	Variables   map[string]TestVariable `json:"variables"`
}

// TestVariable defines a test variable with environment values
type TestVariable struct {
	Key          string `json:"key"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value"`
	DevValue     string `json:"dev_value"`
	StagingValue string `json:"staging_value"`
	ProdValue    string `json:"prod_value"`
	Sensitive    bool   `json:"sensitive"`
	Generator    string `json:"generator"`
	Validation   string `json:"validation"`
}

// ComplianceMapping maps tests to compliance requirements
type ComplianceMapping struct {
	WCAG   []ComplianceRef `json:"wcag"`
	GDPR   []ComplianceRef `json:"gdpr"`
	SOC2   []ComplianceRef `json:"soc2"`
	Custom []ComplianceRef `json:"custom"`
}

// ComplianceRef references a compliance requirement
type ComplianceRef struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Level       string   `json:"level,omitempty"`
	TestIDs     []string `json:"test_ids"`
	Evidence    string   `json:"evidence"`
}

// TestPlanStats provides test plan statistics
type TestPlanStats struct {
	TotalFeatures     int            `json:"total_features"`
	TotalScenarios    int            `json:"total_scenarios"`
	TotalTestCases    int            `json:"total_test_cases"`
	TotalSteps        int            `json:"total_steps"`
	ByType            map[string]int `json:"by_type"`
	ByPriority        map[string]int `json:"by_priority"`
	ByCategory        map[string]int `json:"by_category"`
	ByRole            map[string]int `json:"by_role"`
	EstimatedDuration string         `json:"estimated_duration"`
	CoverageScore     float64        `json:"coverage_score"`
	ComplianceScore   float64        `json:"compliance_score"`
}

// CalculateStats computes statistics for a test suite
func (s *TestSuite) CalculateStats() {
	stats := TestPlanStats{
		ByType:     make(map[string]int),
		ByPriority: make(map[string]int),
		ByCategory: make(map[string]int),
		ByRole:     make(map[string]int),
	}

	totalSteps := 0
	totalDuration := 0 // seconds

	for _, feature := range s.Features {
		stats.TotalFeatures++
		for _, scenario := range feature.Scenarios {
			stats.TotalScenarios++
			for _, tc := range scenario.TestCases {
				stats.TotalTestCases++
				totalSteps += len(tc.Steps)

				// Count by type
				stats.ByType[string(tc.Type)]++

				// Count by priority
				stats.ByPriority[string(tc.Priority)]++

				// Count by category
				stats.ByCategory[string(tc.Category)]++

				// Count by role
				for _, role := range tc.ApplicableRoles {
					stats.ByRole[role]++
				}

				// Estimate duration (default 30s per test)
				totalDuration += 30
			}
		}
	}

	stats.TotalSteps = totalSteps

	// Format duration
	hours := totalDuration / 3600
	minutes := (totalDuration % 3600) / 60
	if hours > 0 {
		stats.EstimatedDuration = formatDuration(hours, minutes)
	} else {
		stats.EstimatedDuration = formatMinutes(minutes)
	}

	// Calculate coverage score (simplified)
	stats.CoverageScore = calculateCoverageScore(stats)

	// Calculate compliance score
	stats.ComplianceScore = calculateComplianceScore(s)

	s.Stats = stats
}

func formatDuration(hours, minutes int) string {
	if minutes > 0 {
		return formatHours(hours) + " " + formatMinutes(minutes)
	}
	return formatHours(hours)
}

func formatHours(h int) string {
	if h == 1 {
		return "1 hour"
	}
	return string(rune('0'+h)) + " hours"
}

func formatMinutes(m int) string {
	if m == 1 {
		return "1 minute"
	}
	return string(rune('0'+m/10)) + string(rune('0'+m%10)) + " minutes"
}

func calculateCoverageScore(stats TestPlanStats) float64 {
	score := 0.0

	// Has smoke tests
	if stats.ByType["smoke"] > 0 {
		score += 20
	}
	// Has regression tests
	if stats.ByType["regression"] > 0 {
		score += 20
	}
	// Has negative tests
	if stats.ByType["negative"] > 0 {
		score += 15
	}
	// Has security tests
	if stats.ByType["security"] > 0 {
		score += 15
	}
	// Has accessibility tests
	if stats.ByType["accessibility"] > 0 {
		score += 15
	}
	// Has E2E tests
	if stats.ByType["e2e"] > 0 {
		score += 15
	}

	return score
}

func calculateComplianceScore(suite *TestSuite) float64 {
	score := 0.0
	total := 0.0

	// WCAG compliance
	if len(suite.Compliance.WCAG) > 0 {
		total += 25
		score += 25
	}

	// SOC2 compliance
	if len(suite.Compliance.SOC2) > 0 {
		total += 25
		score += 25
	}

	// GDPR compliance
	if len(suite.Compliance.GDPR) > 0 {
		total += 25
		score += 25
	}

	// Custom compliance
	if len(suite.Compliance.Custom) > 0 {
		total += 25
		score += 25
	}

	if total == 0 {
		return 0
	}
	return (score / 100) * 100
}
