package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

// =============================================================================
// UNIVERSAL AGENT ORCHESTRATOR
// =============================================================================
//
// The Orchestrator is the central coordinator for all AI agents in TestForge.
// It provides a unified interface for:
// - Discovery agents (PageUnderstanding, ElementDiscovery, Auth, Form, BusinessFlow)
// - Analysis agents (ABA - Autonomous Business Analyst)
// - Visual AI integration (V-JEPA 2)
// - Test design integration (TestArchitect)
// - Self-healing integration (RepairAgent)
//
// Key principles:
// 1. Agents are spawned on demand
// 2. Agents communicate via the shared CrawlContext
// 3. All analysis is semantic, not keyword-based
// 4. Visual AI and LLM analysis are combined for accuracy
// =============================================================================

// Orchestrator coordinates all AI agents for comprehensive analysis
type Orchestrator struct {
	// Core components
	llmClient   LLMClient
	visualAI    VisualAIClient
	logger      *zap.Logger
	config      OrchestratorConfig

	// Browser for page analysis
	pw          *playwright.Playwright
	browser     playwright.Browser

	// Agent registry
	agents      map[AgentType]Agent
	agentMu     sync.RWMutex

	// Shared context across all agents
	context     *CrawlContext

	// Results aggregation
	results     *OrchestratorResults
	resultsMu   sync.Mutex

	// Progress reporting
	onProgress  func(phase string, current, total int, message string)
}

// OrchestratorConfig configures the orchestrator
type OrchestratorConfig struct {
	// Crawling settings
	MaxPages        int
	MaxDepth        int
	Timeout         time.Duration
	MaxDuration     time.Duration
	Headless        bool

	// Agent settings
	EnableVisualAI        bool
	EnableSemanticAnalysis bool
	EnableAccessibility   bool
	EnableABA             bool // Autonomous Business Analyst

	// Domain hints for better analysis
	DomainHints     []string
	CustomContext   string

	// Parallelism
	MaxConcurrentAgents int
}

// DefaultOrchestratorConfig returns sensible defaults
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		MaxPages:              20,
		MaxDepth:              3,
		Timeout:               30 * time.Second,
		MaxDuration:           10 * time.Minute,
		Headless:              true,
		EnableVisualAI:        true,
		EnableSemanticAnalysis: true,
		EnableAccessibility:   true,
		EnableABA:             true,
		MaxConcurrentAgents:   5,
	}
}

// OrchestratorResults contains aggregated results from all agents
type OrchestratorResults struct {
	// Discovery results
	AppModel        *AIAppModel           `json:"app_model"`

	// Business analysis (from ABA)
	BusinessAnalysis *ABAOutput           `json:"business_analysis,omitempty"`

	// Convenience accessors (populated from BusinessAnalysis)
	Requirements    []BusinessRequirement `json:"requirements,omitempty"`
	UserStories     []UserStory           `json:"user_stories,omitempty"`
	SemanticMap     *DomainAnalysis       `json:"semantic_map,omitempty"`

	// Aggregated statistics
	Stats           OrchestratorStats     `json:"stats"`

	// Timeline of agent activities
	AgentTimeline   []AgentActivity       `json:"agent_timeline"`

	// Any errors or warnings
	Errors          []string              `json:"errors,omitempty"`
	Warnings        []string              `json:"warnings,omitempty"`
}

// OrchestratorStats contains statistics from orchestration
type OrchestratorStats struct {
	TotalDuration     time.Duration `json:"total_duration"`
	PagesAnalyzed     int           `json:"pages_analyzed"`
	ElementsAnalyzed  int           `json:"elements_analyzed"`
	AgentsSpawned     int           `json:"agents_spawned"`
	LLMCallsMade      int           `json:"llm_calls_made"`
	VisualAICallsMade int           `json:"visual_ai_calls_made"`
	TokensUsed        int           `json:"tokens_used"`
	UserStoriesGen    int           `json:"user_stories_generated"`
	RequirementsGen   int           `json:"requirements_generated"`
	TestScenariosGen  int           `json:"test_scenarios_generated"`
}

// AgentActivity records an agent's execution
type AgentActivity struct {
	AgentType AgentType     `json:"agent_type"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Success   bool          `json:"success"`
	URL       string        `json:"url,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// NewOrchestrator creates a new agent orchestrator
func NewOrchestrator(llmClient LLMClient, config OrchestratorConfig, logger *zap.Logger) (*Orchestrator, error) {
	// Initialize Playwright
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("starting playwright: %w", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(config.Headless),
	})
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("launching browser: %w", err)
	}

	o := &Orchestrator{
		llmClient: llmClient,
		logger:    logger,
		config:    config,
		pw:        pw,
		browser:   browser,
		agents:    make(map[AgentType]Agent),
		context: &CrawlContext{
			DomainHints:      config.DomainHints,
			VisitedPages:     make([]string, 0),
			DetectedPatterns: make([]string, 0),
		},
		results: &OrchestratorResults{
			AgentTimeline: make([]AgentActivity, 0),
		},
	}

	// Register all agents
	o.registerAllAgents()

	return o, nil
}

// registerAllAgents initializes all available agents
func (o *Orchestrator) registerAllAgents() {
	o.agentMu.Lock()
	defer o.agentMu.Unlock()

	// Discovery agents
	o.agents[AgentPageUnderstanding] = &PageUnderstandingAgent{llm: o.llmClient, logger: o.logger}
	o.agents[AgentElementDiscovery] = &ElementDiscoveryAgent{llm: o.llmClient, logger: o.logger}
	o.agents[AgentAuthentication] = &AuthenticationAgent{llm: o.llmClient, logger: o.logger}
	o.agents[AgentFormAnalysis] = &FormAnalysisAgent{llm: o.llmClient, logger: o.logger}
	o.agents[AgentBusinessFlow] = &BusinessFlowAgent{llm: o.llmClient, logger: o.logger}

	// Analysis agents
	if o.config.EnableABA {
		o.agents[AgentABA] = &AutonomousBusinessAnalyst{llm: o.llmClient, logger: o.logger}
	}

	o.logger.Info("registered agents", zap.Int("count", len(o.agents)))
}

// SetVisualAI sets the visual AI client
func (o *Orchestrator) SetVisualAI(client VisualAIClient) {
	o.visualAI = client
}

// SetProgressCallback sets the progress callback
func (o *Orchestrator) SetProgressCallback(fn func(phase string, current, total int, message string)) {
	o.onProgress = fn
}

// Run executes the full orchestration pipeline
func (o *Orchestrator) Run(ctx context.Context, startURL string) (*OrchestratorResults, error) {
	startTime := time.Now()
	o.context.BaseURL = startURL

	// Create timeout context
	runCtx, cancel := context.WithTimeout(ctx, o.config.MaxDuration)
	defer cancel()

	o.reportProgress("discovery", 0, 100, "Starting discovery...")

	// Phase 1: Discovery - Crawl and analyze all pages
	appModel, err := o.runDiscoveryPhase(runCtx, startURL)
	if err != nil {
		o.results.Errors = append(o.results.Errors, fmt.Sprintf("Discovery phase failed: %v", err))
		// Continue with partial results if any
		if appModel == nil {
			return o.results, err
		}
	}
	o.results.AppModel = appModel

	o.reportProgress("analysis", 50, 100, "Running business analysis...")

	// Phase 2: Business Analysis - Run ABA agent
	if o.config.EnableABA && len(appModel.Pages) > 0 {
		abaOutput, err := o.runBusinessAnalysisPhase(runCtx, appModel)
		if err != nil {
			o.results.Warnings = append(o.results.Warnings, fmt.Sprintf("Business analysis incomplete: %v", err))
		} else {
			o.results.BusinessAnalysis = abaOutput
			// Populate convenience accessors
			o.results.Requirements = abaOutput.Requirements
			o.results.UserStories = abaOutput.UserStories
			o.results.SemanticMap = abaOutput.DomainAnalysis
			o.results.Stats.UserStoriesGen = len(abaOutput.UserStories)
			o.results.Stats.RequirementsGen = len(abaOutput.Requirements)
		}
	}

	o.reportProgress("complete", 100, 100, "Analysis complete")

	// Finalize stats
	o.results.Stats.TotalDuration = time.Since(startTime)
	o.results.Stats.PagesAnalyzed = len(appModel.Pages)

	return o.results, nil
}

// runDiscoveryPhase performs the discovery crawl
func (o *Orchestrator) runDiscoveryPhase(ctx context.Context, startURL string) (*AIAppModel, error) {
	// Create AI crawler with our configuration
	crawlerConfig := AICrawlerConfig{
		MaxPages:              o.config.MaxPages,
		MaxDepth:              o.config.MaxDepth,
		Timeout:               o.config.Timeout,
		MaxDuration:           o.config.MaxDuration,
		Headless:              o.config.Headless,
		EnableVisualAI:        o.config.EnableVisualAI,
		EnableAccessibility:   o.config.EnableAccessibility,
		EnableSemanticAnalysis: o.config.EnableSemanticAnalysis,
		DomainHints:           o.config.DomainHints,
		CustomPromptContext:   o.config.CustomContext,
	}

	crawler, err := NewAICrawler(o.llmClient, crawlerConfig, o.logger)
	if err != nil {
		return nil, fmt.Errorf("creating AI crawler: %w", err)
	}
	defer crawler.Close()

	// Set visual AI if available
	if o.visualAI != nil {
		crawler.SetVisualAI(o.visualAI)
	}

	// Set progress callback
	crawler.SetProgressCallback(func(current, total int, message string) {
		o.reportProgress("discovery", current*50/total, 100, message)
	})

	// Run the crawl
	return crawler.Crawl(ctx, startURL)
}

// runBusinessAnalysisPhase runs the ABA agent for business analysis
func (o *Orchestrator) runBusinessAnalysisPhase(ctx context.Context, appModel *AIAppModel) (*ABAOutput, error) {
	o.agentMu.RLock()
	abaAgent, ok := o.agents[AgentABA]
	o.agentMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("ABA agent not registered")
	}

	// Use the first page as primary analysis target
	// In a full implementation, we'd aggregate across all pages
	var primaryPage *AIPageAnalysis
	var primaryHTML string
	for _, page := range appModel.Pages {
		primaryPage = page
		// We don't have the HTML stored, so we'll use a summary
		primaryHTML = fmt.Sprintf("<html><head><title>%s</title></head><body><!-- Page analysis summary: Type=%s, Purpose=%s, Elements=%d, Inputs=%d, Navigation=%d --></body></html>",
			page.Title, page.PageType, page.Purpose,
			len(page.SemanticElements), len(page.DataInputs), len(page.Navigation))
		break
	}

	if primaryPage == nil {
		return nil, fmt.Errorf("no pages to analyze")
	}

	// Record activity
	activity := AgentActivity{
		AgentType: AgentABA,
		StartTime: time.Now(),
		URL:       primaryPage.URL,
	}

	input := AgentInput{
		URL:     primaryPage.URL,
		Title:   primaryPage.Title,
		HTML:    primaryHTML,
		Context: o.context,
	}

	result, err := abaAgent.Analyze(ctx, input)

	activity.EndTime = time.Now()
	activity.Duration = activity.EndTime.Sub(activity.StartTime)
	activity.Success = err == nil
	if err != nil {
		activity.Error = err.Error()
	}

	o.resultsMu.Lock()
	o.results.AgentTimeline = append(o.results.AgentTimeline, activity)
	o.results.Stats.AgentsSpawned++
	o.resultsMu.Unlock()

	if err != nil {
		return nil, err
	}

	// Extract ABA output from result
	if abaOutput, ok := result.Data["aba_output"].(*ABAOutput); ok {
		return abaOutput, nil
	}

	return nil, fmt.Errorf("invalid ABA output format")
}

// SpawnAgent spawns a specific agent for ad-hoc analysis
func (o *Orchestrator) SpawnAgent(agentType AgentType) (Agent, error) {
	o.agentMu.RLock()
	agent, ok := o.agents[agentType]
	o.agentMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent type %s not registered", agentType)
	}

	o.resultsMu.Lock()
	o.results.Stats.AgentsSpawned++
	o.resultsMu.Unlock()

	return agent, nil
}

// AnalyzePage runs all relevant agents on a single page
func (o *Orchestrator) AnalyzePage(ctx context.Context, url string) (*AIPageAnalysis, error) {
	// Create browser context
	browserCtx, err := o.browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{Width: 1920, Height: 1080},
	})
	if err != nil {
		return nil, fmt.Errorf("creating browser context: %w", err)
	}
	defer browserCtx.Close()

	page, err := browserCtx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("creating page: %w", err)
	}
	defer page.Close()

	// Navigate
	_, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(float64(o.config.Timeout.Milliseconds())),
	})
	if err != nil {
		return nil, fmt.Errorf("navigating: %w", err)
	}

	// Wait for JS
	page.WaitForTimeout(2000)

	// Get page content
	title, _ := page.Title()
	html, _ := page.Content()

	// Build agent input
	input := AgentInput{
		URL:     url,
		Title:   title,
		HTML:    html,
		Context: o.context,
	}

	// Run agents in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[AgentType]AgentOutput)

	agentsToRun := []AgentType{
		AgentPageUnderstanding,
		AgentElementDiscovery,
		AgentFormAnalysis,
		AgentAuthentication,
	}

	for _, agentType := range agentsToRun {
		wg.Add(1)
		go func(at AgentType) {
			defer wg.Done()

			o.agentMu.RLock()
			agent, ok := o.agents[at]
			o.agentMu.RUnlock()

			if !ok {
				return
			}

			if result, err := agent.Analyze(ctx, input); err == nil {
				mu.Lock()
				results[at] = result
				mu.Unlock()
			}
		}(agentType)
	}

	wg.Wait()

	// Build analysis from results
	analysis := &AIPageAnalysis{
		URL:        url,
		Title:      title,
		DOMHash:    HashDOM(html),
		AnalyzedAt: time.Now(),
	}

	// Merge results (simplified)
	if pageResult, ok := results[AgentPageUnderstanding]; ok && pageResult.Success {
		if pt, ok := pageResult.Data["page_type"].(string); ok {
			analysis.PageType = pt
		}
		if purpose, ok := pageResult.Data["purpose"].(string); ok {
			analysis.Purpose = purpose
		}
	}

	if elemResult, ok := results[AgentElementDiscovery]; ok && elemResult.Success {
		analysis.SemanticElements = elemResult.Elements
	}

	return analysis, nil
}

// GetRegisteredAgents returns a list of registered agent types
func (o *Orchestrator) GetRegisteredAgents() []AgentType {
	o.agentMu.RLock()
	defer o.agentMu.RUnlock()

	types := make([]AgentType, 0, len(o.agents))
	for t := range o.agents {
		types = append(types, t)
	}
	return types
}

// GetContext returns the shared crawl context
func (o *Orchestrator) GetContext() *CrawlContext {
	return o.context
}

// Close cleans up orchestrator resources
func (o *Orchestrator) Close() error {
	if o.browser != nil {
		o.browser.Close()
	}
	if o.pw != nil {
		o.pw.Stop()
	}
	return nil
}

// reportProgress reports progress if callback is set
func (o *Orchestrator) reportProgress(phase string, current, total int, message string) {
	if o.onProgress != nil {
		o.onProgress(phase, current, total, message)
	}
}

// =============================================================================
// CONVENIENCE FUNCTIONS
// =============================================================================

// QuickAnalyze performs a quick single-page analysis
func QuickAnalyze(ctx context.Context, url string, llmClient LLMClient, logger *zap.Logger) (*AIPageAnalysis, error) {
	config := DefaultOrchestratorConfig()
	config.MaxPages = 1
	config.MaxDuration = 2 * time.Minute
	config.EnableABA = false

	orch, err := NewOrchestrator(llmClient, config, logger)
	if err != nil {
		return nil, err
	}
	defer orch.Close()

	return orch.AnalyzePage(ctx, url)
}

// FullAnalysis performs complete crawl + business analysis
func FullAnalysis(ctx context.Context, url string, llmClient LLMClient, logger *zap.Logger) (*OrchestratorResults, error) {
	config := DefaultOrchestratorConfig()

	orch, err := NewOrchestrator(llmClient, config, logger)
	if err != nil {
		return nil, err
	}
	defer orch.Close()

	return orch.Run(ctx, url)
}
