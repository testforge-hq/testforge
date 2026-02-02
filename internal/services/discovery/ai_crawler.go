package discovery

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/llm"
)

// =============================================================================
// UNIVERSAL AI CRAWLER - Multi-Agent Architecture
// =============================================================================
//
// This crawler is designed to be truly universal by:
// 1. Using semantic understanding instead of keyword matching
// 2. Spawning specialized agents on demand for different analysis tasks
// 3. Integrating visual AI for UI understanding
// 4. Using meta-prompts that adapt to any site/language/framework
//
// Agent Types:
// - PageUnderstandingAgent: Determines page purpose semantically
// - ElementDiscoveryAgent: Finds all interactive elements regardless of naming
// - FormAnalysisAgent: Understands forms without relying on labels
// - NavigationAgent: Maps site structure
// - AuthenticationAgent: Detects any auth mechanism (login, signin, SSO, OAuth, etc.)
// - BusinessFlowAgent: Infers user journeys from page relationships
// =============================================================================

// =============================================================================
// Agent System - Spawnable specialized agents
// =============================================================================

// AgentType defines the type of specialized agent
type AgentType string

const (
	AgentPageUnderstanding AgentType = "page_understanding"
	AgentElementDiscovery  AgentType = "element_discovery"
	AgentFormAnalysis      AgentType = "form_analysis"
	AgentNavigation        AgentType = "navigation"
	AgentAuthentication    AgentType = "authentication"
	AgentBusinessFlow      AgentType = "business_flow"
	AgentAccessibility     AgentType = "accessibility"
	AgentVisualAnalysis    AgentType = "visual_analysis"
)

// Agent interface for all specialized agents
type Agent interface {
	Type() AgentType
	Analyze(ctx context.Context, input AgentInput) (AgentOutput, error)
}

// AgentInput contains all possible inputs an agent might need
type AgentInput struct {
	URL        string
	Title      string
	HTML       string
	Screenshot []byte
	PageDOM    string // Simplified DOM structure
	Context    *CrawlContext
}

// AgentOutput contains the agent's analysis results
type AgentOutput struct {
	AgentType  AgentType              `json:"agent_type"`
	Success    bool                   `json:"success"`
	Data       map[string]interface{} `json:"data"`
	Elements   []SemanticElement      `json:"elements,omitempty"`
	Flows      []DetectedFlow         `json:"flows,omitempty"`
	Error      string                 `json:"error,omitempty"`
	TokensUsed int64                  `json:"tokens_used,omitempty"`
}

// DetectedFlow represents a flow detected by AI (before conversion to FlowStep)
type DetectedFlow struct {
	Name        string   `json:"name"`
	Purpose     string   `json:"purpose"`
	Type        string   `json:"type"`
	Priority    string   `json:"priority"`
	Confidence  float64  `json:"confidence"`
	StepActions []string `json:"step_actions"` // Human-readable steps
}

// CrawlContext provides context to agents about the overall crawl
type CrawlContext struct {
	BaseURL         string
	VisitedPages    []string
	DetectedPatterns []string // Patterns already detected (auth, checkout, etc.)
	DomainHints     []string
	Language        string // Detected site language
	Framework       string // Detected JS framework
}

// LLMClient interface for LLM operations
type LLMClient interface {
	CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, result interface{}) (*llm.Usage, error)
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, *llm.Usage, error)
}

// AICrawler is an enterprise-grade AI-powered web crawler
// It uses Claude for semantic understanding and visual AI for element detection
type AICrawler struct {
	browser     playwright.Browser
	llmClient   LLMClient
	config      AICrawlerConfig
	logger      *zap.Logger

	// Visual AI client (optional)
	visualAI    VisualAIClient

	// Agent registry - spawn agents on demand
	agents      map[AgentType]Agent

	// Crawl context - shared across agents
	crawlCtx    *CrawlContext

	// Results
	mu          sync.Mutex
	pages       map[string]*AIPageAnalysis
	components  map[string]*Component
	flows       []*BusinessFlow

	// Progress callback
	onProgress  func(current, total int, message string)
}

// AICrawlerConfig configuration for AI-powered crawling
type AICrawlerConfig struct {
	MaxPages           int
	MaxDepth           int
	Timeout            time.Duration
	MaxDuration        time.Duration
	Headless           bool
	ScreenshotDir      string

	// AI options
	EnableVisualAI     bool          // Use V-JEPA 2 for visual analysis
	EnableAccessibility bool         // Run accessibility checks
	EnableSemanticAnalysis bool      // Use Claude for semantic understanding

	// Meta prompt settings
	DomainHints        []string      // Hints about the application domain (e-commerce, social, etc.)
	CustomPromptContext string       // Additional context for AI prompts
}

// DefaultAICrawlerConfig returns default configuration
func DefaultAICrawlerConfig() AICrawlerConfig {
	return AICrawlerConfig{
		MaxPages:              20,
		MaxDepth:              3,
		Timeout:               30 * time.Second,
		MaxDuration:           5 * time.Minute,
		Headless:              true,
		EnableVisualAI:        true,
		EnableAccessibility:   true,
		EnableSemanticAnalysis: true,
	}
}

// AIPageAnalysis contains AI-enhanced page analysis
type AIPageAnalysis struct {
	URL              string                 `json:"url"`
	Title            string                 `json:"title"`
	PageType         string                 `json:"page_type"`          // landing, form, list, detail, auth, checkout, search, etc.
	Purpose          string                 `json:"purpose"`            // AI-determined purpose of the page

	// Semantic elements (AI-detected)
	SemanticElements []SemanticElement      `json:"semantic_elements"`

	// Visual regions (from V-JEPA 2)
	VisualRegions    []VisualRegion         `json:"visual_regions,omitempty"`

	// Accessibility issues
	AccessibilityIssues []AccessibilityIssue `json:"accessibility_issues,omitempty"`

	// Interactions detected
	Interactions     []InteractionPoint     `json:"interactions"`

	// Data inputs
	DataInputs       []DataInput            `json:"data_inputs"`

	// Navigation elements
	Navigation       []NavigationElement    `json:"navigation"`

	// Screenshots
	ScreenshotBase64 string                 `json:"screenshot_base64,omitempty"`

	// Metadata
	LoadTime         time.Duration          `json:"load_time"`
	DOMHash          string                 `json:"dom_hash"`
	AnalyzedAt       time.Time              `json:"analyzed_at"`
}

// SemanticElement represents an AI-detected semantic element
type SemanticElement struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`        // button, input, link, card, modal, dropdown, etc.
	Purpose     string            `json:"purpose"`     // AI-determined purpose (submit, cancel, navigate, filter, etc.)
	Label       string            `json:"label"`       // Human-readable label
	Selector    string            `json:"selector"`    // Best CSS/XPath selector
	AltSelectors []string         `json:"alt_selectors"` // Alternative selectors for resilience
	Confidence  float64           `json:"confidence"`  // AI confidence score
	Attributes  map[string]string `json:"attributes"`
	BoundingBox *BoundingBox      `json:"bounding_box,omitempty"`
}

// VisualRegion represents a visually-detected region from V-JEPA 2
type VisualRegion struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`     // header, sidebar, content, footer, modal, card, form
	Description string       `json:"description"`
	BoundingBox BoundingBox  `json:"bounding_box"`
	Importance  float64      `json:"importance"` // Visual importance score
	Interactive bool         `json:"interactive"`
}

// BoundingBox represents element coordinates
type BoundingBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// AccessibilityIssue represents an accessibility problem
type AccessibilityIssue struct {
	Type        string   `json:"type"`        // missing-alt, low-contrast, no-label, etc.
	Severity    string   `json:"severity"`    // critical, serious, moderate, minor
	Element     string   `json:"element"`     // Selector of affected element
	Description string   `json:"description"`
	WCAGRule    string   `json:"wcag_rule"`   // WCAG guideline violated
	Suggestion  string   `json:"suggestion"`  // How to fix
}

// InteractionPoint represents a clickable/interactive element
type InteractionPoint struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`      // click, hover, drag, scroll, type
	Selector     string   `json:"selector"`
	Label        string   `json:"label"`
	ExpectedAction string `json:"expected_action"` // What happens when interacted
	RequiresAuth bool     `json:"requires_auth"`
	TestPriority string   `json:"test_priority"`   // critical, high, medium, low
}

// DataInput represents a form input or data entry point
type DataInput struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`         // text, email, password, number, date, select, checkbox, etc.
	Name         string            `json:"name"`
	Label        string            `json:"label"`
	Selector     string            `json:"selector"`
	Required     bool              `json:"required"`
	Validation   string            `json:"validation"`   // AI-detected validation rules
	Placeholder  string            `json:"placeholder"`
	Options      []string          `json:"options,omitempty"` // For select/radio
	Constraints  map[string]string `json:"constraints"`  // min, max, pattern, etc.
}

// NavigationElement represents a navigation item
type NavigationElement struct {
	ID          string `json:"id"`
	Type        string `json:"type"`   // link, menu, tab, breadcrumb, pagination
	Label       string `json:"label"`
	URL         string `json:"url"`
	Selector    string `json:"selector"`
	IsExternal  bool   `json:"is_external"`
	OpensNewTab bool   `json:"opens_new_tab"`
}

// VisualAIClient interface for visual AI service
type VisualAIClient interface {
	AnalyzeScreenshot(ctx context.Context, screenshot []byte) (*VisualAnalysisResult, error)
	FindElementByDescription(ctx context.Context, screenshot []byte, description string) ([]ElementMatch, error)
}

// VisualAnalysisResult from visual AI
type VisualAnalysisResult struct {
	Regions     []VisualRegion  `json:"regions"`
	UIType      string          `json:"ui_type"`      // web, mobile, desktop
	Framework   string          `json:"framework"`    // detected framework (React, Vue, etc.)
	Complexity  float64         `json:"complexity"`   // UI complexity score
}

// ElementMatch represents a visually-detected element
type ElementMatch struct {
	Description string      `json:"description"`
	BoundingBox BoundingBox `json:"bounding_box"`
	Confidence  float64     `json:"confidence"`
}

// NewAICrawler creates a new AI-powered crawler
func NewAICrawler(llmClient LLMClient, config AICrawlerConfig, logger *zap.Logger) (*AICrawler, error) {
	// Initialize Playwright
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("starting playwright: %w", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(config.Headless),
	})
	if err != nil {
		return nil, fmt.Errorf("launching browser: %w", err)
	}

	crawler := &AICrawler{
		browser:    browser,
		llmClient:  llmClient,
		config:     config,
		logger:     logger,
		pages:      make(map[string]*AIPageAnalysis),
		components: make(map[string]*Component),
		agents:     make(map[AgentType]Agent),
		crawlCtx: &CrawlContext{
			DomainHints:      config.DomainHints,
			VisitedPages:     make([]string, 0),
			DetectedPatterns: make([]string, 0),
		},
	}

	// Initialize agents - they are spawned on demand but we register them here
	crawler.registerAgents()

	return crawler, nil
}

// registerAgents initializes all available agents
func (c *AICrawler) registerAgents() {
	// Page Understanding Agent - determines what type of page this is semantically
	c.agents[AgentPageUnderstanding] = &PageUnderstandingAgent{llm: c.llmClient, logger: c.logger}

	// Element Discovery Agent - finds ALL interactive elements semantically
	c.agents[AgentElementDiscovery] = &ElementDiscoveryAgent{llm: c.llmClient, logger: c.logger}

	// Authentication Agent - detects any auth mechanism regardless of naming
	c.agents[AgentAuthentication] = &AuthenticationAgent{llm: c.llmClient, logger: c.logger}

	// Form Analysis Agent - understands forms semantically
	c.agents[AgentFormAnalysis] = &FormAnalysisAgent{llm: c.llmClient, logger: c.logger}

	// Business Flow Agent - infers user journeys
	c.agents[AgentBusinessFlow] = &BusinessFlowAgent{llm: c.llmClient, logger: c.logger}

	// ABA - Autonomous Business Analyst - generates requirements and user stories
	c.agents[AgentABA] = &AutonomousBusinessAnalyst{llm: c.llmClient, logger: c.logger}
}

// spawnAgent gets or creates an agent of the given type
func (c *AICrawler) spawnAgent(agentType AgentType) Agent {
	if agent, ok := c.agents[agentType]; ok {
		return agent
	}
	c.logger.Warn("agent not registered", zap.String("type", string(agentType)))
	return nil
}

// =============================================================================
// Meta-Prompt System - Adaptive prompts that work universally
// =============================================================================

// MetaPrompt generates context-aware prompts that work across languages and frameworks
type MetaPrompt struct {
	BaseInstruction string
	ContextRules    []string
	OutputFormat    string
}

// BuildPrompt creates the final prompt with context
func (m *MetaPrompt) BuildPrompt(context *CrawlContext, specificContext string) string {
	var sb strings.Builder

	sb.WriteString(m.BaseInstruction)
	sb.WriteString("\n\n")

	// Add universal rules
	sb.WriteString("CRITICAL RULES FOR UNIVERSAL DETECTION:\n")
	sb.WriteString("1. DO NOT rely on specific keywords like 'login', 'signin', 'register' - detect by PURPOSE and CONTEXT\n")
	sb.WriteString("2. Look at element BEHAVIOR, POSITION, and VISUAL CONTEXT, not just text\n")
	sb.WriteString("3. Consider ALL languages - 'Anmelden' (German), 'Connexion' (French), '登录' (Chinese) are all login\n")
	sb.WriteString("4. Icons without text (lock icon, user icon) often indicate auth-related elements\n")
	sb.WriteString("5. Form structure matters: email/username + password + submit = likely auth form\n")
	sb.WriteString("6. Consider SPA patterns: React, Vue, Angular use different conventions\n\n")

	// Add context-specific rules
	for _, rule := range m.ContextRules {
		sb.WriteString("- ")
		sb.WriteString(rule)
		sb.WriteString("\n")
	}

	// Add crawl context if available
	if context != nil {
		sb.WriteString("\nCRAWL CONTEXT:\n")
		if context.BaseURL != "" {
			sb.WriteString(fmt.Sprintf("- Base URL: %s\n", context.BaseURL))
		}
		if context.Language != "" {
			sb.WriteString(fmt.Sprintf("- Detected Language: %s\n", context.Language))
		}
		if context.Framework != "" {
			sb.WriteString(fmt.Sprintf("- Detected Framework: %s\n", context.Framework))
		}
		if len(context.DomainHints) > 0 {
			sb.WriteString(fmt.Sprintf("- Domain Hints: %s\n", strings.Join(context.DomainHints, ", ")))
		}
		if len(context.DetectedPatterns) > 0 {
			sb.WriteString(fmt.Sprintf("- Already Detected: %s\n", strings.Join(context.DetectedPatterns, ", ")))
		}
	}

	if specificContext != "" {
		sb.WriteString("\nSPECIFIC CONTEXT:\n")
		sb.WriteString(specificContext)
	}

	sb.WriteString("\n\n")
	sb.WriteString(m.OutputFormat)

	return sb.String()
}

// SetVisualAI sets the visual AI client
func (c *AICrawler) SetVisualAI(client VisualAIClient) {
	c.visualAI = client
}

// SetProgressCallback sets progress callback
func (c *AICrawler) SetProgressCallback(fn func(current, total int, message string)) {
	c.onProgress = fn
}

// Crawl performs AI-enhanced crawling
func (c *AICrawler) Crawl(ctx context.Context, startURL string) (*AIAppModel, error) {
	startTime := time.Now()

	// Create context with timeout
	crawlCtx, cancel := context.WithTimeout(ctx, c.config.MaxDuration)
	defer cancel()

	// Queue of URLs to crawl
	queue := []crawlItem{{url: startURL, depth: 0}}
	visited := make(map[string]bool)

	pageCount := 0

	for len(queue) > 0 && pageCount < c.config.MaxPages {
		select {
		case <-crawlCtx.Done():
			c.logger.Info("crawl timeout reached", zap.Int("pages_crawled", pageCount))
			goto buildResult
		default:
		}

		// Dequeue
		item := queue[0]
		queue = queue[1:]

		// Skip if visited
		normalizedURL := normalizeURL(item.url)
		if visited[normalizedURL] {
			continue
		}
		visited[normalizedURL] = true

		// Report progress
		if c.onProgress != nil {
			c.onProgress(pageCount+1, c.config.MaxPages, fmt.Sprintf("Analyzing: %s", item.url))
		}

		// Analyze page with AI
		analysis, err := c.analyzePage(crawlCtx, item.url)
		if err != nil {
			c.logger.Warn("failed to analyze page", zap.String("url", item.url), zap.Error(err))
			continue
		}

		// Store result
		c.mu.Lock()
		c.pages[normalizedURL] = analysis
		c.mu.Unlock()

		pageCount++

		// Extract new URLs to crawl
		if item.depth < c.config.MaxDepth {
			for _, nav := range analysis.Navigation {
				if !nav.IsExternal && nav.URL != "" {
					queue = append(queue, crawlItem{url: nav.URL, depth: item.depth + 1})
				}
			}
		}
	}

buildResult:
	// Build final app model
	return c.buildAppModel(startURL, time.Since(startTime)), nil
}

type crawlItem struct {
	url   string
	depth int
}

// analyzePage performs comprehensive AI analysis of a page
func (c *AICrawler) analyzePage(ctx context.Context, url string) (*AIPageAnalysis, error) {
	startTime := time.Now()

	// Create new browser context and page
	browserCtx, err := c.browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
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

	// Navigate to page
	_, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(float64(c.config.Timeout.Milliseconds())),
	})
	if err != nil {
		return nil, fmt.Errorf("navigating to page: %w", err)
	}

	// Wait for JS frameworks to render
	page.WaitForTimeout(2000)

	// Get page title
	title, _ := page.Title()

	// Take screenshot for AI analysis
	screenshot, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(true),
		Type:     playwright.ScreenshotTypePng,
	})
	if err != nil {
		c.logger.Warn("failed to take screenshot", zap.Error(err))
	}

	// Get page HTML for analysis
	html, err := page.Content()
	if err != nil {
		return nil, fmt.Errorf("getting page content: %w", err)
	}

	analysis := &AIPageAnalysis{
		URL:        url,
		Title:      title,
		LoadTime:   time.Since(startTime),
		DOMHash:    HashDOM(html),
		AnalyzedAt: time.Now(),
	}

	// Store screenshot
	if len(screenshot) > 0 {
		analysis.ScreenshotBase64 = base64.StdEncoding.EncodeToString(screenshot)
	}

	// Run AI semantic analysis using multi-agent system
	if c.config.EnableSemanticAnalysis && c.llmClient != nil {
		agentInput := AgentInput{
			URL:        url,
			Title:      title,
			HTML:       html,
			Screenshot: screenshot,
			Context:    c.crawlCtx,
		}

		// Spawn agents in parallel for comprehensive analysis
		var wg sync.WaitGroup
		var agentMu sync.Mutex
		agentResults := make(map[AgentType]AgentOutput)

		// Page Understanding Agent
		wg.Add(1)
		go func() {
			defer wg.Done()
			if agent := c.spawnAgent(AgentPageUnderstanding); agent != nil {
				if result, err := agent.Analyze(ctx, agentInput); err == nil {
					agentMu.Lock()
					agentResults[AgentPageUnderstanding] = result
					agentMu.Unlock()
				} else {
					c.logger.Warn("page understanding agent failed", zap.Error(err))
				}
			}
		}()

		// Element Discovery Agent
		wg.Add(1)
		go func() {
			defer wg.Done()
			if agent := c.spawnAgent(AgentElementDiscovery); agent != nil {
				if result, err := agent.Analyze(ctx, agentInput); err == nil {
					agentMu.Lock()
					agentResults[AgentElementDiscovery] = result
					agentMu.Unlock()
				} else {
					c.logger.Warn("element discovery agent failed", zap.Error(err))
				}
			}
		}()

		// Form Analysis Agent
		wg.Add(1)
		go func() {
			defer wg.Done()
			if agent := c.spawnAgent(AgentFormAnalysis); agent != nil {
				if result, err := agent.Analyze(ctx, agentInput); err == nil {
					agentMu.Lock()
					agentResults[AgentFormAnalysis] = result
					agentMu.Unlock()
				} else {
					c.logger.Warn("form analysis agent failed", zap.Error(err))
				}
			}
		}()

		// Authentication Agent
		wg.Add(1)
		go func() {
			defer wg.Done()
			if agent := c.spawnAgent(AgentAuthentication); agent != nil {
				if result, err := agent.Analyze(ctx, agentInput); err == nil {
					agentMu.Lock()
					agentResults[AgentAuthentication] = result
					agentMu.Unlock()
				} else {
					c.logger.Warn("auth agent failed", zap.Error(err))
				}
			}
		}()

		wg.Wait()

		// Merge agent results into analysis
		c.mergeAgentResults(analysis, agentResults)

		// Update crawl context with detected patterns
		c.updateCrawlContext(url, agentResults)
	}

	// Run visual AI analysis
	if c.config.EnableVisualAI && c.visualAI != nil && len(screenshot) > 0 {
		visualResult, err := c.visualAI.AnalyzeScreenshot(ctx, screenshot)
		if err != nil {
			c.logger.Warn("visual AI analysis failed", zap.Error(err))
		} else {
			analysis.VisualRegions = visualResult.Regions
		}
	}

	// Run accessibility analysis
	if c.config.EnableAccessibility {
		issues, err := c.runAccessibilityAnalysis(ctx, page)
		if err != nil {
			c.logger.Warn("accessibility analysis failed", zap.Error(err))
		} else {
			analysis.AccessibilityIssues = issues
		}
	}

	return analysis, nil
}

// SemanticAnalysisResult from Claude
type SemanticAnalysisResult struct {
	PageType     string              `json:"page_type"`
	Purpose      string              `json:"purpose"`
	Elements     []SemanticElement   `json:"elements"`
	Interactions []InteractionPoint  `json:"interactions"`
	DataInputs   []DataInput         `json:"data_inputs"`
	Navigation   []NavigationElement `json:"navigation"`
}

// runSemanticAnalysis uses Claude to understand the page semantically
func (c *AICrawler) runSemanticAnalysis(ctx context.Context, url, title, html string, screenshot []byte) (*SemanticAnalysisResult, error) {
	// Build meta-prompt with context
	systemPrompt := c.buildSemanticSystemPrompt()

	// Truncate HTML if too long
	maxHTMLLen := 50000
	if len(html) > maxHTMLLen {
		html = html[:maxHTMLLen] + "\n<!-- truncated -->"
	}

	userPrompt := fmt.Sprintf(`Analyze this web page and extract all interactive elements, data inputs, and navigation.

URL: %s
Title: %s

HTML Content:
%s

%s

Respond with a JSON object containing:
{
  "page_type": "landing|form|list|detail|auth|checkout|search|dashboard|settings|error|other",
  "purpose": "brief description of what this page is for",
  "elements": [
    {
      "id": "unique-id",
      "type": "button|link|input|dropdown|checkbox|radio|toggle|tab|card|modal|menu",
      "purpose": "what this element does",
      "label": "visible text or aria-label",
      "selector": "best CSS selector",
      "alt_selectors": ["alternative selectors for resilience"],
      "confidence": 0.95
    }
  ],
  "interactions": [
    {
      "id": "unique-id",
      "type": "click|hover|type|drag|scroll",
      "selector": "CSS selector",
      "label": "what to interact with",
      "expected_action": "what happens when interacted",
      "test_priority": "critical|high|medium|low"
    }
  ],
  "data_inputs": [
    {
      "id": "unique-id",
      "type": "text|email|password|number|date|select|checkbox|radio|textarea|file",
      "name": "field name",
      "label": "visible label",
      "selector": "CSS selector",
      "required": true,
      "validation": "detected validation rules",
      "placeholder": "placeholder text"
    }
  ],
  "navigation": [
    {
      "id": "unique-id",
      "type": "link|menu|tab|breadcrumb|pagination|button",
      "label": "visible text",
      "url": "destination URL",
      "selector": "CSS selector",
      "is_external": false
    }
  ]
}

Important:
- Find ALL interactive elements, even if they don't use standard HTML tags
- For SPAs (React, Vue, Angular), look for elements with onClick handlers, data-* attributes, role attributes
- Include elements that might be custom components (div with role="button", etc.)
- Generate multiple selector strategies for resilience (CSS, data attributes, text content)
- Prioritize elements that are critical for user workflows`, url, title, html, c.getContextHints())

	var result SemanticAnalysisResult
	_, err := c.llmClient.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return nil, fmt.Errorf("Claude analysis failed: %w", err)
	}

	return &result, nil
}

// buildSemanticSystemPrompt creates the system prompt for semantic analysis
func (c *AICrawler) buildSemanticSystemPrompt() string {
	prompt := `You are an expert web application analyzer specializing in test automation.
Your task is to analyze web pages and identify ALL interactive elements that need testing.

You excel at:
1. Finding interactive elements even in complex SPAs (React, Vue, Angular, Svelte)
2. Identifying elements that don't use standard HTML tags but are still interactive
3. Understanding the semantic purpose of elements from context
4. Generating robust selectors that will work reliably for automated testing
5. Prioritizing elements based on their importance to user workflows

Key patterns to look for:
- Custom buttons: div/span with role="button", onClick, or button-like classes
- Custom inputs: contenteditable, custom form controls
- Dropdowns: Custom select implementations, autocomplete fields
- Modals/Dialogs: Elements with role="dialog", modal classes
- Tabs: Elements with role="tab", tablist patterns
- Cards/Lists: Repeated patterns that might need interaction testing

Always generate multiple selectors for resilience:
1. ID-based (most stable)
2. data-testid or data-* attributes
3. ARIA attributes (role, aria-label)
4. Semantic CSS (button.submit, input[name="email"])
5. Text content (text="Submit", :has-text())
6. Structural (nth-child, relative position)`

	// Add domain-specific hints
	if len(c.config.DomainHints) > 0 {
		prompt += fmt.Sprintf("\n\nDomain context: This is a %s application.", strings.Join(c.config.DomainHints, ", "))
	}

	// Add custom context
	if c.config.CustomPromptContext != "" {
		prompt += fmt.Sprintf("\n\nAdditional context: %s", c.config.CustomPromptContext)
	}

	return prompt
}

// getContextHints returns context hints for the prompt
func (c *AICrawler) getContextHints() string {
	if len(c.config.DomainHints) == 0 && c.config.CustomPromptContext == "" {
		return ""
	}

	hints := "Context hints:\n"
	if len(c.config.DomainHints) > 0 {
		hints += fmt.Sprintf("- Application domain: %s\n", strings.Join(c.config.DomainHints, ", "))
	}
	if c.config.CustomPromptContext != "" {
		hints += fmt.Sprintf("- Additional context: %s\n", c.config.CustomPromptContext)
	}
	return hints
}

// mergeAgentResults combines results from multiple agents into the page analysis
func (c *AICrawler) mergeAgentResults(analysis *AIPageAnalysis, results map[AgentType]AgentOutput) {
	// Page Understanding results
	if pageResult, ok := results[AgentPageUnderstanding]; ok && pageResult.Success {
		if pageType, ok := pageResult.Data["page_type"].(string); ok {
			analysis.PageType = pageType
		}
		if purpose, ok := pageResult.Data["purpose"].(string); ok {
			analysis.Purpose = purpose
		}
	}

	// Element Discovery results
	if elemResult, ok := results[AgentElementDiscovery]; ok && elemResult.Success {
		analysis.SemanticElements = elemResult.Elements

		// Convert elements to interactions and data inputs
		for _, elem := range elemResult.Elements {
			// Create interaction point
			interaction := InteractionPoint{
				ID:             elem.ID,
				Type:           elem.Type,
				Selector:       elem.Selector,
				Label:          elem.Label,
				ExpectedAction: elem.Purpose,
				TestPriority:   c.inferTestPriority(elem),
			}
			analysis.Interactions = append(analysis.Interactions, interaction)

			// Create data input if it's an input type
			if isInputType(elem.Type) {
				dataInput := DataInput{
					ID:       elem.ID,
					Type:     elem.Type,
					Label:    elem.Label,
					Selector: elem.Selector,
				}
				analysis.DataInputs = append(analysis.DataInputs, dataInput)
			}

			// Create navigation if it's a link
			if elem.Type == "link" {
				href := ""
				if elem.Attributes != nil {
					href = elem.Attributes["href"]
				}
				nav := NavigationElement{
					ID:       elem.ID,
					Type:     "link",
					Label:    elem.Label,
					URL:      href,
					Selector: elem.Selector,
				}
				analysis.Navigation = append(analysis.Navigation, nav)
			}
		}
	}

	// Form Analysis results
	if formResult, ok := results[AgentFormAnalysis]; ok && formResult.Success {
		if forms, ok := formResult.Data["forms"].([]interface{}); ok {
			for _, f := range forms {
				if formData, ok := f.(map[string]interface{}); ok {
					c.processFormData(analysis, formData)
				}
			}
		}
	}

	// Authentication results - add auth-related elements
	if authResult, ok := results[AgentAuthentication]; ok && authResult.Success {
		if hasAuth, ok := authResult.Data["has_authentication"].(bool); ok && hasAuth {
			// Mark page as having auth
			if analysis.PageType == "" || analysis.PageType == "other" {
				analysis.PageType = "auth"
			}
		}
	}
}

// processFormData extracts form information from agent output
func (c *AICrawler) processFormData(analysis *AIPageAnalysis, formData map[string]interface{}) {
	if fields, ok := formData["fields"].([]interface{}); ok {
		for _, field := range fields {
			if fieldData, ok := field.(map[string]interface{}); ok {
				input := DataInput{
					ID:       getString(fieldData, "name"),
					Type:     getString(fieldData, "type"),
					Name:     getString(fieldData, "name"),
					Label:    getString(fieldData, "purpose"),
					Selector: getString(fieldData, "selector"),
				}
				if required, ok := fieldData["required"].(bool); ok {
					input.Required = required
				}
				analysis.DataInputs = append(analysis.DataInputs, input)
			}
		}
	}
}

// inferTestPriority determines test priority based on element type and purpose
func (c *AICrawler) inferTestPriority(elem SemanticElement) string {
	// High priority for auth-related elements
	purposeLower := strings.ToLower(elem.Purpose)
	if strings.Contains(purposeLower, "login") || strings.Contains(purposeLower, "submit") ||
		strings.Contains(purposeLower, "checkout") || strings.Contains(purposeLower, "payment") {
		return "critical"
	}
	if strings.Contains(purposeLower, "search") || strings.Contains(purposeLower, "filter") ||
		strings.Contains(purposeLower, "add") || strings.Contains(purposeLower, "create") {
		return "high"
	}
	if elem.Type == "button" || elem.Type == "input" {
		return "medium"
	}
	return "low"
}

// isInputType checks if element type is an input type
func isInputType(elemType string) bool {
	inputTypes := map[string]bool{
		"input": true, "text": true, "email": true, "password": true,
		"number": true, "date": true, "select": true, "checkbox": true,
		"radio": true, "textarea": true, "file": true, "dropdown": true,
	}
	return inputTypes[elemType]
}

// updateCrawlContext updates the shared context based on agent findings
func (c *AICrawler) updateCrawlContext(url string, results map[AgentType]AgentOutput) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add to visited pages
	c.crawlCtx.VisitedPages = append(c.crawlCtx.VisitedPages, url)

	// Update detected patterns
	if pageResult, ok := results[AgentPageUnderstanding]; ok && pageResult.Success {
		if patterns, ok := pageResult.Data["detected_patterns"].([]interface{}); ok {
			for _, p := range patterns {
				if pattern, ok := p.(string); ok {
					// Avoid duplicates
					found := false
					for _, existing := range c.crawlCtx.DetectedPatterns {
						if existing == pattern {
							found = true
							break
						}
					}
					if !found {
						c.crawlCtx.DetectedPatterns = append(c.crawlCtx.DetectedPatterns, pattern)
					}
				}
			}
		}
	}

	// Detect auth patterns
	if authResult, ok := results[AgentAuthentication]; ok && authResult.Success {
		if hasAuth, ok := authResult.Data["has_authentication"].(bool); ok && hasAuth {
			found := false
			for _, p := range c.crawlCtx.DetectedPatterns {
				if p == "authentication" {
					found = true
					break
				}
			}
			if !found {
				c.crawlCtx.DetectedPatterns = append(c.crawlCtx.DetectedPatterns, "authentication")
			}
		}
	}
}

// runAccessibilityAnalysis checks for accessibility issues
func (c *AICrawler) runAccessibilityAnalysis(ctx context.Context, page playwright.Page) ([]AccessibilityIssue, error) {
	var issues []AccessibilityIssue

	// Inject and run axe-core accessibility testing
	// First, inject axe-core library
	axeScript := `
	(function() {
		return new Promise((resolve) => {
			if (window.axe) {
				resolve(true);
				return;
			}
			const script = document.createElement('script');
			script.src = 'https://cdnjs.cloudflare.com/ajax/libs/axe-core/4.8.2/axe.min.js';
			script.onload = () => resolve(true);
			script.onerror = () => resolve(false);
			document.head.appendChild(script);
		});
	})()
	`

	loaded, err := page.Evaluate(axeScript)
	if err != nil || loaded != true {
		// Fallback to basic checks if axe-core fails to load
		return c.runBasicAccessibilityChecks(ctx, page)
	}

	// Run axe-core analysis
	axeRunScript := `
	(async function() {
		try {
			const results = await axe.run();
			return results.violations.map(v => ({
				type: v.id,
				severity: v.impact,
				description: v.description,
				wcag: v.tags.filter(t => t.startsWith('wcag')).join(', '),
				help: v.help,
				nodes: v.nodes.map(n => n.target.join(' ')).slice(0, 5)
			}));
		} catch (e) {
			return [];
		}
	})()
	`

	result, err := page.Evaluate(axeRunScript)
	if err != nil {
		return c.runBasicAccessibilityChecks(ctx, page)
	}

	// Parse results
	if violations, ok := result.([]interface{}); ok {
		for _, v := range violations {
			if vMap, ok := v.(map[string]interface{}); ok {
				issue := AccessibilityIssue{
					Type:        getString(vMap, "type"),
					Severity:    getString(vMap, "severity"),
					Description: getString(vMap, "description"),
					WCAGRule:    getString(vMap, "wcag"),
					Suggestion:  getString(vMap, "help"),
				}
				if nodes, ok := vMap["nodes"].([]interface{}); ok && len(nodes) > 0 {
					if selector, ok := nodes[0].(string); ok {
						issue.Element = selector
					}
				}
				issues = append(issues, issue)
			}
		}
	}

	return issues, nil
}

// runBasicAccessibilityChecks performs basic accessibility checks without axe-core
func (c *AICrawler) runBasicAccessibilityChecks(ctx context.Context, page playwright.Page) ([]AccessibilityIssue, error) {
	var issues []AccessibilityIssue

	// Check for images without alt text
	imgsWithoutAlt, _ := page.Locator("img:not([alt])").Count()
	if imgsWithoutAlt > 0 {
		issues = append(issues, AccessibilityIssue{
			Type:        "image-alt",
			Severity:    "serious",
			Description: fmt.Sprintf("%d images without alt text", imgsWithoutAlt),
			WCAGRule:    "wcag111",
			Suggestion:  "Add descriptive alt text to all images",
		})
	}

	// Check for form inputs without labels
	inputsWithoutLabels, _ := page.Locator("input:not([aria-label]):not([aria-labelledby])").Count()
	labelsCount, _ := page.Locator("label").Count()
	if inputsWithoutLabels > labelsCount {
		issues = append(issues, AccessibilityIssue{
			Type:        "label",
			Severity:    "serious",
			Description: "Form inputs may be missing labels",
			WCAGRule:    "wcag131",
			Suggestion:  "Associate labels with form inputs using 'for' attribute or aria-label",
		})
	}

	// Check for buttons without accessible names
	buttonsWithoutName, _ := page.Locator("button:not([aria-label]):empty").Count()
	if buttonsWithoutName > 0 {
		issues = append(issues, AccessibilityIssue{
			Type:        "button-name",
			Severity:    "serious",
			Description: fmt.Sprintf("%d buttons without accessible names", buttonsWithoutName),
			WCAGRule:    "wcag412",
			Suggestion:  "Add text content or aria-label to buttons",
		})
	}

	// Check for missing document language
	htmlLang, _ := page.Locator("html[lang]").Count()
	if htmlLang == 0 {
		issues = append(issues, AccessibilityIssue{
			Type:        "html-lang",
			Severity:    "serious",
			Description: "Document language is not specified",
			WCAGRule:    "wcag311",
			Suggestion:  "Add lang attribute to html element",
		})
	}

	// Check for missing page title
	title, _ := page.Title()
	if title == "" {
		issues = append(issues, AccessibilityIssue{
			Type:        "document-title",
			Severity:    "serious",
			Description: "Page has no title",
			WCAGRule:    "wcag242",
			Suggestion:  "Add a descriptive title element",
		})
	}

	return issues, nil
}

// AIAppModel is the enhanced app model from AI crawling
type AIAppModel struct {
	ID              string                     `json:"id"`
	BaseURL         string                     `json:"base_url"`
	Pages           []*AIPageAnalysis          `json:"pages"`
	Components      []*Component               `json:"components"`
	BusinessFlows   []*BusinessFlow            `json:"business_flows"`
	AccessibilitySummary *AccessibilitySummary `json:"accessibility_summary"`
	Stats           AIAppStats                 `json:"stats"`
	CrawlDuration   time.Duration              `json:"crawl_duration"`
	AnalyzedAt      time.Time                  `json:"analyzed_at"`
}

// AccessibilitySummary summarizes accessibility findings
type AccessibilitySummary struct {
	TotalIssues    int            `json:"total_issues"`
	BySeverity     map[string]int `json:"by_severity"`
	ByType         map[string]int `json:"by_type"`
	Score          float64        `json:"score"` // 0-100 accessibility score
	Recommendations []string      `json:"recommendations"`
}

// AIAppStats contains statistics from AI analysis
type AIAppStats struct {
	TotalPages        int `json:"total_pages"`
	TotalElements     int `json:"total_elements"`
	TotalInteractions int `json:"total_interactions"`
	TotalDataInputs   int `json:"total_data_inputs"`
	TotalNavigation   int `json:"total_navigation"`
	TotalA11yIssues   int `json:"total_a11y_issues"`
	TotalVisualRegions int `json:"total_visual_regions"`
}

// buildAppModel builds the final app model from crawl results
func (c *AICrawler) buildAppModel(baseURL string, duration time.Duration) *AIAppModel {
	c.mu.Lock()
	defer c.mu.Unlock()

	model := &AIAppModel{
		ID:            generateID(),
		BaseURL:       baseURL,
		Pages:         make([]*AIPageAnalysis, 0, len(c.pages)),
		Components:    make([]*Component, 0),
		BusinessFlows: c.detectBusinessFlows(),
		CrawlDuration: duration,
		AnalyzedAt:    time.Now(),
	}

	// Aggregate pages and stats
	var totalA11yIssues int
	a11ySeverity := make(map[string]int)
	a11yTypes := make(map[string]int)

	for _, page := range c.pages {
		model.Pages = append(model.Pages, page)
		model.Stats.TotalPages++
		model.Stats.TotalElements += len(page.SemanticElements)
		model.Stats.TotalInteractions += len(page.Interactions)
		model.Stats.TotalDataInputs += len(page.DataInputs)
		model.Stats.TotalNavigation += len(page.Navigation)
		model.Stats.TotalVisualRegions += len(page.VisualRegions)

		// Aggregate accessibility issues
		for _, issue := range page.AccessibilityIssues {
			totalA11yIssues++
			a11ySeverity[issue.Severity]++
			a11yTypes[issue.Type]++
		}
	}

	model.Stats.TotalA11yIssues = totalA11yIssues

	// Build accessibility summary
	model.AccessibilitySummary = &AccessibilitySummary{
		TotalIssues: totalA11yIssues,
		BySeverity:  a11ySeverity,
		ByType:      a11yTypes,
		Score:       c.calculateA11yScore(totalA11yIssues, len(c.pages)),
		Recommendations: c.generateA11yRecommendations(a11yTypes),
	}

	return model
}

// detectBusinessFlows uses AI to infer business flows from discovered pages
// This is called at the end of crawling to synthesize flows from all page data
func (c *AICrawler) detectBusinessFlows() []*BusinessFlow {
	var flows []*BusinessFlow

	// Collect all page data for flow inference
	pageTypes := make(map[string][]string) // pageType -> URLs
	allInteractions := make([]string, 0)
	allDataInputs := make([]string, 0)

	for url, page := range c.pages {
		pageTypes[page.PageType] = append(pageTypes[page.PageType], url)

		for _, interaction := range page.Interactions {
			allInteractions = append(allInteractions, fmt.Sprintf("%s: %s (%s)", url, interaction.Label, interaction.Type))
		}

		for _, input := range page.DataInputs {
			allDataInputs = append(allDataInputs, fmt.Sprintf("%s: %s (%s)", url, input.Label, input.Type))
		}
	}

	// Use AI to infer flows from the collected data
	if c.llmClient != nil && len(c.pages) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		flowAgent := c.spawnAgent(AgentBusinessFlow)
		if flowAgent != nil {
			// Build a summary of all pages for flow analysis
			var pageSummary strings.Builder
			for url, page := range c.pages {
				pageSummary.WriteString(fmt.Sprintf("\nPage: %s\n", url))
				pageSummary.WriteString(fmt.Sprintf("  Type: %s\n", page.PageType))
				pageSummary.WriteString(fmt.Sprintf("  Purpose: %s\n", page.Purpose))
				pageSummary.WriteString(fmt.Sprintf("  Interactions: %d\n", len(page.Interactions)))
				pageSummary.WriteString(fmt.Sprintf("  Data Inputs: %d\n", len(page.DataInputs)))
			}

			input := AgentInput{
				URL:     c.crawlCtx.BaseURL,
				Title:   "Multi-page Flow Analysis",
				HTML:    pageSummary.String(), // Using summary instead of HTML
				Context: c.crawlCtx,
			}

			if result, err := flowAgent.Analyze(ctx, input); err == nil && result.Success {
				// Convert detected flows to BusinessFlow format
				for _, detected := range result.Flows {
					flow := c.convertDetectedFlow(detected)
					flows = append(flows, flow)
				}
			}
		}
	}

	// If AI didn't detect any flows, use heuristic-based detection
	if len(flows) == 0 {
		flows = c.detectFlowsHeuristically(pageTypes)
	}

	return flows
}

// convertDetectedFlow converts AI-detected flow to BusinessFlow with proper FlowSteps
func (c *AICrawler) convertDetectedFlow(detected DetectedFlow) *BusinessFlow {
	steps := make([]FlowStep, 0, len(detected.StepActions))

	for i, action := range detected.StepActions {
		// Parse the action to determine step details
		step := FlowStep{
			Order:  i + 1,
			Action: c.inferActionType(action),
			Value:  action, // Store the description in Value for reference
		}
		steps = append(steps, step)
	}

	return &BusinessFlow{
		ID:          generateID(),
		Name:        detected.Name,
		Description: detected.Purpose,
		Type:        detected.Type,
		Priority:    detected.Priority,
		Steps:       steps,
		Confidence:  detected.Confidence,
	}
}

// inferActionType determines the action type from a description
func (c *AICrawler) inferActionType(description string) string {
	descLower := strings.ToLower(description)

	if strings.Contains(descLower, "navigate") || strings.Contains(descLower, "go to") ||
		strings.Contains(descLower, "open") || strings.Contains(descLower, "visit") {
		return "navigate"
	}
	if strings.Contains(descLower, "click") || strings.Contains(descLower, "press") ||
		strings.Contains(descLower, "tap") || strings.Contains(descLower, "select") {
		return "click"
	}
	if strings.Contains(descLower, "fill") || strings.Contains(descLower, "enter") ||
		strings.Contains(descLower, "type") || strings.Contains(descLower, "input") {
		return "fill"
	}
	if strings.Contains(descLower, "wait") || strings.Contains(descLower, "verify") ||
		strings.Contains(descLower, "check") || strings.Contains(descLower, "confirm") {
		return "wait"
	}
	if strings.Contains(descLower, "submit") {
		return "click"
	}

	return "interact"
}

// detectFlowsHeuristically uses page type patterns when AI is unavailable
func (c *AICrawler) detectFlowsHeuristically(pageTypes map[string][]string) []*BusinessFlow {
	var flows []*BusinessFlow

	// Detect flows based on page types found (semantic, not keyword-based)
	if len(pageTypes["auth"]) > 0 || len(pageTypes["login"]) > 0 {
		flows = append(flows, &BusinessFlow{
			ID:          generateID(),
			Name:        "Authentication Flow",
			Description: "User authentication process",
			Type:        "auth",
			Priority:    "critical",
			Steps: []FlowStep{
				{Order: 1, Action: "navigate", Value: "Go to authentication page"},
				{Order: 2, Action: "fill", Value: "Enter credentials"},
				{Order: 3, Action: "click", Value: "Submit authentication"},
				{Order: 4, Action: "wait", Value: "Verify successful authentication"},
			},
			Confidence: 0.7,
		})
	}

	if len(pageTypes["registration"]) > 0 || len(pageTypes["signup"]) > 0 {
		flows = append(flows, &BusinessFlow{
			ID:          generateID(),
			Name:        "Registration Flow",
			Description: "New user registration process",
			Type:        "registration",
			Priority:    "critical",
			Steps: []FlowStep{
				{Order: 1, Action: "navigate", Value: "Go to registration page"},
				{Order: 2, Action: "fill", Value: "Enter user details"},
				{Order: 3, Action: "click", Value: "Submit registration"},
				{Order: 4, Action: "wait", Value: "Verify account creation"},
			},
			Confidence: 0.7,
		})
	}

	if len(pageTypes["search"]) > 0 || len(pageTypes["listing"]) > 0 {
		flows = append(flows, &BusinessFlow{
			ID:          generateID(),
			Name:        "Search Flow",
			Description: "Content search and discovery",
			Type:        "search",
			Priority:    "high",
			Steps: []FlowStep{
				{Order: 1, Action: "fill", Value: "Enter search query"},
				{Order: 2, Action: "click", Value: "Execute search"},
				{Order: 3, Action: "wait", Value: "View search results"},
			},
			Confidence: 0.7,
		})
	}

	if len(pageTypes["checkout"]) > 0 || len(pageTypes["cart"]) > 0 {
		flows = append(flows, &BusinessFlow{
			ID:          generateID(),
			Name:        "Purchase Flow",
			Description: "E-commerce purchase process",
			Type:        "checkout",
			Priority:    "critical",
			Steps: []FlowStep{
				{Order: 1, Action: "click", Value: "Add item to cart"},
				{Order: 2, Action: "navigate", Value: "Go to cart"},
				{Order: 3, Action: "click", Value: "Proceed to checkout"},
				{Order: 4, Action: "fill", Value: "Enter shipping and payment"},
				{Order: 5, Action: "click", Value: "Complete purchase"},
			},
			Confidence: 0.7,
		})
	}

	// If we still have no flows but have pages, create a generic exploration flow
	if len(flows) == 0 && len(c.pages) > 0 {
		flows = append(flows, &BusinessFlow{
			ID:          generateID(),
			Name:        "Site Exploration",
			Description: "General site navigation and interaction",
			Type:        "navigation",
			Priority:    "medium",
			Steps: []FlowStep{
				{Order: 1, Action: "navigate", Value: "Visit main pages"},
				{Order: 2, Action: "interact", Value: "Test interactive elements"},
				{Order: 3, Action: "wait", Value: "Verify page responses"},
			},
			Confidence: 0.5,
		})
	}

	return flows
}

// calculateA11yScore calculates accessibility score
func (c *AICrawler) calculateA11yScore(issues, pages int) float64 {
	if pages == 0 {
		return 100
	}

	// Base score of 100, subtract based on issues
	issuesPerPage := float64(issues) / float64(pages)
	score := 100 - (issuesPerPage * 10)

	if score < 0 {
		score = 0
	}

	return score
}

// generateA11yRecommendations generates accessibility recommendations
func (c *AICrawler) generateA11yRecommendations(issueTypes map[string]int) []string {
	var recommendations []string

	if issueTypes["image-alt"] > 0 {
		recommendations = append(recommendations, "Add descriptive alt text to all images for screen reader users")
	}
	if issueTypes["label"] > 0 {
		recommendations = append(recommendations, "Associate labels with all form inputs")
	}
	if issueTypes["color-contrast"] > 0 {
		recommendations = append(recommendations, "Improve color contrast for better readability")
	}
	if issueTypes["button-name"] > 0 {
		recommendations = append(recommendations, "Add accessible names to all interactive buttons")
	}
	if issueTypes["html-lang"] > 0 {
		recommendations = append(recommendations, "Specify document language in html element")
	}

	return recommendations
}

// Close closes the crawler
func (c *AICrawler) Close() error {
	if c.browser != nil {
		return c.browser.Close()
	}
	return nil
}

// Helper functions
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func normalizeURL(url string) string {
	// Remove trailing slash and fragment
	url = strings.TrimSuffix(url, "/")
	if idx := strings.Index(url, "#"); idx != -1 {
		url = url[:idx]
	}
	return url
}

// ToLegacyAppModel converts AIAppModel to the legacy AppModel format for compatibility
func (m *AIAppModel) ToLegacyAppModel() *AppModel {
	legacy := &AppModel{
		ID:       m.ID,
		BaseURL:  m.BaseURL,
		Pages:    make(map[string]*PageModel),
		Flows:    make([]BusinessFlow, 0, len(m.BusinessFlows)),
		Stats: DiscoveryStats{
			TotalPages: m.Stats.TotalPages,
		},
		DiscoveredAt: m.AnalyzedAt,
	}

	for _, page := range m.Pages {
		legacyPage := &PageModel{
			URL:         page.URL,
			Title:       page.Title,
			Description: page.Purpose,
			PageType:    page.PageType,
			LoadTime:    page.LoadTime,
			DOMHash:     page.DOMHash,
		}

		// Convert semantic elements to legacy format
		for _, elem := range page.SemanticElements {
			switch elem.Type {
			case "button":
				legacyPage.Buttons = append(legacyPage.Buttons, ButtonModel{
					Text:      elem.Label,
					Type:      "button",
					AriaLabel: elem.Label,
				})
			case "input", "textarea":
				legacyPage.Inputs = append(legacyPage.Inputs, InputModel{
					Type:        elem.Type,
					Placeholder: elem.Label,
					AriaLabel:   elem.Label,
				})
			case "link":
				href := ""
				if elem.Attributes != nil {
					href = elem.Attributes["href"]
				}
				legacyPage.Links = append(legacyPage.Links, LinkModel{
					Text: elem.Label,
					Href: href,
				})
			}
		}

		// Convert data inputs
		for _, input := range page.DataInputs {
			legacyPage.Inputs = append(legacyPage.Inputs, InputModel{
				Type:        input.Type,
				Name:        input.Name,
				Placeholder: input.Placeholder,
				AriaLabel:   input.Label,
			})
		}

		// Convert navigation
		for _, nav := range page.Navigation {
			legacyPage.Links = append(legacyPage.Links, LinkModel{
				Text:       nav.Label,
				Href:       nav.URL,
				IsInternal: !nav.IsExternal,
			})
		}

		legacy.Pages[page.URL] = legacyPage
	}

	// Convert flows
	for _, flow := range m.BusinessFlows {
		legacy.Flows = append(legacy.Flows, BusinessFlow{
			ID:          flow.ID,
			Name:        flow.Name,
			Description: flow.Description,
			Type:        flow.Type,
			Priority:    flow.Priority,
			Steps:       flow.Steps,
		})
	}

	// Update stats
	legacy.Stats.TotalPages = len(legacy.Pages)
	for _, p := range legacy.Pages {
		legacy.Stats.TotalForms += len(p.Forms)
		legacy.Stats.TotalButtons += len(p.Buttons)
		legacy.Stats.TotalLinks += len(p.Links)
		legacy.Stats.TotalInputs += len(p.Inputs)
	}

	return legacy
}

// =============================================================================
// SPECIALIZED AGENTS - Semantic understanding agents
// =============================================================================

// PageUnderstandingAgent determines page purpose through semantic analysis
type PageUnderstandingAgent struct {
	llm    LLMClient
	logger *zap.Logger
}

// NewPageUnderstandingAgent creates a new page understanding agent
func NewPageUnderstandingAgent(llm LLMClient, logger *zap.Logger) *PageUnderstandingAgent {
	return &PageUnderstandingAgent{llm: llm, logger: logger}
}

func (a *PageUnderstandingAgent) Type() AgentType { return AgentPageUnderstanding }

func (a *PageUnderstandingAgent) Analyze(ctx context.Context, input AgentInput) (AgentOutput, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are a semantic web page analyzer. Your task is to understand the PURPOSE and FUNCTION of a web page, NOT by matching keywords but by understanding context, structure, and behavior.`,
		ContextRules: []string{
			"Analyze the DOM structure to understand page layout and purpose",
			"Look for form patterns: input fields grouped together suggest specific functions",
			"Consider visual hierarchy: main content area vs sidebars vs headers",
			"Detect interactive patterns: buttons, links, input combinations",
			"Infer purpose from element relationships, not just labels",
		},
		OutputFormat: `Return JSON:
{
  "page_type": "auth|registration|search|listing|detail|form|dashboard|settings|checkout|landing|error|other",
  "purpose": "Clear description of what this page is for",
  "confidence": 0.95,
  "detected_patterns": ["pattern1", "pattern2"],
  "primary_action": "The main thing a user would do here",
  "secondary_actions": ["other", "possible", "actions"]
}`,
	}

	systemPrompt := prompt.BuildPrompt(input.Context, "")
	userPrompt := fmt.Sprintf("Analyze this page:\nURL: %s\nTitle: %s\n\nHTML (truncated):\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 30000))

	var result struct {
		PageType         string   `json:"page_type"`
		Purpose          string   `json:"purpose"`
		Confidence       float64  `json:"confidence"`
		DetectedPatterns []string `json:"detected_patterns"`
		PrimaryAction    string   `json:"primary_action"`
		SecondaryActions []string `json:"secondary_actions"`
	}

	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	return AgentOutput{
		AgentType: a.Type(),
		Success:   true,
		Data: map[string]interface{}{
			"page_type":         result.PageType,
			"purpose":           result.Purpose,
			"confidence":        result.Confidence,
			"detected_patterns": result.DetectedPatterns,
			"primary_action":    result.PrimaryAction,
			"secondary_actions": result.SecondaryActions,
		},
	}, nil
}

// ElementDiscoveryAgent finds ALL interactive elements semantically
type ElementDiscoveryAgent struct {
	llm    LLMClient
	logger *zap.Logger
}

// NewElementDiscoveryAgent creates a new element discovery agent
func NewElementDiscoveryAgent(llm LLMClient, logger *zap.Logger) *ElementDiscoveryAgent {
	return &ElementDiscoveryAgent{llm: llm, logger: logger}
}

func (a *ElementDiscoveryAgent) Type() AgentType { return AgentElementDiscovery }

func (a *ElementDiscoveryAgent) Analyze(ctx context.Context, input AgentInput) (AgentOutput, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are an expert at finding ALL interactive elements on a web page. You understand that modern SPAs use custom components, and you can detect interactivity from:
- Event handlers (onClick, onSubmit, etc.)
- ARIA roles (role="button", role="link", etc.)
- CSS classes suggesting interactivity
- Data attributes (data-action, data-click, etc.)
- Visual patterns (elements that look clickable)
- Keyboard accessibility (tabindex)`,
		ContextRules: []string{
			"Find EVERY element a user could interact with",
			"Include custom components that behave like standard elements",
			"Generate multiple selector strategies for each element",
			"Detect elements by behavior, not by tag name",
			"Consider hidden/conditional elements that may appear",
		},
		OutputFormat: `Return JSON array of elements:
{
  "elements": [
    {
      "id": "unique-generated-id",
      "type": "button|link|input|dropdown|toggle|tab|accordion|modal-trigger|menu|card|other",
      "purpose": "What this element does when interacted with",
      "label": "Human-readable identifier",
      "selectors": {
        "primary": "Most stable selector",
        "by_testid": "data-testid selector if available",
        "by_role": "ARIA role selector",
        "by_text": "Text-based selector",
        "by_position": "Structural selector as fallback"
      },
      "confidence": 0.95,
      "interaction_type": "click|hover|type|drag|focus",
      "expected_result": "What happens after interaction"
    }
  ]
}`,
	}

	systemPrompt := prompt.BuildPrompt(input.Context, "")
	userPrompt := fmt.Sprintf("Find all interactive elements on this page:\nURL: %s\nTitle: %s\n\nHTML:\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 40000))

	var result struct {
		Elements []SemanticElement `json:"elements"`
	}

	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	return AgentOutput{
		AgentType: a.Type(),
		Success:   true,
		Elements:  result.Elements,
	}, nil
}

// AuthenticationAgent detects ANY auth mechanism regardless of naming
type AuthenticationAgent struct {
	llm    LLMClient
	logger *zap.Logger
}

func (a *AuthenticationAgent) Type() AgentType { return AgentAuthentication }

func (a *AuthenticationAgent) Analyze(ctx context.Context, input AgentInput) (AgentOutput, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are an authentication detection expert. You can identify ANY authentication mechanism regardless of what it's called or what language it's in.

AUTHENTICATION PATTERNS TO DETECT:
1. Traditional login: username/email + password form
2. Registration/Signup: form with email, password, confirm password, name fields
3. SSO buttons: "Sign in with Google/Facebook/GitHub/etc."
4. OAuth flows: Redirect-based authentication
5. Magic link: Email-only login
6. OTP/2FA: One-time password fields
7. Passwordless: WebAuthn, biometric
8. Session indicators: "Welcome [user]", logout buttons, profile links

DO NOT rely on keywords. Instead look for:
- Form structure with credential-like inputs
- Password type inputs
- Submit buttons near credential inputs
- Links to account/profile pages
- OAuth provider logos or buttons`,
		ContextRules: []string{
			"Detect auth by STRUCTURE, not by text labels",
			"Consider international sites: 'Connexion', 'Anmelden', '登录' all mean login",
			"OAuth buttons may just be logos without text",
			"Look for password fields as strong auth indicators",
			"Profile/account icons often indicate logged-in state or auth",
		},
		OutputFormat: `Return JSON:
{
  "has_authentication": true,
  "auth_mechanisms": [
    {
      "type": "credential_form|oauth|sso|magic_link|otp|session_indicator",
      "description": "What kind of auth this is",
      "location": "Where on the page",
      "selectors": {
        "form": "form selector if applicable",
        "username_field": "username/email input selector",
        "password_field": "password input selector",
        "submit_button": "submit button selector",
        "oauth_buttons": ["google", "facebook"]
      },
      "confidence": 0.95
    }
  ],
  "is_logged_in": false,
  "logout_selector": "selector for logout if logged in"
}`,
	}

	systemPrompt := prompt.BuildPrompt(input.Context, "")
	userPrompt := fmt.Sprintf("Detect authentication mechanisms on this page:\nURL: %s\nTitle: %s\n\nHTML:\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 30000))

	var result map[string]interface{}
	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	return AgentOutput{
		AgentType: a.Type(),
		Success:   true,
		Data:      result,
	}, nil
}

// FormAnalysisAgent understands forms semantically
type FormAnalysisAgent struct {
	llm    LLMClient
	logger *zap.Logger
}

func (a *FormAnalysisAgent) Type() AgentType { return AgentFormAnalysis }

func (a *FormAnalysisAgent) Analyze(ctx context.Context, input AgentInput) (AgentOutput, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are a form analysis expert. You understand the PURPOSE of forms by analyzing their structure, field types, and context - not by matching labels.

FORM PURPOSE DETECTION:
- Authentication: email/username + password
- Registration: multiple identity fields + password + confirmation
- Search: single input + submit
- Contact: name + email + message
- Checkout: shipping + payment fields
- Profile: editable user information
- Filters: multiple optional inputs affecting a list
- CRUD: forms for creating/updating data`,
		ContextRules: []string{
			"Infer field purpose from type, name, autocomplete, and position",
			"Consider field groupings and visual hierarchy",
			"Detect validation patterns from attributes and JS",
			"Identify required vs optional fields",
			"Map form flow and multi-step patterns",
		},
		OutputFormat: `Return JSON:
{
  "forms": [
    {
      "id": "form-id",
      "purpose": "what this form is for",
      "type": "auth|registration|search|contact|checkout|profile|filter|crud|other",
      "selectors": {
        "form": "form selector",
        "submit": "submit button selector"
      },
      "fields": [
        {
          "name": "field name or id",
          "purpose": "what this field collects",
          "type": "text|email|password|phone|number|date|select|checkbox|radio|textarea|file",
          "selector": "field selector",
          "required": true,
          "validation": "detected validation rules"
        }
      ],
      "confidence": 0.95
    }
  ]
}`,
	}

	systemPrompt := prompt.BuildPrompt(input.Context, "")
	userPrompt := fmt.Sprintf("Analyze all forms on this page:\nURL: %s\nTitle: %s\n\nHTML:\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 40000))

	var result map[string]interface{}
	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	return AgentOutput{
		AgentType: a.Type(),
		Success:   true,
		Data:      result,
	}, nil
}

// BusinessFlowAgent infers user journeys from page analysis
type BusinessFlowAgent struct {
	llm    LLMClient
	logger *zap.Logger
}

func (a *BusinessFlowAgent) Type() AgentType { return AgentBusinessFlow }

func (a *BusinessFlowAgent) Analyze(ctx context.Context, input AgentInput) (AgentOutput, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are a UX flow analyst. You can infer user journeys and business flows from page structure and navigation. You understand that flows are not just about clicking through pages, but about accomplishing user goals.

COMMON FLOW PATTERNS:
- Onboarding: landing → registration → verification → dashboard
- Authentication: any page → login → protected content
- E-commerce: browse → product → cart → checkout → confirmation
- Search & Filter: search → results → filter → detail
- CRUD: list → create/edit form → submit → updated list
- Settings: dashboard → settings → update → save`,
		ContextRules: []string{
			"Infer flows from navigation structure and CTAs",
			"Consider both happy paths and error handling",
			"Identify critical flows (those that generate value)",
			"Map dependencies between flows",
			"Consider conditional flows based on user state",
		},
		OutputFormat: `Return JSON:
{
  "flows": [
    {
      "name": "Flow name",
      "purpose": "What user accomplishes",
      "type": "auth|onboarding|purchase|crud|search|settings|other",
      "priority": "critical|high|medium|low",
      "confidence": 0.95,
      "steps": [
        "Step 1 description",
        "Step 2 description"
      ],
      "entry_points": ["URLs or selectors where flow starts"],
      "exit_points": ["Success states"],
      "error_states": ["Possible failure points"]
    }
  ]
}`,
	}

	systemPrompt := prompt.BuildPrompt(input.Context, "")

	// Include information about visited pages for flow inference
	pagesInfo := ""
	if input.Context != nil && len(input.Context.VisitedPages) > 0 {
		pagesInfo = fmt.Sprintf("\n\nVisited Pages:\n%s", strings.Join(input.Context.VisitedPages, "\n"))
	}

	userPrompt := fmt.Sprintf("Infer business flows from this page and context:\nURL: %s\nTitle: %s%s\n\nHTML:\n%s",
		input.URL, input.Title, pagesInfo, truncateHTML(input.HTML, 30000))

	var result struct {
		Flows []DetectedFlow `json:"flows"`
	}

	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	return AgentOutput{
		AgentType: a.Type(),
		Success:   true,
		Flows:     result.Flows,
	}, nil
}

// truncateHTML truncates HTML to maxLen while trying to preserve structure
func truncateHTML(html string, maxLen int) string {
	if len(html) <= maxLen {
		return html
	}
	// Try to cut at a tag boundary
	cutPoint := maxLen
	for i := maxLen; i > maxLen-1000 && i > 0; i-- {
		if html[i] == '>' {
			cutPoint = i + 1
			break
		}
	}
	return html[:cutPoint] + "\n<!-- truncated -->"
}

// =============================================================================
// ABA - AUTONOMOUS BUSINESS ANALYST AGENT
// =============================================================================

// AgentABA is the agent type for the Autonomous Business Analyst
const AgentABA AgentType = "autonomous_business_analyst"

// UserStory represents a generated user story
type UserStory struct {
	ID               string             `json:"id"`
	Title            string             `json:"title"`
	AsA              string             `json:"as_a"`              // As a [role]
	IWant            string             `json:"i_want"`            // I want [feature]
	SoThat           string             `json:"so_that"`           // So that [benefit]
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	Priority         string             `json:"priority"`          // critical, high, medium, low
	StoryPoints      int                `json:"story_points"`      // Estimated complexity
	RelatedPages     []string           `json:"related_pages"`     // URLs involved
	TestScenarios    []string           `json:"test_scenarios"`    // Suggested test scenarios
}

// AcceptanceCriterion represents a single acceptance criterion
type AcceptanceCriterion struct {
	Given  string `json:"given"`
	When   string `json:"when"`
	Then   string `json:"then"`
	TestID string `json:"test_id,omitempty"` // Linked test case ID
}

// BusinessRequirement represents an inferred business requirement
type BusinessRequirement struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`    // functional, non-functional, security, performance
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Source      string   `json:"source"`      // How this was inferred (page structure, form, flow, etc.)
	Validation  string   `json:"validation"`  // How to validate this requirement
	UserStories []string `json:"user_stories"` // Related user story IDs
}

// DomainAnalysis represents the ABA's understanding of the business domain
type DomainAnalysis struct {
	Domain           string                  `json:"domain"`            // e-commerce, saas, social, healthcare, etc.
	SubDomain        string                  `json:"sub_domain"`        // More specific categorization
	UserRoles        []UserRole              `json:"user_roles"`        // Detected user types
	BusinessEntities []BusinessEntity        `json:"business_entities"` // Key domain objects
	CoreWorkflows    []CoreWorkflow          `json:"core_workflows"`    // Main business processes
	Competitors      []string                `json:"competitors"`       // Similar known products
	Confidence       float64                 `json:"confidence"`
}

// UserRole represents a detected user role/persona
type UserRole struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	KeyActions  []string `json:"key_actions"`
}

// BusinessEntity represents a key domain object
type BusinessEntity struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`       // product, user, order, etc.
	Attributes []string `json:"attributes"`
	Actions    []string `json:"actions"`    // CRUD operations, etc.
}

// CoreWorkflow represents a main business process
type CoreWorkflow struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Actors      []string `json:"actors"`
	Criticality string   `json:"criticality"` // business-critical, important, nice-to-have
}

// ABAOutput contains the full output of the Autonomous Business Analyst
type ABAOutput struct {
	DomainAnalysis   *DomainAnalysis       `json:"domain_analysis"`
	Requirements     []BusinessRequirement `json:"requirements"`
	UserStories      []UserStory           `json:"user_stories"`
	TestCoverage     map[string][]string   `json:"test_coverage"` // requirement_id -> test_ids
	RiskAssessment   []RiskItem            `json:"risk_assessment"`
	Recommendations  []string              `json:"recommendations"`
}

// RiskItem represents a business/technical risk
type RiskItem struct {
	ID          string `json:"id"`
	Category    string `json:"category"`    // security, usability, performance, compliance
	Description string `json:"description"`
	Impact      string `json:"impact"`      // high, medium, low
	Likelihood  string `json:"likelihood"`  // high, medium, low
	Mitigation  string `json:"mitigation"`
}

// AutonomousBusinessAnalyst infers business requirements and generates user stories
type AutonomousBusinessAnalyst struct {
	llm    LLMClient
	logger *zap.Logger
}

// NewAutonomousBusinessAnalyst creates a new ABA agent
func NewAutonomousBusinessAnalyst(llm LLMClient, logger *zap.Logger) *AutonomousBusinessAnalyst {
	return &AutonomousBusinessAnalyst{llm: llm, logger: logger}
}

func (a *AutonomousBusinessAnalyst) Type() AgentType { return AgentABA }

func (a *AutonomousBusinessAnalyst) Analyze(ctx context.Context, input AgentInput) (AgentOutput, error) {
	// First, analyze the domain
	domainAnalysis, err := a.analyzeDomain(ctx, input)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	// Generate requirements based on domain understanding
	requirements, err := a.inferRequirements(ctx, input, domainAnalysis)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	// Generate user stories with acceptance criteria
	userStories, err := a.generateUserStories(ctx, input, domainAnalysis, requirements)
	if err != nil {
		return AgentOutput{AgentType: a.Type(), Success: false, Error: err.Error()}, err
	}

	// Assess risks
	risks := a.assessRisks(domainAnalysis, requirements)

	output := &ABAOutput{
		DomainAnalysis:  domainAnalysis,
		Requirements:    requirements,
		UserStories:     userStories,
		RiskAssessment:  risks,
		Recommendations: a.generateRecommendations(domainAnalysis, requirements, risks),
	}

	return AgentOutput{
		AgentType: a.Type(),
		Success:   true,
		Data: map[string]interface{}{
			"aba_output": output,
		},
	}, nil
}

// analyzeDomain determines the business domain and context
func (a *AutonomousBusinessAnalyst) analyzeDomain(ctx context.Context, input AgentInput) (*DomainAnalysis, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are an expert Business Analyst with deep knowledge of various industry domains.
Your task is to analyze a web application and determine its business domain, user roles, and key entities.

You excel at:
1. Identifying the type of business from UI patterns and terminology
2. Inferring user personas from navigation and access patterns
3. Detecting core business entities from forms, lists, and data displays
4. Understanding industry-specific workflows and requirements`,
		ContextRules: []string{
			"Analyze page structure to identify the business model",
			"Look for domain-specific terminology (cart, patient, invoice, etc.)",
			"Identify user segmentation from menu structures and permissions",
			"Detect CRUD patterns that reveal key business entities",
			"Consider regulatory requirements based on domain (healthcare = HIPAA, finance = SOX)",
		},
		OutputFormat: `Return JSON:
{
  "domain": "e-commerce|saas|healthcare|finance|education|social|media|travel|real-estate|other",
  "sub_domain": "more specific categorization",
  "user_roles": [
    {
      "name": "role name",
      "description": "what this user type does",
      "permissions": ["list", "of", "capabilities"],
      "key_actions": ["primary", "user", "actions"]
    }
  ],
  "business_entities": [
    {
      "name": "entity name (e.g., Product, Order, User)",
      "type": "product|user|transaction|content|configuration",
      "attributes": ["key", "attributes"],
      "actions": ["create", "read", "update", "delete"]
    }
  ],
  "core_workflows": [
    {
      "name": "workflow name",
      "description": "what this workflow accomplishes",
      "steps": ["step1", "step2"],
      "actors": ["who performs this"],
      "criticality": "business-critical|important|nice-to-have"
    }
  ],
  "competitors": ["known similar products"],
  "confidence": 0.95
}`,
	}

	systemPrompt := prompt.BuildPrompt(input.Context, "")
	userPrompt := fmt.Sprintf("Analyze the business domain of this application:\nURL: %s\nTitle: %s\n\nHTML:\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 30000))

	var result DomainAnalysis
	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return nil, fmt.Errorf("domain analysis failed: %w", err)
	}

	return &result, nil
}

// inferRequirements generates business requirements from the analysis
func (a *AutonomousBusinessAnalyst) inferRequirements(ctx context.Context, input AgentInput, domain *DomainAnalysis) ([]BusinessRequirement, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are a Senior Business Analyst creating requirements documentation.
Based on the analyzed web application and its domain, generate comprehensive business requirements.

Requirements should be:
1. SMART: Specific, Measurable, Achievable, Relevant, Time-bound
2. Testable: Each requirement should be verifiable
3. Traceable: Can be linked to user stories and test cases
4. Prioritized: Based on business value and risk`,
		ContextRules: []string{
			"Infer functional requirements from UI elements and flows",
			"Derive non-functional requirements from domain (performance, security, compliance)",
			"Consider accessibility requirements (WCAG compliance)",
			"Include integration requirements if external services are detected",
			"Add data validation requirements from form patterns",
		},
		OutputFormat: `Return JSON:
{
  "requirements": [
    {
      "id": "REQ-001",
      "category": "functional|non-functional|security|performance|compliance|accessibility",
      "title": "Requirement title",
      "description": "Detailed description of the requirement",
      "priority": "critical|high|medium|low",
      "source": "How this was inferred (e.g., login form detected, checkout flow)",
      "validation": "How to verify this requirement is met"
    }
  ]
}`,
	}

	domainContext := fmt.Sprintf("Domain: %s (%s)\nUser Roles: %v\nCore Workflows: %v",
		domain.Domain, domain.SubDomain,
		extractNames(domain.UserRoles),
		extractWorkflowNames(domain.CoreWorkflows))

	systemPrompt := prompt.BuildPrompt(input.Context, domainContext)
	userPrompt := fmt.Sprintf("Generate business requirements for:\nURL: %s\nTitle: %s\n\nHTML:\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 25000))

	var result struct {
		Requirements []BusinessRequirement `json:"requirements"`
	}
	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return nil, fmt.Errorf("requirements generation failed: %w", err)
	}

	return result.Requirements, nil
}

// generateUserStories creates user stories with acceptance criteria
func (a *AutonomousBusinessAnalyst) generateUserStories(ctx context.Context, input AgentInput, domain *DomainAnalysis, requirements []BusinessRequirement) ([]UserStory, error) {
	prompt := &MetaPrompt{
		BaseInstruction: `You are an Agile Coach and Business Analyst creating user stories.
Generate comprehensive user stories in standard format with detailed acceptance criteria.

Each user story should:
1. Follow "As a [role], I want [feature], so that [benefit]" format
2. Include Gherkin-style acceptance criteria (Given/When/Then)
3. Be sized appropriately (estimable story points)
4. Map to specific test scenarios`,
		ContextRules: []string{
			"Create stories for each user role identified",
			"Cover both happy paths and error scenarios",
			"Include edge cases and boundary conditions",
			"Consider accessibility user stories",
			"Add security-focused stories where appropriate",
		},
		OutputFormat: `Return JSON:
{
  "user_stories": [
    {
      "id": "US-001",
      "title": "Story title",
      "as_a": "user role",
      "i_want": "desired feature or capability",
      "so_that": "benefit or value gained",
      "acceptance_criteria": [
        {
          "given": "initial context/state",
          "when": "action performed",
          "then": "expected outcome"
        }
      ],
      "priority": "critical|high|medium|low",
      "story_points": 3,
      "related_pages": ["URLs involved"],
      "test_scenarios": ["scenario descriptions for testing"]
    }
  ]
}`,
	}

	// Build context from domain and requirements
	reqSummary := ""
	for i, req := range requirements {
		if i < 5 { // Limit to avoid token overflow
			reqSummary += fmt.Sprintf("- %s: %s\n", req.ID, req.Title)
		}
	}

	domainContext := fmt.Sprintf("Domain: %s\nUser Roles: %v\nKey Requirements:\n%s",
		domain.Domain, extractNames(domain.UserRoles), reqSummary)

	systemPrompt := prompt.BuildPrompt(input.Context, domainContext)
	userPrompt := fmt.Sprintf("Generate user stories for:\nURL: %s\nTitle: %s\n\nHTML:\n%s",
		input.URL, input.Title, truncateHTML(input.HTML, 25000))

	var result struct {
		UserStories []UserStory `json:"user_stories"`
	}
	_, err := a.llm.CompleteJSON(ctx, systemPrompt, userPrompt, &result)
	if err != nil {
		return nil, fmt.Errorf("user story generation failed: %w", err)
	}

	return result.UserStories, nil
}

// assessRisks identifies business and technical risks
func (a *AutonomousBusinessAnalyst) assessRisks(domain *DomainAnalysis, requirements []BusinessRequirement) []RiskItem {
	var risks []RiskItem
	riskID := 1

	// Domain-specific risks
	switch domain.Domain {
	case "e-commerce":
		risks = append(risks, RiskItem{
			ID:          fmt.Sprintf("RISK-%03d", riskID),
			Category:    "security",
			Description: "Payment data handling requires PCI-DSS compliance",
			Impact:      "high",
			Likelihood:  "medium",
			Mitigation:  "Implement PCI-DSS compliant payment processing",
		})
		riskID++
	case "healthcare":
		risks = append(risks, RiskItem{
			ID:          fmt.Sprintf("RISK-%03d", riskID),
			Category:    "compliance",
			Description: "Patient data requires HIPAA compliance",
			Impact:      "high",
			Likelihood:  "high",
			Mitigation:  "Ensure all PHI handling meets HIPAA requirements",
		})
		riskID++
	case "finance":
		risks = append(risks, RiskItem{
			ID:          fmt.Sprintf("RISK-%03d", riskID),
			Category:    "compliance",
			Description: "Financial data requires SOX/regulatory compliance",
			Impact:      "high",
			Likelihood:  "medium",
			Mitigation:  "Implement audit trails and access controls",
		})
		riskID++
	}

	// Check for auth-related requirements
	for _, req := range requirements {
		if req.Category == "security" {
			risks = append(risks, RiskItem{
				ID:          fmt.Sprintf("RISK-%03d", riskID),
				Category:    "security",
				Description: fmt.Sprintf("Security requirement: %s", req.Title),
				Impact:      "high",
				Likelihood:  "medium",
				Mitigation:  req.Validation,
			})
			riskID++
		}
	}

	// Add generic risks
	risks = append(risks, RiskItem{
		ID:          fmt.Sprintf("RISK-%03d", riskID),
		Category:    "usability",
		Description: "Accessibility compliance for users with disabilities",
		Impact:      "medium",
		Likelihood:  "high",
		Mitigation:  "Conduct WCAG 2.1 AA compliance testing",
	})

	return risks
}

// generateRecommendations creates actionable recommendations
func (a *AutonomousBusinessAnalyst) generateRecommendations(domain *DomainAnalysis, requirements []BusinessRequirement, risks []RiskItem) []string {
	var recommendations []string

	// Domain-specific recommendations
	switch domain.Domain {
	case "e-commerce":
		recommendations = append(recommendations,
			"Implement comprehensive checkout flow testing including payment failures",
			"Add inventory management test scenarios",
			"Test cart persistence across sessions")
	case "saas":
		recommendations = append(recommendations,
			"Test subscription and billing workflows",
			"Verify multi-tenant data isolation",
			"Test user provisioning and deprovisioning")
	}

	// Risk-based recommendations
	highRisks := 0
	for _, risk := range risks {
		if risk.Impact == "high" {
			highRisks++
		}
	}
	if highRisks > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Address %d high-impact risks before production deployment", highRisks))
	}

	// General recommendations
	recommendations = append(recommendations,
		"Implement automated regression testing for critical user flows",
		"Add visual regression testing for UI consistency",
		"Configure self-healing for selector-based test failures")

	return recommendations
}

// Helper functions for ABA
func extractNames(roles []UserRole) []string {
	names := make([]string, len(roles))
	for i, r := range roles {
		names[i] = r.Name
	}
	return names
}

func extractWorkflowNames(workflows []CoreWorkflow) []string {
	names := make([]string, len(workflows))
	for i, w := range workflows {
		names[i] = w.Name
	}
	return names
}
