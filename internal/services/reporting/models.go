package reporting

import (
	"time"

	"github.com/google/uuid"
)

// TestRunReport - Complete enterprise report
type TestRunReport struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	ProjectID   string    `json:"project_id"`
	TenantID    string    `json:"tenant_id"`
	GeneratedAt time.Time `json:"generated_at"`

	// Executive Summary (C-level readable)
	Executive ExecutiveSummary `json:"executive_summary"`

	// Test Results
	Results TestResults `json:"results"`

	// Self-Healing Results
	Healing *HealingReport `json:"healing,omitempty"`

	// AI Analysis
	AIInsights *AIAnalysis `json:"ai_insights,omitempty"`

	// Visual Analysis
	VisualDiffs []VisualDiff `json:"visual_diffs,omitempty"`

	// Artifacts
	Artifacts ArtifactManifest `json:"artifacts"`

	// Compliance
	Compliance ComplianceReport `json:"compliance"`

	// Performance
	Performance PerformanceReport `json:"performance"`

	// Accessibility
	Accessibility A11yReport `json:"accessibility"`

	// Audit Trail
	AuditTrail []AuditEntry `json:"audit_trail"`
}

// ExecutiveSummary - One-page summary for stakeholders
type ExecutiveSummary struct {
	Status    string  `json:"status"`       // passed, failed, unstable
	HealthScore float64 `json:"health_score"` // 0-100
	RiskLevel   string  `json:"risk_level"`   // low, medium, high, critical

	// Counts
	TotalTests int `json:"total_tests"`
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
	Flaky      int `json:"flaky"`
	Healed     int `json:"healed"` // Auto-healed tests

	// Business Impact
	CriticalFailures int      `json:"critical_failures"`
	BlockingIssues   []string `json:"blocking_issues"`

	// Trends
	TrendVsPrevious float64 `json:"trend_vs_previous"` // % change

	// Timing
	Duration    string    `json:"duration"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`

	// AI Recommendations
	TopRecommendations []string `json:"top_recommendations"`

	// One-liner for Slack
	OneLiner string `json:"one_liner"`

	// Deployment recommendation
	DeploymentSafe   bool   `json:"deployment_safe"`
	DeploymentReason string `json:"deployment_reason"`
}

// TestResults - Detailed results
type TestResults struct {
	Suites      []SuiteResult   `json:"suites"`
	FailedTests []TestResult    `json:"failed_tests"`
	FlakyTests  []TestResult    `json:"flaky_tests"`
	SlowTests   []TestResult    `json:"slow_tests"`

	ByStatus   map[string]int `json:"by_status"`
	ByType     map[string]int `json:"by_type"`
	ByPriority map[string]int `json:"by_priority"`
	ByFeature  map[string]int `json:"by_feature"`
}

// SuiteResult represents a test suite
type SuiteResult struct {
	Name     string       `json:"name"`
	Status   string       `json:"status"`
	Duration string       `json:"duration"`
	Passed   int          `json:"passed"`
	Failed   int          `json:"failed"`
	Skipped  int          `json:"skipped"`
	Tests    []TestResult `json:"tests"`
}

// TestResult represents a single test
type TestResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Suite      string `json:"suite"`
	Status     string `json:"status"`
	Duration   string `json:"duration"`
	RetryCount int    `json:"retry_count"`

	// BDD
	Given string `json:"given"`
	When  string `json:"when"`
	Then  string `json:"then"`

	// Failure details
	Error *ErrorDetail `json:"error,omitempty"`

	// Steps
	Steps []StepResult `json:"steps"`

	// Artifacts
	ScreenshotURI string `json:"screenshot_uri,omitempty"`
	VideoURI      string `json:"video_uri,omitempty"`
	TraceURI      string `json:"trace_uri,omitempty"`

	// AI Analysis for this test
	AIAnalysis *TestAIAnalysis `json:"ai_analysis,omitempty"`

	// Healing info
	WasHealed   bool         `json:"was_healed"`
	HealingInfo *HealingInfo `json:"healing_info,omitempty"`

	// Tags
	Tags     []string `json:"tags"`
	Priority string   `json:"priority"`
}

// ErrorDetail contains failure information
type ErrorDetail struct {
	Message        string `json:"message"`
	Stack          string `json:"stack"`
	ScreenshotURI  string `json:"screenshot_uri"`
	VideoTimestamp string `json:"video_timestamp"`
	URL            string `json:"url"`
	Selector       string `json:"selector,omitempty"`
}

// StepResult represents a test step
type StepResult struct {
	Order       int    `json:"order"`
	Action      string `json:"action"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Duration    string `json:"duration"`
	Screenshot  string `json:"screenshot,omitempty"`
}

// HealingReport - Self-healing summary
type HealingReport struct {
	TotalAttempted int           `json:"total_attempted"`
	Healed         int           `json:"healed"`
	Failed         int           `json:"failed"`
	NeedsReview    int           `json:"needs_review"`

	HealingDetails []HealingInfo `json:"healing_details"`

	// Cost/benefit
	TimesSaved     string `json:"times_saved"` // "~2 hours of manual fixes"
	SelectorsFixed int    `json:"selectors_fixed"`
}

// HealingInfo contains healing details for a test
type HealingInfo struct {
	TestID           string  `json:"test_id"`
	TestName         string  `json:"test_name"`
	OriginalSelector string  `json:"original_selector"`
	NewSelector      string  `json:"new_selector"`
	RootCause        string  `json:"root_cause"`
	Confidence       float64 `json:"confidence"`
	VJEPASimilarity  float64 `json:"vjepa_similarity"`
}

// AIAnalysis - Claude-generated insights
type AIAnalysis struct {
	GeneratedAt     time.Time        `json:"generated_at"`
	Model           string           `json:"model"`

	Summary         string           `json:"summary"`
	FailurePatterns []FailurePattern `json:"failure_patterns"`
	RootCauses      []RootCause      `json:"root_causes"`
	Recommendations []Recommendation `json:"recommendations"`
	RiskAssessment  RiskAssessment   `json:"risk_assessment"`
}

// FailurePattern identifies common failure patterns
type FailurePattern struct {
	Pattern       string   `json:"pattern"`
	Occurrences   int      `json:"occurrences"`
	AffectedTests []string `json:"affected_tests"`
	Severity      string   `json:"severity"`
	Suggestion    string   `json:"suggestion"`
}

// RootCause identifies a root cause
type RootCause struct {
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Evidence    string  `json:"evidence"`
	Fix         string  `json:"fix"`
	Confidence  float64 `json:"confidence"`
}

// Recommendation from AI analysis
type Recommendation struct {
	Priority    string `json:"priority"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Effort      string `json:"effort"`
}

// RiskAssessment evaluates deployment risk
type RiskAssessment struct {
	OverallRisk    string            `json:"overall_risk"`
	DeploymentSafe bool              `json:"deployment_safe"`
	BlockingIssues []string          `json:"blocking_issues"`
	Warnings       []string          `json:"warnings"`
	AreaRisks      map[string]string `json:"area_risks"`
}

// TestAIAnalysis is AI analysis for a single test
type TestAIAnalysis struct {
	FailureReason string  `json:"failure_reason"`
	RootCause     string  `json:"root_cause"`
	SuggestedFix  string  `json:"suggested_fix"`
	Confidence    float64 `json:"confidence"`
}

// VisualDiff - V-JEPA/DINOv2 visual analysis
type VisualDiff struct {
	TestID          string  `json:"test_id"`
	TestName        string  `json:"test_name"`
	BaselineURI     string  `json:"baseline_uri"`
	ActualURI       string  `json:"actual_uri"`
	DiffURI         string  `json:"diff_uri"`
	SimilarityScore float64 `json:"similarity_score"`
	SemanticMatch   bool    `json:"semantic_match"`
	Analysis        string  `json:"analysis"`
}

// ArtifactManifest - All collected artifacts
type ArtifactManifest struct {
	BaseURI     string        `json:"base_uri"`
	Screenshots []ArtifactRef `json:"screenshots"`
	Videos      []ArtifactRef `json:"videos"`
	Traces      []ArtifactRef `json:"traces"`
	Logs        *ArtifactRef  `json:"logs,omitempty"`
	TotalSize   string        `json:"total_size"`
	TotalFiles  int           `json:"total_files"`
}

// ArtifactRef references an artifact
type ArtifactRef struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// ComplianceReport - Audit-ready
type ComplianceReport struct {
	Standards      []ComplianceStandard `json:"standards"`
	OverallScore   float64              `json:"overall_score"`
	EvidenceZipURI string               `json:"evidence_zip_uri"`
}

// ComplianceStandard represents a compliance standard
type ComplianceStandard struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	Score    float64 `json:"score"`
	Controls int     `json:"controls_tested"`
	Passed   int     `json:"passed"`
}

// PerformanceReport - Web vitals
type PerformanceReport struct {
	WebVitals       WebVitals   `json:"web_vitals"`
	SlowPages       []SlowPage  `json:"slow_pages"`
	Recommendations []string    `json:"recommendations"`
}

// WebVitals contains core web vitals
type WebVitals struct {
	LCP  VitalMetric `json:"lcp"`
	FID  VitalMetric `json:"fid"`
	CLS  VitalMetric `json:"cls"`
	FCP  VitalMetric `json:"fcp"`
	TTFB VitalMetric `json:"ttfb"`
}

// VitalMetric is a single web vital
type VitalMetric struct {
	Value  float64 `json:"value"`
	Rating string  `json:"rating"` // good, needs-improvement, poor
	Unit   string  `json:"unit"`
}

// SlowPage identifies slow pages
type SlowPage struct {
	URL      string `json:"url"`
	LoadTime string `json:"load_time"`
	Issue    string `json:"issue"`
}

// A11yReport - Accessibility
type A11yReport struct {
	Standard   string      `json:"standard"`
	Score      float64     `json:"score"`
	Violations []A11yIssue `json:"violations"`
	Passes     int         `json:"passes"`
}

// A11yIssue is an accessibility violation
type A11yIssue struct {
	ID          string `json:"id"`
	Impact      string `json:"impact"`
	Description string `json:"description"`
	HelpURL     string `json:"help_url"`
	Count       int    `json:"count"`
}

// AuditEntry - Chain of custody
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	Details   string    `json:"details"`
}

// ReportInput is the input for report generation
type ReportInput struct {
	RunID              string              `json:"run_id"`
	ProjectID          string              `json:"project_id"`
	TenantID           string              `json:"tenant_id"`
	ResultsURI         string              `json:"results_uri"`
	ArtifactsURI       string              `json:"artifacts_uri"`
	HealingResult      *HealingResultInput `json:"healing_result,omitempty"`
	NotificationConfig *NotificationConfig `json:"notification_config,omitempty"`
	BaselineRunID      string              `json:"baseline_run_id,omitempty"`
}

// HealingResultInput from healing phase
type HealingResultInput struct {
	TotalAttempted int           `json:"total_attempted"`
	Healed         int           `json:"healed"`
	Failed         int           `json:"failed"`
	Details        []HealingInfo `json:"details"`
}

// ReportOutput is the output from report generation
type ReportOutput struct {
	ReportID       string  `json:"report_id"`
	ReportURI      string  `json:"report_uri"`
	PDFURI         string  `json:"pdf_uri,omitempty"`
	Status         string  `json:"status"`
	HealthScore    float64 `json:"health_score"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	Healed         int     `json:"healed"`
	DeploymentSafe bool    `json:"deployment_safe"`
	OneLiner       string  `json:"one_liner"`
}

// NotificationConfig for alerts
type NotificationConfig struct {
	Slack   *SlackConfig   `json:"slack,omitempty"`
	Webhook *WebhookConfig `json:"webhook,omitempty"`
	Email   *EmailConfig   `json:"email,omitempty"`
}

// SlackConfig for Slack notifications
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel"`
	OnFailure  bool   `json:"on_failure"`
	OnSuccess  bool   `json:"on_success"`
}

// WebhookConfig for generic webhooks
type WebhookConfig struct {
	URL    string            `json:"url"`
	Secret string            `json:"secret,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// EmailConfig for email notifications
type EmailConfig struct {
	Recipients []string `json:"recipients"`
	OnFailure  bool     `json:"on_failure"`
	OnSuccess  bool     `json:"on_success"`
}

// NewTestRunReport creates a new report with defaults
func NewTestRunReport(runID, projectID, tenantID string) *TestRunReport {
	return &TestRunReport{
		ID:          uuid.New().String(),
		RunID:       runID,
		ProjectID:   projectID,
		TenantID:    tenantID,
		GeneratedAt: time.Now(),
		Results: TestResults{
			ByStatus:   make(map[string]int),
			ByType:     make(map[string]int),
			ByPriority: make(map[string]int),
			ByFeature:  make(map[string]int),
		},
		AuditTrail: []AuditEntry{},
	}
}
