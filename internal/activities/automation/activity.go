package automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"

	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements the automation (script generation) activity
type Activity struct {
	// Dependencies will be added here (template engine, etc.)
}

// NewActivity creates a new automation activity
func NewActivity() *Activity {
	return &Activity{}
}

// Execute generates Playwright scripts from test cases
func (a *Activity) Execute(ctx context.Context, input workflows.AutomationInput) (*workflows.AutomationOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting automation activity",
		"test_run_id", input.TestRunID.String(),
		"test_cases", input.TestPlan.TotalCount,
		"browser", input.Browser,
	)

	activity.RecordHeartbeat(ctx, "Generating Playwright scripts...")

	// TODO: Implement actual script generation with templates
	// For now, generate mock scripts

	scripts := []workflows.TestScript{}

	for _, tc := range input.TestPlan.TestCases {
		script := generatePlaywrightScript(tc, input.Browser, input.Timeout)
		scripts = append(scripts, workflows.TestScript{
			TestCaseID: tc.ID,
			Script:     script,
			Language:   "typescript",
		})
	}

	duration := time.Since(startTime)

	output := &workflows.AutomationOutput{
		Scripts:  scripts,
		Duration: duration,
	}

	logger.Info("Automation activity completed",
		"scripts_generated", len(scripts),
		"duration", duration,
	)

	return output, nil
}

func generatePlaywrightScript(tc workflows.TestCase, browser string, timeout int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`import { test, expect } from '@playwright/test';

test.describe('%s', () => {
  test('%s', async ({ page }) => {
    // %s
    test.setTimeout(%d);

`, escapeString(tc.Category), escapeString(tc.Name), escapeString(tc.Description), timeout))

	for _, step := range tc.Steps {
		sb.WriteString(generateStepCode(step))
		sb.WriteString("\n")
	}

	sb.WriteString(`  });
});
`)

	return sb.String()
}

func generateStepCode(step workflows.TestStep) string {
	switch step.Action {
	case "navigate":
		return fmt.Sprintf("    await page.goto('%s');", escapeString(step.Target))
	case "click":
		if step.Selector != "" {
			return fmt.Sprintf("    await page.click('%s');", escapeString(step.Selector))
		}
		return fmt.Sprintf("    // Click on %s", step.Target)
	case "fill":
		return fmt.Sprintf("    await page.fill('%s', '%s');", escapeString(step.Selector), escapeString(step.Value))
	case "waitForLoad":
		return "    await page.waitForLoadState('networkidle');"
	case "assertVisible":
		if step.Selector != "" {
			return fmt.Sprintf("    await expect(page.locator('%s')).toBeVisible();", escapeString(step.Selector))
		}
		return fmt.Sprintf("    // Assert %s is visible", step.Target)
	case "assertText":
		return fmt.Sprintf("    await expect(page.locator('%s')).toContainText('%s');", escapeString(step.Selector), escapeString(step.Expected))
	case "screenshot":
		return fmt.Sprintf("    await page.screenshot({ path: 'step-%d.png' });", step.Order)
	default:
		return fmt.Sprintf("    // TODO: Implement %s action", step.Action)
	}
}

func escapeString(s string) string {
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
