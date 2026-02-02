package testdesign

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/testforge/testforge/internal/llm"
	"github.com/testforge/testforge/internal/services/discovery"
	"github.com/testforge/testforge/internal/services/testdesign"
	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements the test design activity
type Activity struct {
	claudeClient *llm.ClaudeClient
	config       testdesign.ArchitectConfig
}

// NewActivity creates a new test design activity
func NewActivity() *Activity {
	return &Activity{
		config: testdesign.DefaultArchitectConfig(),
	}
}

// NewActivityWithClient creates a new test design activity with a Claude client
func NewActivityWithClient(client *llm.ClaudeClient) *Activity {
	return &Activity{
		claudeClient: client,
		config:       testdesign.DefaultArchitectConfig(),
	}
}

// WithConfig sets the architect config
func (a *Activity) WithConfig(config testdesign.ArchitectConfig) *Activity {
	a.config = config
	return a
}

// Execute generates test cases from the app model
func (a *Activity) Execute(ctx context.Context, input workflows.TestDesignInput) (*workflows.TestDesignOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting test design activity",
		"test_run_id", input.TestRunID.String(),
		"max_test_cases", input.MaxTestCases,
	)

	activity.RecordHeartbeat(ctx, "Analyzing app model...")

	// If we have a Claude client, use the real architect
	if a.claudeClient != nil {
		return a.executeWithLLM(ctx, input)
	}

	// Otherwise, try to create a client from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		cfg := llm.DefaultConfig()
		cfg.APIKey = apiKey
		client, err := llm.NewClaudeClient(cfg)
		if err == nil {
			a.claudeClient = client
			return a.executeWithLLM(ctx, input)
		}
		logger.Warn("Failed to create Claude client, using mock",
			"error", err,
		)
	}

	// Fall back to mock implementation
	return a.executeMock(ctx, input, startTime)
}

// executeWithLLM uses the real LLM-powered architect
func (a *Activity) executeWithLLM(ctx context.Context, input workflows.TestDesignInput) (*workflows.TestDesignOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	activity.RecordHeartbeat(ctx, "Converting app model...")

	// Convert workflow AppModel to discovery AppModel
	appModel := convertToDiscoveryAppModel(input.AppModel)

	activity.RecordHeartbeat(ctx, "Initializing test architect...")

	// Create the architect
	architect := testdesign.NewTestArchitect(a.claudeClient, a.config)

	// Prepare design input
	designInput := testdesign.DesignInput{
		AppModel:    appModel,
		ProjectID:   input.TestRunID.String(),
		ProjectName: getProjectName(input.AppModel),
		BaseURL:     getBaseURL(input.AppModel),
		Roles:       []string{"anonymous", "user"},
		Environment: "test",
	}

	activity.RecordHeartbeat(ctx, "Generating test suite with AI...")

	// Generate the test suite
	output, err := architect.DesignTestSuite(ctx, designInput)
	if err != nil {
		return nil, fmt.Errorf("designing test suite: %w", err)
	}

	activity.RecordHeartbeat(ctx, "Validating and converting results...")

	// Convert to workflow format
	testPlan := convertToWorkflowTestPlan(output.Suite, input.MaxTestCases)

	duration := time.Since(startTime)

	result := &workflows.TestDesignOutput{
		TestPlan:        testPlan,
		Duration:        duration,
		TokensUsed:      output.TokensUsed,
		InputTokens:     output.InputTokens,
		OutputTokens:    output.OutputTokens,
		EstimatedCost:   output.EstimatedCost,
		ValidationWarns: output.ValidationWarns,
	}

	logger.Info("Test design activity completed",
		"test_cases_generated", testPlan.TotalCount,
		"features", len(output.Suite.Features),
		"tokens_used", output.TokensUsed,
		"estimated_cost", output.EstimatedCost,
		"duration", duration,
	)

	return result, nil
}

// executeMock provides fallback mock implementation
func (a *Activity) executeMock(ctx context.Context, input workflows.TestDesignInput, startTime time.Time) (*workflows.TestDesignOutput, error) {
	logger := activity.GetLogger(ctx)

	logger.Info("Using mock test design (no Claude API key)")

	time.Sleep(1 * time.Second)
	activity.RecordHeartbeat(ctx, "Generating test cases (mock)...")

	testCases := []workflows.TestCase{}

	// Generate test cases based on business flows
	if input.AppModel != nil {
		for _, flow := range input.AppModel.BusinessFlows {
			tc := workflows.TestCase{
				ID:          uuid.New().String(),
				Name:        fmt.Sprintf("Test %s Flow", flow.Name),
				Description: fmt.Sprintf("Verify the %s business flow works correctly", flow.Name),
				Category:    flowTypeToCategory(flow.Type),
				Priority:    flow.Priority,
				Steps:       generateStepsForFlow(flow),
			}
			testCases = append(testCases, tc)
		}

		// Add smoke tests for key pages
		for i, page := range input.AppModel.Pages {
			if i >= 3 {
				break
			}
			tc := workflows.TestCase{
				ID:          uuid.New().String(),
				Name:        fmt.Sprintf("Smoke Test: %s", page.Title),
				Description: fmt.Sprintf("Verify %s page loads correctly", page.Title),
				Category:    "smoke",
				Priority:    "high",
				Steps: []workflows.TestStep{
					{Order: 1, Action: "navigate", Target: page.URL},
					{Order: 2, Action: "waitForLoad", Expected: "page loaded"},
					{Order: 3, Action: "assertVisible", Target: "body", Expected: "page content visible"},
				},
			}
			testCases = append(testCases, tc)
		}

		// Add form validation tests
		for _, page := range input.AppModel.Pages {
			if page.HasForms {
				tc := workflows.TestCase{
					ID:          uuid.New().String(),
					Name:        fmt.Sprintf("Form Validation: %s", page.Title),
					Description: fmt.Sprintf("Verify form validation on %s", page.Title),
					Category:    "functional",
					Priority:    "medium",
					Steps: []workflows.TestStep{
						{Order: 1, Action: "navigate", Target: page.URL},
						{Order: 2, Action: "click", Target: "submit button", Selector: "button[type=submit]"},
						{Order: 3, Action: "assertVisible", Target: "validation errors", Expected: "validation message displayed"},
					},
				}
				testCases = append(testCases, tc)
			}
		}
	}

	// Apply max test cases limit
	if input.MaxTestCases > 0 && len(testCases) > input.MaxTestCases {
		testCases = testCases[:input.MaxTestCases]
	}

	testPlan := &workflows.TestPlan{
		ID:         uuid.New().String(),
		TestCases:  testCases,
		TotalCount: len(testCases),
	}

	duration := time.Since(startTime)

	output := &workflows.TestDesignOutput{
		TestPlan: testPlan,
		Duration: duration,
	}

	logger.Info("Mock test design completed",
		"test_cases_generated", testPlan.TotalCount,
		"duration", duration,
	)

	return output, nil
}

// convertToDiscoveryAppModel converts workflow AppModel to discovery AppModel
func convertToDiscoveryAppModel(input *workflows.AppModel) *discovery.AppModel {
	if input == nil {
		return &discovery.AppModel{
			ID:    uuid.New().String(),
			Pages: make(map[string]*discovery.PageModel),
			Flows: []discovery.BusinessFlow{},
		}
	}

	appModel := &discovery.AppModel{
		ID:    input.ID,
		Pages: make(map[string]*discovery.PageModel),
		Flows: make([]discovery.BusinessFlow, 0),
		Stats: discovery.DiscoveryStats{
			TotalPages: len(input.Pages),
		},
	}

	// Convert pages
	for _, page := range input.Pages {
		pageModel := &discovery.PageModel{
			URL:      page.URL,
			Title:    page.Title,
			PageType: page.PageType,
			HasAuth:  page.HasAuth,
			Forms:    make([]discovery.FormModel, 0),
			Buttons:  make([]discovery.ButtonModel, 0),
			Links:    make([]discovery.LinkModel, 0),
			Inputs:   make([]discovery.InputModel, 0),
		}

		// Convert forms
		if page.HasForms {
			pageModel.Forms = append(pageModel.Forms, discovery.FormModel{
				Name:     "Form",
				FormType: "general",
			})
		}

		appModel.Pages[page.URL] = pageModel
	}

	// Convert flows
	for _, flow := range input.BusinessFlows {
		businessFlow := discovery.BusinessFlow{
			Name:        flow.Name,
			Type:        flow.Type,
			Description: flow.Name + " business flow",
			Priority:    flow.Priority,
			Steps:       make([]discovery.FlowStep, 0),
		}

		for i, stepURL := range flow.Steps {
			businessFlow.Steps = append(businessFlow.Steps, discovery.FlowStep{
				Order:  i + 1,
				Action: "navigate",
				URL:    stepURL,
			})
		}

		appModel.Flows = append(appModel.Flows, businessFlow)
	}

	return appModel
}

// convertToWorkflowTestPlan converts testdesign TestSuite to workflow TestPlan
func convertToWorkflowTestPlan(suite *testdesign.TestSuite, maxTests int) *workflows.TestPlan {
	testCases := []workflows.TestCase{}

	for _, feature := range suite.Features {
		for _, scenario := range feature.Scenarios {
			for _, tc := range scenario.TestCases {
				if maxTests > 0 && len(testCases) >= maxTests {
					break
				}

				workflowTC := workflows.TestCase{
					ID:          tc.ID,
					Name:        tc.Name,
					Description: tc.Description,
					Category:    string(tc.Category),
					Priority:    string(tc.Priority),
					Steps:       convertSteps(tc.Steps),
				}
				testCases = append(testCases, workflowTC)
			}
		}
	}

	return &workflows.TestPlan{
		ID:         suite.ID,
		TestCases:  testCases,
		TotalCount: len(testCases),
	}
}

// convertSteps converts testdesign TestSteps to workflow TestSteps
func convertSteps(steps []testdesign.TestStep) []workflows.TestStep {
	result := make([]workflows.TestStep, len(steps))
	for i, step := range steps {
		selector := step.Target
		if step.Selectors != nil && step.Selectors.Primary != "" {
			selector = step.Selectors.Primary
		}

		result[i] = workflows.TestStep{
			Order:    step.Order,
			Action:   string(step.Action),
			Target:   step.Target,
			Selector: selector,
			Value:    step.Value,
		}

		// Add expected from assertions
		if len(step.Assertions) > 0 {
			result[i].Expected = step.Assertions[0].Value
		}
	}
	return result
}

// getProjectName extracts project name from app model
func getProjectName(appModel *workflows.AppModel) string {
	if appModel == nil || len(appModel.Pages) == 0 {
		return "Unknown Project"
	}

	// Use the title of the first page
	for _, page := range appModel.Pages {
		if page.Title != "" {
			return page.Title
		}
	}

	return "Test Project"
}

// getBaseURL extracts base URL from app model
func getBaseURL(appModel *workflows.AppModel) string {
	if appModel == nil || len(appModel.Pages) == 0 {
		return ""
	}

	for _, page := range appModel.Pages {
		return page.URL
	}

	return ""
}

func flowTypeToCategory(flowType string) string {
	switch flowType {
	case "auth":
		return "authentication"
	case "checkout":
		return "e2e"
	case "crud":
		return "functional"
	default:
		return "functional"
	}
}

func generateStepsForFlow(flow workflows.FlowInfo) []workflows.TestStep {
	steps := []workflows.TestStep{}
	for i, pageURL := range flow.Steps {
		steps = append(steps, workflows.TestStep{
			Order:  i + 1,
			Action: "navigate",
			Target: pageURL,
		})
		steps = append(steps, workflows.TestStep{
			Order:    i + 2,
			Action:   "waitForLoad",
			Expected: "page loaded",
		})
	}
	return steps
}
