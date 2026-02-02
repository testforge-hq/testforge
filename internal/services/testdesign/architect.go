package testdesign

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/testforge/testforge/internal/llm"
	"github.com/testforge/testforge/internal/services/discovery"
)

// TestArchitect designs comprehensive test suites using LLM
type TestArchitect struct {
	client     *llm.ClaudeClient
	config     ArchitectConfig
	validator  *Validator
	totalUsage llm.Usage
}

// ArchitectConfig configures the test architect
type ArchitectConfig struct {
	// Generation settings
	MaxTestsPerFeature int
	MaxRetries         int
	ChunkSize          int // Number of pages to process per LLM call

	// Test type weights (influence distribution)
	TestTypeWeights map[TestType]float64

	// Include flags
	IncludeSecurityTests      bool
	IncludeAccessibilityTests bool
	IncludePerformanceTests   bool

	// Compliance
	ComplianceFrameworks []string // e.g., ["WCAG", "SOC2", "GDPR"]
}

// DefaultArchitectConfig returns sensible defaults
func DefaultArchitectConfig() ArchitectConfig {
	return ArchitectConfig{
		MaxTestsPerFeature: 20,
		MaxRetries:         3,
		ChunkSize:          3,
		TestTypeWeights: map[TestType]float64{
			TestTypeSmoke:         1.0,
			TestTypeRegression:    1.0,
			TestTypeE2E:           0.8,
			TestTypeNegative:      0.7,
			TestTypeBoundary:      0.5,
			TestTypeSecurity:      0.6,
			TestTypeAccessibility: 0.5,
			TestTypePerformance:   0.3,
		},
		IncludeSecurityTests:      true,
		IncludeAccessibilityTests: true,
		IncludePerformanceTests:   false,
		ComplianceFrameworks:      []string{"WCAG"},
	}
}

// NewTestArchitect creates a new test architect
func NewTestArchitect(client *llm.ClaudeClient, config ArchitectConfig) *TestArchitect {
	return &TestArchitect{
		client:    client,
		config:    config,
		validator: NewValidator(),
	}
}

// DesignInput contains input for test suite design
type DesignInput struct {
	AppModel    *discovery.AppModel
	ProjectID   string
	ProjectName string
	BaseURL     string
	Roles       []string
	Environment string
}

// DesignOutput contains the result of test suite design
type DesignOutput struct {
	Suite           *TestSuite
	TokensUsed      int
	InputTokens     int
	OutputTokens    int
	EstimatedCost   float64
	GenerationTime  time.Duration
	Chunks          int
	ValidationWarns []string
}

// DesignTestSuite generates a comprehensive test suite from the app model
func (a *TestArchitect) DesignTestSuite(ctx context.Context, input DesignInput) (*DesignOutput, error) {
	startTime := time.Now()

	// Initialize the test suite
	suite := &TestSuite{
		ID:          uuid.New().String(),
		Name:        fmt.Sprintf("%s Test Suite", input.ProjectName),
		Description: fmt.Sprintf("Comprehensive test suite for %s", input.ProjectName),
		ProjectID:   input.ProjectID,
		Version:     "1.0.0",
		Features:    []Feature{},
		GlobalConfig: TestConfig{
			BaseURL:        input.BaseURL,
			DefaultTimeout: "30s",
			RetryConfig: RetryConfig{
				MaxAttempts:       3,
				Delay:             "1s",
				BackoffMultiplier: 2.0,
			},
			BrowserSettings: BrowserSettings{
				Browser:        "chromium",
				Headless:       true,
				ViewportWidth:  1920,
				ViewportHeight: 1080,
				DeviceScale:    1,
			},
			Screenshots: ScreenshotConfig{
				OnFailure:   true,
				OnSuccess:   false,
				FullPage:    true,
				Format:      "png",
				Quality:     90,
				StoragePath: "screenshots/",
			},
			Environment: input.Environment,
			Variables:   make(map[string]string),
		},
		Roles:      a.buildRoles(input.Roles),
		Compliance: ComplianceMapping{},
		Metadata: SuiteMetadata{
			GeneratedAt: time.Now(),
			GeneratedBy: "TestForge AI Architect",
			ModelUsed:   a.client.GetModel(),
			AppModelRef: input.AppModel.ID,
		},
	}

	// Group pages by type for chunked processing
	pageGroups := a.groupPagesByType(input.AppModel.Pages)

	chunks := 0
	var allWarnings []string

	// Process each page group
	for pageType, pages := range pageGroups {
		features, err := a.generateFeaturesForPages(ctx, pageType, pages, input.AppModel.Pages)
		if err != nil {
			// Log but continue with other groups
			allWarnings = append(allWarnings, fmt.Sprintf("Failed to generate tests for %s pages: %v", pageType, err))
			continue
		}
		suite.Features = append(suite.Features, features...)
		chunks++
	}

	// Generate E2E tests for business flows
	if len(input.AppModel.Flows) > 0 {
		flowFeatures, err := a.generateFlowTests(ctx, input.AppModel.Flows, input.AppModel.Pages)
		if err != nil {
			allWarnings = append(allWarnings, fmt.Sprintf("Failed to generate flow tests: %v", err))
		} else {
			suite.Features = append(suite.Features, flowFeatures...)
			chunks++
		}
	}

	// Generate security tests if enabled
	if a.config.IncludeSecurityTests {
		securityFeature, err := a.generateSecurityTests(ctx, input.AppModel)
		if err != nil {
			allWarnings = append(allWarnings, fmt.Sprintf("Security tests: %v", err))
		} else if securityFeature != nil {
			suite.Features = append(suite.Features, *securityFeature)
			chunks++
		}
	}

	// Generate accessibility tests if enabled
	if a.config.IncludeAccessibilityTests {
		a11yFeature, err := a.generateAccessibilityTests(ctx, input.AppModel)
		if err != nil {
			allWarnings = append(allWarnings, fmt.Sprintf("Accessibility tests: %v", err))
		} else if a11yFeature != nil {
			suite.Features = append(suite.Features, *a11yFeature)
			chunks++
		}
	}

	// Validate the complete suite
	validationWarnings := a.validator.ValidateSuite(suite)
	allWarnings = append(allWarnings, validationWarnings...)

	// Calculate stats
	suite.CalculateStats()

	// Update metadata
	duration := time.Since(startTime)
	suite.Metadata.TokensUsed = int(a.totalUsage.InputTokens + a.totalUsage.OutputTokens)
	suite.Metadata.InputTokens = int(a.totalUsage.InputTokens)
	suite.Metadata.OutputTokens = int(a.totalUsage.OutputTokens)
	suite.Metadata.EstimatedCost = a.calculateCost()
	suite.Metadata.GenerationTime = duration.String()
	suite.Metadata.Chunks = chunks

	return &DesignOutput{
		Suite:           suite,
		TokensUsed:      suite.Metadata.TokensUsed,
		InputTokens:     suite.Metadata.InputTokens,
		OutputTokens:    suite.Metadata.OutputTokens,
		EstimatedCost:   suite.Metadata.EstimatedCost,
		GenerationTime:  duration,
		Chunks:          chunks,
		ValidationWarns: allWarnings,
	}, nil
}

// groupPagesByType groups pages for chunked processing
func (a *TestArchitect) groupPagesByType(pages map[string]*discovery.PageModel) map[string][]*discovery.PageModel {
	groups := make(map[string][]*discovery.PageModel)

	for _, page := range pages {
		pageType := page.PageType
		if pageType == "" {
			pageType = "general"
		}
		groups[pageType] = append(groups[pageType], page)
	}

	return groups
}

// generateFeaturesForPages generates features for a group of pages
func (a *TestArchitect) generateFeaturesForPages(ctx context.Context, pageType string, pages []*discovery.PageModel, allPages map[string]*discovery.PageModel) ([]Feature, error) {
	var features []Feature

	// Process in chunks
	for i := 0; i < len(pages); i += a.config.ChunkSize {
		end := i + a.config.ChunkSize
		if end > len(pages) {
			end = len(pages)
		}
		chunk := pages[i:end]

		for _, page := range chunk {
			feature, err := a.generateFeatureForPage(ctx, page, allPages)
			if err != nil {
				continue // Skip this page on error
			}
			if feature != nil {
				features = append(features, *feature)
			}
		}
	}

	return features, nil
}

// generateFeatureForPage generates a feature for a single page
func (a *TestArchitect) generateFeatureForPage(ctx context.Context, page *discovery.PageModel, allPages map[string]*discovery.PageModel) (*Feature, error) {
	// Determine test types based on page characteristics
	testTypes := a.determineTestTypes(page)

	// Build prompts
	pageAnalysis := FeaturePrompt(page, allPages)
	featureName := a.deriveFeatureName(page)
	prompt := TestGenerationPrompt(featureName, pageAnalysis, testTypes)

	// Call LLM
	var result struct {
		Feature Feature `json:"feature"`
	}

	usage, err := a.client.CompleteJSON(ctx, SystemPrompt(), prompt, &result)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Track usage
	if usage != nil {
		a.totalUsage.InputTokens += usage.InputTokens
		a.totalUsage.OutputTokens += usage.OutputTokens
	}

	// Enrich feature with metadata
	feature := &result.Feature
	if feature.ID == "" {
		feature.ID = uuid.New().String()
	}
	feature.BaseURL = page.URL

	return feature, nil
}

// generateFlowTests generates E2E tests for business flows
func (a *TestArchitect) generateFlowTests(ctx context.Context, flows []discovery.BusinessFlow, pages map[string]*discovery.PageModel) ([]Feature, error) {
	var features []Feature

	for _, flow := range flows {
		prompt := FlowTestPrompt(&flow, pages)

		var result struct {
			Feature Feature `json:"feature"`
		}

		usage, err := a.client.CompleteJSON(ctx, SystemPrompt(), prompt, &result)
		if err != nil {
			continue // Skip this flow on error
		}

		if usage != nil {
			a.totalUsage.InputTokens += usage.InputTokens
			a.totalUsage.OutputTokens += usage.OutputTokens
		}

		feature := &result.Feature
		if feature.ID == "" {
			feature.ID = uuid.New().String()
		}
		feature.Tags = append(feature.Tags, "e2e", "flow")
		features = append(features, *feature)
	}

	return features, nil
}

// generateSecurityTests generates security-focused tests
func (a *TestArchitect) generateSecurityTests(ctx context.Context, appModel *discovery.AppModel) (*Feature, error) {
	// Find pages with forms or auth
	var securityPages []*discovery.PageModel
	for _, page := range appModel.Pages {
		if len(page.Forms) > 0 || page.HasAuth {
			securityPages = append(securityPages, page)
		}
	}

	if len(securityPages) == 0 {
		return nil, nil // No security-relevant pages
	}

	// Use the first page with forms for security testing prompt
	var sb strings.Builder
	sb.WriteString("# Security Test Suite\n\n")
	for _, page := range securityPages {
		sb.WriteString(SecurityTestPrompt(page))
		sb.WriteString("\n---\n")
	}

	var result struct {
		Feature Feature `json:"feature"`
	}

	usage, err := a.client.CompleteJSON(ctx, SystemPrompt(), sb.String(), &result)
	if err != nil {
		return nil, err
	}

	if usage != nil {
		a.totalUsage.InputTokens += usage.InputTokens
		a.totalUsage.OutputTokens += usage.OutputTokens
	}

	feature := &result.Feature
	feature.ID = "security-tests"
	feature.Name = "Security Test Suite"
	feature.Tags = []string{"security", "owasp"}
	feature.Priority = PriorityCritical

	return feature, nil
}

// generateAccessibilityTests generates accessibility-focused tests
func (a *TestArchitect) generateAccessibilityTests(ctx context.Context, appModel *discovery.AppModel) (*Feature, error) {
	var sb strings.Builder
	sb.WriteString("# Accessibility Test Suite\n\n")

	count := 0
	for _, page := range appModel.Pages {
		if count >= 5 { // Limit to 5 pages for a11y tests
			break
		}
		sb.WriteString(AccessibilityTestPrompt(page))
		sb.WriteString("\n---\n")
		count++
	}

	var result struct {
		Feature Feature `json:"feature"`
	}

	usage, err := a.client.CompleteJSON(ctx, SystemPrompt(), sb.String(), &result)
	if err != nil {
		return nil, err
	}

	if usage != nil {
		a.totalUsage.InputTokens += usage.InputTokens
		a.totalUsage.OutputTokens += usage.OutputTokens
	}

	feature := &result.Feature
	feature.ID = "accessibility-tests"
	feature.Name = "Accessibility Test Suite"
	feature.Tags = []string{"accessibility", "wcag", "a11y"}
	feature.Priority = PriorityHigh

	return feature, nil
}

// determineTestTypes determines which test types to generate based on page characteristics
func (a *TestArchitect) determineTestTypes(page *discovery.PageModel) []TestType {
	types := []TestType{TestTypeSmoke, TestTypeRegression}

	// Forms suggest negative and boundary tests
	if len(page.Forms) > 0 {
		types = append(types, TestTypeNegative, TestTypeBoundary)
	}

	// Auth pages need security tests
	if page.HasAuth || strings.Contains(strings.ToLower(page.URL), "login") ||
		strings.Contains(strings.ToLower(page.URL), "auth") {
		types = append(types, TestTypeSecurity)
	}

	// All pages get accessibility tests
	if a.config.IncludeAccessibilityTests {
		types = append(types, TestTypeAccessibility)
	}

	return types
}

// deriveFeatureName derives a feature name from a page
func (a *TestArchitect) deriveFeatureName(page *discovery.PageModel) string {
	if page.Title != "" && page.Title != "Untitled" {
		return page.Title
	}

	// Extract from URL path
	path := strings.TrimPrefix(page.URL, "https://")
	path = strings.TrimPrefix(path, "http://")
	parts := strings.Split(path, "/")

	if len(parts) > 1 {
		lastPart := parts[len(parts)-1]
		if lastPart != "" {
			return strings.Title(strings.ReplaceAll(lastPart, "-", " "))
		}
	}

	return page.PageType + " Page"
}

// buildRoles creates test roles from input
func (a *TestArchitect) buildRoles(roleNames []string) []TestRole {
	if len(roleNames) == 0 {
		roleNames = []string{"anonymous", "user"}
	}

	roles := make([]TestRole, len(roleNames))
	for i, name := range roleNames {
		roles[i] = TestRole{
			ID:          fmt.Sprintf("role-%s", strings.ToLower(name)),
			Name:        name,
			Description: fmt.Sprintf("Test role for %s users", name),
			Permissions: []string{},
			TestData:    make(map[string]string),
		}
	}

	return roles
}

// calculateCost estimates cost based on Claude pricing
func (a *TestArchitect) calculateCost() float64 {
	// Claude Sonnet pricing: $3/1M input, $15/1M output
	inputCost := float64(a.totalUsage.InputTokens) / 1000000 * 3.00
	outputCost := float64(a.totalUsage.OutputTokens) / 1000000 * 15.00
	return inputCost + outputCost
}

// GenerateTestSuiteJSON generates the test suite and returns it as JSON
func (a *TestArchitect) GenerateTestSuiteJSON(ctx context.Context, input DesignInput) ([]byte, *DesignOutput, error) {
	output, err := a.DesignTestSuite(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	jsonData, err := json.MarshalIndent(output.Suite, "", "  ")
	if err != nil {
		return nil, output, fmt.Errorf("marshaling suite: %w", err)
	}

	return jsonData, output, nil
}
