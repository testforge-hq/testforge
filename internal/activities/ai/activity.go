package ai

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/llm"
	"github.com/testforge/testforge/internal/services/discovery"
	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements AI-powered activities for multi-agent analysis
type Activity struct {
	llmClient *llm.ClaudeClient
	logger    *zap.Logger
	useMock   bool
}

// Config contains activity configuration
type Config struct {
	AnthropicAPIKey string
	Model           string
	UseMock         bool
}

// NewActivity creates a new AI activity with default configuration
func NewActivity() *Activity {
	return &Activity{
		useMock: true, // Default to mock for safety
	}
}

// NewActivityWithConfig creates a new AI activity with configuration
func NewActivityWithConfig(cfg Config, logger *zap.Logger) (*Activity, error) {
	a := &Activity{
		logger:  logger,
		useMock: cfg.UseMock,
	}

	// Initialize Claude client if API key is provided
	if cfg.AnthropicAPIKey != "" {
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}

		llmCfg := llm.Config{
			APIKey:       cfg.AnthropicAPIKey,
			Model:        model,
			MaxTokens:    8192,
			RateLimitRPM: 50,
		}

		client, err := llm.NewClaudeClient(llmCfg)
		if err != nil {
			return nil, fmt.Errorf("creating claude client: %w", err)
		}
		a.llmClient = client
		a.useMock = false
	}

	return a, nil
}

// ExecuteAIDiscovery performs AI-powered website discovery with multi-agent analysis
func (a *Activity) ExecuteAIDiscovery(ctx context.Context, input workflows.AIDiscoveryInput) (*workflows.AIDiscoveryOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting AI discovery activity",
		"test_run_id", input.TestRunID.String(),
		"target_url", input.TargetURL,
		"enable_aba", input.EnableABA,
		"enable_semantic_ai", input.EnableSemanticAI,
	)

	activity.RecordHeartbeat(ctx, "Starting AI-powered discovery...")

	// If mock mode or no LLM client, return mock data
	if a.useMock || a.llmClient == nil {
		return a.executeMockAIDiscovery(ctx, input, startTime)
	}

	// Real AI-powered discovery using orchestrator
	return a.executeRealAIDiscovery(ctx, input, startTime)
}

// executeRealAIDiscovery performs actual AI-powered discovery
func (a *Activity) executeRealAIDiscovery(ctx context.Context, input workflows.AIDiscoveryInput, startTime time.Time) (*workflows.AIDiscoveryOutput, error) {
	// Configure the orchestrator
	cfg := discovery.DefaultOrchestratorConfig()
	cfg.MaxPages = input.MaxPages
	cfg.MaxDepth = input.MaxDepth
	cfg.EnableABA = input.EnableABA
	cfg.EnableSemanticAnalysis = input.EnableSemanticAI
	cfg.EnableVisualAI = input.EnableVisualAI
	cfg.Headless = input.Headless

	// Create orchestrator
	orchestrator, err := discovery.NewOrchestrator(a.llmClient, cfg, a.logger)
	if err != nil {
		return nil, fmt.Errorf("creating orchestrator: %w", err)
	}
	defer orchestrator.Close()

	// Set up progress callback for heartbeats
	orchestrator.SetProgressCallback(func(phase string, current, total int, message string) {
		activity.RecordHeartbeat(ctx, fmt.Sprintf("[%s] %d/%d: %s", phase, current, total, message))
	})

	// Run orchestrator
	results, err := orchestrator.Run(ctx, input.TargetURL)
	if err != nil {
		return nil, fmt.Errorf("orchestrator failed: %w", err)
	}

	// Convert results to workflow output
	return a.convertOrchestratorResults(results, startTime), nil
}

// convertOrchestratorResults converts orchestrator results to workflow output
func (a *Activity) convertOrchestratorResults(results *discovery.OrchestratorResults, startTime time.Time) *workflows.AIDiscoveryOutput {
	output := &workflows.AIDiscoveryOutput{
		Duration: time.Since(startTime),
	}

	// Convert app model
	if results.AppModel != nil {
		output.AppModel = convertAppModel(results.AppModel)
		output.PagesFound = len(results.AppModel.Pages)
		output.ElementsFound = results.Stats.ElementsAnalyzed
		output.FlowsDetected = len(results.AppModel.BusinessFlows)
	}

	// Convert requirements (from convenience accessor on results)
	if results.Requirements != nil {
		for _, req := range results.Requirements {
			output.Requirements = append(output.Requirements, workflows.Requirement{
				ID:          req.ID,
				Title:       req.Title,
				Description: req.Description,
				Type:        req.Category, // Category maps to Type
				Priority:    req.Priority,
				Source:      req.Source,
				Tags:        []string{}, // No direct mapping, leave empty
			})
		}
	}

	// Convert user stories (from convenience accessor on results)
	if results.UserStories != nil {
		for _, story := range results.UserStories {
			ws := workflows.UserStory{
				ID:             story.ID,
				Title:          story.Title,
				AsA:            story.AsA,
				IWant:          story.IWant,
				SoThat:         story.SoThat,
				Priority:       story.Priority,
				StoryPoints:    story.StoryPoints,
				RelatedPages:   story.RelatedPages,
				TestScenarios:  story.TestScenarios,
			}

			for _, ac := range story.AcceptanceCriteria {
				ws.AcceptanceCriteria = append(ws.AcceptanceCriteria, workflows.AcceptanceCriterion{
					Given: ac.Given,
					When:  ac.When,
					Then:  ac.Then,
				})
			}

			output.UserStories = append(output.UserStories, ws)
		}
	}

	// Convert semantic map from DomainAnalysis
	if results.SemanticMap != nil {
		var userPersonas []string
		for _, role := range results.SemanticMap.UserRoles {
			userPersonas = append(userPersonas, role.Name)
		}

		output.SemanticMap = &workflows.SemanticMap{
			Domain:       results.SemanticMap.Domain,
			Purpose:      results.SemanticMap.SubDomain,
			UserPersonas: userPersonas,
			CoreFeatures: []string{}, // Would need to extract from CoreWorkflows
		}
	}

	output.TokensUsed = results.Stats.TokensUsed
	return output
}

// convertAppModel converts discovery AIAppModel to workflow AppModel
func convertAppModel(model *discovery.AIAppModel) *workflows.AppModel {
	wfModel := &workflows.AppModel{
		ID: model.ID,
	}

	// Convert pages (Pages is a slice of *AIPageAnalysis)
	for _, page := range model.Pages {
		hasAuth := page.PageType == "auth" || page.PageType == "login"
		hasForms := len(page.DataInputs) > 0

		wfModel.Pages = append(wfModel.Pages, workflows.PageInfo{
			URL:        page.URL,
			Title:      page.Title,
			PageType:   page.PageType,
			HasForms:   hasForms,
			HasAuth:    hasAuth,
			Screenshot: page.ScreenshotBase64,
		})
	}

	// Convert components
	for _, comp := range model.Components {
		wfModel.Components = append(wfModel.Components, workflows.ComponentInfo{
			ID:       comp.ID,
			Type:     comp.Type,
			Selector: comp.Selectors.BestSelector(),
			Pages:    comp.FoundOnPages,
		})
	}

	// Convert business flows
	for _, flow := range model.BusinessFlows {
		var steps []string
		for _, step := range flow.Steps {
			if step.URL != "" {
				steps = append(steps, step.URL)
			} else {
				steps = append(steps, fmt.Sprintf("%s: %s", step.Action, step.Selector))
			}
		}
		wfModel.BusinessFlows = append(wfModel.BusinessFlows, workflows.FlowInfo{
			ID:       flow.ID,
			Name:     flow.Name,
			Type:     flow.Type,
			Steps:    steps,
			Priority: flow.Priority,
		})
	}

	return wfModel
}

// executeMockAIDiscovery returns mock data for testing
func (a *Activity) executeMockAIDiscovery(ctx context.Context, input workflows.AIDiscoveryInput, startTime time.Time) (*workflows.AIDiscoveryOutput, error) {
	logger := activity.GetLogger(ctx)

	// Simulate work with heartbeats
	time.Sleep(1 * time.Second)
	activity.RecordHeartbeat(ctx, "Analyzing page structure...")
	time.Sleep(1 * time.Second)
	activity.RecordHeartbeat(ctx, "Running semantic analysis...")
	time.Sleep(1 * time.Second)
	activity.RecordHeartbeat(ctx, "Generating requirements...")

	// Mock app model
	appModel := &workflows.AppModel{
		ID: "mock-app-model",
		Pages: []workflows.PageInfo{
			{URL: input.TargetURL, Title: "Home", PageType: "landing", HasForms: false},
			{URL: input.TargetURL + "/login", Title: "Login", PageType: "auth", HasForms: true, HasAuth: true},
			{URL: input.TargetURL + "/dashboard", Title: "Dashboard", PageType: "dashboard", HasForms: false},
		},
		Components: []workflows.ComponentInfo{
			{ID: "nav-1", Type: "navbar", Selector: "nav", Pages: []string{"/", "/login", "/dashboard"}},
			{ID: "login-form", Type: "form", Selector: "form.login", Pages: []string{"/login"}},
		},
		BusinessFlows: []workflows.FlowInfo{
			{ID: "auth-flow", Name: "User Authentication", Type: "auth", Steps: []string{"/", "/login", "/dashboard"}, Priority: "critical"},
		},
		TechStack: []string{"React", "TypeScript"},
	}

	// Mock requirements (if ABA enabled)
	var requirements []workflows.Requirement
	var userStories []workflows.UserStory

	if input.EnableABA {
		requirements = []workflows.Requirement{
			{
				ID:          "REQ-001",
				Title:       "User Authentication",
				Description: "Users must be able to authenticate using valid credentials",
				Type:        "functional",
				Priority:    "critical",
				Source:      "/login",
				Tags:        []string{"security", "authentication"},
			},
			{
				ID:          "REQ-002",
				Title:       "Session Management",
				Description: "System must maintain user sessions securely after login",
				Type:        "security",
				Priority:    "high",
				Source:      "/dashboard",
				Tags:        []string{"security", "session"},
			},
		}

		userStories = []workflows.UserStory{
			{
				ID:       "US-001",
				Title:    "User Login",
				AsA:      "registered user",
				IWant:    "to log in with my credentials",
				SoThat:   "I can access my personalized dashboard",
				Priority: "critical",
				StoryPoints: 3,
				RelatedPages: []string{"/login", "/dashboard"},
				TestScenarios: []string{"valid login", "invalid password", "empty fields"},
				AcceptanceCriteria: []workflows.AcceptanceCriterion{
					{Given: "I am on the login page", When: "I enter valid credentials and submit", Then: "I am redirected to the dashboard"},
					{Given: "I am on the login page", When: "I enter invalid credentials", Then: "I see an error message"},
				},
			},
		}
	}

	// Mock semantic map
	var semanticMap *workflows.SemanticMap
	if input.EnableSemanticAI {
		semanticMap = &workflows.SemanticMap{
			Domain:       "web-application",
			Purpose:      "User management and dashboard access",
			UserPersonas: []string{"registered user", "admin"},
			CoreFeatures: []string{"authentication", "dashboard viewing"},
		}
	}

	output := &workflows.AIDiscoveryOutput{
		AppModel:      appModel,
		Requirements:  requirements,
		UserStories:   userStories,
		SemanticMap:   semanticMap,
		PagesFound:    len(appModel.Pages),
		ElementsFound: 15,
		FlowsDetected: len(appModel.BusinessFlows),
		Duration:      time.Since(startTime),
		TokensUsed:    0,
	}

	logger.Info("AI discovery activity completed (mock)",
		"pages_found", output.PagesFound,
		"requirements", len(output.Requirements),
		"user_stories", len(output.UserStories),
	)

	return output, nil
}

// ExecutePageAnalysis performs semantic analysis on a single page
func (a *Activity) ExecutePageAnalysis(ctx context.Context, input workflows.PageAnalysisInput) (*workflows.PageAnalysisOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting page analysis activity",
		"test_run_id", input.TestRunID.String(),
		"page_url", input.PageURL,
		"analysis_type", input.AnalysisType,
	)

	activity.RecordHeartbeat(ctx, "Analyzing page...")

	// If mock mode, return mock data
	if a.useMock || a.llmClient == nil {
		return a.executeMockPageAnalysis(ctx, input, startTime)
	}

	// Use the PageUnderstanding agent
	pageAgent := discovery.NewPageUnderstandingAgent(a.llmClient, a.logger)

	agentInput := discovery.AgentInput{
		HTML: input.HTML,
		URL:  input.PageURL,
	}

	pageResult, err := pageAgent.Analyze(ctx, agentInput)
	if err != nil {
		return nil, fmt.Errorf("page analysis failed: %w", err)
	}

	// Extract page type and purpose from result.Data
	pageType := ""
	purpose := ""
	if pt, ok := pageResult.Data["page_type"].(string); ok {
		pageType = pt
	}
	if p, ok := pageResult.Data["purpose"].(string); ok {
		purpose = p
	}

	output := &workflows.PageAnalysisOutput{
		PageType:   pageType,
		Purpose:    purpose,
		Duration:   time.Since(startTime),
		TokensUsed: int(pageResult.TokensUsed),
	}

	// Run element discovery agent for elements
	elemAgent := discovery.NewElementDiscoveryAgent(a.llmClient, a.logger)
	elemResult, err := elemAgent.Analyze(ctx, agentInput)
	if err == nil && elemResult.Success {
		for _, elem := range elemResult.Elements {
			output.Elements = append(output.Elements, workflows.ElementAnalysis{
				Selector:    elem.Selector,
				Type:        elem.Type,
				Purpose:     elem.Purpose,
				Label:       elem.Label,
				IsRequired:  false, // Would need form analysis to determine
				TestActions: []string{"click"}, // Default action
			})
		}
	}

	return output, nil
}

// executeMockPageAnalysis returns mock page analysis data
func (a *Activity) executeMockPageAnalysis(ctx context.Context, input workflows.PageAnalysisInput, startTime time.Time) (*workflows.PageAnalysisOutput, error) {
	time.Sleep(500 * time.Millisecond)

	return &workflows.PageAnalysisOutput{
		PageType: "form",
		Purpose:  "User authentication page",
		Elements: []workflows.ElementAnalysis{
			{Selector: "input[type='email']", Type: "input", Purpose: "email entry", Label: "Email", IsRequired: true},
			{Selector: "input[type='password']", Type: "input", Purpose: "password entry", Label: "Password", IsRequired: true},
			{Selector: "button[type='submit']", Type: "button", Purpose: "form submission", Label: "Login"},
		},
		Forms: []workflows.FormAnalysis{
			{
				Selector:     "form.login-form",
				Purpose:      "User login authentication",
				SubmitButton: "button[type='submit']",
			},
		},
		Duration:   time.Since(startTime),
		TokensUsed: 0,
	}, nil
}

// ExecuteABA runs the Autonomous Business Analyst to generate requirements and user stories
func (a *Activity) ExecuteABA(ctx context.Context, input workflows.ABAInput) (*workflows.ABAOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting ABA activity",
		"test_run_id", input.TestRunID.String(),
	)

	activity.RecordHeartbeat(ctx, "Analyzing application for business requirements...")

	// If mock mode, return mock data
	if a.useMock || a.llmClient == nil {
		return a.executeMockABA(ctx, input, startTime)
	}

	// Create ABA agent
	abaAgent := discovery.NewAutonomousBusinessAnalyst(a.llmClient, a.logger)

	// Build HTML summary from app model for ABA analysis
	htmlSummary := buildHTMLSummary(input.AppModel)

	// Convert workflow AppModel to discovery format for agent input
	agentInput := discovery.AgentInput{
		URL:   getFirstPageURL(input.AppModel),
		Title: "Application Analysis",
		HTML:  htmlSummary,
	}

	result, err := abaAgent.Analyze(ctx, agentInput)
	if err != nil {
		return nil, fmt.Errorf("ABA analysis failed: %w", err)
	}

	// Extract ABA output from result.Data
	abaOutput, ok := result.Data["aba_output"].(*discovery.ABAOutput)
	if !ok {
		return nil, fmt.Errorf("invalid ABA output format")
	}

	// Convert to workflow output
	output := &workflows.ABAOutput{
		Duration:   time.Since(startTime),
		TokensUsed: int(result.TokensUsed),
	}

	// Convert requirements
	for _, req := range abaOutput.Requirements {
		output.Requirements = append(output.Requirements, workflows.Requirement{
			ID:          req.ID,
			Title:       req.Title,
			Description: req.Description,
			Type:        req.Category,
			Priority:    req.Priority,
			Source:      req.Source,
			Tags:        []string{},
		})
	}

	// Convert user stories
	for _, story := range abaOutput.UserStories {
		ws := workflows.UserStory{
			ID:            story.ID,
			Title:         story.Title,
			AsA:           story.AsA,
			IWant:         story.IWant,
			SoThat:        story.SoThat,
			Priority:      story.Priority,
			StoryPoints:   story.StoryPoints,
			RelatedPages:  story.RelatedPages,
			TestScenarios: story.TestScenarios,
		}
		for _, ac := range story.AcceptanceCriteria {
			ws.AcceptanceCriteria = append(ws.AcceptanceCriteria, workflows.AcceptanceCriterion{
				Given: ac.Given,
				When:  ac.When,
				Then:  ac.Then,
			})
		}
		output.UserStories = append(output.UserStories, ws)
	}

	logger.Info("ABA activity completed",
		"requirements", len(output.Requirements),
		"user_stories", len(output.UserStories),
		"duration", output.Duration,
	)

	return output, nil
}

// buildHTMLSummary creates an HTML summary from the app model for ABA analysis
func buildHTMLSummary(appModel *workflows.AppModel) string {
	if appModel == nil {
		return "<html><body>No app model available</body></html>"
	}

	html := "<html><head><title>Application Model Summary</title></head><body>"
	html += "<h1>Pages</h1><ul>"
	for _, page := range appModel.Pages {
		html += fmt.Sprintf("<li>%s - %s (Type: %s, HasForms: %t)</li>",
			page.URL, page.Title, page.PageType, page.HasForms)
	}
	html += "</ul>"

	html += "<h1>Business Flows</h1><ul>"
	for _, flow := range appModel.BusinessFlows {
		html += fmt.Sprintf("<li>%s - %s (Priority: %s)</li>",
			flow.Name, flow.Type, flow.Priority)
	}
	html += "</ul>"

	html += "</body></html>"
	return html
}

// executeMockABA returns mock ABA output
func (a *Activity) executeMockABA(ctx context.Context, input workflows.ABAInput, startTime time.Time) (*workflows.ABAOutput, error) {
	time.Sleep(1 * time.Second)
	activity.RecordHeartbeat(ctx, "Generating requirements...")
	time.Sleep(1 * time.Second)
	activity.RecordHeartbeat(ctx, "Creating user stories...")

	return &workflows.ABAOutput{
		Requirements: []workflows.Requirement{
			{
				ID:          "REQ-001",
				Title:       "Core Functionality",
				Description: "Application must provide core business functionality as observed",
				Type:        "functional",
				Priority:    "high",
				Tags:        []string{"core"},
			},
		},
		UserStories: []workflows.UserStory{
			{
				ID:          "US-001",
				Title:       "Primary User Flow",
				AsA:         "user",
				IWant:       "to complete the primary workflow",
				SoThat:      "I can achieve my goal",
				Priority:    "high",
				StoryPoints: 5,
				AcceptanceCriteria: []workflows.AcceptanceCriterion{
					{Given: "I am a user", When: "I navigate the application", Then: "I can complete my task"},
				},
			},
		},
		Duration:   time.Since(startTime),
		TokensUsed: 0,
	}, nil
}

// Helper functions

func getFirstScreenshot(screenshots []string) string {
	if len(screenshots) > 0 {
		return screenshots[0]
	}
	return ""
}

func getFirstPageURL(appModel *workflows.AppModel) string {
	if appModel != nil && len(appModel.Pages) > 0 {
		return appModel.Pages[0].URL
	}
	return ""
}
