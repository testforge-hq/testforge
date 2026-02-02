package healing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SelectorRepairSystemPrompt guides Claude for selector repair
const SelectorRepairSystemPrompt = `You are an expert Playwright test automation engineer specializing in fixing broken selectors.

Your task is to analyze a failed Playwright test and provide repaired selectors that will work with the current page structure.

## Analysis Process
1. Understand what element the original selector was trying to target
2. Analyze the provided HTML to find the correct element
3. Generate multiple selector strategies (CSS, XPath, text, role, test-id)
4. Rank selectors by reliability and maintainability

## Selector Best Practices
1. **Prefer stable attributes**: data-testid, aria-label, role
2. **Avoid brittle selectors**: nth-child, complex CSS paths, generated classes
3. **Use semantic selectors**: getByRole, getByText, getByLabel when possible
4. **Keep selectors simple**: Shorter paths are more maintainable

## Output Format
Respond with ONLY a JSON object (no markdown, no explanation outside JSON):
{
  "repaired_selector": "the best selector to use",
  "alternative_selectors": [
    {"selector": "...", "type": "css|xpath|text|role|testid", "confidence": 0.0-1.0, "reasoning": "..."}
  ],
  "explanation": "why the original selector failed and how the repair works",
  "confidence": 0.0-1.0,
  "change_type": "id_changed|class_changed|structure_changed|text_changed|element_removed|element_moved|unknown",
  "root_cause": "brief explanation of what changed in the application"
}`

// RepairAgent handles Claude-based selector repair
type RepairAgent struct {
	apiKey     string
	model      string
	maxTokens  int
	httpClient *http.Client
	logger     *zap.Logger
}

// NewRepairAgent creates a new Claude-based repair agent
func NewRepairAgent(apiKey, model string, maxTokens int, logger *zap.Logger) *RepairAgent {
	return &RepairAgent{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: logger,
	}
}

// ClaudeMessage represents a message in Claude's format
type ClaudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ClaudeRequest represents a request to Claude API
type ClaudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []ClaudeMessage `json:"messages"`
}

// ClaudeResponse represents Claude's API response
type ClaudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// RepairSelector uses Claude to repair a broken selector
func (a *RepairAgent) RepairSelector(ctx context.Context, req *SelectorRepairRequest) (*SelectorRepairResponse, error) {
	// Build the user prompt
	userPrompt := a.buildRepairPrompt(req)

	// Call Claude API
	response, err := a.callClaude(ctx, SelectorRepairSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("claude API call failed: %w", err)
	}

	// Parse the response
	repairResponse, err := a.parseRepairResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse claude response: %w", err)
	}

	return repairResponse, nil
}

// buildRepairPrompt constructs the prompt for selector repair
func (a *RepairAgent) buildRepairPrompt(req *SelectorRepairRequest) string {
	var sb strings.Builder

	sb.WriteString("## Failed Selector Analysis Request\n\n")

	sb.WriteString("### Failed Selector\n")
	sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", req.FailedSelector))

	sb.WriteString("### Error Message\n")
	sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", req.ErrorMessage))

	sb.WriteString("### Page URL\n")
	sb.WriteString(fmt.Sprintf("%s\n\n", req.PageURL))

	if req.TestContext != "" {
		sb.WriteString("### Test Context\n")
		sb.WriteString(fmt.Sprintf("%s\n\n", req.TestContext))
	}

	if req.TestCode != "" {
		sb.WriteString("### Relevant Test Code\n")
		sb.WriteString(fmt.Sprintf("```typescript\n%s\n```\n\n", req.TestCode))
		if req.FailedLine > 0 {
			sb.WriteString(fmt.Sprintf("Failed at line: %d\n\n", req.FailedLine))
		}
	}

	if len(req.Hints) > 0 {
		sb.WriteString("### Hints\n")
		for _, hint := range req.Hints {
			sb.WriteString(fmt.Sprintf("- %s\n", hint))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### Current Page HTML (relevant portion)\n")
	sb.WriteString(fmt.Sprintf("```html\n%s\n```\n\n", req.PageHTML))

	sb.WriteString("Please analyze the failure and provide repaired selectors.")

	return sb.String()
}

// callClaude makes a request to the Claude API
func (a *RepairAgent) callClaude(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := ClaudeRequest{
		Model:     a.model,
		MaxTokens: a.maxTokens,
		System:    systemPrompt,
		Messages: []ClaudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	a.logger.Debug("calling Claude API",
		zap.String("model", a.model),
		zap.Int("prompt_length", len(userPrompt)))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		a.logger.Error("Claude API error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return "", fmt.Errorf("Claude API returned status %d: %s", resp.StatusCode, string(body))
	}

	var claudeResp ClaudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}

	a.logger.Debug("Claude API response",
		zap.Int("input_tokens", claudeResp.Usage.InputTokens),
		zap.Int("output_tokens", claudeResp.Usage.OutputTokens),
		zap.String("stop_reason", claudeResp.StopReason))

	return claudeResp.Content[0].Text, nil
}

// parseRepairResponse parses Claude's JSON response
func (a *RepairAgent) parseRepairResponse(response string) (*SelectorRepairResponse, error) {
	// Clean up the response - remove any markdown code blocks
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
	}
	response = strings.TrimSpace(response)

	var repairResp SelectorRepairResponse
	if err := json.Unmarshal([]byte(response), &repairResp); err != nil {
		a.logger.Error("failed to parse Claude response as JSON",
			zap.String("response", response),
			zap.Error(err))
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	// Validate required fields
	if repairResp.RepairedSelector == "" {
		return nil, fmt.Errorf("response missing repaired_selector")
	}

	return &repairResp, nil
}

// GenerateTestFix generates a complete test fix including the repaired code
func (a *RepairAgent) GenerateTestFix(ctx context.Context, req *HealingRequest, repairResp *SelectorRepairResponse) (string, error) {
	// Build a prompt to generate the fixed test code
	systemPrompt := `You are an expert Playwright test automation engineer.
Given a test failure and repaired selector, generate the corrected test code.
Output ONLY the corrected code block, no explanations.`

	var sb strings.Builder
	sb.WriteString("## Original Test Code\n")
	sb.WriteString(fmt.Sprintf("```typescript\n%s\n```\n\n", req.TestCode))
	sb.WriteString(fmt.Sprintf("## Failed Line: %d\n\n", req.FailedLine))
	sb.WriteString(fmt.Sprintf("## Original Selector: %s\n\n", req.Selector))
	sb.WriteString(fmt.Sprintf("## Repaired Selector: %s\n\n", repairResp.RepairedSelector))
	sb.WriteString(fmt.Sprintf("## Change Explanation: %s\n\n", repairResp.Explanation))
	sb.WriteString("Generate the corrected test code with the repaired selector.")

	response, err := a.callClaude(ctx, systemPrompt, sb.String())
	if err != nil {
		return "", fmt.Errorf("failed to generate test fix: %w", err)
	}

	// Clean up the response
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```typescript") {
		response = strings.TrimPrefix(response, "```typescript")
		response = strings.TrimSuffix(response, "```")
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
	}

	return strings.TrimSpace(response), nil
}

// AnalyzeFailure uses Claude to deeply analyze a test failure
func (a *RepairAgent) AnalyzeFailure(ctx context.Context, req *HealingRequest) (*FailureAnalysis, error) {
	systemPrompt := `You are an expert test automation engineer analyzing a test failure.
Analyze the failure and provide insights about the root cause and recommended fixes.
Output ONLY a JSON object with the following structure:
{
  "failure_type": "selector|timeout|assertion|navigation|visual|network|unknown",
  "root_cause": "detailed explanation of what caused the failure",
  "is_flaky": true/false,
  "flakiness_reason": "if flaky, explain why",
  "recommended_strategy": "selector_repair|visual_locator|wait_adjustment|retry|skip",
  "confidence": 0.0-1.0,
  "additional_context": "any other relevant information"
}`

	var sb strings.Builder
	sb.WriteString("## Test Failure Analysis Request\n\n")
	sb.WriteString(fmt.Sprintf("### Test: %s\n", req.TestName))
	sb.WriteString(fmt.Sprintf("### File: %s\n\n", req.TestFile))
	sb.WriteString(fmt.Sprintf("### Error Message\n```\n%s\n```\n\n", req.ErrorMessage))
	sb.WriteString(fmt.Sprintf("### Failed Step\n%s\n\n", req.FailedStep))
	if req.Selector != "" {
		sb.WriteString(fmt.Sprintf("### Selector\n%s\n\n", req.Selector))
	}
	if req.PageURL != "" {
		sb.WriteString(fmt.Sprintf("### Page URL\n%s\n\n", req.PageURL))
	}
	sb.WriteString(fmt.Sprintf("### Test Code\n```typescript\n%s\n```\n\n", req.TestCode))

	response, err := a.callClaude(ctx, systemPrompt, sb.String())
	if err != nil {
		return nil, fmt.Errorf("failed to analyze failure: %w", err)
	}

	// Parse response
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
	}
	response = strings.TrimSpace(response)

	var analysis FailureAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		return nil, fmt.Errorf("failed to parse analysis: %w", err)
	}

	return &analysis, nil
}

// FailureAnalysis represents Claude's analysis of a failure
type FailureAnalysis struct {
	FailureType         string          `json:"failure_type"`
	RootCause           string          `json:"root_cause"`
	IsFlaky             bool            `json:"is_flaky"`
	FlakinessReason     string          `json:"flakiness_reason,omitempty"`
	RecommendedStrategy HealingStrategy `json:"recommended_strategy"`
	Confidence          float64         `json:"confidence"`
	AdditionalContext   string          `json:"additional_context,omitempty"`
}
