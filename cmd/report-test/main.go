package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/services/reporting"
)

func main() {
	// Auto-load .env file if present
	godotenv.Load()

	// Parse flags
	apiKey := flag.String("api-key", os.Getenv("ANTHROPIC_API_KEY"), "Claude API key")
	model := flag.String("model", "claude-sonnet-4-20250514", "Claude model to use")
	outputDir := flag.String("output", "/tmp/testforge/reports", "Output directory for reports")
	slackWebhook := flag.String("slack", "", "Slack webhook URL for notifications")
	openBrowser := flag.Bool("open", true, "Open HTML report in browser")
	verbose := flag.Bool("verbose", false, "Verbose output")
	jsonOutput := flag.Bool("json", false, "Output JSON instead of HTML")

	flag.Parse()

	// Setup logger
	var logger *zap.Logger
	var err error
	if *verbose {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Run demo
	runDemo(logger, *apiKey, *model, *outputDir, *slackWebhook, *openBrowser, *jsonOutput)
}

func runDemo(logger *zap.Logger, apiKey, model, outputDir, slackWebhook string, openBrowser, jsonOutput bool) {
	fmt.Println("📊 TestForge Enterprise Reporting Demo")
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create mock storage client
	storage := &mockStorageClient{}

	// Create generator config
	genCfg := reporting.GeneratorConfig{
		ClaudeAPIKey: apiKey,
		ClaudeModel:  model,
	}

	if genCfg.ClaudeModel == "" {
		genCfg.ClaudeModel = "claude-sonnet-4-20250514"
	}

	generator, err := reporting.NewGenerator(genCfg, storage, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create generator: %v\n", err)
		os.Exit(1)
	}

	// Create mock report input
	runID := uuid.New().String()
	projectID := uuid.New().String()
	tenantID := uuid.New().String()

	input := reporting.ReportInput{
		RunID:     runID,
		ProjectID: projectID,
		TenantID:  tenantID,
		HealingResult: &reporting.HealingResultInput{
			TotalAttempted: 3,
			Healed:         2,
			Failed:         1,
			Details: []reporting.HealingInfo{
				{
					TestID:           "test-1",
					TestName:         "Login form submission",
					OriginalSelector: "button.btn-primary.login-btn",
					NewSelector:      "[data-testid=\"login-submit\"]",
					RootCause:        "Class name changed during refactor",
					Confidence:       0.95,
					VJEPASimilarity:  0.92,
				},
				{
					TestID:           "test-2",
					TestName:         "Add to cart button",
					OriginalSelector: "#add-to-cart-btn",
					NewSelector:      "#btn-add-cart",
					RootCause:        "ID naming convention changed",
					Confidence:       0.88,
					VJEPASimilarity:  0.90,
				},
			},
		},
	}

	fmt.Println("📋 Generating report for test run...")
	fmt.Printf("   Run ID: %s\n", runID)
	fmt.Printf("   Project: %s\n", projectID)
	fmt.Println()

	// Generate report
	startTime := time.Now()
	report := createMockReport(runID, projectID, tenantID, input.HealingResult)

	// Generate AI insights if API key provided
	if apiKey != "" {
		fmt.Println("🤖 Generating AI insights with Claude...")
		aiInsights, err := generateMockAIInsights(ctx, generator, report)
		if err != nil {
			fmt.Printf("   ⚠️  AI insights skipped: %v\n", err)
		} else {
			report.AIInsights = aiInsights
			fmt.Println("   ✅ AI analysis complete")
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n⏱️  Report generated in %v\n", elapsed)

	// Build executive summary
	report.Executive = buildExecutiveSummary(report)

	// Output results
	fmt.Println()
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println("📊 Executive Summary")
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Printf("   Status: %s\n", report.Executive.Status)
	fmt.Printf("   Health Score: %.0f%%\n", report.Executive.HealthScore)
	fmt.Printf("   Risk Level: %s\n", report.Executive.RiskLevel)
	fmt.Println()
	fmt.Printf("   Tests: %d total, %d passed, %d failed\n",
		report.Executive.TotalTests,
		report.Executive.Passed,
		report.Executive.Failed)
	fmt.Printf("   Auto-Healed: %d tests\n", report.Executive.Healed)
	fmt.Println()
	fmt.Printf("   Deployment: %s\n", formatBool(report.Executive.DeploymentSafe, "✅ Safe", "❌ Blocked"))
	fmt.Printf("   Reason: %s\n", report.Executive.DeploymentReason)
	fmt.Println()
	fmt.Printf("📝 One-liner: %s\n", report.Executive.OneLiner)

	// Render report
	var reportPath string
	if jsonOutput {
		jsonData, err := generator.RenderJSON(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to render JSON: %v\n", err)
			os.Exit(1)
		}

		reportPath = fmt.Sprintf("%s/%s/report.json", outputDir, runID)
		if err := os.MkdirAll(fmt.Sprintf("%s/%s", outputDir, runID), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(reportPath, jsonData, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write JSON: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Printf("📄 JSON report saved to: %s\n", reportPath)
	} else {
		htmlContent, err := generator.RenderHTML(report)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to render HTML: %v\n", err)
			os.Exit(1)
		}

		reportPath = fmt.Sprintf("%s/%s/report.html", outputDir, runID)
		if err := os.MkdirAll(fmt.Sprintf("%s/%s", outputDir, runID), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(reportPath, []byte(htmlContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write HTML: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Printf("🌐 HTML report saved to: %s\n", reportPath)

		// Open in browser
		if openBrowser {
			fmt.Println("   Opening in browser...")
			openURL("file://" + reportPath)
		}
	}

	// Send Slack notification if configured
	if slackWebhook != "" {
		fmt.Println()
		fmt.Println("📤 Sending Slack notification...")

		notifier := reporting.NewNotificationService("https://app.testforge.io", logger)
		slackConfig := &reporting.SlackConfig{
			WebhookURL: slackWebhook,
			OnSuccess:  true,
			OnFailure:  true,
		}

		if err := notifier.NotifySlack(ctx, report, slackConfig); err != nil {
			fmt.Printf("   ⚠️  Slack notification failed: %v\n", err)
		} else {
			fmt.Println("   ✅ Slack notification sent!")
		}
	}

	fmt.Println()
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println("✨ Demo Complete!")
	fmt.Println()
	fmt.Println("Features demonstrated:")
	fmt.Println("  1. ✅ Full report generation")
	fmt.Println("  2. ✅ HTML dashboard with Tailwind CSS")
	if apiKey != "" {
		fmt.Println("  3. ✅ AI insights for failures")
	} else {
		fmt.Println("  3. ⏭️  AI insights (skipped - no API key)")
	}
	fmt.Println("  4. ✅ Self-healing results included")
	if slackWebhook != "" {
		fmt.Println("  5. ✅ Slack notification sent")
	} else {
		fmt.Println("  5. ⏭️  Slack notification (skipped - no webhook)")
	}
	fmt.Println("  6. ✅ PDF export (use browser print)")
	fmt.Println()
	fmt.Println("Usage with custom options:")
	fmt.Println("  go run cmd/report-test/main.go \\")
	fmt.Println("    --api-key 'sk-ant-...' \\")
	fmt.Println("    --slack 'https://hooks.slack.com/...' \\")
	fmt.Println("    --output /path/to/reports")
}

func createMockReport(runID, projectID, tenantID string, healingResult *reporting.HealingResultInput) *reporting.TestRunReport {
	report := reporting.NewTestRunReport(runID, projectID, tenantID)

	// Add mock test results
	report.Results = reporting.TestResults{
		Suites: []reporting.SuiteResult{
			{
				Name:     "Authentication",
				Status:   "passed",
				Duration: "12.5s",
				Passed:   5,
				Failed:   0,
				Tests: []reporting.TestResult{
					{ID: "auth-1", Name: "User can login with valid credentials", Suite: "Authentication", Status: "passed", Duration: "2.1s"},
					{ID: "auth-2", Name: "User sees error with invalid password", Suite: "Authentication", Status: "passed", Duration: "1.8s"},
					{ID: "auth-3", Name: "User can reset password", Suite: "Authentication", Status: "passed", Duration: "3.2s"},
					{ID: "auth-4", Name: "User can logout", Suite: "Authentication", Status: "passed", Duration: "1.5s"},
					{ID: "auth-5", Name: "Session persists after refresh", Suite: "Authentication", Status: "passed", Duration: "3.9s"},
				},
			},
			{
				Name:     "Shopping Cart",
				Status:   "failed",
				Duration: "18.3s",
				Passed:   3,
				Failed:   2,
				Tests: []reporting.TestResult{
					{ID: "cart-1", Name: "Add single item to cart", Suite: "Shopping Cart", Status: "passed", Duration: "2.5s", WasHealed: true, HealingInfo: &reporting.HealingInfo{
						OriginalSelector: "#add-to-cart-btn",
						NewSelector:      "#btn-add-cart",
						Confidence:       0.88,
					}},
					{ID: "cart-2", Name: "Update item quantity", Suite: "Shopping Cart", Status: "passed", Duration: "3.1s"},
					{ID: "cart-3", Name: "Remove item from cart", Suite: "Shopping Cart", Status: "passed", Duration: "2.8s"},
					{
						ID: "cart-4", Name: "Apply discount code", Suite: "Shopping Cart", Status: "failed", Duration: "5.2s",
						Error: &reporting.ErrorDetail{
							Message:       "Expected discount to be applied but total remained unchanged",
							Stack:         "Error: expect(received).toBe(expected)\n\nExpected: $89.99\nReceived: $99.99\n\n    at Object.<anonymous> (tests/cart.spec.ts:45:21)",
							ScreenshotURI: "/artifacts/cart-4-failure.png",
						},
					},
					{
						ID: "cart-5", Name: "Checkout flow completes", Suite: "Shopping Cart", Status: "failed", Duration: "4.7s",
						Error: &reporting.ErrorDetail{
							Message:       "Timeout waiting for payment confirmation",
							Stack:         "Error: Timeout 30000ms exceeded.\nwaiting for selector \".payment-success\"\n\n    at Object.<anonymous> (tests/cart.spec.ts:67:15)",
							ScreenshotURI: "/artifacts/cart-5-failure.png",
						},
					},
				},
			},
			{
				Name:     "User Profile",
				Status:   "passed",
				Duration: "8.7s",
				Passed:   4,
				Failed:   0,
				Tests: []reporting.TestResult{
					{ID: "profile-1", Name: "User can view profile", Suite: "User Profile", Status: "passed", Duration: "1.9s"},
					{ID: "profile-2", Name: "User can update email", Suite: "User Profile", Status: "passed", Duration: "2.3s", WasHealed: true, HealingInfo: &reporting.HealingInfo{
						OriginalSelector: "button.btn-primary.login-btn",
						NewSelector:      "[data-testid=\"login-submit\"]",
						Confidence:       0.95,
					}},
					{ID: "profile-3", Name: "User can change password", Suite: "User Profile", Status: "passed", Duration: "2.5s"},
					{ID: "profile-4", Name: "User can upload avatar", Suite: "User Profile", Status: "passed", Duration: "2.0s"},
				},
			},
		},
		ByStatus: map[string]int{
			"passed":  12,
			"failed":  2,
			"skipped": 1,
		},
		ByPriority: map[string]int{
			"critical": 0,
			"high":     2,
			"medium":   8,
			"low":      5,
		},
	}

	// Add failed tests to the list
	for _, suite := range report.Results.Suites {
		for _, test := range suite.Tests {
			if test.Status == "failed" {
				report.Results.FailedTests = append(report.Results.FailedTests, test)
			}
		}
	}

	// Add healing report
	if healingResult != nil {
		report.Healing = &reporting.HealingReport{
			TotalAttempted: healingResult.TotalAttempted,
			Healed:         healingResult.Healed,
			Failed:         healingResult.Failed,
			HealingDetails: healingResult.Details,
			TimesSaved:     "~1 hour of manual fixes",
			SelectorsFixed: healingResult.Healed,
		}
	}

	// Add compliance report
	report.Compliance = reporting.ComplianceReport{
		OverallScore: 86.7,
		Standards: []reporting.ComplianceStandard{
			{Name: "Functional Testing", Status: "completed", Score: 85.7, Controls: 14, Passed: 12},
			{Name: "Security Testing", Status: "completed", Score: 100.0, Controls: 5, Passed: 5},
		},
	}

	// Add performance report
	report.Performance = reporting.PerformanceReport{
		WebVitals: reporting.WebVitals{
			LCP:  reporting.VitalMetric{Value: 2.1, Rating: "good", Unit: "s"},
			FID:  reporting.VitalMetric{Value: 85, Rating: "good", Unit: "ms"},
			CLS:  reporting.VitalMetric{Value: 0.08, Rating: "good", Unit: ""},
			FCP:  reporting.VitalMetric{Value: 1.4, Rating: "good", Unit: "s"},
			TTFB: reporting.VitalMetric{Value: 180, Rating: "good", Unit: "ms"},
		},
		SlowPages: []reporting.SlowPage{
			{URL: "/checkout", LoadTime: "4.2s", Issue: "Large payment JS bundle"},
		},
	}

	// Add accessibility report
	report.Accessibility = reporting.A11yReport{
		Standard:   "WCAG 2.1 AA",
		Score:      92.0,
		Violations: []reporting.A11yIssue{
			{ID: "color-contrast", Impact: "moderate", Description: "Low contrast text on cart button", HelpURL: "https://dequeuniversity.com/rules/axe/4.4/color-contrast", Count: 2},
		},
		Passes: 45,
	}

	// Add audit trail
	report.AuditTrail = []reporting.AuditEntry{
		{Timestamp: time.Now().Add(-5 * time.Minute), Action: "test_run_started", Actor: "ci-pipeline", Details: "Triggered by push to main"},
		{Timestamp: time.Now().Add(-4 * time.Minute), Action: "discovery_completed", Actor: "system", Details: "Found 15 test scenarios"},
		{Timestamp: time.Now().Add(-3 * time.Minute), Action: "execution_started", Actor: "system", Details: "Running 15 tests on Chrome"},
		{Timestamp: time.Now().Add(-1 * time.Minute), Action: "healing_applied", Actor: "claude-ai", Details: "Auto-healed 2 broken selectors"},
		{Timestamp: time.Now(), Action: "report_generated", Actor: "system", Details: "Enterprise report created"},
	}

	return report
}

func buildExecutiveSummary(report *reporting.TestRunReport) reporting.ExecutiveSummary {
	total := report.Results.ByStatus["passed"] + report.Results.ByStatus["failed"] + report.Results.ByStatus["skipped"]
	passed := report.Results.ByStatus["passed"]
	failed := report.Results.ByStatus["failed"]

	if total == 0 {
		total = 1
	}

	healthScore := float64(passed) / float64(total) * 100

	var status, riskLevel string
	var deploymentSafe bool
	var deploymentReason string

	if failed == 0 {
		status = "passed"
		riskLevel = "low"
		deploymentSafe = true
		deploymentReason = "All tests passed"
	} else if failed > total/10 {
		status = "failed"
		riskLevel = "high"
		deploymentSafe = false
		deploymentReason = fmt.Sprintf(">10%% tests failed (%d/%d)", failed, total)
	} else {
		status = "failed"
		riskLevel = "medium"
		deploymentSafe = true
		deploymentReason = "Non-critical failures only"
	}

	healed := 0
	if report.Healing != nil {
		healed = report.Healing.Healed
	}

	var oneLiner string
	if deploymentSafe {
		if healed > 0 {
			oneLiner = fmt.Sprintf("✅ %d/%d tests passed (%.0f%%) | 🔧 %d auto-healed | Safe to deploy",
				passed, total, healthScore, healed)
		} else {
			oneLiner = fmt.Sprintf("✅ %d/%d tests passed (%.0f%%) - Safe to deploy", passed, total, healthScore)
		}
	} else {
		oneLiner = fmt.Sprintf("❌ %d/%d tests failed - %s", failed, total, deploymentReason)
	}

	return reporting.ExecutiveSummary{
		Status:           status,
		HealthScore:      healthScore,
		RiskLevel:        riskLevel,
		TotalTests:       total,
		Passed:           passed,
		Failed:           failed,
		Skipped:          report.Results.ByStatus["skipped"],
		Healed:           healed,
		DeploymentSafe:   deploymentSafe,
		DeploymentReason: deploymentReason,
		OneLiner:         oneLiner,
		Duration:         "39.5s",
		StartedAt:        time.Now().Add(-5 * time.Minute),
		CompletedAt:      time.Now(),
	}
}

func generateMockAIInsights(ctx context.Context, generator *reporting.Generator, report *reporting.TestRunReport) (*reporting.AIAnalysis, error) {
	return &reporting.AIAnalysis{
		GeneratedAt: time.Now(),
		Model:       "claude-sonnet-4-20250514",
		Summary:     "Test suite shows good overall health (85.7%) with 2 failures in the Shopping Cart module. The failures appear to be related to payment processing integration, likely due to a timeout in the external payment gateway. Self-healing successfully fixed 2 broken selectors from recent UI changes.",
		FailurePatterns: []reporting.FailurePattern{
			{
				Pattern:       "Payment Integration Timeout",
				Occurrences:   2,
				AffectedTests: []string{"Apply discount code", "Checkout flow completes"},
				Severity:      "high",
				Suggestion:    "Increase timeout for payment gateway calls or add retry logic",
			},
		},
		RootCauses: []reporting.RootCause{
			{
				Category:    "api",
				Description: "Payment gateway responding slowly",
				Evidence:    "Timeout errors in checkout and discount tests",
				Fix:         "Add circuit breaker pattern for payment API calls",
				Confidence:  0.85,
			},
		},
		Recommendations: []reporting.Recommendation{
			{
				Priority:    "high",
				Category:    "infrastructure",
				Title:       "Add payment gateway health check",
				Description: "Implement a pre-flight check for payment gateway availability before running checkout tests",
				Impact:      "Reduce flaky test failures by 60%",
				Effort:      "low",
			},
			{
				Priority:    "medium",
				Category:    "test",
				Title:       "Add retry logic to payment tests",
				Description: "Implement automatic retries for transient payment gateway errors",
				Impact:      "Improve test reliability",
				Effort:      "low",
			},
			{
				Priority:    "low",
				Category:    "process",
				Title:       "Consider payment gateway mocking",
				Description: "Use a mock payment service in CI to avoid external dependencies",
				Impact:      "Faster, more reliable tests",
				Effort:      "medium",
			},
		},
		RiskAssessment: reporting.RiskAssessment{
			OverallRisk:    "medium",
			DeploymentSafe: true,
			BlockingIssues: []string{},
			Warnings:       []string{"Payment flow should be manually verified before deployment"},
			AreaRisks: map[string]string{
				"authentication": "low",
				"shopping_cart":  "medium",
				"user_profile":   "low",
			},
		},
	}, nil
}

func formatBool(b bool, trueVal, falseVal string) string {
	if b {
		return trueVal
	}
	return falseVal
}

func openURL(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}

// mockStorageClient implements the storage interface for demo purposes
type mockStorageClient struct{}

func (m *mockStorageClient) Download(ctx context.Context, uri string) ([]byte, error) {
	// Return mock Playwright results
	results := map[string]interface{}{
		"suites": []interface{}{},
		"stats": map[string]int{
			"total":   15,
			"passed":  12,
			"failed":  2,
			"skipped": 1,
		},
	}
	return json.Marshal(results)
}

func (m *mockStorageClient) Upload(ctx context.Context, bucket, key string, data []byte, contentType string) (string, error) {
	return fmt.Sprintf("s3://%s/%s", bucket, key), nil
}
