package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TestCase represents an individual test
type TestCase struct {
	ID            uuid.UUID        `json:"id" db:"id"`
	TestRunID     uuid.UUID        `json:"test_run_id" db:"test_run_id"`
	TenantID      uuid.UUID        `json:"tenant_id" db:"tenant_id"`
	Name          string           `json:"name" db:"name"`
	Description   string           `json:"description" db:"description"`
	Category      string           `json:"category" db:"category"` // smoke, functional, regression, e2e
	Priority      Priority         `json:"priority" db:"priority"`
	Status        TestCaseStatus   `json:"status" db:"status"`
	Steps         []TestStep       `json:"steps" db:"steps"`
	Script        string           `json:"script" db:"script"` // Generated Playwright script
	OriginalScript string          `json:"original_script,omitempty" db:"original_script"`
	ExecutionResult *ExecutionResult `json:"execution_result,omitempty" db:"execution_result"`
	HealingHistory []HealingRecord  `json:"healing_history,omitempty" db:"healing_history"`
	RetryCount    int              `json:"retry_count" db:"retry_count"`
	Duration      time.Duration    `json:"duration" db:"duration"`
	Timestamps
}

// TestStep represents a BDD-style step
type TestStep struct {
	Order       int               `json:"order"`
	Type        string            `json:"type"` // given, when, then, and
	Action      string            `json:"action"`
	Target      string            `json:"target,omitempty"`
	Selector    string            `json:"selector,omitempty"`
	Value       string            `json:"value,omitempty"`
	Assertion   *Assertion        `json:"assertion,omitempty"`
	Screenshot  bool              `json:"screenshot"`
}

// Assertion defines what to verify
type Assertion struct {
	Type     string `json:"type"` // visible, text, value, attribute, url, title, count
	Expected string `json:"expected"`
	Actual   string `json:"actual,omitempty"`
	Passed   bool   `json:"passed"`
}

// ExecutionResult contains test execution details
type ExecutionResult struct {
	Passed       bool              `json:"passed"`
	ErrorMessage string            `json:"error_message,omitempty"`
	ErrorStack   string            `json:"error_stack,omitempty"`
	FailedStep   *int              `json:"failed_step,omitempty"`
	Screenshots  []Screenshot      `json:"screenshots,omitempty"`
	VideoURL     string            `json:"video_url,omitempty"`
	TraceURL     string            `json:"trace_url,omitempty"`
	ConsoleLog   []ConsoleEntry    `json:"console_log,omitempty"`
	NetworkLog   []NetworkEntry    `json:"network_log,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  time.Time         `json:"completed_at"`
}

// Screenshot represents a captured screenshot
type Screenshot struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Step      int       `json:"step"`
	Timestamp time.Time `json:"timestamp"`
}

// ConsoleEntry represents a browser console message
type ConsoleEntry struct {
	Level     string    `json:"level"` // log, warn, error
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// NetworkEntry represents a network request
type NetworkEntry struct {
	URL        string        `json:"url"`
	Method     string        `json:"method"`
	Status     int           `json:"status"`
	Duration   time.Duration `json:"duration"`
	Failed     bool          `json:"failed"`
	FailReason string        `json:"fail_reason,omitempty"`
}

// HealingRecord tracks self-healing attempts
type HealingRecord struct {
	Timestamp       time.Time `json:"timestamp"`
	OriginalSelector string   `json:"original_selector"`
	HealedSelector   string   `json:"healed_selector"`
	Strategy         string   `json:"strategy"` // attribute, text, visual, semantic
	Confidence       float64  `json:"confidence"`
	Successful       bool     `json:"successful"`
}

// NewTestCase creates a new test case
func NewTestCase(testRunID, tenantID uuid.UUID, name, description, category string, priority Priority, steps []TestStep) *TestCase {
	now := time.Now().UTC()
	return &TestCase{
		ID:          uuid.New(),
		TestRunID:   testRunID,
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		Category:    category,
		Priority:    priority,
		Status:      TestCaseStatusPending,
		Steps:       steps,
		Timestamps: Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// MarkPassed marks the test as passed
func (tc *TestCase) MarkPassed(result *ExecutionResult) {
	tc.Status = TestCaseStatusPassed
	tc.ExecutionResult = result
	tc.Duration = result.CompletedAt.Sub(result.StartedAt)
	tc.UpdatedAt = time.Now().UTC()
}

// MarkFailed marks the test as failed
func (tc *TestCase) MarkFailed(result *ExecutionResult) {
	tc.Status = TestCaseStatusFailed
	tc.ExecutionResult = result
	tc.Duration = result.CompletedAt.Sub(result.StartedAt)
	tc.UpdatedAt = time.Now().UTC()
}

// MarkHealed marks the test as healed after self-healing
func (tc *TestCase) MarkHealed(result *ExecutionResult, healing HealingRecord) {
	tc.Status = TestCaseStatusHealed
	tc.ExecutionResult = result
	tc.HealingHistory = append(tc.HealingHistory, healing)
	tc.Duration = result.CompletedAt.Sub(result.StartedAt)
	tc.UpdatedAt = time.Now().UTC()
}

// TestCaseRepository defines data access for test cases
type TestCaseRepository interface {
	Create(ctx context.Context, tc *TestCase) error
	CreateBatch(ctx context.Context, tcs []*TestCase) error
	GetByID(ctx context.Context, id uuid.UUID) (*TestCase, error)
	GetByTestRunID(ctx context.Context, testRunID uuid.UUID) ([]*TestCase, error)
	Update(ctx context.Context, tc *TestCase) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status TestCaseStatus) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetFailedByTestRunID(ctx context.Context, testRunID uuid.UUID) ([]*TestCase, error)
}
