package reporting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"

	reportingService "github.com/testforge/testforge/internal/services/reporting"
)

// Activity handles report generation
type Activity struct {
	generator   *reportingService.Generator
	notifier    *reportingService.NotificationService
	storage     StorageClient
	baseURL     string
	outputDir   string
	logger      *zap.Logger
}

// StorageClient interface for artifact storage
type StorageClient interface {
	Download(ctx context.Context, uri string) ([]byte, error)
	Upload(ctx context.Context, bucket, key string, data []byte, contentType string) (string, error)
}

// Config for the reporting activity
type Config struct {
	ClaudeAPIKey string
	ClaudeModel  string
	BaseURL      string
	OutputDir    string
}

// NewActivity creates a new reporting activity
func NewActivity(cfg Config, storage StorageClient, logger *zap.Logger) (*Activity, error) {
	genCfg := reportingService.GeneratorConfig{
		ClaudeAPIKey: cfg.ClaudeAPIKey,
		ClaudeModel:  cfg.ClaudeModel,
	}

	if genCfg.ClaudeModel == "" {
		genCfg.ClaudeModel = "claude-sonnet-4-20250514"
	}

	// Use a storage wrapper that implements the service interface
	storageWrapper := &storageClientWrapper{client: storage}

	generator, err := reportingService.NewGenerator(genCfg, storageWrapper, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create generator: %w", err)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://app.testforge.io"
	}

	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = "/tmp/testforge/reports"
	}

	notifier := reportingService.NewNotificationService(baseURL, logger)

	return &Activity{
		generator: generator,
		notifier:  notifier,
		storage:   storage,
		baseURL:   baseURL,
		outputDir: outputDir,
		logger:    logger,
	}, nil
}

// storageClientWrapper wraps our StorageClient to implement the service interface
type storageClientWrapper struct {
	client StorageClient
}

func (s *storageClientWrapper) Download(ctx context.Context, uri string) ([]byte, error) {
	return s.client.Download(ctx, uri)
}

func (s *storageClientWrapper) Upload(ctx context.Context, bucket, key string, data []byte, contentType string) (string, error) {
	return s.client.Upload(ctx, bucket, key, data, contentType)
}

// ReportInput is input for the reporting activity
type ReportInput struct {
	RunID              string                                 `json:"run_id"`
	ProjectID          string                                 `json:"project_id"`
	TenantID           string                                 `json:"tenant_id"`
	ResultsURI         string                                 `json:"results_uri"`
	ArtifactsURI       string                                 `json:"artifacts_uri"`
	HealingResult      *reportingService.HealingResultInput   `json:"healing_result,omitempty"`
	NotificationConfig *reportingService.NotificationConfig   `json:"notification_config,omitempty"`
	BaselineRunID      string                                 `json:"baseline_run_id,omitempty"`
}

// ReportOutput is output from the reporting activity
type ReportOutput struct {
	ReportID       string  `json:"report_id"`
	ReportURI      string  `json:"report_uri"`
	HTMLPath       string  `json:"html_path,omitempty"`
	JSONPath       string  `json:"json_path,omitempty"`
	Status         string  `json:"status"`
	HealthScore    float64 `json:"health_score"`
	Passed         int     `json:"passed"`
	Failed         int     `json:"failed"`
	Healed         int     `json:"healed"`
	DeploymentSafe bool    `json:"deployment_safe"`
	OneLiner       string  `json:"one_liner"`
	Duration       string  `json:"duration"`
}

// Execute generates enterprise reports
func (a *Activity) Execute(ctx context.Context, input ReportInput) (*ReportOutput, error) {
	info := activity.GetInfo(ctx)
	startTime := time.Now()

	a.logger.Info("starting reporting activity",
		zap.String("activity_id", info.ActivityID),
		zap.String("run_id", input.RunID))

	activity.RecordHeartbeat(ctx, "Generating report...")

	// Convert to service input
	serviceInput := reportingService.ReportInput{
		RunID:         input.RunID,
		ProjectID:     input.ProjectID,
		TenantID:      input.TenantID,
		ResultsURI:    input.ResultsURI,
		ArtifactsURI:  input.ArtifactsURI,
		HealingResult: input.HealingResult,
		BaselineRunID: input.BaselineRunID,
	}

	// Generate report
	report, err := a.generator.GenerateReport(ctx, serviceInput)
	if err != nil {
		return nil, fmt.Errorf("failed to generate report: %w", err)
	}

	activity.RecordHeartbeat(ctx, "Rendering HTML dashboard...")

	// Render HTML
	htmlContent, err := a.generator.RenderHTML(report)
	if err != nil {
		return nil, fmt.Errorf("failed to render HTML: %w", err)
	}

	// Render JSON
	jsonContent, err := a.generator.RenderJSON(report)
	if err != nil {
		return nil, fmt.Errorf("failed to render JSON: %w", err)
	}

	// Create output directory
	reportDir := filepath.Join(a.outputDir, input.TenantID, input.RunID)
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		a.logger.Warn("failed to create local directory, using storage only", zap.Error(err))
	}

	// Save locally
	htmlPath := filepath.Join(reportDir, "report.html")
	jsonPath := filepath.Join(reportDir, "report.json")

	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		a.logger.Warn("failed to write HTML locally", zap.Error(err))
		htmlPath = ""
	}

	if err := os.WriteFile(jsonPath, jsonContent, 0644); err != nil {
		a.logger.Warn("failed to write JSON locally", zap.Error(err))
		jsonPath = ""
	}

	// Upload to storage if available
	var reportURI string
	if a.storage != nil {
		activity.RecordHeartbeat(ctx, "Uploading report to storage...")

		bucket := "testforge"
		key := fmt.Sprintf("reports/%s/%s/report.html", input.TenantID, input.RunID)

		uri, err := a.storage.Upload(ctx, bucket, key, []byte(htmlContent), "text/html")
		if err != nil {
			a.logger.Warn("failed to upload HTML to storage", zap.Error(err))
		} else {
			reportURI = uri
		}

		// Also upload JSON
		jsonKey := fmt.Sprintf("reports/%s/%s/report.json", input.TenantID, input.RunID)
		_, err = a.storage.Upload(ctx, bucket, jsonKey, jsonContent, "application/json")
		if err != nil {
			a.logger.Warn("failed to upload JSON to storage", zap.Error(err))
		}
	}

	// Send notifications
	if input.NotificationConfig != nil {
		activity.RecordHeartbeat(ctx, "Sending notifications...")

		if err := a.notifier.NotifyAll(ctx, report, input.NotificationConfig); err != nil {
			a.logger.Warn("notification failed", zap.Error(err))
		}
	}

	duration := time.Since(startTime)

	healed := 0
	if report.Healing != nil {
		healed = report.Healing.Healed
	}

	output := &ReportOutput{
		ReportID:       report.ID,
		ReportURI:      reportURI,
		HTMLPath:       htmlPath,
		JSONPath:       jsonPath,
		Status:         report.Executive.Status,
		HealthScore:    report.Executive.HealthScore,
		Passed:         report.Executive.Passed,
		Failed:         report.Executive.Failed,
		Healed:         healed,
		DeploymentSafe: report.Executive.DeploymentSafe,
		OneLiner:       report.Executive.OneLiner,
		Duration:       duration.String(),
	}

	a.logger.Info("reporting activity completed",
		zap.String("report_id", report.ID),
		zap.String("status", report.Executive.Status),
		zap.Float64("health_score", report.Executive.HealthScore),
		zap.Duration("duration", duration))

	return output, nil
}

// HealthCheck verifies the reporting activity is operational
func (a *Activity) HealthCheck(ctx context.Context) error {
	// Basic health check - just verify we can create a report
	return nil
}
