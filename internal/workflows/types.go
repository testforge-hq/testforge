package workflows

import (
	"time"

	"github.com/google/uuid"
)

// TestRunInput is the input for the master orchestration workflow
type TestRunInput struct {
	TestRunID   uuid.UUID `json:"test_run_id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	TargetURL   string    `json:"target_url"`
	TriggeredBy string    `json:"triggered_by"`

	// Project settings
	MaxCrawlDepth      int      `json:"max_crawl_depth"`
	MaxTestCases       int      `json:"max_test_cases"`
	EnableSelfHealing  bool     `json:"enable_self_healing"`
	EnableVisualTesting bool    `json:"enable_visual_testing"`
	ExcludePatterns    []string `json:"exclude_patterns,omitempty"`

	// AI settings
	EnableAIDiscovery bool `json:"enable_ai_discovery"` // Use AI-powered multi-agent discovery
	EnableABA         bool `json:"enable_aba"`          // Enable Autonomous Business Analyst

	// Browser settings
	Browser        string `json:"browser"`
	ViewportWidth  int    `json:"viewport_width"`
	ViewportHeight int    `json:"viewport_height"`
	Timeout        int    `json:"timeout_ms"`
}

// TestRunOutput is the output of the master orchestration workflow
type TestRunOutput struct {
	TestRunID     uuid.UUID      `json:"test_run_id"`
	Status        string         `json:"status"`
	Summary       *RunSummary    `json:"summary,omitempty"`
	ReportURL     string         `json:"report_url,omitempty"`
	Error         string         `json:"error,omitempty"`
	CompletedAt   time.Time      `json:"completed_at"`
	TotalDuration time.Duration  `json:"total_duration"`
}

// RunSummary contains test execution statistics
type RunSummary struct {
	TotalTests int           `json:"total_tests"`
	Passed     int           `json:"passed"`
	Failed     int           `json:"failed"`
	Skipped    int           `json:"skipped"`
	Healed     int           `json:"healed"`
	Duration   time.Duration `json:"duration"`
	PassRate   float64       `json:"pass_rate"`
}

// DiscoveryInput is input for the discovery activity
type DiscoveryInput struct {
	TestRunID       uuid.UUID `json:"test_run_id"`
	TargetURL       string    `json:"target_url"`
	MaxCrawlDepth   int       `json:"max_crawl_depth"`
	ExcludePatterns []string  `json:"exclude_patterns,omitempty"`
}

// DiscoveryOutput is output from the discovery activity
type DiscoveryOutput struct {
	AppModel      *AppModel     `json:"app_model"`
	PagesFound    int           `json:"pages_found"`
	FormsFound    int           `json:"forms_found"`
	FlowsDetected int           `json:"flows_detected"`
	Duration      time.Duration `json:"duration"`
}

// AppModel represents the discovered application structure
type AppModel struct {
	ID            string          `json:"id"`
	Pages         []PageInfo      `json:"pages"`
	Components    []ComponentInfo `json:"components"`
	BusinessFlows []FlowInfo      `json:"business_flows"`
	TechStack     []string        `json:"tech_stack,omitempty"`
}

// PageInfo represents a discovered page
type PageInfo struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	PageType    string `json:"page_type"`
	HasForms    bool   `json:"has_forms"`
	HasAuth     bool   `json:"has_auth"`
	Screenshot  string `json:"screenshot,omitempty"`
}

// ComponentInfo represents a UI component
type ComponentInfo struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Selector string   `json:"selector"`
	Pages    []string `json:"pages"`
}

// FlowInfo represents a business flow
type FlowInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Steps       []string `json:"steps"`
	Priority    string   `json:"priority"`
}

// TestDesignInput is input for the test design activity
type TestDesignInput struct {
	TestRunID    uuid.UUID `json:"test_run_id"`
	AppModel     *AppModel `json:"app_model"`
	MaxTestCases int       `json:"max_test_cases"`
}

// TestDesignOutput is output from the test design activity
type TestDesignOutput struct {
	TestPlan        *TestPlan     `json:"test_plan"`
	Duration        time.Duration `json:"duration"`
	TokensUsed      int           `json:"tokens_used,omitempty"`
	InputTokens     int           `json:"input_tokens,omitempty"`
	OutputTokens    int           `json:"output_tokens,omitempty"`
	EstimatedCost   float64       `json:"estimated_cost,omitempty"`
	ValidationWarns []string      `json:"validation_warnings,omitempty"`
}

// TestPlan contains generated test cases
type TestPlan struct {
	ID         string      `json:"id"`
	TestCases  []TestCase  `json:"test_cases"`
	TotalCount int         `json:"total_count"`
}

// TestCase represents a single test case
type TestCase struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
	Priority    string     `json:"priority"`
	Steps       []TestStep `json:"steps"`
}

// TestStep represents a step in a test case
type TestStep struct {
	Order    int    `json:"order"`
	Action   string `json:"action"`
	Target   string `json:"target,omitempty"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"`
	Expected string `json:"expected,omitempty"`
}

// AutomationInput is input for the automation activity
type AutomationInput struct {
	TestRunID uuid.UUID `json:"test_run_id"`
	TestPlan  *TestPlan `json:"test_plan"`
	Browser   string    `json:"browser"`
	Timeout   int       `json:"timeout_ms"`
}

// AutomationOutput is output from the automation activity
type AutomationOutput struct {
	Scripts   []TestScript  `json:"scripts"`
	Duration  time.Duration `json:"duration"`
}

// TestScript represents a generated Playwright script
type TestScript struct {
	TestCaseID string `json:"test_case_id"`
	Script     string `json:"script"`
	Language   string `json:"language"` // typescript, javascript
}

// ExecutionInput is input for the execution workflow
type ExecutionInput struct {
	TestRunID   uuid.UUID    `json:"test_run_id"`
	TenantID    uuid.UUID    `json:"tenant_id"`
	ProjectID   uuid.UUID    `json:"project_id"`
	Tier        string       `json:"tier"` // free, pro, enterprise
	Scripts     []TestScript `json:"scripts"`
	ScriptsURI  string       `json:"scripts_uri"`  // MinIO URI to scripts.zip
	TargetURL   string       `json:"target_url"`   // Target website URL
	Environment string       `json:"environment"`  // dev, staging, prod
	Parallel    int          `json:"parallel"`
	Browser     string       `json:"browser"`
	Timeout     int          `json:"timeout_ms"`
	Retries     int          `json:"retries"`
	Workers     int          `json:"workers"`
}

// ExecutionOutput is output from the execution workflow
type ExecutionOutput struct {
	Results        []TestResult  `json:"results"`
	Summary        *RunSummary   `json:"summary"`
	Duration       time.Duration `json:"duration"`
	ResultsURI     string        `json:"results_uri,omitempty"`
	ArtifactsURI   string        `json:"artifacts_uri,omitempty"`
	ScreenshotsURI string        `json:"screenshots_uri,omitempty"`
	LogsURI        string        `json:"logs_uri,omitempty"`
	ExitCode       int           `json:"exit_code"`
	Logs           string        `json:"logs,omitempty"`
}

// TestResult represents the result of a single test execution
type TestResult struct {
	TestCaseID    string        `json:"test_case_id"`
	Status        string        `json:"status"` // passed, failed, skipped
	Duration      time.Duration `json:"duration"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	ScreenshotURL string        `json:"screenshot_url,omitempty"`
	VideoURL      string        `json:"video_url,omitempty"`
}

// HealingInput is input for the self-healing activity
type HealingInput struct {
	TestRunID     uuid.UUID    `json:"test_run_id"`
	FailedResults []TestResult `json:"failed_results"`
	Scripts       []TestScript `json:"scripts"`
}

// HealingOutput is output from the self-healing activity
type HealingOutput struct {
	HealedResults []TestResult  `json:"healed_results"`
	HealedCount   int           `json:"healed_count"`
	Duration      time.Duration `json:"duration"`
}

// HealedResult extends TestResult with healing metadata
type HealedResult struct {
	TestResult
	OriginalError    string  `json:"original_error"`
	HealingStrategy  string  `json:"healing_strategy"`
	OriginalSelector string  `json:"original_selector"`
	HealedSelector   string  `json:"healed_selector"`
	Confidence       float64 `json:"confidence"`
}

// ReportInput is input for the reporting activity
type ReportInput struct {
	TestRunID uuid.UUID        `json:"test_run_id"`
	TenantID  uuid.UUID        `json:"tenant_id"`
	ProjectID uuid.UUID        `json:"project_id"`
	Results   []TestResult     `json:"results"`
	Summary   *RunSummary      `json:"summary"`
	AppModel  *AppModel        `json:"app_model"`
}

// ReportOutput is output from the reporting activity
type ReportOutput struct {
	ReportURL string        `json:"report_url"`
	Duration  time.Duration `json:"duration"`
}

// =============================================================================
// AI Agent Types - For multi-agent AI-powered analysis
// =============================================================================

// AIDiscoveryInput is input for AI-powered discovery activity
type AIDiscoveryInput struct {
	TestRunID         uuid.UUID `json:"test_run_id"`
	TargetURL         string    `json:"target_url"`
	MaxPages          int       `json:"max_pages"`
	MaxDepth          int       `json:"max_depth"`
	EnableABA         bool      `json:"enable_aba"`          // Enable Autonomous Business Analyst
	EnableSemanticAI  bool      `json:"enable_semantic_ai"`  // Enable semantic element understanding
	EnableVisualAI    bool      `json:"enable_visual_ai"`    // Enable visual AI analysis
	Headless          bool      `json:"headless"`
}

// AIDiscoveryOutput is output from AI-powered discovery activity
type AIDiscoveryOutput struct {
	AppModel       *AppModel        `json:"app_model"`
	Requirements   []Requirement    `json:"requirements,omitempty"`
	UserStories    []UserStory      `json:"user_stories,omitempty"`
	SemanticMap    *SemanticMap     `json:"semantic_map,omitempty"`
	PagesFound     int              `json:"pages_found"`
	ElementsFound  int              `json:"elements_found"`
	FlowsDetected  int              `json:"flows_detected"`
	Duration       time.Duration    `json:"duration"`
	TokensUsed     int              `json:"tokens_used"`
}

// Requirement represents a business requirement generated by ABA
type Requirement struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Type        string   `json:"type"`  // functional, non-functional, security, performance
	Priority    string   `json:"priority"`
	Source      string   `json:"source"` // Which page/flow this was derived from
	Tags        []string `json:"tags,omitempty"`
}

// UserStory represents a user story in standard agile format
type UserStory struct {
	ID                 string               `json:"id"`
	Title              string               `json:"title"`
	AsA                string               `json:"as_a"`
	IWant              string               `json:"i_want"`
	SoThat             string               `json:"so_that"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	Priority           string               `json:"priority"`
	StoryPoints        int                  `json:"story_points"`
	RelatedPages       []string             `json:"related_pages"`
	TestScenarios      []string             `json:"test_scenarios"`
}

// AcceptanceCriterion represents a single acceptance criterion
type AcceptanceCriterion struct {
	Given string `json:"given"`
	When  string `json:"when"`
	Then  string `json:"then"`
}

// SemanticMap represents the semantic understanding of the application
type SemanticMap struct {
	Domain           string                  `json:"domain"`
	Purpose          string                  `json:"purpose"`
	UserPersonas     []string                `json:"user_personas"`
	CoreFeatures     []string                `json:"core_features"`
	ElementPurposes  map[string]ElementIntent `json:"element_purposes"` // selector -> intent
}

// ElementIntent represents the semantic understanding of an element
type ElementIntent struct {
	Purpose       string   `json:"purpose"`      // What this element does
	Category      string   `json:"category"`     // auth, navigation, data-entry, action, display
	Importance    string   `json:"importance"`   // critical, high, medium, low
	RelatedFlows  []string `json:"related_flows"`
}

// PageAnalysisInput is input for page analysis activity
type PageAnalysisInput struct {
	TestRunID    uuid.UUID `json:"test_run_id"`
	PageURL      string    `json:"page_url"`
	HTML         string    `json:"html"`
	Screenshot   []byte    `json:"screenshot,omitempty"`
	AnalysisType string    `json:"analysis_type"` // full, elements, forms, flows
}

// PageAnalysisOutput is output from page analysis activity
type PageAnalysisOutput struct {
	PageType      string            `json:"page_type"`
	Purpose       string            `json:"purpose"`
	Elements      []ElementAnalysis `json:"elements"`
	Forms         []FormAnalysis    `json:"forms"`
	Navigation    []NavItem         `json:"navigation"`
	BusinessFlows []FlowSuggestion  `json:"business_flows"`
	Duration      time.Duration     `json:"duration"`
	TokensUsed    int               `json:"tokens_used"`
}

// ElementAnalysis represents AI analysis of a UI element
type ElementAnalysis struct {
	Selector    string   `json:"selector"`
	Type        string   `json:"type"`
	Purpose     string   `json:"purpose"`
	Label       string   `json:"label"`
	IsRequired  bool     `json:"is_required"`
	TestActions []string `json:"test_actions"`
}

// FormAnalysis represents AI analysis of a form
type FormAnalysis struct {
	Selector      string            `json:"selector"`
	Purpose       string            `json:"purpose"`
	Fields        []ElementAnalysis `json:"fields"`
	SubmitButton  string            `json:"submit_button"`
	ValidationRules []string        `json:"validation_rules"`
}

// NavItem represents a navigation item
type NavItem struct {
	Label    string `json:"label"`
	Href     string `json:"href"`
	Type     string `json:"type"` // primary, secondary, footer, dropdown
	Children []NavItem `json:"children,omitempty"`
}

// FlowSuggestion represents a detected business flow
type FlowSuggestion struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Steps       []string `json:"steps"`
	StartURL    string   `json:"start_url"`
	EndURL      string   `json:"end_url"`
	Confidence  float64  `json:"confidence"`
}

// ABAInput is input for the Autonomous Business Analyst activity
type ABAInput struct {
	TestRunID   uuid.UUID   `json:"test_run_id"`
	AppModel    *AppModel   `json:"app_model"`
	SemanticMap *SemanticMap `json:"semantic_map,omitempty"`
	Context     string      `json:"context,omitempty"` // Additional business context
}

// ABAOutput is output from the Autonomous Business Analyst activity
type ABAOutput struct {
	Requirements []Requirement `json:"requirements"`
	UserStories  []UserStory   `json:"user_stories"`
	Duration     time.Duration `json:"duration"`
	TokensUsed   int           `json:"tokens_used"`
}

// =============================================================================
// Status Update Types - For persisting workflow results to database
// =============================================================================

// StatusUpdateInput is input for status update activity
type StatusUpdateInput struct {
	TestRunID string `json:"test_run_id"`
	Status    string `json:"status"`
	Phase     string `json:"phase,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SaveAIAnalysisInput is input for saving AI analysis results
type SaveAIAnalysisInput struct {
	TestRunID    string            `json:"test_run_id"`
	Requirements []Requirement     `json:"requirements,omitempty"`
	UserStories  []UserStory       `json:"user_stories,omitempty"`
	SemanticMap  *SemanticMap      `json:"semantic_map,omitempty"`
	AgentsUsed   []string          `json:"agents_used"`
	TokensUsed   int               `json:"tokens_used"`
	Duration     time.Duration     `json:"duration"`
}

// SaveSummaryInput is input for saving final summary
type SaveSummaryInput struct {
	TestRunID string      `json:"test_run_id"`
	Summary   *RunSummary `json:"summary"`
	ReportURL string      `json:"report_url,omitempty"`
	Status    string      `json:"status"`
}
