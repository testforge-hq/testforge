package status

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/repository/postgres"
	"github.com/testforge/testforge/internal/workflows"
)

// Activity handles test run status updates
type Activity struct {
	repo   *postgres.TestRunRepository
	logger *zap.Logger
}

// NewActivity creates a new status update activity
func NewActivity(repo *postgres.TestRunRepository, logger *zap.Logger) *Activity {
	return &Activity{
		repo:   repo,
		logger: logger,
	}
}

// UpdateStatusInput is the input for status updates
type UpdateStatusInput struct {
	TestRunID string            `json:"test_run_id"`
	Status    domain.RunStatus  `json:"status"`
	Phase     string            `json:"phase,omitempty"` // discovery, test_design, execution, etc.
	Message   string            `json:"message,omitempty"`
}

// UpdateStatus updates the test run status
func (a *Activity) UpdateStatus(ctx context.Context, input UpdateStatusInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Updating test run status",
		"test_run_id", input.TestRunID,
		"status", input.Status,
		"phase", input.Phase,
	)

	id, err := uuid.Parse(input.TestRunID)
	if err != nil {
		return fmt.Errorf("invalid test run ID: %w", err)
	}

	return a.repo.UpdateStatus(ctx, id, input.Status)
}

// SaveDiscoveryResultInput is the input for saving discovery results
type SaveDiscoveryResultInput struct {
	TestRunID       string                   `json:"test_run_id"`
	DiscoveryResult *domain.DiscoveryResult  `json:"discovery_result"`
}

// SaveDiscoveryResult saves discovery results to the test run
func (a *Activity) SaveDiscoveryResult(ctx context.Context, input SaveDiscoveryResultInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Saving discovery result", "test_run_id", input.TestRunID)

	id, err := uuid.Parse(input.TestRunID)
	if err != nil {
		return fmt.Errorf("invalid test run ID: %w", err)
	}

	run, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting test run: %w", err)
	}

	run.DiscoveryResult = input.DiscoveryResult
	run.Status = domain.RunStatusDesigning

	return a.repo.Update(ctx, run)
}

// SaveAIAnalysisInput is the input for saving AI analysis results
type SaveAIAnalysisInput struct {
	TestRunID  string                    `json:"test_run_id"`
	AIAnalysis *domain.AIAnalysisResult  `json:"ai_analysis"`
}

// SaveAIAnalysis saves AI analysis results to the test run
func (a *Activity) SaveAIAnalysis(ctx context.Context, input SaveAIAnalysisInput) error {
	logger := activity.GetLogger(ctx)

	// Handle nil AIAnalysis
	reqCount, storyCount := 0, 0
	if input.AIAnalysis != nil {
		reqCount = len(input.AIAnalysis.Requirements)
		storyCount = len(input.AIAnalysis.UserStories)
	}

	logger.Info("Saving AI analysis result",
		"test_run_id", input.TestRunID,
		"requirements", reqCount,
		"user_stories", storyCount,
	)

	id, err := uuid.Parse(input.TestRunID)
	if err != nil {
		return fmt.Errorf("invalid test run ID: %w", err)
	}

	run, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting test run: %w", err)
	}

	run.AIAnalysis = input.AIAnalysis

	return a.repo.Update(ctx, run)
}

// SaveTestPlanInput is the input for saving test plan
type SaveTestPlanInput struct {
	TestRunID string           `json:"test_run_id"`
	TestPlan  *domain.TestPlan `json:"test_plan"`
}

// SaveTestPlan saves test plan to the test run
func (a *Activity) SaveTestPlan(ctx context.Context, input SaveTestPlanInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Saving test plan", "test_run_id", input.TestRunID)

	id, err := uuid.Parse(input.TestRunID)
	if err != nil {
		return fmt.Errorf("invalid test run ID: %w", err)
	}

	run, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting test run: %w", err)
	}

	run.TestPlan = input.TestPlan
	run.Status = domain.RunStatusAutomating

	return a.repo.Update(ctx, run)
}

// SaveSummaryInput is the input for saving run summary
type SaveSummaryInput struct {
	TestRunID string             `json:"test_run_id"`
	Summary   *domain.RunSummary `json:"summary"`
	ReportURL string             `json:"report_url,omitempty"`
	Status    domain.RunStatus   `json:"status"`
}

// SaveSummary saves the final summary to the test run
func (a *Activity) SaveSummary(ctx context.Context, input SaveSummaryInput) error {
	logger := activity.GetLogger(ctx)
	logger.Info("Saving run summary",
		"test_run_id", input.TestRunID,
		"status", input.Status,
	)

	id, err := uuid.Parse(input.TestRunID)
	if err != nil {
		return fmt.Errorf("invalid test run ID: %w", err)
	}

	run, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting test run: %w", err)
	}

	run.Summary = input.Summary
	run.ReportURL = input.ReportURL
	run.Status = domain.RunStatus(input.Status)
	run.Complete(input.Summary, input.ReportURL)

	return a.repo.Update(ctx, run)
}

// ConvertAIDiscoveryToAnalysis converts workflow AI discovery output to domain AI analysis
func ConvertAIDiscoveryToAnalysis(output *workflows.AIDiscoveryOutput) *domain.AIAnalysisResult {
	if output == nil {
		return nil
	}

	result := &domain.AIAnalysisResult{
		AgentsUsed:       []string{"PageUnderstanding", "ElementDiscovery", "BusinessFlow", "ABA"},
		TokensUsed:       output.TokensUsed,
		AnalysisDuration: output.Duration,
	}

	// Convert requirements
	for _, req := range output.Requirements {
		result.Requirements = append(result.Requirements, domain.BusinessRequirement{
			ID:          req.ID,
			Title:       req.Title,
			Description: req.Description,
			Category:    req.Type,
			Priority:    domain.Priority(req.Priority),
			Source:      req.Source,
		})
	}

	// Convert user stories
	for _, story := range output.UserStories {
		us := domain.UserStory{
			ID:            story.ID,
			Title:         story.Title,
			AsA:           story.AsA,
			IWant:         story.IWant,
			SoThat:        story.SoThat,
			Priority:      domain.Priority(story.Priority),
			StoryPoints:   story.StoryPoints,
			RelatedPages:  story.RelatedPages,
			TestScenarios: story.TestScenarios,
		}
		for _, ac := range story.AcceptanceCriteria {
			us.AcceptanceCriteria = append(us.AcceptanceCriteria, domain.AcceptanceCriterion{
				Given: ac.Given,
				When:  ac.When,
				Then:  ac.Then,
			})
		}
		result.UserStories = append(result.UserStories, us)
	}

	// Convert semantic map
	if output.SemanticMap != nil {
		result.SemanticMap = &domain.SemanticMap{
			Purpose:       output.SemanticMap.Purpose,
			UserPersonas:  output.SemanticMap.UserPersonas,
			CoreWorkflows: output.SemanticMap.CoreFeatures,
		}
	}

	return result
}
