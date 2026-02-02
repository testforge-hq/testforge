// Package intelligence provides AI-powered learning and suggestions
// This creates a competitive moat through cross-tenant learning effects
package intelligence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// KnowledgeType represents the type of learned knowledge
type KnowledgeType string

const (
	// KnowledgeTypeHealing represents selector healing patterns
	KnowledgeTypeHealing KnowledgeType = "healing"
	// KnowledgeTypeElement represents UI element patterns
	KnowledgeTypeElement KnowledgeType = "element"
	// KnowledgeTypeFlow represents user journey patterns
	KnowledgeTypeFlow KnowledgeType = "flow"
	// KnowledgeTypeError represents common error patterns
	KnowledgeTypeError KnowledgeType = "error"
)

// KnowledgeEntry represents a piece of learned knowledge
type KnowledgeEntry struct {
	ID            uuid.UUID         `db:"id" json:"id"`
	Type          KnowledgeType     `db:"type" json:"type"`
	Category      string            `db:"category" json:"category"`       // e.g., "e-commerce", "saas", "auth"
	Pattern       string            `db:"pattern" json:"pattern"`         // Anonymized pattern description
	PatternHash   string            `db:"pattern_hash" json:"pattern_hash"` // Hash for deduplication
	SuccessCount  int64             `db:"success_count" json:"success_count"`
	FailureCount  int64             `db:"failure_count" json:"failure_count"`
	Confidence    float64           `db:"confidence" json:"confidence"`   // Success rate
	Metadata      json.RawMessage   `db:"metadata" json:"metadata"`
	ContributedBy int               `db:"contributed_by" json:"contributed_by"` // Number of tenants
	LastUsedAt    *time.Time        `db:"last_used_at" json:"last_used_at,omitempty"`
	CreatedAt     time.Time         `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time         `db:"updated_at" json:"updated_at"`
}

// HealingPattern represents a selector healing pattern
type HealingPattern struct {
	OriginalSelector  string            `json:"original_selector"`
	HealedSelector    string            `json:"healed_selector"`
	Strategy          string            `json:"strategy"`           // "text", "attribute", "relative", "visual"
	ElementType       string            `json:"element_type"`       // "button", "input", "link", etc.
	ContextHints      []string          `json:"context_hints"`      // Anonymized context clues
	SuccessRate       float64           `json:"success_rate"`
	ApplicableFrameworks []string       `json:"applicable_frameworks"` // "react", "vue", "angular", etc.
}

// ElementPattern represents a UI element pattern
type ElementPattern struct {
	ElementType       string            `json:"element_type"`
	CommonSelectors   []string          `json:"common_selectors"`   // Anonymized selector patterns
	CommonAttributes  map[string]string `json:"common_attributes"`
	CommonLabels      []string          `json:"common_labels"`      // e.g., "Submit", "Login", "Add to Cart"
	Industry          string            `json:"industry"`
	Confidence        float64           `json:"confidence"`
}

// FlowPattern represents a user journey pattern
type FlowPattern struct {
	FlowType          string            `json:"flow_type"`          // "signup", "checkout", "search"
	Steps             []FlowStep        `json:"steps"`
	Industry          string            `json:"industry"`
	AverageSteps      int               `json:"average_steps"`
	CommonVariations  []string          `json:"common_variations"`
	SuccessRate       float64           `json:"success_rate"`
}

// FlowStep represents a step in a flow
type FlowStep struct {
	Action            string            `json:"action"`             // "click", "fill", "navigate"
	TargetType        string            `json:"target_type"`        // "button", "input", "link"
	Purpose           string            `json:"purpose"`            // Anonymized purpose
	OptionalStep      bool              `json:"optional_step"`
}

// KnowledgeBase provides cross-tenant learning capabilities
type KnowledgeBase struct {
	db     *sqlx.DB
	logger *zap.Logger

	// In-memory cache for frequently accessed patterns
	cache      map[string]*KnowledgeEntry
	cacheMu    sync.RWMutex
	cacheExpiry time.Duration

	// Stats tracking
	stats      *KBStats
	statsMu    sync.Mutex
}

// KBStats tracks knowledge base statistics
type KBStats struct {
	TotalPatterns      int64     `json:"total_patterns"`
	HealingPatterns    int64     `json:"healing_patterns"`
	ElementPatterns    int64     `json:"element_patterns"`
	FlowPatterns       int64     `json:"flow_patterns"`
	ContributingTenants int64    `json:"contributing_tenants"`
	QueriesPerMinute   int64     `json:"queries_per_minute"`
	LastUpdated        time.Time `json:"last_updated"`
}

// NewKnowledgeBase creates a new knowledge base
func NewKnowledgeBase(db *sqlx.DB, logger *zap.Logger) *KnowledgeBase {
	kb := &KnowledgeBase{
		db:          db,
		logger:      logger,
		cache:       make(map[string]*KnowledgeEntry),
		cacheExpiry: 5 * time.Minute,
		stats:       &KBStats{},
	}

	// Start background cache refresh
	go kb.backgroundRefresh()

	return kb
}

// LearnHealing records a successful healing pattern
func (kb *KnowledgeBase) LearnHealing(ctx context.Context, pattern HealingPattern, tenantID uuid.UUID) error {
	// Anonymize the pattern
	anonymized := kb.anonymizeHealingPattern(pattern)

	// Create pattern hash for deduplication
	hash := kb.hashPattern(anonymized)

	metadata, err := json.Marshal(anonymized)
	if err != nil {
		return fmt.Errorf("marshaling pattern: %w", err)
	}

	// Upsert the pattern
	_, err = kb.db.ExecContext(ctx, `
		INSERT INTO knowledge_entries (id, type, category, pattern, pattern_hash, metadata, success_count, contributed_by)
		VALUES ($1, $2, $3, $4, $5, $6, 1, 1)
		ON CONFLICT (pattern_hash) DO UPDATE SET
			success_count = knowledge_entries.success_count + 1,
			contributed_by = (
				SELECT COUNT(DISTINCT tenant_id) FROM knowledge_contributions
				WHERE entry_id = knowledge_entries.id
			),
			confidence = (knowledge_entries.success_count + 1)::float /
				NULLIF(knowledge_entries.success_count + knowledge_entries.failure_count + 1, 0),
			updated_at = NOW()`,
		uuid.New(),
		KnowledgeTypeHealing,
		pattern.ElementType,
		anonymized.Strategy,
		hash,
		metadata,
	)

	if err != nil {
		return fmt.Errorf("recording healing pattern: %w", err)
	}

	// Record contribution (for counting unique tenants)
	kb.recordContribution(ctx, hash, tenantID)

	kb.logger.Debug("learned healing pattern",
		zap.String("strategy", pattern.Strategy),
		zap.String("element_type", pattern.ElementType),
	)

	return nil
}

// LearnElement records a UI element pattern
func (kb *KnowledgeBase) LearnElement(ctx context.Context, pattern ElementPattern, tenantID uuid.UUID) error {
	anonymized := kb.anonymizeElementPattern(pattern)
	hash := kb.hashPattern(anonymized)

	metadata, err := json.Marshal(anonymized)
	if err != nil {
		return fmt.Errorf("marshaling pattern: %w", err)
	}

	_, err = kb.db.ExecContext(ctx, `
		INSERT INTO knowledge_entries (id, type, category, pattern, pattern_hash, metadata, success_count, contributed_by)
		VALUES ($1, $2, $3, $4, $5, $6, 1, 1)
		ON CONFLICT (pattern_hash) DO UPDATE SET
			success_count = knowledge_entries.success_count + 1,
			updated_at = NOW()`,
		uuid.New(),
		KnowledgeTypeElement,
		pattern.Industry,
		pattern.ElementType,
		hash,
		metadata,
	)

	if err != nil {
		return fmt.Errorf("recording element pattern: %w", err)
	}

	kb.recordContribution(ctx, hash, tenantID)
	return nil
}

// LearnFlow records a user journey pattern
func (kb *KnowledgeBase) LearnFlow(ctx context.Context, pattern FlowPattern, tenantID uuid.UUID) error {
	anonymized := kb.anonymizeFlowPattern(pattern)
	hash := kb.hashPattern(anonymized)

	metadata, err := json.Marshal(anonymized)
	if err != nil {
		return fmt.Errorf("marshaling pattern: %w", err)
	}

	_, err = kb.db.ExecContext(ctx, `
		INSERT INTO knowledge_entries (id, type, category, pattern, pattern_hash, metadata, success_count, contributed_by)
		VALUES ($1, $2, $3, $4, $5, $6, 1, 1)
		ON CONFLICT (pattern_hash) DO UPDATE SET
			success_count = knowledge_entries.success_count + 1,
			updated_at = NOW()`,
		uuid.New(),
		KnowledgeTypeFlow,
		pattern.Industry,
		pattern.FlowType,
		hash,
		metadata,
	)

	if err != nil {
		return fmt.Errorf("recording flow pattern: %w", err)
	}

	kb.recordContribution(ctx, hash, tenantID)
	return nil
}

// RecordFailure records a pattern that didn't work
func (kb *KnowledgeBase) RecordFailure(ctx context.Context, patternHash string) error {
	_, err := kb.db.ExecContext(ctx, `
		UPDATE knowledge_entries
		SET failure_count = failure_count + 1,
			confidence = success_count::float / NULLIF(success_count + failure_count + 1, 0),
			updated_at = NOW()
		WHERE pattern_hash = $1`,
		patternHash,
	)
	return err
}

// QueryHealingPatterns finds relevant healing patterns for a selector
func (kb *KnowledgeBase) QueryHealingPatterns(ctx context.Context, query HealingQuery) ([]HealingPattern, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("healing:%s:%s", query.ElementType, query.Strategy)
	if cached := kb.getFromCache(cacheKey); cached != nil {
		var pattern HealingPattern
		if err := json.Unmarshal(cached.Metadata, &pattern); err == nil {
			return []HealingPattern{pattern}, nil
		}
	}

	rows, err := kb.db.QueryxContext(ctx, `
		SELECT metadata, confidence
		FROM knowledge_entries
		WHERE type = $1
		  AND (category = $2 OR $2 = '')
		  AND confidence >= $3
		  AND success_count >= $4
		ORDER BY confidence DESC, success_count DESC
		LIMIT $5`,
		KnowledgeTypeHealing,
		query.ElementType,
		query.MinConfidence,
		query.MinSuccessCount,
		query.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying healing patterns: %w", err)
	}
	defer rows.Close()

	var patterns []HealingPattern
	for rows.Next() {
		var metadata json.RawMessage
		var confidence float64
		if err := rows.Scan(&metadata, &confidence); err != nil {
			continue
		}

		var pattern HealingPattern
		if err := json.Unmarshal(metadata, &pattern); err != nil {
			continue
		}
		pattern.SuccessRate = confidence
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// HealingQuery represents a query for healing patterns
type HealingQuery struct {
	ElementType     string
	Strategy        string
	Framework       string
	MinConfidence   float64
	MinSuccessCount int64
	Limit           int
}

// QueryElementPatterns finds relevant element patterns
func (kb *KnowledgeBase) QueryElementPatterns(ctx context.Context, elementType, industry string, limit int) ([]ElementPattern, error) {
	rows, err := kb.db.QueryxContext(ctx, `
		SELECT metadata, confidence
		FROM knowledge_entries
		WHERE type = $1
		  AND (pattern = $2 OR $2 = '')
		  AND (category = $3 OR $3 = '')
		ORDER BY success_count DESC
		LIMIT $4`,
		KnowledgeTypeElement,
		elementType,
		industry,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying element patterns: %w", err)
	}
	defer rows.Close()

	var patterns []ElementPattern
	for rows.Next() {
		var metadata json.RawMessage
		var confidence float64
		if err := rows.Scan(&metadata, &confidence); err != nil {
			continue
		}

		var pattern ElementPattern
		if err := json.Unmarshal(metadata, &pattern); err != nil {
			continue
		}
		pattern.Confidence = confidence
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// QueryFlowPatterns finds relevant flow patterns
func (kb *KnowledgeBase) QueryFlowPatterns(ctx context.Context, flowType, industry string, limit int) ([]FlowPattern, error) {
	rows, err := kb.db.QueryxContext(ctx, `
		SELECT metadata, confidence
		FROM knowledge_entries
		WHERE type = $1
		  AND (pattern = $2 OR $2 = '')
		  AND (category = $3 OR $3 = '')
		ORDER BY success_count DESC
		LIMIT $4`,
		KnowledgeTypeFlow,
		flowType,
		industry,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("querying flow patterns: %w", err)
	}
	defer rows.Close()

	var patterns []FlowPattern
	for rows.Next() {
		var metadata json.RawMessage
		var confidence float64
		if err := rows.Scan(&metadata, &confidence); err != nil {
			continue
		}

		var pattern FlowPattern
		if err := json.Unmarshal(metadata, &pattern); err != nil {
			continue
		}
		pattern.SuccessRate = confidence
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// GetStats returns knowledge base statistics
func (kb *KnowledgeBase) GetStats(ctx context.Context) (*KBStats, error) {
	kb.statsMu.Lock()
	defer kb.statsMu.Unlock()

	// Refresh stats if stale
	if time.Since(kb.stats.LastUpdated) > time.Minute {
		if err := kb.refreshStats(ctx); err != nil {
			return nil, err
		}
	}

	return kb.stats, nil
}

// Private methods

func (kb *KnowledgeBase) refreshStats(ctx context.Context) error {
	row := kb.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE type = 'healing') as healing,
			COUNT(*) FILTER (WHERE type = 'element') as element,
			COUNT(*) FILTER (WHERE type = 'flow') as flow,
			COALESCE(SUM(contributed_by), 0) as contributing
		FROM knowledge_entries`)

	err := row.Scan(
		&kb.stats.TotalPatterns,
		&kb.stats.HealingPatterns,
		&kb.stats.ElementPatterns,
		&kb.stats.FlowPatterns,
		&kb.stats.ContributingTenants,
	)
	if err != nil {
		return err
	}

	kb.stats.LastUpdated = time.Now()
	return nil
}

func (kb *KnowledgeBase) recordContribution(ctx context.Context, patternHash string, tenantID uuid.UUID) {
	// Record that this tenant contributed to this pattern (for counting unique contributors)
	kb.db.ExecContext(ctx, `
		INSERT INTO knowledge_contributions (pattern_hash, tenant_id)
		VALUES ($1, $2)
		ON CONFLICT (pattern_hash, tenant_id) DO NOTHING`,
		patternHash,
		tenantID,
	)
}

func (kb *KnowledgeBase) anonymizeHealingPattern(pattern HealingPattern) HealingPattern {
	// Remove any tenant-specific or PII data
	anonymized := HealingPattern{
		Strategy:             pattern.Strategy,
		ElementType:          pattern.ElementType,
		ApplicableFrameworks: pattern.ApplicableFrameworks,
	}

	// Generalize selectors - remove IDs, keep structure
	anonymized.OriginalSelector = generalizeSelector(pattern.OriginalSelector)
	anonymized.HealedSelector = generalizeSelector(pattern.HealedSelector)

	// Keep only generic context hints
	for _, hint := range pattern.ContextHints {
		if isGenericHint(hint) {
			anonymized.ContextHints = append(anonymized.ContextHints, hint)
		}
	}

	return anonymized
}

func (kb *KnowledgeBase) anonymizeElementPattern(pattern ElementPattern) ElementPattern {
	anonymized := ElementPattern{
		ElementType: pattern.ElementType,
		Industry:    pattern.Industry,
	}

	// Generalize selectors
	for _, sel := range pattern.CommonSelectors {
		anonymized.CommonSelectors = append(anonymized.CommonSelectors, generalizeSelector(sel))
	}

	// Keep only common attributes (no values)
	anonymized.CommonAttributes = make(map[string]string)
	for attr := range pattern.CommonAttributes {
		if isCommonAttribute(attr) {
			anonymized.CommonAttributes[attr] = "*" // Placeholder for any value
		}
	}

	// Keep only generic labels
	for _, label := range pattern.CommonLabels {
		if isGenericLabel(label) {
			anonymized.CommonLabels = append(anonymized.CommonLabels, label)
		}
	}

	return anonymized
}

func (kb *KnowledgeBase) anonymizeFlowPattern(pattern FlowPattern) FlowPattern {
	anonymized := FlowPattern{
		FlowType:     pattern.FlowType,
		Industry:     pattern.Industry,
		AverageSteps: pattern.AverageSteps,
	}

	// Keep step structure without specific details
	for _, step := range pattern.Steps {
		anonymized.Steps = append(anonymized.Steps, FlowStep{
			Action:       step.Action,
			TargetType:   step.TargetType,
			Purpose:      generalizePurpose(step.Purpose),
			OptionalStep: step.OptionalStep,
		})
	}

	return anonymized
}

func (kb *KnowledgeBase) hashPattern(pattern interface{}) string {
	data, _ := json.Marshal(pattern)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (kb *KnowledgeBase) getFromCache(key string) *KnowledgeEntry {
	kb.cacheMu.RLock()
	defer kb.cacheMu.RUnlock()
	return kb.cache[key]
}

func (kb *KnowledgeBase) setCache(key string, entry *KnowledgeEntry) {
	kb.cacheMu.Lock()
	defer kb.cacheMu.Unlock()
	kb.cache[key] = entry
}

func (kb *KnowledgeBase) backgroundRefresh() {
	ticker := time.NewTicker(kb.cacheExpiry)
	defer ticker.Stop()

	for range ticker.C {
		kb.cacheMu.Lock()
		kb.cache = make(map[string]*KnowledgeEntry)
		kb.cacheMu.Unlock()
	}
}

// Helper functions

func generalizeSelector(selector string) string {
	// Remove dynamic IDs while keeping structure
	// This is a simplified version - real implementation would be more sophisticated
	// e.g., "#user-123 > .button" becomes "#* > .button"
	return selector // Placeholder - implement regex-based generalization
}

func isGenericHint(hint string) bool {
	genericHints := map[string]bool{
		"near_form":       true,
		"near_header":     true,
		"in_modal":        true,
		"in_sidebar":      true,
		"in_footer":       true,
		"primary_action":  true,
		"secondary_action": true,
	}
	return genericHints[hint]
}

func isCommonAttribute(attr string) bool {
	commonAttrs := map[string]bool{
		"type":        true,
		"role":        true,
		"aria-label":  true,
		"data-testid": true,
		"placeholder": true,
		"name":        true,
	}
	return commonAttrs[attr]
}

func isGenericLabel(label string) bool {
	genericLabels := map[string]bool{
		"Submit":       true,
		"Login":        true,
		"Sign In":      true,
		"Sign Up":      true,
		"Register":     true,
		"Continue":     true,
		"Next":         true,
		"Back":         true,
		"Cancel":       true,
		"Add to Cart":  true,
		"Checkout":     true,
		"Buy Now":      true,
		"Search":       true,
		"Save":         true,
		"Delete":       true,
		"Edit":         true,
	}
	return genericLabels[label]
}

func generalizePurpose(purpose string) string {
	// Map specific purposes to generic categories
	purposeMap := map[string]string{
		"enter_email":    "enter_credentials",
		"enter_password": "enter_credentials",
		"enter_username": "enter_credentials",
		"click_submit":   "submit_form",
		"click_login":    "authenticate",
		"fill_address":   "enter_shipping",
		"select_payment": "enter_payment",
	}

	if generic, ok := purposeMap[purpose]; ok {
		return generic
	}
	return purpose
}
