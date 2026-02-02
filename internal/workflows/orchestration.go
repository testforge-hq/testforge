package workflows

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Activity names - must match registered activity names
const (
	DiscoveryActivityName  = "DiscoveryActivity"
	TestDesignActivityName = "TestDesignActivity"
	AutomationActivityName = "AutomationActivity"
	ExecutionActivityName  = "ExecutionActivity"
	HealingActivityName    = "HealingActivity"
	ReportActivityName     = "ReportActivity"

	// AI Agent Activities
	AIDiscoveryActivityName   = "AIDiscoveryActivity"   // Multi-agent AI-powered discovery
	PageAnalysisActivityName  = "PageAnalysisActivity"  // Single page semantic analysis
	ABAActivityName           = "ABAActivity"           // Autonomous Business Analyst

	// Status Update Activities
	UpdateStatusActivityName       = "UpdateStatusActivity"
	SaveDiscoveryResultActivityName = "SaveDiscoveryResultActivity"
	SaveAIAnalysisActivityName     = "SaveAIAnalysisActivity"
	SaveTestPlanActivityName       = "SaveTestPlanActivity"
	SaveSummaryActivityName        = "SaveSummaryActivity"
)

// MasterOrchestrationWorkflow orchestrates the entire test run process
func MasterOrchestrationWorkflow(ctx workflow.Context, input TestRunInput) (*TestRunOutput, error) {
	logger := workflow.GetLogger(ctx)
	startTime := workflow.Now(ctx)

	logger.Info("Starting master orchestration workflow",
		"test_run_id", input.TestRunID.String(),
		"target_url", input.TargetURL,
	)

	output := &TestRunOutput{
		TestRunID: input.TestRunID,
		Status:    "running",
	}

	// Phase 1: Discovery
	logger.Info("Phase 1: Starting discovery")
	discoveryOutput, err := executeDiscovery(ctx, input)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("discovery failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil // Return output even on failure for visibility
	}
	logger.Info("Phase 1: Discovery completed",
		"pages_found", discoveryOutput.PagesFound,
		"flows_detected", discoveryOutput.FlowsDetected,
	)

	// Phase 2: Test Design
	logger.Info("Phase 2: Starting test design")
	testDesignOutput, err := executeTestDesign(ctx, input, discoveryOutput)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("test design failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}
	logger.Info("Phase 2: Test design completed",
		"test_cases", testDesignOutput.TestPlan.TotalCount,
	)

	// Phase 3: Automation (Script Generation)
	logger.Info("Phase 3: Starting automation")
	automationOutput, err := executeAutomation(ctx, input, testDesignOutput)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("automation failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}
	logger.Info("Phase 3: Automation completed",
		"scripts_generated", len(automationOutput.Scripts),
	)

	// Phase 4: Execution (as child workflow for parallel execution)
	logger.Info("Phase 4: Starting execution")
	executionOutput, err := executeTests(ctx, input, automationOutput)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("execution failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}
	logger.Info("Phase 4: Execution completed",
		"total", executionOutput.Summary.TotalTests,
		"passed", executionOutput.Summary.Passed,
		"failed", executionOutput.Summary.Failed,
	)

	// Phase 5: Self-Healing (if enabled and there are failures)
	finalResults := executionOutput.Results
	if input.EnableSelfHealing && executionOutput.Summary.Failed > 0 {
		logger.Info("Phase 5: Starting self-healing",
			"failed_tests", executionOutput.Summary.Failed,
		)
		healingOutput, err := executeSelfHealing(ctx, input, executionOutput, automationOutput)
		if err != nil {
			logger.Warn("Self-healing failed, continuing with original results", "error", err)
		} else if healingOutput.HealedCount > 0 {
			logger.Info("Phase 5: Self-healing completed",
				"healed_count", healingOutput.HealedCount,
			)
			// Merge healed results
			finalResults = mergeResults(executionOutput.Results, healingOutput.HealedResults)
		}
	} else {
		logger.Info("Phase 5: Self-healing skipped",
			"enabled", input.EnableSelfHealing,
			"failed_count", executionOutput.Summary.Failed,
		)
	}

	// Recalculate summary with potentially healed results
	finalSummary := calculateSummary(finalResults)

	// Phase 6: Reporting
	logger.Info("Phase 6: Starting report generation")
	reportOutput, err := executeReporting(ctx, input, finalResults, finalSummary, discoveryOutput)
	if err != nil {
		logger.Warn("Report generation failed", "error", err)
		// Continue even if reporting fails
	} else {
		output.ReportURL = reportOutput.ReportURL
		logger.Info("Phase 6: Report generated", "url", reportOutput.ReportURL)
	}

	// Finalize output
	output.Status = "completed"
	output.Summary = finalSummary
	output.CompletedAt = workflow.Now(ctx)
	output.TotalDuration = output.CompletedAt.Sub(startTime)

	logger.Info("Master orchestration workflow completed",
		"test_run_id", input.TestRunID.String(),
		"status", output.Status,
		"duration", output.TotalDuration,
		"pass_rate", finalSummary.PassRate,
	)

	return output, nil
}

// executeDiscovery runs the discovery activity
func executeDiscovery(ctx workflow.Context, input TestRunInput) (*DiscoveryOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	discoveryInput := DiscoveryInput{
		TestRunID:       input.TestRunID,
		TargetURL:       input.TargetURL,
		MaxCrawlDepth:   input.MaxCrawlDepth,
		ExcludePatterns: input.ExcludePatterns,
	}

	var output DiscoveryOutput
	err := workflow.ExecuteActivity(ctx, DiscoveryActivityName, discoveryInput).Get(ctx, &output)
	return &output, err
}

// executeTestDesign runs the test design activity
func executeTestDesign(ctx workflow.Context, input TestRunInput, discovery *DiscoveryOutput) (*TestDesignOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	testDesignInput := TestDesignInput{
		TestRunID:    input.TestRunID,
		AppModel:     discovery.AppModel,
		MaxTestCases: input.MaxTestCases,
	}

	var output TestDesignOutput
	err := workflow.ExecuteActivity(ctx, TestDesignActivityName, testDesignInput).Get(ctx, &output)
	return &output, err
}

// executeAutomation runs the automation (script generation) activity
func executeAutomation(ctx workflow.Context, input TestRunInput, testDesign *TestDesignOutput) (*AutomationOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	automationInput := AutomationInput{
		TestRunID: input.TestRunID,
		TestPlan:  testDesign.TestPlan,
		Browser:   input.Browser,
		Timeout:   input.Timeout,
	}

	var output AutomationOutput
	err := workflow.ExecuteActivity(ctx, AutomationActivityName, automationInput).Get(ctx, &output)
	return &output, err
}

// executeTests runs the execution activity (or child workflow for parallel)
func executeTests(ctx workflow.Context, input TestRunInput, automation *AutomationOutput) (*ExecutionOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    1, // Child workflow handles retries
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	executionInput := ExecutionInput{
		TestRunID: input.TestRunID,
		TenantID:  input.TenantID,
		Scripts:   automation.Scripts,
		Parallel:  4, // Default parallelism
		Browser:   input.Browser,
		Timeout:   input.Timeout,
	}

	var output ExecutionOutput
	err := workflow.ExecuteActivity(ctx, ExecutionActivityName, executionInput).Get(ctx, &output)
	return &output, err
}

// executeSelfHealing runs the self-healing activity
func executeSelfHealing(ctx workflow.Context, input TestRunInput, execution *ExecutionOutput, automation *AutomationOutput) (*HealingOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 3 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    1,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Filter failed results
	var failedResults []TestResult
	for _, r := range execution.Results {
		if r.Status == "failed" {
			failedResults = append(failedResults, r)
		}
	}

	healingInput := HealingInput{
		TestRunID:     input.TestRunID,
		FailedResults: failedResults,
		Scripts:       automation.Scripts,
	}

	var output HealingOutput
	err := workflow.ExecuteActivity(ctx, HealingActivityName, healingInput).Get(ctx, &output)
	return &output, err
}

// executeReporting runs the reporting activity
func executeReporting(ctx workflow.Context, input TestRunInput, results []TestResult, summary *RunSummary, discovery *DiscoveryOutput) (*ReportOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	reportInput := ReportInput{
		TestRunID: input.TestRunID,
		TenantID:  input.TenantID,
		ProjectID: input.ProjectID,
		Results:   results,
		Summary:   summary,
		AppModel:  discovery.AppModel,
	}

	var output ReportOutput
	err := workflow.ExecuteActivity(ctx, ReportActivityName, reportInput).Get(ctx, &output)
	return &output, err
}

// mergeResults merges original and healed results
func mergeResults(original, healed []TestResult) []TestResult {
	healedMap := make(map[string]TestResult)
	for _, r := range healed {
		healedMap[r.TestCaseID] = r
	}

	results := make([]TestResult, 0, len(original))
	for _, r := range original {
		if healedResult, ok := healedMap[r.TestCaseID]; ok {
			results = append(results, healedResult)
		} else {
			results = append(results, r)
		}
	}
	return results
}

// calculateSummary calculates summary statistics from results
func calculateSummary(results []TestResult) *RunSummary {
	summary := &RunSummary{
		TotalTests: len(results),
	}

	var totalDuration time.Duration
	for _, r := range results {
		totalDuration += r.Duration
		switch r.Status {
		case "passed":
			summary.Passed++
		case "failed":
			summary.Failed++
		case "skipped":
			summary.Skipped++
		case "healed":
			summary.Healed++
			summary.Passed++ // Healed tests count as passed
		}
	}

	summary.Duration = totalDuration
	if summary.TotalTests > 0 {
		summary.PassRate = float64(summary.Passed) / float64(summary.TotalTests) * 100
	}

	return summary
}

// =============================================================================
// AI-Powered Activity Execution Functions
// =============================================================================

// executeAIDiscovery runs the AI-powered discovery activity with multi-agent analysis
func executeAIDiscovery(ctx workflow.Context, input TestRunInput) (*AIDiscoveryOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute, // AI analysis may take longer
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	aiDiscoveryInput := AIDiscoveryInput{
		TestRunID:        input.TestRunID,
		TargetURL:        input.TargetURL,
		MaxPages:         50,
		MaxDepth:         input.MaxCrawlDepth,
		EnableABA:        true,
		EnableSemanticAI: true,
		EnableVisualAI:   input.EnableVisualTesting,
		Headless:         true,
	}

	var output AIDiscoveryOutput
	err := workflow.ExecuteActivity(ctx, AIDiscoveryActivityName, aiDiscoveryInput).Get(ctx, &output)
	return &output, err
}

// executePageAnalysis runs semantic analysis on a single page
func executePageAnalysis(ctx workflow.Context, input TestRunInput, pageURL, html string) (*PageAnalysisOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	pageAnalysisInput := PageAnalysisInput{
		TestRunID:    input.TestRunID,
		PageURL:      pageURL,
		HTML:         html,
		AnalysisType: "full",
	}

	var output PageAnalysisOutput
	err := workflow.ExecuteActivity(ctx, PageAnalysisActivityName, pageAnalysisInput).Get(ctx, &output)
	return &output, err
}

// executeABA runs the Autonomous Business Analyst for requirements and user stories
func executeABA(ctx workflow.Context, input TestRunInput, appModel *AppModel) (*ABAOutput, error) {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    2,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	abaInput := ABAInput{
		TestRunID: input.TestRunID,
		AppModel:  appModel,
	}

	var output ABAOutput
	err := workflow.ExecuteActivity(ctx, ABAActivityName, abaInput).Get(ctx, &output)
	return &output, err
}

// AIEnhancedOrchestrationWorkflow is an alternative workflow that uses AI-powered discovery
func AIEnhancedOrchestrationWorkflow(ctx workflow.Context, input TestRunInput) (*TestRunOutput, error) {
	logger := workflow.GetLogger(ctx)
	startTime := workflow.Now(ctx)

	logger.Info("Starting AI-enhanced orchestration workflow",
		"test_run_id", input.TestRunID.String(),
		"target_url", input.TargetURL,
	)

	output := &TestRunOutput{
		TestRunID: input.TestRunID,
		Status:    "running",
	}

	// Phase 1: AI-Powered Discovery with multi-agent analysis
	logger.Info("Phase 1: Starting AI-powered discovery")
	aiDiscoveryOutput, err := executeAIDiscovery(ctx, input)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("AI discovery failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}
	logger.Info("Phase 1: AI discovery completed",
		"pages_found", aiDiscoveryOutput.PagesFound,
		"elements_found", aiDiscoveryOutput.ElementsFound,
		"flows_detected", aiDiscoveryOutput.FlowsDetected,
		"requirements", len(aiDiscoveryOutput.Requirements),
		"user_stories", len(aiDiscoveryOutput.UserStories),
	)

	// Convert AI discovery output to standard discovery output for downstream activities
	discoveryOutput := &DiscoveryOutput{
		AppModel:      aiDiscoveryOutput.AppModel,
		PagesFound:    aiDiscoveryOutput.PagesFound,
		FormsFound:    0, // Would need to count from app model
		FlowsDetected: aiDiscoveryOutput.FlowsDetected,
		Duration:      aiDiscoveryOutput.Duration,
	}

	// Save AI analysis results to database
	if len(aiDiscoveryOutput.Requirements) > 0 || len(aiDiscoveryOutput.UserStories) > 0 {
		saveAIInput := SaveAIAnalysisInput{
			TestRunID:    input.TestRunID.String(),
			Requirements: aiDiscoveryOutput.Requirements,
			UserStories:  aiDiscoveryOutput.UserStories,
			SemanticMap:  aiDiscoveryOutput.SemanticMap,
			AgentsUsed:   []string{"PageUnderstanding", "ElementDiscovery", "BusinessFlow", "ABA"},
			TokensUsed:   aiDiscoveryOutput.TokensUsed,
			Duration:     aiDiscoveryOutput.Duration,
		}
		if err := saveAIAnalysis(ctx, saveAIInput); err != nil {
			logger.Warn("Failed to save AI analysis results", "error", err)
		}
	}

	// Continue with standard workflow phases using the AI-enhanced discovery data
	// Phase 2: Test Design
	logger.Info("Phase 2: Starting test design")
	testDesignOutput, err := executeTestDesign(ctx, input, discoveryOutput)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("test design failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}
	logger.Info("Phase 2: Test design completed",
		"test_cases", testDesignOutput.TestPlan.TotalCount,
	)

	// Phase 3: Automation
	logger.Info("Phase 3: Starting automation")
	automationOutput, err := executeAutomation(ctx, input, testDesignOutput)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("automation failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}
	logger.Info("Phase 3: Automation completed",
		"scripts_generated", len(automationOutput.Scripts),
	)

	// Phase 4: Execution
	logger.Info("Phase 4: Starting execution")
	executionOutput, err := executeTests(ctx, input, automationOutput)
	if err != nil {
		output.Status = "failed"
		output.Error = fmt.Sprintf("execution failed: %v", err)
		output.CompletedAt = workflow.Now(ctx)
		output.TotalDuration = output.CompletedAt.Sub(startTime)
		return output, nil
	}

	// Phase 5: Self-Healing
	finalResults := executionOutput.Results
	if input.EnableSelfHealing && executionOutput.Summary.Failed > 0 {
		logger.Info("Phase 5: Starting self-healing")
		healingOutput, err := executeSelfHealing(ctx, input, executionOutput, automationOutput)
		if err == nil && healingOutput.HealedCount > 0 {
			finalResults = mergeResults(executionOutput.Results, healingOutput.HealedResults)
		}
	}

	finalSummary := calculateSummary(finalResults)

	// Phase 6: Reporting
	logger.Info("Phase 6: Starting report generation")
	reportOutput, err := executeReporting(ctx, input, finalResults, finalSummary, discoveryOutput)
	if err == nil {
		output.ReportURL = reportOutput.ReportURL
	}

	// Finalize
	output.Status = "completed"
	output.Summary = finalSummary
	output.CompletedAt = workflow.Now(ctx)
	output.TotalDuration = output.CompletedAt.Sub(startTime)

	// Save final summary to database
	saveSummaryInput := SaveSummaryInput{
		TestRunID: input.TestRunID.String(),
		Summary:   finalSummary,
		ReportURL: output.ReportURL,
		Status:    "completed",
	}
	if err := saveSummary(ctx, saveSummaryInput); err != nil {
		logger.Warn("Failed to save summary", "error", err)
	}

	logger.Info("AI-enhanced orchestration workflow completed",
		"test_run_id", input.TestRunID.String(),
		"status", output.Status,
		"duration", output.TotalDuration,
		"pass_rate", finalSummary.PassRate,
	)

	return output, nil
}

// saveAIAnalysis saves AI analysis results to the database
func saveAIAnalysis(ctx workflow.Context, input SaveAIAnalysisInput) error {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	return workflow.ExecuteActivity(ctx, SaveAIAnalysisActivityName, input).Get(ctx, nil)
}

// saveSummary saves the final summary to the database
func saveSummary(ctx workflow.Context, input SaveSummaryInput) error {
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	return workflow.ExecuteActivity(ctx, SaveSummaryActivityName, input).Get(ctx, nil)
}
