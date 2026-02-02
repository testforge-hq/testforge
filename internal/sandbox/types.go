package sandbox

import (
	"time"
)

// SandboxStatus represents the status of a sandbox execution
type SandboxStatus string

const (
	SandboxStatusPending   SandboxStatus = "pending"
	SandboxStatusRunning   SandboxStatus = "running"
	SandboxStatusSucceeded SandboxStatus = "succeeded"
	SandboxStatusFailed    SandboxStatus = "failed"
	SandboxStatusTimeout   SandboxStatus = "timeout"
	SandboxStatusError     SandboxStatus = "error"
)

// ManagerConfig configures the sandbox manager
type ManagerConfig struct {
	Namespace      string
	DefaultTimeout time.Duration

	// MinIO configuration
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string

	// API endpoint for callbacks
	APIEndpoint string

	// Resource limits by tier
	FreeTier       ResourceLimits
	ProTier        ResourceLimits
	EnterpriseTier ResourceLimits

	// Local mode for development
	LocalMode    bool
	LocalWorkDir string
}

// DefaultManagerConfig returns sensible defaults
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		Namespace:      "testforge-sandboxes",
		DefaultTimeout: 15 * time.Minute,
		MinIOBucket:    "testforge",
		FreeTier: ResourceLimits{
			RequestCPU:    "500m",
			RequestMemory: "1Gi",
			LimitCPU:      "1",
			LimitMemory:   "2Gi",
		},
		ProTier: ResourceLimits{
			RequestCPU:    "1",
			RequestMemory: "2Gi",
			LimitCPU:      "2",
			LimitMemory:   "4Gi",
		},
		EnterpriseTier: ResourceLimits{
			RequestCPU:    "2",
			RequestMemory: "4Gi",
			LimitCPU:      "4",
			LimitMemory:   "8Gi",
		},
		LocalMode:    true, // Default to local mode for development
		LocalWorkDir: "/tmp/testforge-sandboxes",
	}
}

// ResourceLimits defines CPU and memory limits for a tier
type ResourceLimits struct {
	RequestCPU    string
	RequestMemory string
	LimitCPU      string
	LimitMemory   string
}

// SandboxRequest contains the parameters for running tests in a sandbox
type SandboxRequest struct {
	RunID       string
	TenantID    string
	ProjectID   string
	Tier        string // free, pro, enterprise
	ScriptsURI  string // MinIO URI to scripts.zip
	TargetURL   string // Target website URL
	Environment string // dev, staging, prod
	TestFilter  string // e.g., "@smoke" or specific test file
	Timeout     time.Duration
	Parallelism int // Number of parallel workers (for K8s mode)

	// Playwright options
	Browser     string   // chromium, firefox, webkit
	Headed      bool     // Run headed (for debugging)
	Retries     int      // Number of retries for flaky tests
	Workers     int      // Number of test workers
	TestFiles   []string // Specific test files to run
	GrepPattern string   // Pattern to filter tests
}

// SandboxResult contains the result of a sandbox execution
type SandboxResult struct {
	RunID    string
	TenantID string

	// Execution status
	Status   SandboxStatus
	ExitCode int
	Duration time.Duration

	// Test results
	TestsPassed  int
	TestsFailed  int
	TestsSkipped int
	TotalTests   int

	// Artifacts
	ResultsURI     string // MinIO URI to results.json
	ArtifactsURI   string // MinIO URI to artifacts directory
	ScreenshotsURI string
	VideosURI      string
	TracesURI      string
	LogsURI        string

	// Raw data
	Logs       string
	RawResults []byte // Raw JSON results from Playwright

	// Error information
	Error        string
	ErrorDetails string
}

// PlaywrightResults represents the JSON output from Playwright
type PlaywrightResults struct {
	Config struct {
		RootDir string `json:"rootDir"`
	} `json:"config"`
	Suites []PlaywrightSuite `json:"suites"`
	Stats  PlaywrightStats   `json:"stats"`
}

// PlaywrightSuite represents a test suite in results
type PlaywrightSuite struct {
	Title  string            `json:"title"`
	File   string            `json:"file"`
	Specs  []PlaywrightSpec  `json:"specs"`
	Suites []PlaywrightSuite `json:"suites"`
}

// PlaywrightSpec represents a test spec
type PlaywrightSpec struct {
	Title string           `json:"title"`
	OK    bool             `json:"ok"`
	Tests []PlaywrightTest `json:"tests"`
}

// PlaywrightTest represents a test result
type PlaywrightTest struct {
	Timeout   int     `json:"timeout"`
	Retries   int     `json:"retries"`
	ProjectID string  `json:"projectId"`
	Status    string  `json:"status"` // passed, failed, skipped, timedOut
	Duration  float64 `json:"duration"`
	Error     *struct {
		Message string `json:"message"`
		Stack   string `json:"stack"`
	} `json:"error,omitempty"`
}

// PlaywrightStats contains aggregate test statistics
type PlaywrightStats struct {
	StartTime  string  `json:"startTime"`
	Duration   float64 `json:"duration"` // Duration in ms (can be decimal)
	Expected   int     `json:"expected"`
	Unexpected int     `json:"unexpected"`
	Flaky      int     `json:"flaky"`
	Skipped    int     `json:"skipped"`
}

// BatchExecutionInput for parallel batch execution
type BatchExecutionInput struct {
	RunID        string
	TenantID     string
	Tier         string
	ScriptsURI   string
	TargetURL    string
	Environment  string
	TestFiles    []string
	BatchIndex   int
	TotalBatches int
	Timeout      time.Duration
}

// BatchExecutionResult from a single batch
type BatchExecutionResult struct {
	BatchIndex   int
	Status       SandboxStatus
	Duration     time.Duration
	TestsPassed  int
	TestsFailed  int
	TestsSkipped int
	Error        string
	ResultsURI   string
}
