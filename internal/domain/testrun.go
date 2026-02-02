package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TestRun represents a single test execution
type TestRun struct {
	ID              uuid.UUID        `json:"id" db:"id"`
	TenantID        uuid.UUID        `json:"tenant_id" db:"tenant_id"`
	ProjectID       uuid.UUID        `json:"project_id" db:"project_id"`
	Status          RunStatus        `json:"status" db:"status"`
	TargetURL       string           `json:"target_url" db:"target_url"`
	WorkflowID      string           `json:"workflow_id" db:"workflow_id"`
	WorkflowRunID   string           `json:"workflow_run_id" db:"workflow_run_id"`
	DiscoveryResult *DiscoveryResult `json:"discovery_result,omitempty" db:"discovery_result"`
	TestPlan        *TestPlan        `json:"test_plan,omitempty" db:"test_plan"`
	Summary         *RunSummary      `json:"summary,omitempty" db:"summary"`
	ReportURL       string           `json:"report_url,omitempty" db:"report_url"`
	TriggeredBy     string           `json:"triggered_by" db:"triggered_by"` // user_id, api, schedule, webhook
	StartedAt       *time.Time       `json:"started_at,omitempty" db:"started_at"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty" db:"completed_at"`
	Timestamps
}

// DiscoveryResult contains website analysis output
type DiscoveryResult struct {
	Pages          []DiscoveredPage     `json:"pages"`
	Components     []UIComponent        `json:"components"`
	BusinessFlows  []BusinessFlow       `json:"business_flows"`
	TotalPages     int                  `json:"total_pages"`
	TotalForms     int                  `json:"total_forms"`
	TotalLinks     int                  `json:"total_links"`
	TechStack      []string             `json:"tech_stack,omitempty"`
	CrawlDuration  time.Duration        `json:"crawl_duration"`
	Errors         []string             `json:"errors,omitempty"`
}

// DiscoveredPage represents a crawled page
type DiscoveredPage struct {
	URL          string            `json:"url"`
	Title        string            `json:"title"`
	Description  string            `json:"description,omitempty"`
	PageType     string            `json:"page_type"` // landing, form, list, detail, auth, error
	Elements     ElementCounts     `json:"elements"`
	Screenshots  []string          `json:"screenshots,omitempty"`
	LoadTime     time.Duration     `json:"load_time"`
	StatusCode   int               `json:"status_code"`
}

// ElementCounts tracks UI elements on a page
type ElementCounts struct {
	Forms    int `json:"forms"`
	Buttons  int `json:"buttons"`
	Links    int `json:"links"`
	Inputs   int `json:"inputs"`
	Tables   int `json:"tables"`
	Modals   int `json:"modals"`
	Images   int `json:"images"`
}

// UIComponent represents a reusable UI component
type UIComponent struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"` // navbar, sidebar, footer, form, modal, card, table
	Selector     string   `json:"selector"`
	Pages        []string `json:"pages"`
	IsGlobal     bool     `json:"is_global"`
}

// BusinessFlow represents a detected user journey
type BusinessFlow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"` // ordered page URLs
	FlowType    string   `json:"flow_type"` // auth, checkout, registration, search, crud
	Priority    Priority `json:"priority"`
}

// TestPlan contains generated test cases
type TestPlan struct {
	ID          string     `json:"id"`
	TestCases   []TestCase `json:"test_cases"`
	TotalCount  int        `json:"total_count"`
	ByPriority  map[Priority]int `json:"by_priority"`
	ByCategory  map[string]int   `json:"by_category"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// RunSummary provides execution statistics
type RunSummary struct {
	TotalTests    int           `json:"total_tests"`
	Passed        int           `json:"passed"`
	Failed        int           `json:"failed"`
	Skipped       int           `json:"skipped"`
	Healed        int           `json:"healed"`
	Flaky         int           `json:"flaky"`
	Duration      time.Duration `json:"duration"`
	PassRate      float64       `json:"pass_rate"`
	HealRate      float64       `json:"heal_rate"`
}

// NewTestRun creates a new test run
func NewTestRun(tenantID, projectID uuid.UUID, targetURL, triggeredBy string) *TestRun {
	now := time.Now().UTC()
	return &TestRun{
		ID:          uuid.New(),
		TenantID:    tenantID,
		ProjectID:   projectID,
		Status:      RunStatusPending,
		TargetURL:   targetURL,
		TriggeredBy: triggeredBy,
		Timestamps: Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// SetWorkflowInfo updates workflow tracking info
func (r *TestRun) SetWorkflowInfo(workflowID, runID string) {
	r.WorkflowID = workflowID
	r.WorkflowRunID = runID
	r.UpdatedAt = time.Now().UTC()
}

// Start marks the run as started
func (r *TestRun) Start() {
	now := time.Now().UTC()
	r.StartedAt = &now
	r.UpdatedAt = now
}

// Complete marks the run as completed
func (r *TestRun) Complete(summary *RunSummary, reportURL string) {
	now := time.Now().UTC()
	r.Status = RunStatusCompleted
	r.Summary = summary
	r.ReportURL = reportURL
	r.CompletedAt = &now
	r.UpdatedAt = now
}

// Fail marks the run as failed
func (r *TestRun) Fail(reason string) {
	now := time.Now().UTC()
	r.Status = RunStatusFailed
	r.CompletedAt = &now
	r.UpdatedAt = now
}

// TestRunRepository defines data access for test runs
type TestRunRepository interface {
	Create(ctx context.Context, run *TestRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*TestRun, error)
	GetByWorkflowID(ctx context.Context, workflowID string) (*TestRun, error)
	GetByProjectID(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]*TestRun, int, error)
	GetByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*TestRun, int, error)
	Update(ctx context.Context, run *TestRun) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status RunStatus) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountActiveByTenant(ctx context.Context, tenantID uuid.UUID) (int, error)
}

// CreateTestRunInput for API requests
type CreateTestRunInput struct {
	ProjectID uuid.UUID `json:"project_id"`
	TargetURL string    `json:"target_url,omitempty"` // defaults to project base_url
}
