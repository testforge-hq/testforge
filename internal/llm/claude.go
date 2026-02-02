package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// ClaudeClient provides access to Claude API with enterprise features
type ClaudeClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client

	// Rate limiting
	rateLimiter *rate.Limiter

	// Caching
	cache     *Cache
	cacheTTL  time.Duration

	// Metrics
	metrics *Metrics
	mu      sync.RWMutex
}

// Config for Claude client
type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	Timeout         time.Duration
	RateLimitRPM    int           // Requests per minute
	CacheTTL        time.Duration
	MaxRetries      int
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		BaseURL:      "https://api.anthropic.com",
		Model:        "claude-sonnet-4-20250514",
		Timeout:      120 * time.Second,
		RateLimitRPM: 50,
		CacheTTL:     24 * time.Hour,
		MaxRetries:   3,
	}
}

// Metrics tracks API usage
type Metrics struct {
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	TotalTokensIn   int64
	TotalTokensOut  int64
	TotalCost       float64
	TotalLatencyMs  int64
	CacheHits       int64
	CacheMisses     int64
}

// Cache for LLM responses
type Cache struct {
	data map[string]cacheEntry
	mu   sync.RWMutex
}

type cacheEntry struct {
	response  []byte
	expiresAt time.Time
}

// NewCache creates a new cache
func NewCache() *Cache {
	return &Cache{
		data: make(map[string]cacheEntry),
	}
}

// Get retrieves from cache
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.response, true
}

// Set stores in cache
func (c *Cache) Set(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = cacheEntry{
		response:  value,
		expiresAt: time.Now().Add(ttl),
	}
}

// NewClaudeClient creates a new Claude API client
func NewClaudeClient(cfg Config) (*ClaudeClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Merge with defaults
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultConfig().BaseURL
	}
	if cfg.Model == "" {
		cfg.Model = DefaultConfig().Model
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultConfig().Timeout
	}
	if cfg.RateLimitRPM == 0 {
		cfg.RateLimitRPM = DefaultConfig().RateLimitRPM
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = DefaultConfig().CacheTTL
	}

	// Create rate limiter (tokens per second = RPM / 60)
	limiter := rate.NewLimiter(rate.Limit(float64(cfg.RateLimitRPM)/60.0), 1)

	return &ClaudeClient{
		apiKey:  cfg.APIKey,
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		rateLimiter: limiter,
		cache:       NewCache(),
		cacheTTL:    cfg.CacheTTL,
		metrics:     &Metrics{},
	}, nil
}

// Request represents a Claude API request
type Request struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

// Message represents a conversation message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response represents a Claude API response
type Response struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// ContentBlock represents a content block in the response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage contains token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Complete sends a completion request to Claude
func (c *ClaudeClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, *Usage, error) {
	atomic.AddInt64(&c.metrics.TotalRequests, 1)

	// Check cache
	cacheKey := c.cacheKey(systemPrompt, userPrompt)
	if cached, ok := c.cache.Get(cacheKey); ok {
		atomic.AddInt64(&c.metrics.CacheHits, 1)
		return string(cached), nil, nil
	}
	atomic.AddInt64(&c.metrics.CacheMisses, 1)

	// Rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		atomic.AddInt64(&c.metrics.FailedRequests, 1)
		return "", nil, fmt.Errorf("rate limit: %w", err)
	}

	start := time.Now()

	// Build request
	req := Request{
		Model:     c.model,
		MaxTokens: 8192,
		System:    systemPrompt,
		Messages: []Message{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3, // Lower temperature for more deterministic output
	}

	// Make request
	resp, err := c.doRequest(ctx, req)
	if err != nil {
		atomic.AddInt64(&c.metrics.FailedRequests, 1)
		return "", nil, err
	}

	// Update metrics
	atomic.AddInt64(&c.metrics.SuccessRequests, 1)
	atomic.AddInt64(&c.metrics.TotalTokensIn, int64(resp.Usage.InputTokens))
	atomic.AddInt64(&c.metrics.TotalTokensOut, int64(resp.Usage.OutputTokens))
	atomic.AddInt64(&c.metrics.TotalLatencyMs, time.Since(start).Milliseconds())

	c.mu.Lock()
	c.metrics.TotalCost += c.calculateCost(resp.Usage)
	c.mu.Unlock()

	// Extract text
	if len(resp.Content) == 0 {
		return "", &resp.Usage, fmt.Errorf("empty response")
	}

	text := resp.Content[0].Text

	// Cache response
	c.cache.Set(cacheKey, []byte(text), c.cacheTTL)

	return text, &resp.Usage, nil
}

// CompleteJSON sends a completion request and parses JSON response
func (c *ClaudeClient) CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, result interface{}) (*Usage, error) {
	// Add JSON instruction to system prompt
	jsonSystemPrompt := systemPrompt + "\n\nIMPORTANT: Return ONLY valid JSON. No markdown, no code blocks, no explanations."

	var lastErr error
	var totalUsage Usage

	for attempt := 0; attempt < 3; attempt++ {
		text, usage, err := c.Complete(ctx, jsonSystemPrompt, userPrompt)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * time.Second)
			continue
		}

		if usage != nil {
			totalUsage.InputTokens += usage.InputTokens
			totalUsage.OutputTokens += usage.OutputTokens
		}

		// Extract JSON from response
		jsonStr := extractJSON(text)
		if jsonStr == "" {
			lastErr = fmt.Errorf("no JSON found in response")
			continue
		}

		// Parse JSON
		if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
			lastErr = fmt.Errorf("invalid JSON: %w", err)
			continue
		}

		return &totalUsage, nil
	}

	return &totalUsage, fmt.Errorf("failed after 3 attempts: %w", lastErr)
}

// doRequest performs the HTTP request
func (c *ClaudeClient) doRequest(ctx context.Context, req Request) (*Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp Response
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &apiResp, nil
}

// calculateCost calculates the cost of a request
func (c *ClaudeClient) calculateCost(usage Usage) float64 {
	// Claude Sonnet pricing (as of 2024)
	// $3 per million input tokens, $15 per million output tokens
	inputCost := float64(usage.InputTokens) / 1000000 * 3.00
	outputCost := float64(usage.OutputTokens) / 1000000 * 15.00
	return inputCost + outputCost
}

// cacheKey generates a cache key from prompts
func (c *ClaudeClient) cacheKey(systemPrompt, userPrompt string) string {
	// Simple hash - in production use a proper hash function
	combined := systemPrompt + "|" + userPrompt
	if len(combined) > 100 {
		combined = combined[:100]
	}
	return fmt.Sprintf("%s_%d", c.model, len(combined))
}

// GetMetrics returns current metrics
func (c *ClaudeClient) GetMetrics() Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Metrics{
		TotalRequests:   atomic.LoadInt64(&c.metrics.TotalRequests),
		SuccessRequests: atomic.LoadInt64(&c.metrics.SuccessRequests),
		FailedRequests:  atomic.LoadInt64(&c.metrics.FailedRequests),
		TotalTokensIn:   atomic.LoadInt64(&c.metrics.TotalTokensIn),
		TotalTokensOut:  atomic.LoadInt64(&c.metrics.TotalTokensOut),
		TotalCost:       c.metrics.TotalCost,
		TotalLatencyMs:  atomic.LoadInt64(&c.metrics.TotalLatencyMs),
		CacheHits:       atomic.LoadInt64(&c.metrics.CacheHits),
		CacheMisses:     atomic.LoadInt64(&c.metrics.CacheMisses),
	}
}

// GetModel returns the model being used
func (c *ClaudeClient) GetModel() string {
	return c.model
}

// extractJSON extracts JSON from a string that might contain markdown or other text
func extractJSON(text string) string {
	// First, try to find JSON in code blocks
	codeBlockPattern := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	matches := codeBlockPattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// Try to find JSON object or array directly
	text = strings.TrimSpace(text)

	// Find the first { or [
	startObj := strings.Index(text, "{")
	startArr := strings.Index(text, "[")

	start := -1
	isArray := false

	if startObj >= 0 && (startArr < 0 || startObj < startArr) {
		start = startObj
	} else if startArr >= 0 {
		start = startArr
		isArray = true
	}

	if start < 0 {
		return ""
	}

	// Find matching closing bracket
	text = text[start:]
	depth := 0
	inString := false
	escaped := false

	openBracket := byte('{')
	closeBracket := byte('}')
	if isArray {
		openBracket = '['
		closeBracket = ']'
	}

	for i := 0; i < len(text); i++ {
		c := text[i]

		if escaped {
			escaped = false
			continue
		}

		if c == '\\' && inString {
			escaped = true
			continue
		}

		if c == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		if c == openBracket {
			depth++
		} else if c == closeBracket {
			depth--
			if depth == 0 {
				return text[:i+1]
			}
		}
	}

	return ""
}
