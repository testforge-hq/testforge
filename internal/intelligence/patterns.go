package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// PatternRepository manages healing and element patterns
type PatternRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
	kb     *KnowledgeBase

	// Local pattern cache for fast lookup
	healingCache map[string][]HealingPattern
	elementCache map[string][]ElementPattern
	flowCache    map[string][]FlowPattern
	cacheMu      sync.RWMutex
	cacheTime    time.Time
	cacheTTL     time.Duration
}

// NewPatternRepository creates a new pattern repository
func NewPatternRepository(db *sqlx.DB, kb *KnowledgeBase, logger *zap.Logger) *PatternRepository {
	pr := &PatternRepository{
		db:           db,
		logger:       logger,
		kb:           kb,
		healingCache: make(map[string][]HealingPattern),
		elementCache: make(map[string][]ElementPattern),
		flowCache:    make(map[string][]FlowPattern),
		cacheTTL:     10 * time.Minute,
	}

	// Initialize cache
	go pr.warmCache(context.Background())

	return pr
}

// FindHealingStrategies returns healing strategies for a broken selector
func (pr *PatternRepository) FindHealingStrategies(ctx context.Context, req HealingRequest) ([]HealingStrategy, error) {
	// Check local cache first
	cacheKey := fmt.Sprintf("%s:%s", req.ElementType, req.Framework)

	pr.cacheMu.RLock()
	cached, hasCached := pr.healingCache[cacheKey]
	cacheValid := time.Since(pr.cacheTime) < pr.cacheTTL
	pr.cacheMu.RUnlock()

	var patterns []HealingPattern

	if hasCached && cacheValid {
		patterns = cached
	} else {
		// Query knowledge base
		var err error
		patterns, err = pr.kb.QueryHealingPatterns(ctx, HealingQuery{
			ElementType:     req.ElementType,
			Strategy:        "",
			Framework:       req.Framework,
			MinConfidence:   0.6,
			MinSuccessCount: 3,
			Limit:           20,
		})
		if err != nil {
			return nil, fmt.Errorf("querying patterns: %w", err)
		}

		// Update cache
		pr.cacheMu.Lock()
		pr.healingCache[cacheKey] = patterns
		pr.cacheTime = time.Now()
		pr.cacheMu.Unlock()
	}

	// Convert patterns to strategies
	strategies := make([]HealingStrategy, 0, len(patterns))
	for _, p := range patterns {
		if pr.patternApplies(p, req) {
			strategies = append(strategies, HealingStrategy{
				Strategy:      p.Strategy,
				Confidence:    p.SuccessRate,
				Selector:      pr.adaptSelector(p.HealedSelector, req),
				Explanation:   pr.explainStrategy(p),
				PatternHash:   pr.kb.hashPattern(p),
			})
		}
	}

	// Sort by confidence
	sortStrategies(strategies)

	return strategies, nil
}

// HealingRequest represents a request to find healing strategies
type HealingRequest struct {
	OriginalSelector string            `json:"original_selector"`
	ElementType      string            `json:"element_type"`
	Framework        string            `json:"framework"`
	PageContext      map[string]string `json:"page_context"`
	NearbyElements   []NearbyElement   `json:"nearby_elements"`
}

// NearbyElement represents an element near the target
type NearbyElement struct {
	Selector string `json:"selector"`
	Text     string `json:"text"`
	Type     string `json:"type"`
	Distance int    `json:"distance"` // pixels
}

// HealingStrategy represents a healing strategy to try
type HealingStrategy struct {
	Strategy    string  `json:"strategy"`
	Confidence  float64 `json:"confidence"`
	Selector    string  `json:"selector"`
	Explanation string  `json:"explanation"`
	PatternHash string  `json:"pattern_hash"`
}

// FindElementPatterns returns patterns for a specific element type
func (pr *PatternRepository) FindElementPatterns(ctx context.Context, elementType, industry string) ([]ElementPattern, error) {
	cacheKey := fmt.Sprintf("%s:%s", elementType, industry)

	pr.cacheMu.RLock()
	cached, hasCached := pr.elementCache[cacheKey]
	cacheValid := time.Since(pr.cacheTime) < pr.cacheTTL
	pr.cacheMu.RUnlock()

	if hasCached && cacheValid {
		return cached, nil
	}

	patterns, err := pr.kb.QueryElementPatterns(ctx, elementType, industry, 20)
	if err != nil {
		return nil, err
	}

	pr.cacheMu.Lock()
	pr.elementCache[cacheKey] = patterns
	pr.cacheMu.Unlock()

	return patterns, nil
}

// FindFlowPatterns returns patterns for a specific flow type
func (pr *PatternRepository) FindFlowPatterns(ctx context.Context, flowType, industry string) ([]FlowPattern, error) {
	cacheKey := fmt.Sprintf("%s:%s", flowType, industry)

	pr.cacheMu.RLock()
	cached, hasCached := pr.flowCache[cacheKey]
	cacheValid := time.Since(pr.cacheTime) < pr.cacheTTL
	pr.cacheMu.RUnlock()

	if hasCached && cacheValid {
		return cached, nil
	}

	patterns, err := pr.kb.QueryFlowPatterns(ctx, flowType, industry, 10)
	if err != nil {
		return nil, err
	}

	pr.cacheMu.Lock()
	pr.flowCache[cacheKey] = patterns
	pr.cacheMu.Unlock()

	return patterns, nil
}

// RecordHealingResult records whether a healing strategy worked
func (pr *PatternRepository) RecordHealingResult(ctx context.Context, patternHash string, success bool, tenantID uuid.UUID) error {
	if success {
		// Pattern already recorded as success during learning
		pr.logger.Debug("healing strategy succeeded", zap.String("pattern_hash", patternHash))
		return nil
	}

	// Record failure
	if err := pr.kb.RecordFailure(ctx, patternHash); err != nil {
		return fmt.Errorf("recording failure: %w", err)
	}

	pr.logger.Debug("healing strategy failed", zap.String("pattern_hash", patternHash))
	return nil
}

// ContributeHealing adds a new healing pattern from a successful healing
func (pr *PatternRepository) ContributeHealing(ctx context.Context, original, healed string, elementType, strategy string, tenantID uuid.UUID) error {
	pattern := HealingPattern{
		OriginalSelector: original,
		HealedSelector:   healed,
		Strategy:         strategy,
		ElementType:      elementType,
	}

	return pr.kb.LearnHealing(ctx, pattern, tenantID)
}

// GetIndustryTemplates returns flow templates for an industry
func (pr *PatternRepository) GetIndustryTemplates(ctx context.Context, industry string) ([]FlowTemplate, error) {
	rows, err := pr.db.QueryxContext(ctx, `
		SELECT id, name, description, industry, flow_type, template
		FROM flow_templates
		WHERE industry = $1 OR industry = 'general'
		ORDER BY usage_count DESC
		LIMIT 20`,
		industry,
	)
	if err != nil {
		return nil, fmt.Errorf("querying templates: %w", err)
	}
	defer rows.Close()

	var templates []FlowTemplate
	for rows.Next() {
		var t FlowTemplate
		var templateJSON json.RawMessage
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Industry, &t.FlowType, &templateJSON); err != nil {
			continue
		}
		if err := json.Unmarshal(templateJSON, &t.Steps); err != nil {
			continue
		}
		templates = append(templates, t)
	}

	return templates, nil
}

// FlowTemplate represents a pre-built test flow template
type FlowTemplate struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Industry    string            `json:"industry"`
	FlowType    string            `json:"flow_type"`
	Steps       []TemplateStep    `json:"steps"`
	Variables   []TemplateVariable `json:"variables"`
}

// TemplateStep represents a step in a flow template
type TemplateStep struct {
	Order       int               `json:"order"`
	Action      string            `json:"action"`
	Target      string            `json:"target"`
	Value       string            `json:"value,omitempty"`
	WaitFor     string            `json:"wait_for,omitempty"`
	Assertions  []TemplateAssertion `json:"assertions,omitempty"`
}

// TemplateAssertion represents an assertion in a template step
type TemplateAssertion struct {
	Type     string `json:"type"`     // "visible", "text", "url", "count"
	Target   string `json:"target"`
	Expected string `json:"expected"`
}

// TemplateVariable represents a variable in a template
type TemplateVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`        // "string", "email", "password", "number"
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// Private methods

func (pr *PatternRepository) warmCache(ctx context.Context) {
	// Pre-load common patterns
	commonElements := []string{"button", "input", "link", "form"}
	commonFlows := []string{"login", "signup", "checkout", "search"}

	for _, elem := range commonElements {
		pr.kb.QueryHealingPatterns(ctx, HealingQuery{
			ElementType:     elem,
			MinConfidence:   0.5,
			MinSuccessCount: 1,
			Limit:           50,
		})
	}

	for _, flow := range commonFlows {
		pr.kb.QueryFlowPatterns(ctx, flow, "", 20)
	}

	pr.logger.Info("pattern cache warmed")
}

func (pr *PatternRepository) patternApplies(pattern HealingPattern, req HealingRequest) bool {
	// Check if pattern is applicable to this request
	if pattern.ElementType != "" && pattern.ElementType != req.ElementType {
		return false
	}

	// Check framework compatibility
	if len(pattern.ApplicableFrameworks) > 0 && req.Framework != "" {
		found := false
		for _, fw := range pattern.ApplicableFrameworks {
			if fw == req.Framework {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (pr *PatternRepository) adaptSelector(patternSelector string, req HealingRequest) string {
	// Adapt the pattern selector to the specific request context
	// This is a placeholder - real implementation would do smart adaptation
	return patternSelector
}

func (pr *PatternRepository) explainStrategy(pattern HealingPattern) string {
	explanations := map[string]string{
		"text":      "Find element by its visible text content",
		"attribute": "Find element by stable attributes like data-testid, role, or aria-label",
		"relative":  "Find element relative to nearby stable elements",
		"visual":    "Find element by its visual appearance and position",
		"xpath":     "Use XPath for complex element relationships",
	}

	if exp, ok := explanations[pattern.Strategy]; ok {
		return exp
	}
	return fmt.Sprintf("Use %s strategy", pattern.Strategy)
}

func sortStrategies(strategies []HealingStrategy) {
	// Simple insertion sort (small lists)
	for i := 1; i < len(strategies); i++ {
		for j := i; j > 0 && strategies[j].Confidence > strategies[j-1].Confidence; j-- {
			strategies[j], strategies[j-1] = strategies[j-1], strategies[j]
		}
	}
}

// GetTopPatterns returns the most successful patterns across all types
func (pr *PatternRepository) GetTopPatterns(ctx context.Context, limit int) (*TopPatterns, error) {
	result := &TopPatterns{}

	// Get top healing patterns
	rows, err := pr.db.QueryxContext(ctx, `
		SELECT pattern, metadata, success_count, confidence
		FROM knowledge_entries
		WHERE type = 'healing'
		ORDER BY success_count DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var pattern string
		var metadata json.RawMessage
		var successCount int64
		var confidence float64
		if err := rows.Scan(&pattern, &metadata, &successCount, &confidence); err != nil {
			continue
		}

		var hp HealingPattern
		json.Unmarshal(metadata, &hp)
		hp.SuccessRate = confidence

		result.HealingPatterns = append(result.HealingPatterns, PatternSummary{
			Pattern:      pattern,
			SuccessCount: successCount,
			Confidence:   confidence,
		})
	}
	rows.Close()

	// Get top element patterns
	rows, err = pr.db.QueryxContext(ctx, `
		SELECT pattern, success_count, confidence
		FROM knowledge_entries
		WHERE type = 'element'
		ORDER BY success_count DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var pattern string
		var successCount int64
		var confidence float64
		if err := rows.Scan(&pattern, &successCount, &confidence); err != nil {
			continue
		}

		result.ElementPatterns = append(result.ElementPatterns, PatternSummary{
			Pattern:      pattern,
			SuccessCount: successCount,
			Confidence:   confidence,
		})
	}
	rows.Close()

	return result, nil
}

// TopPatterns contains the most successful patterns
type TopPatterns struct {
	HealingPatterns []PatternSummary `json:"healing_patterns"`
	ElementPatterns []PatternSummary `json:"element_patterns"`
	FlowPatterns    []PatternSummary `json:"flow_patterns"`
}

// PatternSummary summarizes a pattern's performance
type PatternSummary struct {
	Pattern      string  `json:"pattern"`
	SuccessCount int64   `json:"success_count"`
	Confidence   float64 `json:"confidence"`
}
