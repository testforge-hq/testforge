package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// SuggestionType represents the type of suggestion
type SuggestionType string

const (
	// SuggestionTypeTest suggests new test cases
	SuggestionTypeTest SuggestionType = "test"
	// SuggestionTypeHealing suggests healing strategies
	SuggestionTypeHealing SuggestionType = "healing"
	// SuggestionTypeOptimization suggests test optimizations
	SuggestionTypeOptimization SuggestionType = "optimization"
	// SuggestionTypeCoverage suggests coverage improvements
	SuggestionTypeCoverage SuggestionType = "coverage"
	// SuggestionTypeFlow suggests user flow tests
	SuggestionTypeFlow SuggestionType = "flow"
)

// SuggestionPriority represents the priority of a suggestion
type SuggestionPriority string

const (
	PriorityHigh   SuggestionPriority = "high"
	PriorityMedium SuggestionPriority = "medium"
	PriorityLow    SuggestionPriority = "low"
)

// Suggestion represents an AI-powered suggestion
type Suggestion struct {
	ID          uuid.UUID          `json:"id"`
	Type        SuggestionType     `json:"type"`
	Priority    SuggestionPriority `json:"priority"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Rationale   string             `json:"rationale"`
	Action      SuggestedAction    `json:"action"`
	Confidence  float64            `json:"confidence"`
	Impact      string             `json:"impact"`
	CreatedAt   time.Time          `json:"created_at"`
}

// SuggestedAction represents the action to take for a suggestion
type SuggestedAction struct {
	ActionType string                 `json:"action_type"` // "add_test", "modify_selector", "add_flow", "optimize"
	Target     string                 `json:"target"`      // What to modify
	Details    map[string]interface{} `json:"details"`     // Action-specific details
	Code       string                 `json:"code,omitempty"` // Generated code if applicable
}

// SuggestionEngine provides AI-powered suggestions
type SuggestionEngine struct {
	db         *sqlx.DB
	logger     *zap.Logger
	kb         *KnowledgeBase
	patterns   *PatternRepository
}

// NewSuggestionEngine creates a new suggestion engine
func NewSuggestionEngine(db *sqlx.DB, kb *KnowledgeBase, patterns *PatternRepository, logger *zap.Logger) *SuggestionEngine {
	return &SuggestionEngine{
		db:       db,
		logger:   logger,
		kb:       kb,
		patterns: patterns,
	}
}

// GenerateSuggestions generates suggestions for a project
func (se *SuggestionEngine) GenerateSuggestions(ctx context.Context, req SuggestionRequest) ([]Suggestion, error) {
	var suggestions []Suggestion

	// Generate test suggestions
	if req.IncludeTests {
		testSuggestions, err := se.generateTestSuggestions(ctx, req)
		if err != nil {
			se.logger.Warn("failed to generate test suggestions", zap.Error(err))
		} else {
			suggestions = append(suggestions, testSuggestions...)
		}
	}

	// Generate healing suggestions
	if req.IncludeHealing {
		healingSuggestions, err := se.generateHealingSuggestions(ctx, req)
		if err != nil {
			se.logger.Warn("failed to generate healing suggestions", zap.Error(err))
		} else {
			suggestions = append(suggestions, healingSuggestions...)
		}
	}

	// Generate optimization suggestions
	if req.IncludeOptimizations {
		optSuggestions, err := se.generateOptimizationSuggestions(ctx, req)
		if err != nil {
			se.logger.Warn("failed to generate optimization suggestions", zap.Error(err))
		} else {
			suggestions = append(suggestions, optSuggestions...)
		}
	}

	// Generate coverage suggestions
	if req.IncludeCoverage {
		covSuggestions, err := se.generateCoverageSuggestions(ctx, req)
		if err != nil {
			se.logger.Warn("failed to generate coverage suggestions", zap.Error(err))
		} else {
			suggestions = append(suggestions, covSuggestions...)
		}
	}

	// Sort by priority and confidence
	sort.Slice(suggestions, func(i, j int) bool {
		if priorityScore(suggestions[i].Priority) != priorityScore(suggestions[j].Priority) {
			return priorityScore(suggestions[i].Priority) > priorityScore(suggestions[j].Priority)
		}
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	// Limit results
	if req.Limit > 0 && len(suggestions) > req.Limit {
		suggestions = suggestions[:req.Limit]
	}

	return suggestions, nil
}

// SuggestionRequest represents a request for suggestions
type SuggestionRequest struct {
	TenantID             uuid.UUID `json:"tenant_id"`
	ProjectID            uuid.UUID `json:"project_id"`
	Industry             string    `json:"industry"`
	AppType              string    `json:"app_type"` // "e-commerce", "saas", "marketing", etc.
	CurrentTests         []TestSummary `json:"current_tests"`
	RecentFailures       []FailureSummary `json:"recent_failures"`
	PagesCovered         []string  `json:"pages_covered"`
	IncludeTests         bool      `json:"include_tests"`
	IncludeHealing       bool      `json:"include_healing"`
	IncludeOptimizations bool      `json:"include_optimizations"`
	IncludeCoverage      bool      `json:"include_coverage"`
	Limit                int       `json:"limit"`
}

// TestSummary summarizes an existing test
type TestSummary struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	FlowType    string    `json:"flow_type"`
	Steps       int       `json:"steps"`
	PassRate    float64   `json:"pass_rate"`
	LastRun     time.Time `json:"last_run"`
	Selectors   []string  `json:"selectors"`
}

// FailureSummary summarizes a test failure
type FailureSummary struct {
	TestID       uuid.UUID `json:"test_id"`
	TestName     string    `json:"test_name"`
	FailureType  string    `json:"failure_type"` // "selector", "timeout", "assertion", "network"
	Selector     string    `json:"selector,omitempty"`
	ErrorMessage string    `json:"error_message"`
	OccurredAt   time.Time `json:"occurred_at"`
	Frequency    int       `json:"frequency"`
}

// SuggestHealing returns healing suggestions for a broken selector
func (se *SuggestionEngine) SuggestHealing(ctx context.Context, req HealingSuggestionRequest) ([]Suggestion, error) {
	// Get healing patterns from knowledge base
	strategies, err := se.patterns.FindHealingStrategies(ctx, HealingRequest{
		OriginalSelector: req.BrokenSelector,
		ElementType:      req.ElementType,
		Framework:        req.Framework,
		PageContext:      req.PageContext,
	})
	if err != nil {
		return nil, fmt.Errorf("finding healing strategies: %w", err)
	}

	var suggestions []Suggestion

	for i, strategy := range strategies {
		suggestion := Suggestion{
			ID:          uuid.New(),
			Type:        SuggestionTypeHealing,
			Priority:    se.strategyPriority(strategy.Confidence),
			Title:       fmt.Sprintf("Try %s healing strategy", strategy.Strategy),
			Description: strategy.Explanation,
			Rationale:   fmt.Sprintf("Based on %d similar successful healings with %.0f%% success rate", i+1, strategy.Confidence*100),
			Action: SuggestedAction{
				ActionType: "modify_selector",
				Target:     req.BrokenSelector,
				Details: map[string]interface{}{
					"strategy":    strategy.Strategy,
					"new_selector": strategy.Selector,
					"pattern_hash": strategy.PatternHash,
				},
			},
			Confidence: strategy.Confidence,
			Impact:     "Fixes broken test",
			CreatedAt:  time.Now(),
		}

		suggestions = append(suggestions, suggestion)

		// Limit to top 5 healing suggestions
		if len(suggestions) >= 5 {
			break
		}
	}

	return suggestions, nil
}

// HealingSuggestionRequest represents a request for healing suggestions
type HealingSuggestionRequest struct {
	BrokenSelector string            `json:"broken_selector"`
	ElementType    string            `json:"element_type"`
	Framework      string            `json:"framework"`
	PageContext    map[string]string `json:"page_context"`
	PageHTML       string            `json:"page_html,omitempty"`
}

// SuggestFlows returns suggested user flows based on industry patterns
func (se *SuggestionEngine) SuggestFlows(ctx context.Context, industry string, existingFlows []string) ([]Suggestion, error) {
	// Get industry templates
	templates, err := se.patterns.GetIndustryTemplates(ctx, industry)
	if err != nil {
		return nil, fmt.Errorf("getting templates: %w", err)
	}

	// Get flow patterns from knowledge base
	patterns, err := se.kb.QueryFlowPatterns(ctx, "", industry, 20)
	if err != nil {
		return nil, fmt.Errorf("querying flow patterns: %w", err)
	}

	var suggestions []Suggestion

	// Suggest missing standard flows from templates
	for _, template := range templates {
		if !contains(existingFlows, template.FlowType) {
			suggestion := Suggestion{
				ID:          uuid.New(),
				Type:        SuggestionTypeFlow,
				Priority:    PriorityHigh,
				Title:       fmt.Sprintf("Add %s test flow", template.Name),
				Description: template.Description,
				Rationale:   fmt.Sprintf("Standard %s flow for %s applications", template.FlowType, industry),
				Action: SuggestedAction{
					ActionType: "add_flow",
					Target:     template.FlowType,
					Details: map[string]interface{}{
						"template_id": template.ID,
						"steps":       template.Steps,
						"variables":   template.Variables,
					},
				},
				Confidence: 0.9,
				Impact:     "Increases test coverage",
				CreatedAt:  time.Now(),
			}
			suggestions = append(suggestions, suggestion)
		}
	}

	// Suggest flows based on learned patterns
	for _, pattern := range patterns {
		if !contains(existingFlows, pattern.FlowType) && pattern.SuccessRate > 0.7 {
			suggestion := Suggestion{
				ID:          uuid.New(),
				Type:        SuggestionTypeFlow,
				Priority:    PriorityMedium,
				Title:       fmt.Sprintf("Consider adding %s flow", pattern.FlowType),
				Description: fmt.Sprintf("Common user journey in %s applications", industry),
				Rationale:   fmt.Sprintf("Based on patterns from %d similar applications with %.0f%% success rate", len(pattern.CommonVariations), pattern.SuccessRate*100),
				Action: SuggestedAction{
					ActionType: "add_flow",
					Target:     pattern.FlowType,
					Details: map[string]interface{}{
						"steps": pattern.Steps,
					},
				},
				Confidence: pattern.SuccessRate,
				Impact:     "Covers common user journey",
				CreatedAt:  time.Now(),
			}
			suggestions = append(suggestions, suggestion)
		}
	}

	return suggestions, nil
}

// Private methods

func (se *SuggestionEngine) generateTestSuggestions(ctx context.Context, req SuggestionRequest) ([]Suggestion, error) {
	var suggestions []Suggestion

	// Find flows that are commonly tested but missing
	flowPatterns, err := se.kb.QueryFlowPatterns(ctx, "", req.Industry, 20)
	if err != nil {
		return nil, err
	}

	coveredFlows := make(map[string]bool)
	for _, test := range req.CurrentTests {
		coveredFlows[test.FlowType] = true
	}

	for _, pattern := range flowPatterns {
		if !coveredFlows[pattern.FlowType] && pattern.SuccessRate > 0.6 {
			suggestions = append(suggestions, Suggestion{
				ID:          uuid.New(),
				Type:        SuggestionTypeTest,
				Priority:    PriorityHigh,
				Title:       fmt.Sprintf("Add test for %s flow", pattern.FlowType),
				Description: fmt.Sprintf("This flow is commonly tested in %s applications", req.Industry),
				Rationale:   fmt.Sprintf("%.0f%% of similar apps test this flow", pattern.SuccessRate*100),
				Action: SuggestedAction{
					ActionType: "add_test",
					Target:     pattern.FlowType,
					Details: map[string]interface{}{
						"suggested_steps": pattern.Steps,
					},
				},
				Confidence: pattern.SuccessRate,
				Impact:     "High - covers common user journey",
				CreatedAt:  time.Now(),
			})
		}
	}

	return suggestions, nil
}

func (se *SuggestionEngine) generateHealingSuggestions(ctx context.Context, req SuggestionRequest) ([]Suggestion, error) {
	var suggestions []Suggestion

	// Find selectors that frequently break
	for _, failure := range req.RecentFailures {
		if failure.FailureType == "selector" && failure.Frequency >= 2 {
			healings, err := se.patterns.FindHealingStrategies(ctx, HealingRequest{
				OriginalSelector: failure.Selector,
			})
			if err != nil {
				continue
			}

			if len(healings) > 0 {
				best := healings[0]
				suggestions = append(suggestions, Suggestion{
					ID:          uuid.New(),
					Type:        SuggestionTypeHealing,
					Priority:    PriorityHigh,
					Title:       fmt.Sprintf("Fix flaky selector in %s", failure.TestName),
					Description: fmt.Sprintf("Selector '%s' has failed %d times", truncate(failure.Selector, 50), failure.Frequency),
					Rationale:   fmt.Sprintf("Suggested %s strategy has %.0f%% success rate", best.Strategy, best.Confidence*100),
					Action: SuggestedAction{
						ActionType: "modify_selector",
						Target:     failure.Selector,
						Details: map[string]interface{}{
							"new_selector": best.Selector,
							"strategy":     best.Strategy,
						},
					},
					Confidence: best.Confidence,
					Impact:     "High - reduces test flakiness",
					CreatedAt:  time.Now(),
				})
			}
		}
	}

	return suggestions, nil
}

func (se *SuggestionEngine) generateOptimizationSuggestions(ctx context.Context, req SuggestionRequest) ([]Suggestion, error) {
	var suggestions []Suggestion

	// Find slow tests
	for _, test := range req.CurrentTests {
		if test.Steps > 20 {
			suggestions = append(suggestions, Suggestion{
				ID:          uuid.New(),
				Type:        SuggestionTypeOptimization,
				Priority:    PriorityMedium,
				Title:       fmt.Sprintf("Consider splitting %s", test.Name),
				Description: fmt.Sprintf("Test has %d steps, which may be too long", test.Steps),
				Rationale:   "Shorter tests are more maintainable and easier to debug",
				Action: SuggestedAction{
					ActionType: "optimize",
					Target:     test.ID.String(),
					Details: map[string]interface{}{
						"recommendation": "split_test",
						"suggested_split": test.Steps / 2,
					},
				},
				Confidence: 0.7,
				Impact:     "Medium - improves maintainability",
				CreatedAt:  time.Now(),
			})
		}

		// Check for low pass rate
		if test.PassRate < 0.8 && test.PassRate > 0 {
			suggestions = append(suggestions, Suggestion{
				ID:          uuid.New(),
				Type:        SuggestionTypeOptimization,
				Priority:    PriorityHigh,
				Title:       fmt.Sprintf("Improve reliability of %s", test.Name),
				Description: fmt.Sprintf("Test has %.0f%% pass rate", test.PassRate*100),
				Rationale:   "Flaky tests reduce confidence in the test suite",
				Action: SuggestedAction{
					ActionType: "optimize",
					Target:     test.ID.String(),
					Details: map[string]interface{}{
						"recommendation": "add_waits",
						"pass_rate":      test.PassRate,
					},
				},
				Confidence: 0.8,
				Impact:     "High - reduces test flakiness",
				CreatedAt:  time.Now(),
			})
		}
	}

	return suggestions, nil
}

func (se *SuggestionEngine) generateCoverageSuggestions(ctx context.Context, req SuggestionRequest) ([]Suggestion, error) {
	var suggestions []Suggestion

	// Get common pages for this app type that should be tested
	commonPages := getCommonPages(req.AppType)

	for _, page := range commonPages {
		if !containsPage(req.PagesCovered, page.URL) {
			suggestions = append(suggestions, Suggestion{
				ID:          uuid.New(),
				Type:        SuggestionTypeCoverage,
				Priority:    page.Priority,
				Title:       fmt.Sprintf("Add tests for %s page", page.Name),
				Description: fmt.Sprintf("The %s page is typically tested in %s applications", page.Name, req.AppType),
				Rationale:   page.Rationale,
				Action: SuggestedAction{
					ActionType: "add_test",
					Target:     page.URL,
					Details: map[string]interface{}{
						"page_type": page.Type,
						"suggested_tests": page.SuggestedTests,
					},
				},
				Confidence: 0.85,
				Impact:     "Medium - increases page coverage",
				CreatedAt:  time.Now(),
			})
		}
	}

	return suggestions, nil
}

func (se *SuggestionEngine) strategyPriority(confidence float64) SuggestionPriority {
	if confidence >= 0.8 {
		return PriorityHigh
	}
	if confidence >= 0.5 {
		return PriorityMedium
	}
	return PriorityLow
}

// AcceptSuggestion records that a suggestion was accepted
func (se *SuggestionEngine) AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID, tenantID uuid.UUID) error {
	_, err := se.db.ExecContext(ctx, `
		INSERT INTO suggestion_feedback (suggestion_id, tenant_id, accepted, created_at)
		VALUES ($1, $2, true, NOW())`,
		suggestionID,
		tenantID,
	)
	return err
}

// RejectSuggestion records that a suggestion was rejected
func (se *SuggestionEngine) RejectSuggestion(ctx context.Context, suggestionID uuid.UUID, tenantID uuid.UUID, reason string) error {
	_, err := se.db.ExecContext(ctx, `
		INSERT INTO suggestion_feedback (suggestion_id, tenant_id, accepted, rejection_reason, created_at)
		VALUES ($1, $2, false, $3, NOW())`,
		suggestionID,
		tenantID,
		reason,
	)
	return err
}

// Helper functions

func priorityScore(p SuggestionPriority) int {
	switch p {
	case PriorityHigh:
		return 3
	case PriorityMedium:
		return 2
	case PriorityLow:
		return 1
	default:
		return 0
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func containsPage(covered []string, url string) bool {
	for _, c := range covered {
		if strings.Contains(url, c) || strings.Contains(c, url) {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

type commonPage struct {
	Name           string
	URL            string
	Type           string
	Priority       SuggestionPriority
	Rationale      string
	SuggestedTests []string
}

func getCommonPages(appType string) []commonPage {
	commonByType := map[string][]commonPage{
		"e-commerce": {
			{Name: "Homepage", URL: "/", Type: "landing", Priority: PriorityHigh, Rationale: "First impression for users", SuggestedTests: []string{"hero_visible", "search_works", "navigation"}},
			{Name: "Product List", URL: "/products", Type: "listing", Priority: PriorityHigh, Rationale: "Core shopping experience", SuggestedTests: []string{"products_load", "filters_work", "pagination"}},
			{Name: "Product Detail", URL: "/product", Type: "detail", Priority: PriorityHigh, Rationale: "Purchase decision page", SuggestedTests: []string{"add_to_cart", "images_load", "reviews"}},
			{Name: "Cart", URL: "/cart", Type: "cart", Priority: PriorityHigh, Rationale: "Pre-checkout validation", SuggestedTests: []string{"items_display", "quantity_update", "remove_item"}},
			{Name: "Checkout", URL: "/checkout", Type: "checkout", Priority: PriorityHigh, Rationale: "Revenue-critical page", SuggestedTests: []string{"payment_form", "address_form", "order_summary"}},
		},
		"saas": {
			{Name: "Login", URL: "/login", Type: "auth", Priority: PriorityHigh, Rationale: "User access point", SuggestedTests: []string{"login_form", "forgot_password", "social_login"}},
			{Name: "Signup", URL: "/signup", Type: "auth", Priority: PriorityHigh, Rationale: "User acquisition", SuggestedTests: []string{"signup_form", "email_validation", "terms"}},
			{Name: "Dashboard", URL: "/dashboard", Type: "dashboard", Priority: PriorityHigh, Rationale: "Main user interface", SuggestedTests: []string{"data_loads", "navigation", "quick_actions"}},
			{Name: "Settings", URL: "/settings", Type: "settings", Priority: PriorityMedium, Rationale: "User preferences", SuggestedTests: []string{"profile_update", "password_change", "preferences"}},
			{Name: "Billing", URL: "/billing", Type: "billing", Priority: PriorityHigh, Rationale: "Revenue management", SuggestedTests: []string{"plan_display", "payment_update", "invoice_history"}},
		},
	}

	if pages, ok := commonByType[appType]; ok {
		return pages
	}

	// Default pages for any app
	return []commonPage{
		{Name: "Homepage", URL: "/", Type: "landing", Priority: PriorityHigh, Rationale: "Entry point", SuggestedTests: []string{"page_loads", "navigation"}},
		{Name: "Login", URL: "/login", Type: "auth", Priority: PriorityHigh, Rationale: "User access", SuggestedTests: []string{"login_form"}},
	}
}

// StoreSuggestion persists a suggestion for later retrieval
func (se *SuggestionEngine) StoreSuggestion(ctx context.Context, suggestion Suggestion, tenantID, projectID uuid.UUID) error {
	actionJSON, err := json.Marshal(suggestion.Action)
	if err != nil {
		return fmt.Errorf("marshaling action: %w", err)
	}

	_, err = se.db.ExecContext(ctx, `
		INSERT INTO suggestions (id, tenant_id, project_id, type, priority, title, description, rationale, action, confidence, impact, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		suggestion.ID,
		tenantID,
		projectID,
		suggestion.Type,
		suggestion.Priority,
		suggestion.Title,
		suggestion.Description,
		suggestion.Rationale,
		actionJSON,
		suggestion.Confidence,
		suggestion.Impact,
		suggestion.CreatedAt,
	)

	return err
}

// GetStoredSuggestions retrieves stored suggestions for a project
func (se *SuggestionEngine) GetStoredSuggestions(ctx context.Context, projectID uuid.UUID, limit int) ([]Suggestion, error) {
	rows, err := se.db.QueryxContext(ctx, `
		SELECT id, type, priority, title, description, rationale, action, confidence, impact, created_at
		FROM suggestions
		WHERE project_id = $1
		  AND dismissed = false
		ORDER BY priority DESC, confidence DESC
		LIMIT $2`,
		projectID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suggestions []Suggestion
	for rows.Next() {
		var s Suggestion
		var actionJSON json.RawMessage
		if err := rows.Scan(&s.ID, &s.Type, &s.Priority, &s.Title, &s.Description, &s.Rationale, &actionJSON, &s.Confidence, &s.Impact, &s.CreatedAt); err != nil {
			continue
		}
		json.Unmarshal(actionJSON, &s.Action)
		suggestions = append(suggestions, s)
	}

	return suggestions, nil
}

// DismissSuggestion marks a suggestion as dismissed
func (se *SuggestionEngine) DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	_, err := se.db.ExecContext(ctx, `
		UPDATE suggestions SET dismissed = true, dismissed_at = NOW()
		WHERE id = $1`,
		suggestionID,
	)
	return err
}
