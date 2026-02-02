package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/testforge/testforge/internal/sandbox"
	"github.com/testforge/testforge/internal/storage"
	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements the test execution activity
type Activity struct {
	sandboxManager SandboxManager
	storage        *storage.MinIOClient
	logger         *zap.Logger
	config         Config
}

// Config holds execution activity configuration
type Config struct {
	// Local mode for development without K8s
	LocalMode    bool
	LocalWorkDir string

	// K8s configuration
	Namespace      string
	DefaultTimeout time.Duration
	Kubeconfig     string // Path to kubeconfig (empty for in-cluster)

	// MinIO configuration
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string

	// API endpoint for callbacks
	APIEndpoint string
}

// SandboxManager interface for test execution
type SandboxManager interface {
	RunTests(ctx context.Context, req sandbox.SandboxRequest) (*sandbox.SandboxResult, error)
	Cleanup(runID string) error
}

// NewActivity creates a new execution activity
func NewActivity(cfg Config, storageClient *storage.MinIOClient, logger *zap.Logger) (*Activity, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
	}

	var manager SandboxManager

	if cfg.LocalMode {
		// Use mock manager for local development
		logger.Info("Initializing execution activity in LOCAL mode",
			zap.String("work_dir", cfg.LocalWorkDir))
		manager = sandbox.NewMockManager(cfg.LocalWorkDir, storageClient, logger)
	} else {
		// Use K8s manager for production
		logger.Info("Initializing execution activity in K8S mode",
			zap.String("namespace", cfg.Namespace))

		// Create K8s client
		k8sClient, err := createK8sClient(cfg.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("creating K8s client: %w", err)
		}

		k8sCfg := sandbox.ManagerConfig{
			Namespace:      cfg.Namespace,
			DefaultTimeout: cfg.DefaultTimeout,
			MinIOEndpoint:  cfg.MinIOEndpoint,
			MinIOAccessKey: cfg.MinIOAccessKey,
			MinIOSecretKey: cfg.MinIOSecretKey,
			MinIOBucket:    cfg.MinIOBucket,
			APIEndpoint:    cfg.APIEndpoint,
		}

		manager = sandbox.NewK8sManager(k8sClient, k8sCfg, logger)
	}

	return &Activity{
		sandboxManager: manager,
		storage:        storageClient,
		logger:         logger,
		config:         cfg,
	}, nil
}

// createK8sClient creates a Kubernetes client from kubeconfig or in-cluster config
func createK8sClient(kubeconfig string) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error

	if kubeconfig != "" {
		// Use provided kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("building config from kubeconfig: %w", err)
		}
	} else {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("getting in-cluster config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}

	return clientset, nil
}

// NewMockActivity creates an activity with mock sandbox for testing
func NewMockActivity(logger *zap.Logger) *Activity {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
	}

	return &Activity{
		sandboxManager: sandbox.NewMockManager("/tmp/testforge-sandboxes", nil, logger),
		logger:         logger,
		config: Config{
			LocalMode:    true,
			LocalWorkDir: "/tmp/testforge-sandboxes",
		},
	}
}

// Execute runs the test scripts in a sandbox and returns results
func (a *Activity) Execute(ctx context.Context, input workflows.ExecutionInput) (*workflows.ExecutionOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	runID := input.TestRunID.String()

	logger.Info("Starting execution activity",
		"run_id", runID,
		"tenant_id", input.TenantID.String(),
		"scripts_uri", input.ScriptsURI,
		"target_url", input.TargetURL,
		"browser", input.Browser,
		"parallel", input.Parallel,
	)

	// Record initial heartbeat
	activity.RecordHeartbeat(ctx, map[string]interface{}{
		"phase":    "initializing",
		"progress": 0,
	})

	// Build sandbox request
	timeout := time.Duration(input.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 15 * time.Minute
	}

	workers := input.Workers
	if workers <= 0 {
		workers = input.Parallel
	}
	if workers <= 0 {
		workers = 2
	}

	req := sandbox.SandboxRequest{
		RunID:       runID,
		TenantID:    input.TenantID.String(),
		ProjectID:   input.ProjectID.String(),
		Tier:        input.Tier,
		ScriptsURI:  input.ScriptsURI,
		TargetURL:   input.TargetURL,
		Environment: input.Environment,
		Timeout:     timeout,
		Parallelism: input.Parallel,
		Browser:     input.Browser,
		Retries:     input.Retries,
		Workers:     workers,
	}

	// Record heartbeat for sandbox creation
	activity.RecordHeartbeat(ctx, map[string]interface{}{
		"phase":    "creating_sandbox",
		"progress": 10,
	})

	// Create a context for long-polling with heartbeats
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start heartbeat goroutine
	heartbeatDone := make(chan struct{})
	go a.heartbeatLoop(execCtx, heartbeatDone)

	// Run tests in sandbox
	a.logger.Info("Running tests in sandbox",
		zap.String("run_id", runID),
		zap.String("scripts_uri", req.ScriptsURI),
		zap.String("target_url", req.TargetURL),
	)

	result, err := a.sandboxManager.RunTests(execCtx, req)

	// Stop heartbeat loop
	cancel()
	<-heartbeatDone

	if err != nil {
		logger.Error("Sandbox execution failed", "error", err)
		return &workflows.ExecutionOutput{
			Results:  []workflows.TestResult{},
			Summary:  &workflows.RunSummary{},
			Duration: time.Since(startTime),
			ExitCode: -1,
			Logs:     fmt.Sprintf("Sandbox execution error: %v", err),
		}, err
	}

	// Record completion heartbeat
	activity.RecordHeartbeat(ctx, map[string]interface{}{
		"phase":    "completed",
		"progress": 100,
	})

	// Convert sandbox result to workflow output
	output := a.convertResult(result, input, startTime)

	logger.Info("Execution activity completed",
		"total", output.Summary.TotalTests,
		"passed", output.Summary.Passed,
		"failed", output.Summary.Failed,
		"pass_rate", output.Summary.PassRate,
		"duration", output.Duration,
	)

	// Cleanup sandbox resources
	if err := a.sandboxManager.Cleanup(runID); err != nil {
		a.logger.Warn("Failed to cleanup sandbox", zap.Error(err))
	}

	return output, nil
}

// heartbeatLoop sends periodic heartbeats during test execution
func (a *Activity) heartbeatLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	progress := 20
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Increment progress gradually
			if progress < 90 {
				progress += 5
			}

			activity.RecordHeartbeat(ctx, map[string]interface{}{
				"phase":    "executing_tests",
				"progress": progress,
			})
		}
	}
}

// convertResult converts sandbox result to workflow execution output
func (a *Activity) convertResult(result *sandbox.SandboxResult, input workflows.ExecutionInput, startTime time.Time) *workflows.ExecutionOutput {
	output := &workflows.ExecutionOutput{
		Results:        []workflows.TestResult{},
		Duration:       time.Since(startTime),
		ResultsURI:     result.ResultsURI,
		ArtifactsURI:   result.ArtifactsURI,
		ScreenshotsURI: result.ScreenshotsURI,
		LogsURI:        result.LogsURI,
		ExitCode:       result.ExitCode,
		Logs:           result.Logs,
	}

	// Parse detailed results from raw JSON if available
	if result.RawResults != nil {
		var playwrightResults sandbox.PlaywrightResults
		if err := json.Unmarshal(result.RawResults, &playwrightResults); err == nil {
			output.Results = a.parsePlaywrightResults(&playwrightResults, input.Scripts)
		}
	}

	// Build summary
	passRate := float64(0)
	if result.TotalTests > 0 {
		passRate = float64(result.TestsPassed) / float64(result.TotalTests) * 100
	}

	output.Summary = &workflows.RunSummary{
		TotalTests: result.TotalTests,
		Passed:     result.TestsPassed,
		Failed:     result.TestsFailed,
		Skipped:    result.TestsSkipped,
		Duration:   result.Duration,
		PassRate:   passRate,
	}

	// If no detailed results, create summary results from counts
	if len(output.Results) == 0 && len(input.Scripts) > 0 {
		output.Results = a.generateResultsFromSummary(input.Scripts, result)
	}

	return output
}

// parsePlaywrightResults extracts test results from Playwright JSON output
func (a *Activity) parsePlaywrightResults(results *sandbox.PlaywrightResults, scripts []workflows.TestScript) []workflows.TestResult {
	testResults := []workflows.TestResult{}

	// Create a map of test case IDs from scripts for matching
	scriptMap := make(map[string]bool)
	for _, script := range scripts {
		scriptMap[script.TestCaseID] = true
	}

	// Recursively extract results from suites
	var extractFromSuite func(suite sandbox.PlaywrightSuite)
	extractFromSuite = func(suite sandbox.PlaywrightSuite) {
		for _, spec := range suite.Specs {
			for _, test := range spec.Tests {
				result := workflows.TestResult{
					TestCaseID: spec.Title,
					Status:     a.mapPlaywrightStatus(test.Status),
					Duration:   time.Duration(test.Duration * float64(time.Millisecond)),
				}

				if test.Error != nil {
					result.ErrorMessage = test.Error.Message
				}

				testResults = append(testResults, result)
			}
		}

		// Recurse into nested suites
		for _, nested := range suite.Suites {
			extractFromSuite(nested)
		}
	}

	for _, suite := range results.Suites {
		extractFromSuite(suite)
	}

	return testResults
}

// mapPlaywrightStatus converts Playwright status to our status
func (a *Activity) mapPlaywrightStatus(status string) string {
	switch status {
	case "passed", "expected":
		return "passed"
	case "failed", "unexpected", "timedOut":
		return "failed"
	case "skipped":
		return "skipped"
	default:
		return status
	}
}

// generateResultsFromSummary creates basic results when detailed data isn't available
func (a *Activity) generateResultsFromSummary(scripts []workflows.TestScript, result *sandbox.SandboxResult) []workflows.TestResult {
	testResults := []workflows.TestResult{}

	passed := result.TestsPassed
	failed := result.TestsFailed
	skipped := result.TestsSkipped

	for _, script := range scripts {
		var status string
		var errorMsg string

		if passed > 0 {
			status = "passed"
			passed--
		} else if failed > 0 {
			status = "failed"
			failed--
			errorMsg = result.Error
		} else if skipped > 0 {
			status = "skipped"
			skipped--
		} else {
			status = "unknown"
		}

		testResults = append(testResults, workflows.TestResult{
			TestCaseID:   script.TestCaseID,
			Status:       status,
			ErrorMessage: errorMsg,
		})
	}

	return testResults
}

// ExecuteBatch runs a batch of tests in parallel sandboxes (for enterprise tier)
func (a *Activity) ExecuteBatch(ctx context.Context, input BatchExecutionInput) (*BatchExecutionOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting batch execution",
		"run_id", input.RunID,
		"batches", len(input.Batches),
	)

	results := make([]*workflows.ExecutionOutput, len(input.Batches))
	errors := make([]error, len(input.Batches))

	// Execute batches sequentially for now (can be parallelized with worker pool)
	for i, batch := range input.Batches {
		activity.RecordHeartbeat(ctx, map[string]interface{}{
			"phase":         "executing_batch",
			"current_batch": i + 1,
			"total_batches": len(input.Batches),
			"progress":      float64(i) / float64(len(input.Batches)) * 100,
		})

		batchInput := workflows.ExecutionInput{
			TestRunID:   input.TestRunID,
			TenantID:    input.TenantID,
			ProjectID:   input.ProjectID,
			Tier:        input.Tier,
			Scripts:     batch.Scripts,
			ScriptsURI:  input.ScriptsURI,
			TargetURL:   input.TargetURL,
			Environment: input.Environment,
			Parallel:    input.Parallel,
			Browser:     input.Browser,
			Timeout:     input.Timeout,
			Retries:     input.Retries,
			Workers:     input.Workers,
		}

		results[i], errors[i] = a.Execute(ctx, batchInput)
	}

	// Aggregate results
	output := a.aggregateBatchResults(results, errors, startTime)

	logger.Info("Batch execution completed",
		"total", output.AggregatedSummary.TotalTests,
		"passed", output.AggregatedSummary.Passed,
		"failed", output.AggregatedSummary.Failed,
		"duration", output.Duration,
	)

	return output, nil
}

// aggregateBatchResults combines results from multiple batches
func (a *Activity) aggregateBatchResults(results []*workflows.ExecutionOutput, errors []error, startTime time.Time) *BatchExecutionOutput {
	output := &BatchExecutionOutput{
		BatchResults: results,
		Duration:     time.Since(startTime),
	}

	allResults := []workflows.TestResult{}
	totalPassed := 0
	totalFailed := 0
	totalSkipped := 0
	var totalDuration time.Duration

	for i, result := range results {
		if errors[i] != nil {
			output.Errors = append(output.Errors, fmt.Sprintf("Batch %d: %v", i, errors[i]))
			continue
		}

		if result != nil {
			allResults = append(allResults, result.Results...)
			if result.Summary != nil {
				totalPassed += result.Summary.Passed
				totalFailed += result.Summary.Failed
				totalSkipped += result.Summary.Skipped
				totalDuration += result.Summary.Duration
			}
		}
	}

	total := totalPassed + totalFailed + totalSkipped
	passRate := float64(0)
	if total > 0 {
		passRate = float64(totalPassed) / float64(total) * 100
	}

	output.AggregatedSummary = &workflows.RunSummary{
		TotalTests: total,
		Passed:     totalPassed,
		Failed:     totalFailed,
		Skipped:    totalSkipped,
		Duration:   totalDuration,
		PassRate:   passRate,
	}

	output.AllResults = allResults

	return output
}

// BatchExecutionInput for running multiple test batches
type BatchExecutionInput struct {
	TestRunID   uuid.UUID
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	RunID       string
	Tier        string
	ScriptsURI  string
	TargetURL   string
	Environment string
	Parallel    int
	Browser     string
	Timeout     int
	Retries     int
	Workers     int
	Batches     []TestBatch
}

// TestBatch represents a batch of tests to run
type TestBatch struct {
	BatchIndex int
	Scripts    []workflows.TestScript
	TestFiles  []string
}

// BatchExecutionOutput contains results from batch execution
type BatchExecutionOutput struct {
	BatchResults      []*workflows.ExecutionOutput
	AllResults        []workflows.TestResult
	AggregatedSummary *workflows.RunSummary
	Duration          time.Duration
	Errors            []string
}
