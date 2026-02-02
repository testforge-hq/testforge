package healing

import (
	"context"
	"math/rand"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements the self-healing activity
type Activity struct {
	// Dependencies will be added here (V-JEPA client, LLM client, etc.)
}

// NewActivity creates a new healing activity
func NewActivity() *Activity {
	return &Activity{}
}

// Execute attempts to heal failed tests using semantic matching
func (a *Activity) Execute(ctx context.Context, input workflows.HealingInput) (*workflows.HealingOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting self-healing activity",
		"test_run_id", input.TestRunID.String(),
		"failed_tests", len(input.FailedResults),
	)

	activity.RecordHeartbeat(ctx, "Analyzing failures...")

	// TODO: Implement actual self-healing with V-JEPA and LLM
	// For now, simulate healing with 50% success rate

	healedResults := []workflows.HealedResult{}
	healedCount := 0

	for i, failedResult := range input.FailedResults {
		activity.RecordHeartbeat(ctx, map[string]interface{}{
			"progress": float64(i+1) / float64(len(input.FailedResults)) * 100,
			"current":  i + 1,
			"total":    len(input.FailedResults),
		})

		// Simulate healing attempt
		time.Sleep(200 * time.Millisecond)

		// 50% chance of successful healing
		if rand.Intn(100) < 50 {
			healedResult := workflows.TestResult{
				TestCaseID:    failedResult.TestCaseID,
				Status:        "healed",
				Duration:      failedResult.Duration + 500*time.Millisecond,
				ScreenshotURL: failedResult.ScreenshotURL,
				VideoURL:      failedResult.VideoURL,
			}
			healedResults = append(healedResults, workflows.HealedResult{
				TestResult:       healedResult,
				OriginalError:    failedResult.ErrorMessage,
				HealingStrategy:  selectHealingStrategy(failedResult.ErrorMessage),
				OriginalSelector: "button.submit",
				HealedSelector:   "button[data-testid='submit']",
				Confidence:       0.85 + rand.Float64()*0.14,
			})
			healedCount++

			logger.Info("Test healed successfully",
				"test_case_id", failedResult.TestCaseID,
				"strategy", selectHealingStrategy(failedResult.ErrorMessage),
			)
		} else {
			logger.Info("Healing attempt failed",
				"test_case_id", failedResult.TestCaseID,
				"error", failedResult.ErrorMessage,
			)
		}
	}

	duration := time.Since(startTime)

	// Convert to TestResult slice
	testResults := make([]workflows.TestResult, len(healedResults))
	for i, hr := range healedResults {
		testResults[i] = hr.TestResult
	}

	output := &workflows.HealingOutput{
		HealedResults: testResults,
		HealedCount:   healedCount,
		Duration:      duration,
	}

	logger.Info("Self-healing activity completed",
		"attempted", len(input.FailedResults),
		"healed", healedCount,
		"duration", duration,
	)

	return output, nil
}

func selectHealingStrategy(errorMessage string) string {
	strategies := []string{
		"semantic_match",
		"attribute_fallback",
		"visual_match",
		"text_content_match",
		"xpath_alternative",
	}

	// In real implementation, this would analyze the error and select appropriate strategy
	return strategies[rand.Intn(len(strategies))]
}
