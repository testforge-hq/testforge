package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/storage"
)

// MockManager executes tests locally for development
type MockManager struct {
	workDir string
	storage *storage.MinIOClient
	logger  *zap.Logger
}

// NewMockManager creates a new mock sandbox manager for local development
func NewMockManager(workDir string, storage *storage.MinIOClient, logger *zap.Logger) *MockManager {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
	}
	return &MockManager{
		workDir: workDir,
		storage: storage,
		logger:  logger,
	}
}

// RunTests executes tests locally (for development without K8s)
func (m *MockManager) RunTests(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
	startTime := time.Now()

	m.logger.Info("Starting local test execution",
		zap.String("run_id", req.RunID),
		zap.String("target_url", req.TargetURL),
		zap.String("test_filter", req.TestFilter),
	)

	// Create working directory for this run
	runDir := filepath.Join(m.workDir, req.RunID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("creating run directory: %w", err)
	}

	result := &SandboxResult{
		RunID:    req.RunID,
		TenantID: req.TenantID,
		Status:   SandboxStatusRunning,
	}

	// Step 1: Download scripts from MinIO (or use local path)
	scriptsDir := filepath.Join(runDir, "scripts")
	if err := m.downloadScripts(ctx, req.ScriptsURI, scriptsDir); err != nil {
		result.Status = SandboxStatusError
		result.Error = fmt.Sprintf("failed to download scripts: %v", err)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	m.logger.Info("Scripts downloaded", zap.String("dir", scriptsDir))

	// Step 2: Run npm install
	if err := m.runNpmInstall(ctx, scriptsDir); err != nil {
		result.Status = SandboxStatusError
		result.Error = fmt.Sprintf("npm install failed: %v", err)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	m.logger.Info("npm install completed")

	// Step 3: Install Playwright browsers (if needed)
	if err := m.installPlaywrightBrowsers(ctx, scriptsDir); err != nil {
		m.logger.Warn("Failed to install browsers, continuing anyway", zap.Error(err))
	}

	// Step 4: Run Playwright tests
	testResult, logs, err := m.runPlaywrightTests(ctx, scriptsDir, req)
	result.Logs = logs

	if err != nil {
		result.Status = SandboxStatusFailed
		result.ExitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		result.Error = err.Error()
	} else {
		result.Status = SandboxStatusSucceeded
		result.ExitCode = 0
	}

	result.Duration = time.Since(startTime)

	// Step 5: Parse test results
	if testResult != nil {
		result.TestsPassed = testResult.Stats.Expected
		result.TestsFailed = testResult.Stats.Unexpected
		result.TestsSkipped = testResult.Stats.Skipped
		result.TotalTests = result.TestsPassed + result.TestsFailed + result.TestsSkipped
		result.RawResults, _ = json.Marshal(testResult)
	}

	// Step 6: Upload results to MinIO (if storage is available)
	if m.storage != nil {
		m.uploadResults(ctx, req, result, runDir)
	}

	m.logger.Info("Test execution completed",
		zap.String("status", string(result.Status)),
		zap.Int("passed", result.TestsPassed),
		zap.Int("failed", result.TestsFailed),
		zap.Duration("duration", result.Duration),
	)

	return result, nil
}

// downloadScripts downloads and extracts scripts from MinIO or local path
func (m *MockManager) downloadScripts(ctx context.Context, scriptsURI string, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	// Check if it's a local path
	if strings.HasPrefix(scriptsURI, "/") || strings.HasPrefix(scriptsURI, "./") {
		// Local path - just use it directly or copy
		if _, err := os.Stat(scriptsURI); err == nil {
			// Copy directory contents
			return m.copyDir(scriptsURI, targetDir)
		}
		return fmt.Errorf("local scripts path not found: %s", scriptsURI)
	}

	// Download from MinIO
	if m.storage == nil {
		return fmt.Errorf("storage not configured for MinIO URI: %s", scriptsURI)
	}

	// Parse MinIO URI (format: bucket/path/to/scripts.zip)
	zipPath := filepath.Join(targetDir, "scripts.zip")

	data, err := m.storage.Download(ctx, scriptsURI)
	if err != nil {
		return fmt.Errorf("downloading from MinIO: %w", err)
	}

	if err := os.WriteFile(zipPath, data, 0644); err != nil {
		return fmt.Errorf("writing zip file: %w", err)
	}

	// Extract zip
	return m.extractZip(zipPath, targetDir)
}

// copyDir copies a directory recursively (skipping node_modules since we run npm install)
func (m *MockManager) copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Skip node_modules - we'll run npm install
		if relPath == "node_modules" || strings.HasPrefix(relPath, "node_modules/") ||
			strings.Contains(relPath, "/node_modules/") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip test-results and reports directories
		if relPath == "test-results" || relPath == "reports" || relPath == "playwright-report" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		targetPath := filepath.Join(dst, relPath)

		// Check if it's a symlink using Lstat
		linfo, lerr := os.Lstat(path)
		if lerr != nil {
			return lerr
		}

		if linfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink - copy it as symlink
			return m.copyFile(path, targetPath)
		}

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		return m.copyFile(path, targetPath)
	})
}

// copyFile copies a single file (or creates symlink for symlinks)
func (m *MockManager) copyFile(src, dst string) error {
	// Check if source is a symlink
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		// It's a symlink - read the link target and create a new symlink
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	}

	// Regular file copy
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// extractZip extracts a zip file
func (m *MockManager) extractZip(zipPath, targetDir string) error {
	cmd := exec.Command("unzip", "-o", zipPath, "-d", targetDir)
	return cmd.Run()
}

// runNpmInstall runs npm install in the scripts directory
func (m *MockManager) runNpmInstall(ctx context.Context, scriptsDir string) error {
	cmd := exec.CommandContext(ctx, "npm", "ci", "--prefer-offline")
	cmd.Dir = scriptsDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Try npm install if npm ci fails
		cmd = exec.CommandContext(ctx, "npm", "install")
		cmd.Dir = scriptsDir
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %s", err, stderr.String())
		}
	}

	return nil
}

// installPlaywrightBrowsers installs Playwright browsers
func (m *MockManager) installPlaywrightBrowsers(ctx context.Context, scriptsDir string) error {
	cmd := exec.CommandContext(ctx, "npx", "playwright", "install", "chromium")
	cmd.Dir = scriptsDir
	return cmd.Run()
}

// runPlaywrightTests runs the Playwright tests and returns results
func (m *MockManager) runPlaywrightTests(ctx context.Context, scriptsDir string, req SandboxRequest) (*PlaywrightResults, string, error) {
	// Build command arguments
	args := []string{"playwright", "test"}

	// Add reporter for JSON output
	resultsFile := filepath.Join(scriptsDir, "test-results", "results.json")
	args = append(args, "--reporter=json")

	// Add test filter if specified
	if req.TestFilter != "" {
		args = append(args, "--grep", req.TestFilter)
	}

	if req.GrepPattern != "" {
		args = append(args, "--grep", req.GrepPattern)
	}

	// Add specific test files if specified
	for _, file := range req.TestFiles {
		args = append(args, file)
	}

	// Add browser project
	browser := req.Browser
	if browser == "" {
		browser = "chromium"
	}
	args = append(args, "--project="+browser)

	// Add workers
	workers := req.Workers
	if workers <= 0 {
		workers = 2
	}
	args = append(args, fmt.Sprintf("--workers=%d", workers))

	// Add retries
	if req.Retries > 0 {
		args = append(args, fmt.Sprintf("--retries=%d", req.Retries))
	}

	m.logger.Info("Running Playwright tests",
		zap.Strings("args", args),
		zap.String("dir", scriptsDir),
	)

	// Create command
	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Dir = scriptsDir

	// Set environment
	cmd.Env = append(os.Environ(),
		"CI=true",
		fmt.Sprintf("BASE_URL=%s", req.TargetURL),
		fmt.Sprintf("TEST_ENV=%s", req.Environment),
		fmt.Sprintf("TESTFORGE_RUN_ID=%s", req.RunID),
		fmt.Sprintf("PLAYWRIGHT_JSON_OUTPUT_NAME=%s", resultsFile),
	)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run tests
	err := cmd.Run()
	logs := stdout.String() + "\n" + stderr.String()

	stderrStr := stderr.String()
	stderrPreview := stderrStr
	if len(stderrPreview) > 1000 {
		stderrPreview = stderrPreview[:1000]
	}
	m.logger.Debug("Playwright command output",
		zap.String("stdout_len", fmt.Sprintf("%d bytes", len(stdout.String()))),
		zap.String("stderr_len", fmt.Sprintf("%d bytes", len(stderrStr))),
		zap.String("stderr_content", stderrPreview),
		zap.Error(err),
	)

	// Parse results
	var results *PlaywrightResults

	// Try to parse JSON from stdout first (that's where --reporter=json writes)
	if stdout.Len() > 0 && strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		results = &PlaywrightResults{}
		if jsonErr := json.Unmarshal(stdout.Bytes(), results); jsonErr != nil {
			m.logger.Warn("Failed to parse JSON from stdout", zap.Error(jsonErr))
			results = nil
		} else {
			m.logger.Info("Parsed results from stdout JSON")
			return results, logs, err
		}
	}
	if data, readErr := os.ReadFile(resultsFile); readErr == nil {
		results = &PlaywrightResults{}
		if jsonErr := json.Unmarshal(data, results); jsonErr != nil {
			m.logger.Warn("Failed to parse results JSON", zap.Error(jsonErr))
			results = nil
		}
	} else {
		// Try to find results in different locations
		altPaths := []string{
			filepath.Join(scriptsDir, "results.json"),
			filepath.Join(scriptsDir, "playwright-report", "results.json"),
		}
		for _, p := range altPaths {
			if data, readErr := os.ReadFile(p); readErr == nil {
				results = &PlaywrightResults{}
				if json.Unmarshal(data, results) == nil {
					break
				}
			}
		}
	}

	// If we still don't have results, try to parse from stdout
	if results == nil {
		results = m.parseResultsFromOutput(logs)
	}

	return results, logs, err
}

// parseResultsFromOutput attempts to extract test results from Playwright output
func (m *MockManager) parseResultsFromOutput(output string) *PlaywrightResults {
	results := &PlaywrightResults{}

	// Look for the summary line like "5 passed (3.9s)"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse "X passed"
		if strings.Contains(line, "passed") {
			var passed int
			fmt.Sscanf(line, "%d passed", &passed)
			results.Stats.Expected = passed
		}

		// Parse "X failed"
		if strings.Contains(line, "failed") {
			var failed int
			fmt.Sscanf(line, "%d failed", &failed)
			results.Stats.Unexpected = failed
		}

		// Parse "X skipped"
		if strings.Contains(line, "skipped") {
			var skipped int
			fmt.Sscanf(line, "%d skipped", &skipped)
			results.Stats.Skipped = skipped
		}
	}

	return results
}

// uploadResults uploads test results and artifacts to MinIO
func (m *MockManager) uploadResults(ctx context.Context, req SandboxRequest, result *SandboxResult, runDir string) {
	basePath := fmt.Sprintf("%s/%s", req.TenantID, req.RunID)

	// Upload logs
	if result.Logs != "" {
		logsURI, err := m.storage.UploadJSON(ctx, basePath+"/execution.log", []byte(result.Logs))
		if err == nil {
			result.LogsURI = logsURI
		}
	}

	// Upload raw results
	if result.RawResults != nil {
		resultsURI, err := m.storage.UploadJSON(ctx, basePath+"/results.json", result.RawResults)
		if err == nil {
			result.ResultsURI = resultsURI
		}
	}

	// Upload screenshots
	screenshotsDir := filepath.Join(runDir, "scripts", "test-results")
	if files, err := filepath.Glob(filepath.Join(screenshotsDir, "**", "*.png")); err == nil && len(files) > 0 {
		for _, file := range files {
			if data, err := os.ReadFile(file); err == nil {
				filename := filepath.Base(file)
				key := basePath + "/screenshots/" + filename
				m.storage.Upload(ctx, key, data, "image/png")
			}
		}
		result.ScreenshotsURI = basePath + "/screenshots/"
	}
}

// Cleanup removes the working directory for a run
func (m *MockManager) Cleanup(runID string) error {
	runDir := filepath.Join(m.workDir, runID)
	return os.RemoveAll(runDir)
}
