package sandbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSandboxStatus_Constants(t *testing.T) {
	// Verify status constants have expected values
	assert.Equal(t, SandboxStatus("pending"), SandboxStatusPending)
	assert.Equal(t, SandboxStatus("running"), SandboxStatusRunning)
	assert.Equal(t, SandboxStatus("succeeded"), SandboxStatusSucceeded)
	assert.Equal(t, SandboxStatus("failed"), SandboxStatusFailed)
	assert.Equal(t, SandboxStatus("timeout"), SandboxStatusTimeout)
	assert.Equal(t, SandboxStatus("error"), SandboxStatusError)
}

func TestDefaultManagerConfig(t *testing.T) {
	config := DefaultManagerConfig()

	// Verify namespace
	assert.Equal(t, "testforge-sandboxes", config.Namespace)

	// Verify default timeout
	assert.Equal(t, 15*time.Minute, config.DefaultTimeout)

	// Verify MinIO bucket
	assert.Equal(t, "testforge", config.MinIOBucket)

	// Verify local mode defaults
	assert.True(t, config.LocalMode)
	assert.Equal(t, "/tmp/testforge-sandboxes", config.LocalWorkDir)

	// Verify FreeTier defaults
	assert.Equal(t, "500m", config.FreeTier.RequestCPU)
	assert.Equal(t, "1Gi", config.FreeTier.RequestMemory)
	assert.Equal(t, "1", config.FreeTier.LimitCPU)
	assert.Equal(t, "2Gi", config.FreeTier.LimitMemory)

	// Verify ProTier defaults
	assert.Equal(t, "1", config.ProTier.RequestCPU)
	assert.Equal(t, "2Gi", config.ProTier.RequestMemory)
	assert.Equal(t, "2", config.ProTier.LimitCPU)
	assert.Equal(t, "4Gi", config.ProTier.LimitMemory)

	// Verify EnterpriseTier defaults
	assert.Equal(t, "2", config.EnterpriseTier.RequestCPU)
	assert.Equal(t, "4Gi", config.EnterpriseTier.RequestMemory)
	assert.Equal(t, "4", config.EnterpriseTier.LimitCPU)
	assert.Equal(t, "8Gi", config.EnterpriseTier.LimitMemory)
}

func TestResourceLimits_Struct(t *testing.T) {
	limits := ResourceLimits{
		RequestCPU:    "100m",
		RequestMemory: "128Mi",
		LimitCPU:      "500m",
		LimitMemory:   "512Mi",
	}

	assert.Equal(t, "100m", limits.RequestCPU)
	assert.Equal(t, "128Mi", limits.RequestMemory)
	assert.Equal(t, "500m", limits.LimitCPU)
	assert.Equal(t, "512Mi", limits.LimitMemory)
}

func TestSandboxRequest_Fields(t *testing.T) {
	req := SandboxRequest{
		RunID:       "run-123",
		TenantID:    "tenant-456",
		ProjectID:   "project-789",
		Tier:        "pro",
		ScriptsURI:  "bucket/scripts.zip",
		TargetURL:   "https://example.com",
		Environment: "staging",
		TestFilter:  "@smoke",
		Timeout:     10 * time.Minute,
		Parallelism: 4,
		Browser:     "chromium",
		Headed:      false,
		Retries:     2,
		Workers:     4,
		TestFiles:   []string{"test1.spec.ts", "test2.spec.ts"},
		GrepPattern: "login",
	}

	assert.Equal(t, "run-123", req.RunID)
	assert.Equal(t, "tenant-456", req.TenantID)
	assert.Equal(t, "project-789", req.ProjectID)
	assert.Equal(t, "pro", req.Tier)
	assert.Equal(t, "bucket/scripts.zip", req.ScriptsURI)
	assert.Equal(t, "https://example.com", req.TargetURL)
	assert.Equal(t, "staging", req.Environment)
	assert.Equal(t, "@smoke", req.TestFilter)
	assert.Equal(t, 10*time.Minute, req.Timeout)
	assert.Equal(t, 4, req.Parallelism)
	assert.Equal(t, "chromium", req.Browser)
	assert.False(t, req.Headed)
	assert.Equal(t, 2, req.Retries)
	assert.Equal(t, 4, req.Workers)
	assert.Equal(t, []string{"test1.spec.ts", "test2.spec.ts"}, req.TestFiles)
	assert.Equal(t, "login", req.GrepPattern)
}

func TestSandboxResult_Fields(t *testing.T) {
	result := SandboxResult{
		RunID:          "run-123",
		TenantID:       "tenant-456",
		Status:         SandboxStatusSucceeded,
		ExitCode:       0,
		Duration:       5 * time.Minute,
		TestsPassed:    10,
		TestsFailed:    2,
		TestsSkipped:   1,
		TotalTests:     13,
		ResultsURI:     "bucket/results.json",
		ArtifactsURI:   "bucket/artifacts/",
		ScreenshotsURI: "bucket/screenshots/",
		VideosURI:      "bucket/videos/",
		TracesURI:      "bucket/traces/",
		LogsURI:        "bucket/logs/",
		Logs:           "Test output...",
		RawResults:     []byte(`{"stats":{}}`),
		Error:          "",
		ErrorDetails:   "",
	}

	assert.Equal(t, "run-123", result.RunID)
	assert.Equal(t, "tenant-456", result.TenantID)
	assert.Equal(t, SandboxStatusSucceeded, result.Status)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, 5*time.Minute, result.Duration)
	assert.Equal(t, 10, result.TestsPassed)
	assert.Equal(t, 2, result.TestsFailed)
	assert.Equal(t, 1, result.TestsSkipped)
	assert.Equal(t, 13, result.TotalTests)
	assert.Equal(t, "bucket/results.json", result.ResultsURI)
	assert.Equal(t, "bucket/artifacts/", result.ArtifactsURI)
	assert.Equal(t, "bucket/screenshots/", result.ScreenshotsURI)
	assert.Equal(t, "bucket/videos/", result.VideosURI)
	assert.Equal(t, "bucket/traces/", result.TracesURI)
	assert.Equal(t, "bucket/logs/", result.LogsURI)
	assert.Equal(t, "Test output...", result.Logs)
	assert.Equal(t, []byte(`{"stats":{}}`), result.RawResults)
	assert.Empty(t, result.Error)
	assert.Empty(t, result.ErrorDetails)
}

func TestPlaywrightResults_Struct(t *testing.T) {
	results := PlaywrightResults{
		Config: struct {
			RootDir string `json:"rootDir"`
		}{RootDir: "/workspace"},
		Suites: []PlaywrightSuite{
			{
				Title: "Login Tests",
				File:  "login.spec.ts",
				Specs: []PlaywrightSpec{
					{
						Title: "should login",
						OK:    true,
						Tests: []PlaywrightTest{
							{
								Timeout:   30000,
								Retries:   0,
								ProjectID: "chromium",
								Status:    "passed",
								Duration:  1234.5,
							},
						},
					},
				},
			},
		},
		Stats: PlaywrightStats{
			StartTime:  "2024-01-01T00:00:00Z",
			Duration:   5000.5,
			Expected:   10,
			Unexpected: 2,
			Flaky:      1,
			Skipped:    0,
		},
	}

	assert.Equal(t, "/workspace", results.Config.RootDir)
	assert.Len(t, results.Suites, 1)
	assert.Equal(t, "Login Tests", results.Suites[0].Title)
	assert.Equal(t, "login.spec.ts", results.Suites[0].File)
	assert.Len(t, results.Suites[0].Specs, 1)
	assert.Equal(t, "should login", results.Suites[0].Specs[0].Title)
	assert.True(t, results.Suites[0].Specs[0].OK)
	assert.Len(t, results.Suites[0].Specs[0].Tests, 1)
	assert.Equal(t, "passed", results.Suites[0].Specs[0].Tests[0].Status)
	assert.Equal(t, 1234.5, results.Suites[0].Specs[0].Tests[0].Duration)
	assert.Equal(t, 10, results.Stats.Expected)
	assert.Equal(t, 2, results.Stats.Unexpected)
	assert.Equal(t, 1, results.Stats.Flaky)
}

func TestPlaywrightTest_WithError(t *testing.T) {
	test := PlaywrightTest{
		Timeout:   30000,
		Retries:   2,
		ProjectID: "chromium",
		Status:    "failed",
		Duration:  5000,
		Error: &struct {
			Message string `json:"message"`
			Stack   string `json:"stack"`
		}{
			Message: "Element not found",
			Stack:   "at test.spec.ts:10",
		},
	}

	assert.Equal(t, "failed", test.Status)
	assert.NotNil(t, test.Error)
	assert.Equal(t, "Element not found", test.Error.Message)
	assert.Equal(t, "at test.spec.ts:10", test.Error.Stack)
}

func TestBatchExecutionInput_Fields(t *testing.T) {
	input := BatchExecutionInput{
		RunID:        "run-123",
		TenantID:     "tenant-456",
		Tier:         "enterprise",
		ScriptsURI:   "bucket/scripts.zip",
		TargetURL:    "https://example.com",
		Environment:  "production",
		TestFiles:    []string{"test1.spec.ts"},
		BatchIndex:   0,
		TotalBatches: 4,
		Timeout:      20 * time.Minute,
	}

	assert.Equal(t, "run-123", input.RunID)
	assert.Equal(t, "tenant-456", input.TenantID)
	assert.Equal(t, "enterprise", input.Tier)
	assert.Equal(t, 0, input.BatchIndex)
	assert.Equal(t, 4, input.TotalBatches)
	assert.Equal(t, 20*time.Minute, input.Timeout)
}

func TestBatchExecutionResult_Fields(t *testing.T) {
	result := BatchExecutionResult{
		BatchIndex:   2,
		Status:       SandboxStatusSucceeded,
		Duration:     3 * time.Minute,
		TestsPassed:  5,
		TestsFailed:  0,
		TestsSkipped: 1,
		Error:        "",
		ResultsURI:   "bucket/batch-2/results.json",
	}

	assert.Equal(t, 2, result.BatchIndex)
	assert.Equal(t, SandboxStatusSucceeded, result.Status)
	assert.Equal(t, 3*time.Minute, result.Duration)
	assert.Equal(t, 5, result.TestsPassed)
	assert.Equal(t, 0, result.TestsFailed)
	assert.Equal(t, 1, result.TestsSkipped)
	assert.Empty(t, result.Error)
	assert.Equal(t, "bucket/batch-2/results.json", result.ResultsURI)
}

func TestBatchExecutionResult_WithError(t *testing.T) {
	result := BatchExecutionResult{
		BatchIndex:   1,
		Status:       SandboxStatusFailed,
		Duration:     1 * time.Minute,
		TestsPassed:  0,
		TestsFailed:  3,
		TestsSkipped: 0,
		Error:        "Test execution failed",
		ResultsURI:   "",
	}

	assert.Equal(t, SandboxStatusFailed, result.Status)
	assert.Equal(t, "Test execution failed", result.Error)
	assert.Empty(t, result.ResultsURI)
}
