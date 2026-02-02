package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Report represents a generated test report
type Report struct {
	ID          uuid.UUID     `json:"id" db:"id"`
	TestRunID   uuid.UUID     `json:"test_run_id" db:"test_run_id"`
	TenantID    uuid.UUID     `json:"tenant_id" db:"tenant_id"`
	Type        ReportType    `json:"type" db:"type"`
	Format      ReportFormat  `json:"format" db:"format"`
	URL         string        `json:"url" db:"url"`
	Size        int64         `json:"size" db:"size"`
	Summary     ReportSummary `json:"summary" db:"summary"`
	GeneratedAt time.Time     `json:"generated_at" db:"generated_at"`
	ExpiresAt   *time.Time    `json:"expires_at,omitempty" db:"expires_at"`
	Timestamps
}

// ReportType indicates what kind of report
type ReportType string

const (
	ReportTypeFull       ReportType = "full"
	ReportTypeSummary    ReportType = "summary"
	ReportTypeFailures   ReportType = "failures"
	ReportTypeHealing    ReportType = "healing"
	ReportTypeComparison ReportType = "comparison"
)

// ReportFormat indicates output format
type ReportFormat string

const (
	ReportFormatHTML ReportFormat = "html"
	ReportFormatPDF  ReportFormat = "pdf"
	ReportFormatJSON ReportFormat = "json"
	ReportFormatJUnit ReportFormat = "junit"
)

// ReportSummary contains report metadata
type ReportSummary struct {
	TestRunID    uuid.UUID            `json:"test_run_id"`
	ProjectName  string               `json:"project_name"`
	TargetURL    string               `json:"target_url"`
	ExecutedAt   time.Time            `json:"executed_at"`
	Duration     time.Duration        `json:"duration"`
	TotalTests   int                  `json:"total_tests"`
	Passed       int                  `json:"passed"`
	Failed       int                  `json:"failed"`
	Skipped      int                  `json:"skipped"`
	Healed       int                  `json:"healed"`
	PassRate     float64              `json:"pass_rate"`
	ByCategory   map[string]CategoryStats `json:"by_category"`
	ByPriority   map[Priority]PriorityStats `json:"by_priority"`
	TopFailures  []FailureSummary     `json:"top_failures,omitempty"`
	Trends       *TrendData           `json:"trends,omitempty"`
}

// CategoryStats provides stats per test category
type CategoryStats struct {
	Total   int     `json:"total"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	PassRate float64 `json:"pass_rate"`
}

// PriorityStats provides stats per priority level
type PriorityStats struct {
	Total   int     `json:"total"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	PassRate float64 `json:"pass_rate"`
}

// FailureSummary for quick failure overview
type FailureSummary struct {
	TestName     string `json:"test_name"`
	ErrorMessage string `json:"error_message"`
	Category     string `json:"category"`
	Priority     Priority `json:"priority"`
	ScreenshotURL string `json:"screenshot_url,omitempty"`
}

// TrendData for historical comparison
type TrendData struct {
	PreviousRuns []RunTrend `json:"previous_runs"`
	PassRateTrend string    `json:"pass_rate_trend"` // improving, stable, declining
	AvgDuration   time.Duration `json:"avg_duration"`
}

// RunTrend contains historical run data
type RunTrend struct {
	RunID     uuid.UUID     `json:"run_id"`
	Date      time.Time     `json:"date"`
	PassRate  float64       `json:"pass_rate"`
	Duration  time.Duration `json:"duration"`
	Total     int           `json:"total"`
}

// NewReport creates a new report
func NewReport(testRunID, tenantID uuid.UUID, reportType ReportType, format ReportFormat, url string, size int64, summary ReportSummary) *Report {
	now := time.Now().UTC()
	return &Report{
		ID:          uuid.New(),
		TestRunID:   testRunID,
		TenantID:    tenantID,
		Type:        reportType,
		Format:      format,
		URL:         url,
		Size:        size,
		Summary:     summary,
		GeneratedAt: now,
		Timestamps: Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// ReportRepository defines data access for reports
type ReportRepository interface {
	Create(ctx context.Context, report *Report) error
	GetByID(ctx context.Context, id uuid.UUID) (*Report, error)
	GetByTestRunID(ctx context.Context, testRunID uuid.UUID) ([]*Report, error)
	GetLatestByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*Report, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context) (int, error)
}
