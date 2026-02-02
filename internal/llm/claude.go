package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"testforge/internal/resilience"
)

var (
	// ErrServiceUnavailable is returned when the Claude API is unavailable
	ErrServiceUnavailable = errors.New("claude API is temporarily unavailable")

	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open - too many recent failures")
)

// ClaudeClient provides access to Claude API with enterprise features
type ClaudeClient struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client

	// Rate limiting
	rateLimiter *rate.Limiter

	// Circuit breaker for resilience
	circuitBreaker *resilience.CircuitBreaker

	// Caching with LRU eviction
	cache    *LRUCache
	cacheTTL time.Duration

	// Metrics
	metrics *Metrics
	mu      sync.RWMutex

	// Fallback behavior
	fallbackEnabled bool
}

// Config for Claude client
type Config struct {
	APIKey       string
	BaseURL      string
	Model        string
	MaxTokens    int
	Timeout      time.Duration
	RateLimitRPM int           // Requests per minute
	CacheTTL     time.Duration
	CacheSize    int           // Max cache entries
	MaxRetries   int

	// Circuit breaker settings
	CircuitBreakerEnabled  bool          // Enable circuit breaker (default: true)
	CircuitBreakerTimeout  time.Duration // Time before trying again after circuit opens
	CircuitBreakerInterval time.Duration // Interval for clearing failure counts in closed state
	CircuitBreakerMinReqs  int           // Minimum requests before tripping

	// Fallback settings
	FallbackEnabled bool // Return cached response on circuit open
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		BaseURL:                "https://api.anthropic.com",
		Model:                  "claude-sonnet-4-20250514",
		MaxTokens:              8192,
		Timeout:                120 * time.Second,
		RateLimitRPM:           50,
		CacheTTL:               24 * time.Hour,
		CacheSize:              1000,
		MaxRetries:             3,
		CircuitBreakerEnabled:  true,
		CircuitBreakerTimeout:  30 * time.Second,
		CircuitBreakerInterval: 60 * time.Second,
		CircuitBreakerMinReqs:  5,
		FallbackEnabled:        true,
	}
}

// Metrics tracks API usage
type Metrics struct {
	TotalRequests    int64
	SuccessRequests  int64
	FailedRequests   int64
	TotalTokensIn    int64
	TotalTokensOut   int64
	TotalCost        float64
	TotalLatencyMs   int64
	CacheHits        int64
	CacheMisses      int64
	CircuitBreaks    int64
	FallbacksUsed    int64
}

// LRUCache implements a thread-safe LRU cache with TTL
type LRUCache struct {
	maxSize int
	ttl     time.Duration
	data    map[string]*cacheEntry
	order   []string // LRU order (oldest first)
	mu      sync.RWMutex
}

type cacheEntry struct {
	response  []byte
	expiresAt time.Time
	key       string
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(maxSize int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		maxSize: maxSize,
		ttl:     ttl,
		data:    make(map[string]*cacheEntry),
		order:   make([]string, 0, maxSize),
	}
}

// Get retrieves from cache
func (c *LRUCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}

	// Check TTL
	if time.Now().After(entry.expiresAt) {
		c.removeEntry(key)
		return nil, false
	}

	// Move to end (most recently used)
	c.moveToEnd(key)

	return entry.response, true
}

// Set stores in cache with LRU eviction
func (c *LRUCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If key exists, update and move to end
	if _, exists := c.data[key]; exists {
		c.data[key] = &cacheEntry{
			response:  value,
			expiresAt: time.Now().Add(c.ttl),
			key:       key,
		}
		c.moveToEnd(key)
		return
	}

	// Evict oldest if at capacity
	for len(c.data) >= c.maxSize && len(c.order) > 0 {
		oldest := c.order[0]
		c.removeEntry(oldest)
	}

	// Add new entry
	c.data[key] = &cacheEntry{
		response:  value,
		expiresAt: time.Now().Add(c.ttl),
		key:       key,
	}
	c.order = append(c.order, key)
}

func (c *LRUCache) removeEntry(key string) {
	delete(c.data, key)
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

func (c *LRUCache) moveToEnd(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)
			break
		}
	}
}

// Size returns current cache size
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// NewClaudeClient creates a new Claude API client
func NewClaudeClient(cfg Config) (*ClaudeClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Merge with defaults
	defaults := DefaultConfig()
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaults.BaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaults.Model
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = defaults.MaxTokens
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.RateLimitRPM == 0 {
		cfg.RateLimitRPM = defaults.RateLimitRPM
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = defaults.CacheTTL
	}
	if cfg.CacheSize == 0 {
		cfg.CacheSize = defaults.CacheSize
	}
	if cfg.CircuitBreakerTimeout == 0 {
		cfg.CircuitBreakerTimeout = defaults.CircuitBreakerTimeout
	}
	if cfg.CircuitBreakerInterval == 0 {
		cfg.CircuitBreakerInterval = defaults.CircuitBreakerInterval
	}
	if cfg.CircuitBreakerMinReqs == 0 {
		cfg.CircuitBreakerMinReqs = defaults.CircuitBreakerMinReqs
	}

	// Create rate limiter with burst capacity
	// RPM/60 = requests per second, burst of 5 for handling spikes
	limiter := rate.NewLimiter(rate.Limit(float64(cfg.RateLimitRPM)/60.0), 5)

	client := &ClaudeClient{
		apiKey:          cfg.APIKey,
		baseURL:         cfg.BaseURL,
		model:           cfg.Model,
		maxTokens:       cfg.MaxTokens,
		httpClient:      &http.Client{Timeout: cfg.Timeout},
		rateLimiter:     limiter,
		cache:           NewLRUCache(cfg.CacheSize, cfg.CacheTTL),
		cacheTTL:        cfg.CacheTTL,
		metrics:         &Metrics{},
		fallbackEnabled: cfg.FallbackEnabled,
	}

	// Configure circuit breaker
	if cfg.CircuitBreakerEnabled {
		minReqs := uint32(cfg.CircuitBreakerMinReqs)
		cbConfig := resilience.CircuitBreakerConfig{
			Name:        "claude-api",
			MaxRequests: 3, // Allow 3 requests in half-open state
			Interval:    cfg.CircuitBreakerInterval,
			Timeout:     cfg.CircuitBreakerTimeout,
			ReadyToTrip: func(counts resilience.Counts) bool {
				// Trip if failure rate exceeds 60% with at least minReqs requests
				if counts.Requests < minReqs {
					return false
				}
				failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
				return failureRatio >= 0.6
			},
			OnStateChange: func(name string, from, to resilience.CircuitBreakerState) {
				// Log state changes (metrics tracking)
				atomic.AddInt64(&client.metrics.CircuitBreaks, 1)
			},
			IsSuccessful: func(err error) bool {
				// Consider rate limit errors as failures that should trip the breaker
				return err == nil
			},
		}
		client.circuitBreaker = resilience.NewCircuitBreaker(cbConfig)
	}

	return client, nil
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
	return c.CompleteWithOptions(ctx, systemPrompt, userPrompt, 0.3, true)
}

// CompleteWithOptions sends a completion request with custom options
func (c *ClaudeClient) CompleteWithOptions(ctx context.Context, systemPrompt, userPrompt string, temperature float64, useCache bool) (string, *Usage, error) {
	atomic.AddInt64(&c.metrics.TotalRequests, 1)

	// Check cache
	cacheKey := c.cacheKey(systemPrompt, userPrompt)
	if useCache {
		if cached, ok := c.cache.Get(cacheKey); ok {
			atomic.AddInt64(&c.metrics.CacheHits, 1)
			return string(cached), nil, nil
		}
	}
	atomic.AddInt64(&c.metrics.CacheMisses, 1)

	// Check circuit breaker before making request
	if c.circuitBreaker != nil {
		state := c.circuitBreaker.State()
		if state == resilience.StateOpen {
			// Try fallback if enabled
			if c.fallbackEnabled {
				if cached, ok := c.cache.Get(cacheKey); ok {
					atomic.AddInt64(&c.metrics.FallbacksUsed, 1)
					return string(cached), nil, nil
				}
			}
			atomic.AddInt64(&c.metrics.FailedRequests, 1)
			return "", nil, ErrCircuitOpen
		}
	}

	// Rate limiting with context
	if err := c.rateLimiter.Wait(ctx); err != nil {
		atomic.AddInt64(&c.metrics.FailedRequests, 1)
		return "", nil, fmt.Errorf("rate limit: %w", err)
	}

	start := time.Now()

	// Build request
	req := Request{
		Model:     c.model,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Messages: []Message{
			{Role: "user", Content: userPrompt},
		},
		Temperature: temperature,
	}

	// Make request with circuit breaker if enabled
	var resp *Response
	var err error

	if c.circuitBreaker != nil {
		result, cbErr := c.circuitBreaker.ExecuteWithContext(ctx, func(ctx context.Context) (interface{}, error) {
			return c.doRequest(ctx, req)
		})
		if cbErr != nil {
			// Check if it's a circuit breaker error
			if errors.Is(cbErr, resilience.ErrCircuitOpen) || errors.Is(cbErr, resilience.ErrTooManyRequests) {
				// Try fallback
				if c.fallbackEnabled {
					if cached, ok := c.cache.Get(cacheKey); ok {
						atomic.AddInt64(&c.metrics.FallbacksUsed, 1)
						return string(cached), nil, nil
					}
				}
				atomic.AddInt64(&c.metrics.FailedRequests, 1)
				return "", nil, ErrCircuitOpen
			}
			err = cbErr
		} else if result != nil {
			resp = result.(*Response)
		}
	} else {
		resp, err = c.doRequest(ctx, req)
	}

	if err != nil {
		atomic.AddInt64(&c.metrics.FailedRequests, 1)
		// Try fallback on error if enabled
		if c.fallbackEnabled {
			if cached, ok := c.cache.Get(cacheKey); ok {
				atomic.AddInt64(&c.metrics.FallbacksUsed, 1)
				return string(cached), nil, nil
			}
		}
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
		return "", &resp.Usage, fmt.Errorf("empty response from Claude")
	}

	text := resp.Content[0].Text

	// Cache response
	if useCache {
		c.cache.Set(cacheKey, []byte(text))
	}

	return text, &resp.Usage, nil
}

// CompleteJSON sends a completion request and parses JSON response
func (c *ClaudeClient) CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, result interface{}) (*Usage, error) {
	// Add JSON instruction to system prompt
	jsonSystemPrompt := systemPrompt + "\n\nIMPORTANT: Return ONLY valid JSON. No markdown, no code blocks, no explanations outside the JSON."

	var lastErr error
	var totalUsage Usage

	for attempt := 0; attempt < 3; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return &totalUsage, ctx.Err()
		}

		text, usage, err := c.CompleteWithOptions(ctx, jsonSystemPrompt, userPrompt, 0.2, attempt == 0) // Only cache first attempt
		if err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return &totalUsage, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
			continue
		}

		if usage != nil {
			totalUsage.InputTokens += usage.InputTokens
			totalUsage.OutputTokens += usage.OutputTokens
		}

		// Extract JSON from response
		jsonStr := extractJSON(text)
		if jsonStr == "" {
			lastErr = fmt.Errorf("no JSON found in response: %s", truncateString(text, 200))
			continue
		}

		// Parse JSON
		if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
			lastErr = fmt.Errorf("invalid JSON: %w (response: %s)", err, truncateString(jsonStr, 200))
			continue
		}

		return &totalUsage, nil
	}

	return &totalUsage, fmt.Errorf("failed after 3 attempts: %w", lastErr)
}

// doRequest performs the HTTP request with proper context handling
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
		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, truncateString(string(respBody), 500))
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

// cacheKey generates a proper cache key using SHA256 hash
func (c *ClaudeClient) cacheKey(systemPrompt, userPrompt string) string {
	combined := systemPrompt + "\x00" + userPrompt // Use null byte as separator
	hash := sha256.Sum256([]byte(combined))
	return c.model + "_" + hex.EncodeToString(hash[:16]) // 16 bytes = 32 hex chars
}

// GetMetrics returns current metrics (thread-safe copy)
func (c *ClaudeClient) GetMetrics() Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return Metrics{
		TotalRequests:    atomic.LoadInt64(&c.metrics.TotalRequests),
		SuccessRequests:  atomic.LoadInt64(&c.metrics.SuccessRequests),
		FailedRequests:   atomic.LoadInt64(&c.metrics.FailedRequests),
		TotalTokensIn:    atomic.LoadInt64(&c.metrics.TotalTokensIn),
		TotalTokensOut:   atomic.LoadInt64(&c.metrics.TotalTokensOut),
		TotalCost:        c.metrics.TotalCost,
		TotalLatencyMs:   atomic.LoadInt64(&c.metrics.TotalLatencyMs),
		CacheHits:        atomic.LoadInt64(&c.metrics.CacheHits),
		CacheMisses:      atomic.LoadInt64(&c.metrics.CacheMisses),
		CircuitBreaks:    atomic.LoadInt64(&c.metrics.CircuitBreaks),
		FallbacksUsed:    atomic.LoadInt64(&c.metrics.FallbacksUsed),
	}
}

// GetCircuitBreakerState returns the current circuit breaker state
func (c *ClaudeClient) GetCircuitBreakerState() string {
	if c.circuitBreaker == nil {
		return "disabled"
	}
	return c.circuitBreaker.State().String()
}

// IsHealthy returns true if the client can accept requests
func (c *ClaudeClient) IsHealthy() bool {
	if c.circuitBreaker == nil {
		return true
	}
	return c.circuitBreaker.State() != resilience.StateOpen
}

// GetModel returns the model being used
func (c *ClaudeClient) GetModel() string {
	return c.model
}

// GetCacheSize returns current cache size
func (c *ClaudeClient) GetCacheSize() int {
	return c.cache.Size()
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

// truncateString truncates a string to maxLen with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
