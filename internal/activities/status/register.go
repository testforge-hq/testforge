package status

import (
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/repository/postgres"
	"github.com/testforge/testforge/internal/workflows"
)

// RegisterActivities registers all status update activities with the Temporal worker
func RegisterActivities(w worker.Worker, repo *postgres.TestRunRepository, logger *zap.Logger) {
	statusActivity := NewActivity(repo, logger)

	w.RegisterActivityWithOptions(statusActivity.UpdateStatus, activity.RegisterOptions{
		Name: workflows.UpdateStatusActivityName,
	})

	w.RegisterActivityWithOptions(statusActivity.SaveDiscoveryResult, activity.RegisterOptions{
		Name: workflows.SaveDiscoveryResultActivityName,
	})

	w.RegisterActivityWithOptions(statusActivity.SaveAIAnalysis, activity.RegisterOptions{
		Name: workflows.SaveAIAnalysisActivityName,
	})

	w.RegisterActivityWithOptions(statusActivity.SaveTestPlan, activity.RegisterOptions{
		Name: workflows.SaveTestPlanActivityName,
	})

	w.RegisterActivityWithOptions(statusActivity.SaveSummary, activity.RegisterOptions{
		Name: workflows.SaveSummaryActivityName,
	})
}
