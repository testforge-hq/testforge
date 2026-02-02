package healing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/vjepa"
)

// Service coordinates V-JEPA visual validation with Claude-based repair
type Service struct {
	config      HealingConfig
	repairAgent *RepairAgent
	vjepaClient *vjepa.Client
	logger      *zap.Logger
}

// NewService creates a new healing service
func NewService(config HealingConfig, logger *zap.Logger) (*Service, error) {
	// Create repair agent
	repairAgent := NewRepairAgent(
		config.ClaudeAPIKey,
		config.ClaudeModel,
		config.ClaudeMaxTokens,
		logger,
	)

	// Create V-JEPA client if visual healing is enabled
	var vjepaClient *vjepa.Client
	if config.EnableVisualHealing && config.VJEPAEndpoint != "" {
		var err error
		vjepaClient, err = vjepa.NewClient(vjepa.ClientConfig{
			Address: config.VJEPAEndpoint,
			Logger:  logger,
		})
		if err != nil {
			logger.Warn("failed to create V-JEPA client, visual healing disabled",
				zap.Error(err))
		}
	}

	return &Service{
		config:      config,
		repairAgent: repairAgent,
		vjepaClient: vjepaClient,
		logger:      logger,
	}, nil
}

// Close closes the service connections
func (s *Service) Close() error {
	if s.vjepaClient != nil {
		return s.vjepaClient.Close()
	}
	return nil
}

// Heal attempts to heal a failed test
func (s *Service) Heal(ctx context.Context, req *HealingRequest) (*HealingResult, error) {
	startTime := time.Now()
	requestID := uuid.New()

	s.logger.Info("starting healing process",
		zap.String("request_id", requestID.String()),
		zap.String("test_name", req.TestName),
		zap.String("failure_type", string(req.FailureType)))

	result := &HealingResult{
		RequestID:     requestID,
		TestRunID:     req.TestRunID,
		Status:        HealingStatusInProgress,
		OriginalError: req.ErrorMessage,
		Attempts:      0,
		Metadata:      make(map[string]interface{}),
	}

	// Auto-detect failure type if not specified
	if req.FailureType == "" || req.FailureType == FailureTypeUnknown {
		req.FailureType = DetectFailureType(req.ErrorMessage)
		s.logger.Debug("auto-detected failure type",
			zap.String("failure_type", string(req.FailureType)))
	}

	// Choose healing strategy based on failure type
	strategy := s.chooseStrategy(req)
	result.Strategy = strategy

	s.logger.Info("chosen healing strategy",
		zap.String("strategy", string(strategy)),
		zap.String("failure_type", string(req.FailureType)))

	// Execute healing based on strategy
	var err error
	for attempt := 1; attempt <= s.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		s.logger.Debug("healing attempt",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", s.config.MaxAttempts))

		switch strategy {
		case StrategySelectoRepair:
			err = s.healSelector(ctx, req, result)
		case StrategyVisualLocator:
			err = s.healVisual(ctx, req, result)
		case StrategyWaitAdjustment:
			err = s.healWait(ctx, req, result)
		case StrategyRetry:
			result.Status = HealingStatusSkipped
			result.Explanation = "Retry strategy - no code changes needed, recommend re-running the test"
			result.Confidence = 0.5
			err = nil
		case StrategySkip:
			result.Status = HealingStatusSkipped
			result.Explanation = "Test cannot be automatically healed"
			result.Confidence = 0.0
			err = nil
		default:
			err = fmt.Errorf("unknown healing strategy: %s", strategy)
		}

		if err == nil && result.Status == HealingStatusSuccess {
			break
		}

		if err != nil {
			s.logger.Warn("healing attempt failed",
				zap.Int("attempt", attempt),
				zap.Error(err))
		}
	}

	// Set final status
	if result.Status == HealingStatusInProgress {
		if err != nil {
			result.Status = HealingStatusFailed
			result.Explanation = fmt.Sprintf("Healing failed after %d attempts: %v", result.Attempts, err)
		}
	}

	result.Duration = time.Since(startTime)

	s.logger.Info("healing process completed",
		zap.String("request_id", requestID.String()),
		zap.String("status", string(result.Status)),
		zap.Float64("confidence", result.Confidence),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// chooseStrategy determines the best healing strategy
func (s *Service) chooseStrategy(req *HealingRequest) HealingStrategy {
	switch req.FailureType {
	case FailureTypeSelector:
		// Prefer visual healing if baseline is available
		if s.config.EnableVisualHealing && req.BaselineURI != "" && s.vjepaClient != nil {
			return StrategyVisualLocator
		}
		return StrategySelectoRepair

	case FailureTypeTimeout:
		return StrategyWaitAdjustment

	case FailureTypeAssertion:
		// Assertions may need selector repair if element text changed
		if req.Selector != "" {
			return StrategySelectoRepair
		}
		return StrategySkip

	case FailureTypeNavigation:
		return StrategyRetry

	case FailureTypeNetwork:
		return StrategyRetry

	case FailureTypeVisual:
		if s.config.EnableVisualHealing && s.vjepaClient != nil {
			return StrategyVisualLocator
		}
		return StrategySkip

	default:
		// Default to selector repair for unknown failures
		if req.Selector != "" {
			return StrategySelectoRepair
		}
		return StrategySkip
	}
}

// healSelector uses Claude to repair broken selectors
func (s *Service) healSelector(ctx context.Context, req *HealingRequest, result *HealingResult) error {
	// Truncate HTML if too large
	pageHTML := req.PageHTML
	if len(pageHTML) > s.config.MaxHTMLSize {
		// Try to extract relevant portion around the selector
		pageHTML = s.extractRelevantHTML(pageHTML, req.Selector)
	}

	// Build repair request
	repairReq := &SelectorRepairRequest{
		FailedSelector: req.Selector,
		ErrorMessage:   req.ErrorMessage,
		PageHTML:       pageHTML,
		PageURL:        req.PageURL,
		TestContext:    req.FailedStep,
		TestCode:       req.TestCode,
		FailedLine:     req.FailedLine,
	}

	// Call Claude for repair
	repairResp, err := s.repairAgent.RepairSelector(ctx, repairReq)
	if err != nil {
		return fmt.Errorf("selector repair failed: %w", err)
	}

	// Check confidence threshold
	if repairResp.Confidence < s.config.MinConfidence {
		s.logger.Warn("repair confidence below threshold",
			zap.Float64("confidence", repairResp.Confidence),
			zap.Float64("threshold", s.config.MinConfidence))
	}

	// Update result
	result.HealedSelector = repairResp.RepairedSelector
	result.Explanation = repairResp.Explanation
	result.Confidence = repairResp.Confidence
	result.Metadata["change_type"] = repairResp.ChangeType
	result.Metadata["root_cause"] = repairResp.RootCause

	// Generate suggestions from alternatives
	if s.config.EnableSuggestions && len(repairResp.AlternativeSelectors) > 0 {
		result.Suggestions = make([]HealingSuggestion, 0, len(repairResp.AlternativeSelectors))
		for _, alt := range repairResp.AlternativeSelectors {
			result.Suggestions = append(result.Suggestions, HealingSuggestion{
				Strategy:    StrategySelectoRepair,
				Description: fmt.Sprintf("Alternative %s selector", alt.Type),
				Selector:    alt.Selector,
				Confidence:  alt.Confidence,
				Reasoning:   alt.Reasoning,
			})
		}
	}

	// Optionally generate complete fixed code
	if req.TestCode != "" {
		fixedCode, err := s.repairAgent.GenerateTestFix(ctx, req, repairResp)
		if err != nil {
			s.logger.Warn("failed to generate fixed test code", zap.Error(err))
		} else {
			result.HealedCode = fixedCode
		}
	}

	// Validate with V-JEPA if available
	if s.config.EnableVisualHealing && s.vjepaClient != nil && req.ScreenshotURI != "" && req.BaselineURI != "" {
		validated, score := s.validateWithVJEPA(ctx, req)
		result.Validated = validated
		result.ValidationScore = score
	}

	result.Status = HealingStatusSuccess
	return nil
}

// healVisual uses V-JEPA for visual-based healing
func (s *Service) healVisual(ctx context.Context, req *HealingRequest, result *HealingResult) error {
	if s.vjepaClient == nil {
		return fmt.Errorf("V-JEPA client not available")
	}

	// Compare current frame with baseline
	compareResp, err := s.vjepaClient.CompareFramesByURI(ctx, req.BaselineURI, req.ScreenshotURI, "healing comparison")
	if err != nil {
		return fmt.Errorf("V-JEPA comparison failed: %w", err)
	}

	similarity := float64(compareResp.SimilarityScore)
	isSimilar := compareResp.SemanticMatch

	result.ValidationScore = similarity
	result.Metadata["vjepa_similarity"] = similarity
	result.Metadata["is_similar"] = isSimilar

	// Analyze changes - note: AnalyzeChange requires actual image bytes, not URIs
	// For now we skip deep analysis when using URIs
	if len(compareResp.ChangedRegions) > 0 {
		result.Metadata["changed_regions_count"] = len(compareResp.ChangedRegions)
		result.Metadata["change_analysis"] = compareResp.Analysis
	}

	// If visual difference detected, try to use Claude to interpret
	if !isSimilar && req.Selector != "" {
		// Fall back to selector repair with visual context
		visualContext := fmt.Sprintf(
			"Visual comparison shows %.2f%% similarity (threshold: %.2f%%). Changed regions detected: %d",
			similarity*100,
			s.config.SimilarityThreshold*100,
			len(compareResp.ChangedRegions),
		)

		repairReq := &SelectorRepairRequest{
			FailedSelector: req.Selector,
			ErrorMessage:   req.ErrorMessage,
			PageHTML:       req.PageHTML,
			PageURL:        req.PageURL,
			TestContext:    req.FailedStep,
			TestCode:       req.TestCode,
			FailedLine:     req.FailedLine,
			Hints:          []string{visualContext},
		}

		repairResp, err := s.repairAgent.RepairSelector(ctx, repairReq)
		if err != nil {
			return fmt.Errorf("visual-guided selector repair failed: %w", err)
		}

		result.HealedSelector = repairResp.RepairedSelector
		result.Explanation = fmt.Sprintf("Visual analysis: %s. %s", visualContext, repairResp.Explanation)
		result.Confidence = repairResp.Confidence * similarity // Adjust confidence
	} else if isSimilar {
		result.Explanation = "Visual comparison shows page is similar to baseline, failure may be timing-related"
		result.Confidence = similarity
		result.Suggestions = append(result.Suggestions, HealingSuggestion{
			Strategy:    StrategyWaitAdjustment,
			Description: "Consider adding explicit wait",
			Confidence:  0.7,
			Reasoning:   "Page appears visually correct, element may not be ready",
		})
	}

	result.Status = HealingStatusSuccess
	return nil
}

// healWait handles timeout-related failures
func (s *Service) healWait(ctx context.Context, req *HealingRequest, result *HealingResult) error {
	// Analyze the failure to understand the timeout context
	analysis, err := s.repairAgent.AnalyzeFailure(ctx, req)
	if err != nil {
		s.logger.Warn("failure analysis failed", zap.Error(err))
	}

	// Generate wait adjustment suggestions
	result.Explanation = "Timeout detected - recommend adjusting wait strategies"
	result.Confidence = 0.6

	suggestions := []HealingSuggestion{
		{
			Strategy:    StrategyWaitAdjustment,
			Description: "Add explicit waitForSelector",
			Code:        fmt.Sprintf("await page.waitForSelector('%s', { state: 'visible', timeout: 30000 });", req.Selector),
			Confidence:  0.7,
			Reasoning:   "Element may take longer to appear",
		},
		{
			Strategy:    StrategyWaitAdjustment,
			Description: "Add waitForLoadState",
			Code:        "await page.waitForLoadState('networkidle');",
			Confidence:  0.6,
			Reasoning:   "Page may have ongoing network requests",
		},
		{
			Strategy:    StrategyWaitAdjustment,
			Description: "Increase timeout",
			Code:        "test.setTimeout(60000);",
			Confidence:  0.5,
			Reasoning:   "Operation may legitimately take longer",
		},
	}

	if analysis != nil {
		if analysis.IsFlaky {
			suggestions = append(suggestions, HealingSuggestion{
				Strategy:    StrategyRetry,
				Description: "Add retry logic",
				Code:        "test.describe.configure({ retries: 2 });",
				Confidence:  0.65,
				Reasoning:   analysis.FlakinessReason,
			})
		}
		result.Metadata["is_flaky"] = analysis.IsFlaky
		result.Metadata["root_cause"] = analysis.RootCause
	}

	result.Suggestions = suggestions
	result.Status = HealingStatusSuccess

	return nil
}

// validateWithVJEPA validates the repair using visual comparison
func (s *Service) validateWithVJEPA(ctx context.Context, req *HealingRequest) (bool, float64) {
	if s.vjepaClient == nil {
		return false, 0
	}

	resp, err := s.vjepaClient.CompareFramesByURI(ctx, req.BaselineURI, req.ScreenshotURI, "validation")
	if err != nil {
		s.logger.Warn("V-JEPA validation failed", zap.Error(err))
		return false, 0
	}

	return resp.SemanticMatch, float64(resp.SimilarityScore)
}

// extractRelevantHTML extracts HTML around the selector
func (s *Service) extractRelevantHTML(html, selector string) string {
	// Try to find relevant portions of HTML
	// This is a simple heuristic - in production you'd want DOM parsing

	// Extract element IDs, classes, or text from selector
	searchTerms := extractSearchTerms(selector)

	if len(searchTerms) == 0 {
		// Just truncate to max size
		if len(html) > s.config.MaxHTMLSize {
			return html[:s.config.MaxHTMLSize] + "\n<!-- truncated -->"
		}
		return html
	}

	// Find the most relevant section
	var bestMatch string
	var bestScore int

	// Split HTML into chunks and score each
	chunkSize := s.config.MaxHTMLSize / 2
	for i := 0; i < len(html)-chunkSize; i += chunkSize / 2 {
		end := i + chunkSize
		if end > len(html) {
			end = len(html)
		}
		chunk := html[i:end]

		score := 0
		for _, term := range searchTerms {
			if strings.Contains(chunk, term) {
				score++
			}
		}

		if score > bestScore {
			bestScore = score
			bestMatch = chunk
		}
	}

	if bestMatch != "" {
		return "<!-- extracted relevant section -->\n" + bestMatch
	}

	// Fallback to beginning
	if len(html) > s.config.MaxHTMLSize {
		return html[:s.config.MaxHTMLSize] + "\n<!-- truncated -->"
	}
	return html
}

// extractSearchTerms extracts searchable terms from a selector
func extractSearchTerms(selector string) []string {
	var terms []string

	// Extract IDs (#id)
	for _, part := range strings.Split(selector, "#") {
		if len(part) > 0 {
			// Get first word
			word := strings.FieldsFunc(part, func(r rune) bool {
				return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_')
			})
			if len(word) > 0 && len(word[0]) > 2 {
				terms = append(terms, word[0])
			}
		}
	}

	// Extract classes (.class)
	for _, part := range strings.Split(selector, ".") {
		if len(part) > 0 {
			word := strings.FieldsFunc(part, func(r rune) bool {
				return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_')
			})
			if len(word) > 0 && len(word[0]) > 2 {
				terms = append(terms, word[0])
			}
		}
	}

	// Extract data attributes
	if strings.Contains(selector, "data-") {
		start := strings.Index(selector, "data-")
		end := start + 20
		if end > len(selector) {
			end = len(selector)
		}
		terms = append(terms, selector[start:end])
	}

	// Extract text content from getByText, getByRole, etc.
	patterns := []string{"getByText(", "getByRole(", "getByLabel(", "getByPlaceholder("}
	for _, pattern := range patterns {
		if idx := strings.Index(selector, pattern); idx >= 0 {
			start := idx + len(pattern)
			// Find the closing quote
			if start < len(selector) {
				quote := selector[start]
				if quote == '\'' || quote == '"' {
					end := strings.IndexByte(selector[start+1:], quote)
					if end > 0 {
						terms = append(terms, selector[start+1:start+1+end])
					}
				}
			}
		}
	}

	return terms
}

// BatchHeal heals multiple failures in parallel
func (s *Service) BatchHeal(ctx context.Context, requests []*HealingRequest) ([]*HealingResult, error) {
	results := make([]*HealingResult, len(requests))

	// Process in parallel with semaphore
	type indexedResult struct {
		index  int
		result *HealingResult
		err    error
	}

	resultChan := make(chan indexedResult, len(requests))

	for i, req := range requests {
		go func(idx int, r *HealingRequest) {
			result, err := s.Heal(ctx, r)
			resultChan <- indexedResult{index: idx, result: result, err: err}
		}(i, req)
	}

	// Collect results
	var firstErr error
	for range requests {
		ir := <-resultChan
		if ir.err != nil && firstErr == nil {
			firstErr = ir.err
		}
		results[ir.index] = ir.result
	}

	return results, firstErr
}

// HealthCheck verifies the healing service is operational
func (s *Service) HealthCheck(ctx context.Context) error {
	// Check V-JEPA if enabled
	if s.config.EnableVisualHealing && s.vjepaClient != nil {
		_, err := s.vjepaClient.HealthCheck(ctx)
		if err != nil {
			return fmt.Errorf("V-JEPA health check failed: %w", err)
		}
	}

	// Check Claude API key is configured
	if s.config.ClaudeAPIKey == "" {
		return fmt.Errorf("Claude API key not configured")
	}

	return nil
}
