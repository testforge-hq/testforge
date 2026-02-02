package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/sandbox"
)

func main() {
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	logger.Info("Starting TestForge Integration Test")

	// Check for scripts directory
	scriptsDir := "/tmp/playwright-tests"
	if len(os.Args) > 1 {
		scriptsDir = os.Args[1]
	}

	if _, err := os.Stat(scriptsDir); os.IsNotExist(err) {
		logger.Fatal("Scripts directory not found", zap.String("path", scriptsDir))
	}

	logger.Info("Using scripts directory", zap.String("path", scriptsDir))

	// Create mock sandbox manager
	workDir := "/tmp/testforge-integration"
	manager := sandbox.NewMockManager(workDir, nil, logger)

	// Create sandbox request
	req := sandbox.SandboxRequest{
		RunID:       fmt.Sprintf("integration-test-%d", time.Now().Unix()),
		TenantID:    "test-tenant",
		ProjectID:   "test-project",
		Tier:        "pro",
		ScriptsURI:  scriptsDir, // Local path for mock manager
		TargetURL:   "https://example.com",
		Environment: "dev",
		Timeout:     10 * time.Minute,
		Browser:     "chromium",
		Workers:     2,
		TestFilter:  "",
		TestFiles:   []string{"tests/smoke/"}, // Only run smoke tests
	}

	logger.Info("Starting sandbox execution",
		zap.String("run_id", req.RunID),
		zap.String("target_url", req.TargetURL),
		zap.String("test_filter", req.TestFilter),
	)

	// Execute tests
	ctx := context.Background()
	startTime := time.Now()

	result, err := manager.RunTests(ctx, req)
	if err != nil {
		logger.Fatal("Sandbox execution failed", zap.Error(err))
	}

	duration := time.Since(startTime)

	// Print results
	divider := strings.Repeat("=", 60)
	fmt.Println("\n" + divider)
	fmt.Println("INTEGRATION TEST RESULTS")
	fmt.Println(divider + "\n")

	fmt.Printf("Run ID:      %s\n", result.RunID)
	fmt.Printf("Status:      %s\n", result.Status)
	fmt.Printf("Exit Code:   %d\n", result.ExitCode)
	fmt.Printf("Duration:    %s\n", duration.Round(time.Millisecond))
	fmt.Println()

	fmt.Println("Test Summary:")
	fmt.Printf("  Total:     %d\n", result.TotalTests)
	fmt.Printf("  Passed:    %d\n", result.TestsPassed)
	fmt.Printf("  Failed:    %d\n", result.TestsFailed)
	fmt.Printf("  Skipped:   %d\n", result.TestsSkipped)
	fmt.Println()

	if result.TotalTests > 0 {
		passRate := float64(result.TestsPassed) / float64(result.TotalTests) * 100
		fmt.Printf("Pass Rate:   %.1f%%\n", passRate)
	}

	if result.Error != "" {
		fmt.Printf("\nError: %s\n", result.Error)
	}

	// Print raw results if available
	if result.RawResults != nil {
		fmt.Println("\nRaw Playwright Results:")
		var prettyJSON map[string]interface{}
		if err := json.Unmarshal(result.RawResults, &prettyJSON); err == nil {
			if stats, ok := prettyJSON["stats"]; ok {
				statsJSON, _ := json.MarshalIndent(stats, "  ", "  ")
				fmt.Printf("  Stats: %s\n", statsJSON)
			}
		}
	}

	// Print logs summary
	if result.Logs != "" && len(result.Logs) > 0 {
		fmt.Println("\nLogs (last 500 chars):")
		logsTail := result.Logs
		if len(logsTail) > 500 {
			logsTail = "..." + logsTail[len(logsTail)-500:]
		}
		fmt.Println(logsTail)
	}

	fmt.Println("\n" + divider)

	// Cleanup
	if err := manager.Cleanup(req.RunID); err != nil {
		logger.Warn("Cleanup failed", zap.Error(err))
	}

	// Exit with appropriate code
	if result.Status == sandbox.SandboxStatusSucceeded {
		logger.Info("Integration test PASSED!")
		os.Exit(0)
	} else {
		logger.Info("Integration test completed with failures",
			zap.String("status", string(result.Status)),
		)
		os.Exit(1)
	}
}
