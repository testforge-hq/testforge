package ai

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/workflows"
)

// RegisterActivities registers all AI agent activities with the Temporal worker
func RegisterActivities(w worker.Worker, cfg Config, logger *zap.Logger) error {
	aiActivity, err := NewActivityWithConfig(cfg, logger)
	if err != nil {
		return err
	}

	// Register AI discovery activity
	w.RegisterActivityWithOptions(aiActivity.ExecuteAIDiscovery, activity.RegisterOptions{
		Name: workflows.AIDiscoveryActivityName,
	})

	// Register page analysis activity
	w.RegisterActivityWithOptions(aiActivity.ExecutePageAnalysis, activity.RegisterOptions{
		Name: workflows.PageAnalysisActivityName,
	})

	// Register ABA activity
	w.RegisterActivityWithOptions(aiActivity.ExecuteABA, activity.RegisterOptions{
		Name: workflows.ABAActivityName,
	})

	return nil
}
