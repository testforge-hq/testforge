package healing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"

	healingService "github.com/testforge/testforge/internal/services/healing"
)

// Activity handles self-healing operations
type Activity struct {
	service *healingService.Service
	logger  *zap.Logger
}

// Config for the healing activity
type Config struct {
	ClaudeAPIKey        string
	ClaudeModel         string
	VJEPAEndpoint       string
	SimilarityThreshold float64
	MaxAttempts         int
	TimeoutSeconds      int
	EnableVisualHealing bool
}

// NewActivity creates a new healing activity
func NewActivity(cfg Config, logger *zap.Logger) (*Activity, error) {
	serviceConfig := healingService.HealingConfig{
		ClaudeAPIKey:        cfg.ClaudeAPIKey,
		ClaudeModel:         cfg.ClaudeModel,
		ClaudeMaxTokens:     4096,
		VJEPAEndpoint:       cfg.VJEPAEndpoint,
		SimilarityThreshold: cfg.SimilarityThreshold,
		MaxAttempts:         cfg.MaxAttempts,
		TimeoutSeconds:      cfg.TimeoutSeconds,
		MinConfidence:       0.7,
		EnableVisualHealing: cfg.EnableVisualHealing,
		EnableAutoApply:     false,
		EnableSuggestions:   true,
	}

	// Apply defaults
	if serviceConfig.ClaudeModel == "" {
		serviceConfig.ClaudeModel = "claude-sonnet-4-20250514"
	}
	if serviceConfig.VJEPAEndpoint == "" {
		serviceConfig.VJEPAEndpoint = "localhost:50051"
	}
	if serviceConfig.SimilarityThreshold == 0 {
		serviceConfig.SimilarityThreshold = 0.85
	}
	if serviceConfig.MaxAttempts == 0 {
		serviceConfig.MaxAttempts = 3
	}
	if serviceConfig.TimeoutSeconds == 0 {
		serviceConfig.TimeoutSeconds = 60
	}

	service, err := healingService.NewService(serviceConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create healing service: %w", err)
	}

	return &Activity{
		service: service,
		logger:  logger,
	}, nil
}

// Close closes the activity resources
func (a *Activity) Close() error {
	return a.service.Close()
}

// HealInput is the input for the Heal activity
type HealInput struct {
	TestRunID uuid.UUID              `json:"test_run_id"`
	TenantID  uuid.UUID              `json:"tenant_id"`
	ProjectID uuid.UUID              `json:"project_id"`
	Failures  []FailureInfo          `json:"failures"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// FailureInfo contains information about a single failure
type FailureInfo struct {
	TestName      string `json:"test_name"`
	TestFile      string `json:"test_file"`
	ErrorMessage  string `json:"error_message"`
	FailedStep    string `json:"failed_step"`
	FailedLine    int    `json:"failed_line"`
	Selector      string `json:"selector,omitempty"`
	PageHTML      string `json:"page_html,omitempty"`
	PageURL       string `json:"page_url,omitempty"`
	ScreenshotURI string `json:"screenshot_uri,omitempty"`
	BaselineURI   string `json:"baseline_uri,omitempty"`
	TestCode      string `json:"test_code"`
}

// HealOutput is the output from the Heal activity
type HealOutput struct {
	TestRunID    uuid.UUID     `json:"test_run_id"`
	Results      []HealResult  `json:"results"`
	TotalHealed  int           `json:"total_healed"`
	TotalFailed  int           `json:"total_failed"`
	TotalSkipped int           `json:"total_skipped"`
	Duration     time.Duration `json:"duration"`
}

// HealResult contains the result for a single healing attempt
type HealResult struct {
	TestName        string                         `json:"test_name"`
	TestFile        string                         `json:"test_file"`
	Status          healingService.HealingStatus   `json:"status"`
	Strategy        healingService.HealingStrategy `json:"strategy"`
	HealedSelector  string                         `json:"healed_selector,omitempty"`
	HealedCode      string                         `json:"healed_code,omitempty"`
	Explanation     string                         `json:"explanation"`
	Confidence      float64                        `json:"confidence"`
	Validated       bool                           `json:"validated"`
	ValidationScore float64                        `json:"validation_score,omitempty"`
	Suggestions     []Suggestion                   `json:"suggestions,omitempty"`
	Error           string                         `json:"error,omitempty"`
}

// Suggestion represents a healing suggestion
type Suggestion struct {
	Strategy    string  `json:"strategy"`
	Description string  `json:"description"`
	Code        string  `json:"code,omitempty"`
	Selector    string  `json:"selector,omitempty"`
	Confidence  float64 `json:"confidence"`
	Reasoning   string  `json:"reasoning"`
}

// Heal attempts to heal failed tests
func (a *Activity) Heal(ctx context.Context, input HealInput) (*HealOutput, error) {
	info := activity.GetInfo(ctx)
	startTime := time.Now()

	a.logger.Info("starting healing activity",
		zap.String("activity_id", info.ActivityID),
		zap.String("test_run_id", input.TestRunID.String()),
		zap.Int("failure_count", len(input.Failures)))

	output := &HealOutput{
		TestRunID: input.TestRunID,
		Results:   make([]HealResult, 0, len(input.Failures)),
	}

	// Process each failure
	for i, failure := range input.Failures {
		// Record heartbeat
		activity.RecordHeartbeat(ctx, fmt.Sprintf("processing failure %d/%d: %s",
			i+1, len(input.Failures), failure.TestName))

		// Check for cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Build healing request
		healReq := &healingService.HealingRequest{
			TestRunID:     input.TestRunID,
			TenantID:      input.TenantID,
			ProjectID:     input.ProjectID,
			TestName:      failure.TestName,
			TestFile:      failure.TestFile,
			ErrorMessage:  failure.ErrorMessage,
			FailedStep:    failure.FailedStep,
			FailedLine:    failure.FailedLine,
			Selector:      failure.Selector,
			PageHTML:      failure.PageHTML,
			PageURL:       failure.PageURL,
			ScreenshotURI: failure.ScreenshotURI,
			BaselineURI:   failure.BaselineURI,
			TestCode:      failure.TestCode,
			Context:       input.Context,
		}

		// Auto-detect failure type
		healReq.FailureType = healingService.DetectFailureType(failure.ErrorMessage)

		// Attempt healing
		healResult, err := a.service.Heal(ctx, healReq)

		result := HealResult{
			TestName: failure.TestName,
			TestFile: failure.TestFile,
		}

		if err != nil {
			a.logger.Error("healing failed",
				zap.String("test_name", failure.TestName),
				zap.Error(err))
			result.Status = healingService.HealingStatusFailed
			result.Error = err.Error()
			output.TotalFailed++
		} else {
			result.Status = healResult.Status
			result.Strategy = healResult.Strategy
			result.HealedSelector = healResult.HealedSelector
			result.HealedCode = healResult.HealedCode
			result.Explanation = healResult.Explanation
			result.Confidence = healResult.Confidence
			result.Validated = healResult.Validated
			result.ValidationScore = healResult.ValidationScore

			// Convert suggestions
			if len(healResult.Suggestions) > 0 {
				result.Suggestions = make([]Suggestion, len(healResult.Suggestions))
				for j, s := range healResult.Suggestions {
					result.Suggestions[j] = Suggestion{
						Strategy:    string(s.Strategy),
						Description: s.Description,
						Code:        s.Code,
						Selector:    s.Selector,
						Confidence:  s.Confidence,
						Reasoning:   s.Reasoning,
					}
				}
			}

			switch healResult.Status {
			case healingService.HealingStatusSuccess:
				output.TotalHealed++
			case healingService.HealingStatusSkipped:
				output.TotalSkipped++
			default:
				output.TotalFailed++
			}
		}

		output.Results = append(output.Results, result)

		a.logger.Info("healing result",
			zap.String("test_name", failure.TestName),
			zap.String("status", string(result.Status)),
			zap.Float64("confidence", result.Confidence))
	}

	output.Duration = time.Since(startTime)

	a.logger.Info("healing activity completed",
		zap.String("test_run_id", input.TestRunID.String()),
		zap.Int("total_healed", output.TotalHealed),
		zap.Int("total_failed", output.TotalFailed),
		zap.Int("total_skipped", output.TotalSkipped),
		zap.Duration("duration", output.Duration))

	return output, nil
}

// HealthCheck verifies the healing activity is operational
func (a *Activity) HealthCheck(ctx context.Context) error {
	return a.service.HealthCheck(ctx)
}
