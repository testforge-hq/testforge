package healing

import (
	"time"

	"github.com/google/uuid"
)

// FailureType categorizes the type of test failure
type FailureType string

const (
	FailureTypeSelector     FailureType = "selector"
	FailureTypeTimeout      FailureType = "timeout"
	FailureTypeAssertion    FailureType = "assertion"
	FailureTypeNavigation   FailureType = "navigation"
	FailureTypeVisual       FailureType = "visual"
	FailureTypeNetwork      FailureType = "network"
	FailureTypeUnknown      FailureType = "unknown"
)

// HealingStrategy defines how to attempt healing
type HealingStrategy string

const (
	StrategySelectoRepair   HealingStrategy = "selector_repair"
	StrategyVisualLocator   HealingStrategy = "visual_locator"
	StrategyWaitAdjustment  HealingStrategy = "wait_adjustment"
	StrategyRetry           HealingStrategy = "retry"
	StrategySkip            HealingStrategy = "skip"
)

// HealingStatus tracks the healing attempt status
type HealingStatus string

const (
	HealingStatusPending    HealingStatus = "pending"
	HealingStatusInProgress HealingStatus = "in_progress"
	HealingStatusSuccess    HealingStatus = "success"
	HealingStatusFailed     HealingStatus = "failed"
	HealingStatusSkipped    HealingStatus = "skipped"
)

// HealingRequest represents a request to heal a failed test
type HealingRequest struct {
	TestRunID     uuid.UUID              `json:"test_run_id"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	ProjectID     uuid.UUID              `json:"project_id"`
	TestName      string                 `json:"test_name"`
	TestFile      string                 `json:"test_file"`
	FailureType   FailureType            `json:"failure_type"`
	ErrorMessage  string                 `json:"error_message"`
	FailedStep    string                 `json:"failed_step"`
	FailedLine    int                    `json:"failed_line"`
	Selector      string                 `json:"selector,omitempty"`
	PageHTML      string                 `json:"page_html,omitempty"`
	PageURL       string                 `json:"page_url,omitempty"`
	ScreenshotURI string                 `json:"screenshot_uri,omitempty"`
	BaselineURI   string                 `json:"baseline_uri,omitempty"`
	TestCode      string                 `json:"test_code"`
	Context       map[string]interface{} `json:"context,omitempty"`
}

// HealingResult represents the outcome of a healing attempt
type HealingResult struct {
	RequestID       uuid.UUID              `json:"request_id"`
	TestRunID       uuid.UUID              `json:"test_run_id"`
	Status          HealingStatus          `json:"status"`
	Strategy        HealingStrategy        `json:"strategy"`
	OriginalError   string                 `json:"original_error"`
	HealedCode      string                 `json:"healed_code,omitempty"`
	HealedSelector  string                 `json:"healed_selector,omitempty"`
	Explanation     string                 `json:"explanation"`
	Confidence      float64                `json:"confidence"`
	Validated       bool                   `json:"validated"`
	ValidationScore float64                `json:"validation_score,omitempty"`
	Attempts        int                    `json:"attempts"`
	Duration        time.Duration          `json:"duration"`
	Suggestions     []HealingSuggestion    `json:"suggestions,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// HealingSuggestion provides alternative fixes
type HealingSuggestion struct {
	Strategy    HealingStrategy `json:"strategy"`
	Description string          `json:"description"`
	Code        string          `json:"code,omitempty"`
	Selector    string          `json:"selector,omitempty"`
	Confidence  float64         `json:"confidence"`
	Reasoning   string          `json:"reasoning"`
}

// SelectorRepairRequest is sent to Claude for selector repair
type SelectorRepairRequest struct {
	FailedSelector string   `json:"failed_selector"`
	ErrorMessage   string   `json:"error_message"`
	PageHTML       string   `json:"page_html"`
	PageURL        string   `json:"page_url"`
	TestContext    string   `json:"test_context"`
	TestCode       string   `json:"test_code"`
	FailedLine     int      `json:"failed_line"`
	Hints          []string `json:"hints,omitempty"`
}

// SelectorRepairResponse from Claude
type SelectorRepairResponse struct {
	RepairedSelector    string              `json:"repaired_selector"`
	AlternativeSelectors []AlternativeSelector `json:"alternative_selectors"`
	Explanation         string              `json:"explanation"`
	Confidence          float64             `json:"confidence"`
	ChangeType          SelectorChangeType  `json:"change_type"`
	RootCause           string              `json:"root_cause"`
}

// AlternativeSelector provides backup selectors
type AlternativeSelector struct {
	Selector   string  `json:"selector"`
	Type       string  `json:"type"` // css, xpath, text, role, testid
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// SelectorChangeType categorizes what changed
type SelectorChangeType string

const (
	ChangeTypeIDChanged      SelectorChangeType = "id_changed"
	ChangeTypeClassChanged   SelectorChangeType = "class_changed"
	ChangeTypeStructure      SelectorChangeType = "structure_changed"
	ChangeTypeTextChanged    SelectorChangeType = "text_changed"
	ChangeTypeElementRemoved SelectorChangeType = "element_removed"
	ChangeTypeElementMoved   SelectorChangeType = "element_moved"
	ChangeTypeUnknown        SelectorChangeType = "unknown"
)

// VisualHealingRequest for V-JEPA based healing
type VisualHealingRequest struct {
	TestRunID       uuid.UUID `json:"test_run_id"`
	CurrentFrame    []byte    `json:"current_frame"`
	BaselineFrame   []byte    `json:"baseline_frame"`
	CurrentFrameURI string    `json:"current_frame_uri,omitempty"`
	BaselineURI     string    `json:"baseline_uri,omitempty"`
	FailedSelector  string    `json:"failed_selector"`
	ExpectedRegion  *Region   `json:"expected_region,omitempty"`
}

// Region defines a rectangular area on screen
type Region struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// VisualHealingResponse from V-JEPA analysis
type VisualHealingResponse struct {
	ElementFound    bool      `json:"element_found"`
	NewLocation     *Region   `json:"new_location,omitempty"`
	Similarity      float64   `json:"similarity"`
	SuggestedAction string    `json:"suggested_action"`
	Confidence      float64   `json:"confidence"`
	ChangedRegions  []Region  `json:"changed_regions,omitempty"`
}

// HealingAttempt tracks individual healing attempts
type HealingAttempt struct {
	ID          uuid.UUID       `json:"id"`
	RequestID   uuid.UUID       `json:"request_id"`
	Strategy    HealingStrategy `json:"strategy"`
	Input       string          `json:"input"`
	Output      string          `json:"output"`
	Success     bool            `json:"success"`
	Error       string          `json:"error,omitempty"`
	Duration    time.Duration   `json:"duration"`
	Timestamp   time.Time       `json:"timestamp"`
}

// HealingConfig configures the healing service
type HealingConfig struct {
	// Claude API settings
	ClaudeAPIKey     string `json:"claude_api_key"`
	ClaudeModel      string `json:"claude_model"`
	ClaudeMaxTokens  int    `json:"claude_max_tokens"`

	// V-JEPA settings
	VJEPAEndpoint    string  `json:"vjepa_endpoint"`
	SimilarityThreshold float64 `json:"similarity_threshold"`

	// Healing behavior
	MaxAttempts      int     `json:"max_attempts"`
	MaxHTMLSize      int     `json:"max_html_size"`
	TimeoutSeconds   int     `json:"timeout_seconds"`
	MinConfidence    float64 `json:"min_confidence"`

	// Feature flags
	EnableVisualHealing  bool `json:"enable_visual_healing"`
	EnableAutoApply      bool `json:"enable_auto_apply"`
	EnableSuggestions    bool `json:"enable_suggestions"`
}

// DefaultHealingConfig returns sensible defaults
func DefaultHealingConfig() HealingConfig {
	return HealingConfig{
		ClaudeModel:         "claude-sonnet-4-20250514",
		ClaudeMaxTokens:     4096,
		VJEPAEndpoint:       "localhost:50051",
		SimilarityThreshold: 0.85,
		MaxAttempts:         3,
		MaxHTMLSize:         100000, // 100KB
		TimeoutSeconds:      60,
		MinConfidence:       0.7,
		EnableVisualHealing: true,
		EnableAutoApply:     false,
		EnableSuggestions:   true,
	}
}

// DetectFailureType analyzes error message to determine failure type
func DetectFailureType(errorMessage string) FailureType {
	// Common patterns for different failure types
	selectorPatterns := []string{
		"waiting for selector",
		"selector resolved to",
		"element not found",
		"locator resolved to",
		"no element matching",
		"getByRole",
		"getByText",
		"getByTestId",
	}

	timeoutPatterns := []string{
		"timeout",
		"exceeded",
		"timed out",
	}

	assertionPatterns := []string{
		"expect(",
		"toHaveText",
		"toBeVisible",
		"toBe(",
		"assertion failed",
		"AssertionError",
	}

	navigationPatterns := []string{
		"navigation",
		"net::ERR",
		"ERR_NAME",
		"ERR_CONNECTION",
		"page.goto",
	}

	networkPatterns := []string{
		"ECONNREFUSED",
		"ENOTFOUND",
		"fetch failed",
		"network error",
	}

	for _, pattern := range selectorPatterns {
		if containsIgnoreCase(errorMessage, pattern) {
			return FailureTypeSelector
		}
	}

	for _, pattern := range timeoutPatterns {
		if containsIgnoreCase(errorMessage, pattern) {
			return FailureTypeTimeout
		}
	}

	for _, pattern := range assertionPatterns {
		if containsIgnoreCase(errorMessage, pattern) {
			return FailureTypeAssertion
		}
	}

	for _, pattern := range navigationPatterns {
		if containsIgnoreCase(errorMessage, pattern) {
			return FailureTypeNavigation
		}
	}

	for _, pattern := range networkPatterns {
		if containsIgnoreCase(errorMessage, pattern) {
			return FailureTypeNetwork
		}
	}

	return FailureTypeUnknown
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
		 len(substr) == 0 ||
		 findIgnoreCase(s, substr) >= 0)
}

func findIgnoreCase(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if matchIgnoreCase(s[i:i+len(substr)], substr) {
			return i
		}
	}
	return -1
}

func matchIgnoreCase(s, substr string) bool {
	if len(s) != len(substr) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c1, c2 := s[i], substr[i]
		if c1 >= 'A' && c1 <= 'Z' {
			c1 += 32
		}
		if c2 >= 'A' && c2 <= 'Z' {
			c2 += 32
		}
		if c1 != c2 {
			return false
		}
	}
	return true
}
