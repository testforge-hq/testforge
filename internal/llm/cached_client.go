package llm

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CachedClaudeClient wraps ClaudeClient with advanced caching and cost tracking
type CachedClaudeClient struct {
	*ClaudeClient
	tokenCache  *TokenCache
	costTracker *CostTracker
	logger      *zap.Logger
}

// CachedClientConfig holds configuration for the cached client
type CachedClientConfig struct {
	ClaudeConfig Config
	CacheConfig  TokenCacheConfig
	CostConfig   CostConfig
}

// DefaultCachedClientConfig returns default configuration
func DefaultCachedClientConfig() CachedClientConfig {
	return CachedClientConfig{
		ClaudeConfig: DefaultConfig(),
		CacheConfig:  DefaultTokenCacheConfig(),
		CostConfig: CostConfig{
			InputTokenCost:  3.0,   // $3 per 1M input tokens (Sonnet)
			OutputTokenCost: 15.0,  // $15 per 1M output tokens (Sonnet)
			DailyBudget:     100.0, // $100 daily limit
			AlertThreshold:  0.8,   // Alert at 80%
		},
	}
}

// NewCachedClaudeClient creates a new cached Claude client
func NewCachedClaudeClient(config CachedClientConfig, redis *redis.Client, logger *zap.Logger) (*CachedClaudeClient, error) {
	// Create base client
	baseClient, err := NewClaudeClient(config.ClaudeConfig)
	if err != nil {
		return nil, err
	}

	// Create token cache
	tokenCache := NewTokenCache(config.CacheConfig, redis, logger)

	// Create cost tracker
	costTracker := NewCostTracker(config.CostConfig, redis, logger)

	return &CachedClaudeClient{
		ClaudeClient: baseClient,
		tokenCache:   tokenCache,
		costTracker:  costTracker,
		logger:       logger,
	}, nil
}

// CompleteWithCaching attempts to get from cache first
func (c *CachedClaudeClient) CompleteWithCaching(ctx context.Context, prompt string, options *CompletionOptions) (string, error) {
	// Check budget
	if c.costTracker.IsOverBudget(ctx) {
		c.logger.Warn("daily budget exceeded, using cache only")
		// Try cache, but don't make API call
		key := c.getCacheKey(prompt, options)
		if response, _, found := c.tokenCache.Get(ctx, key); found {
			return response, nil
		}
		return "", ErrServiceUnavailable
	}

	// Generate cache key
	key := c.getCacheKey(prompt, options)

	// Try cache
	if response, tokens, found := c.tokenCache.Get(ctx, key); found {
		c.logger.Debug("cache hit",
			zap.String("key", key[:16]),
			zap.Int("tokens", tokens.InputTokens+tokens.OutputTokens),
		)
		c.costTracker.RecordUsage(ctx, tokens.InputTokens, tokens.OutputTokens, true)
		return response, nil
	}

	// Make API call
	startTime := time.Now()
	response, err := c.CompleteWithOptions(ctx, prompt, options)
	duration := time.Since(startTime)

	if err != nil {
		return "", err
	}

	// Estimate tokens (rough estimation)
	inputTokens := estimateTokens(prompt)
	outputTokens := estimateTokens(response)
	if options != nil && options.System != "" {
		inputTokens += estimateTokens(options.System)
	}

	// Cache the response
	c.tokenCache.Set(ctx, key, response, TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	})

	// Track cost
	c.costTracker.RecordUsage(ctx, inputTokens, outputTokens, false)

	c.logger.Debug("API call completed",
		zap.Duration("duration", duration),
		zap.Int("input_tokens", inputTokens),
		zap.Int("output_tokens", outputTokens),
	)

	return response, nil
}

// GetCacheStats returns cache statistics
func (c *CachedClaudeClient) GetCacheStats() CacheStats {
	return c.tokenCache.GetStats()
}

// GetCostStats returns cost statistics for today
func (c *CachedClaudeClient) GetCostStats(ctx context.Context) (*DailyCost, error) {
	today := time.Now().Format("2006-01-02")
	return c.costTracker.GetDailyCost(ctx, today)
}

// GetMonthlyCost returns cost for the current month
func (c *CachedClaudeClient) GetMonthlyCost(ctx context.Context) (*DailyCost, error) {
	return c.costTracker.GetMonthlyCost(ctx)
}

// ClearCache clears all caches
func (c *CachedClaudeClient) ClearCache(ctx context.Context) error {
	return c.tokenCache.Clear(ctx)
}

// Private helpers

func (c *CachedClaudeClient) getCacheKey(prompt string, options *CompletionOptions) string {
	opts := make(map[string]interface{})
	if options != nil {
		if options.Temperature != 0 {
			opts["temperature"] = options.Temperature
		}
		if options.MaxTokens != 0 {
			opts["max_tokens"] = options.MaxTokens
		}
		if options.System != "" {
			opts["system"] = options.System
		}
	}
	return c.tokenCache.CacheKey(prompt, c.model, opts)
}

// estimateTokens provides a rough token count estimation
// Claude uses roughly 4 characters per token on average
func estimateTokens(text string) int {
	return len(text) / 4
}

// CostEstimator helps estimate costs before making API calls
type CostEstimator struct {
	inputCostPerMillion  float64
	outputCostPerMillion float64
}

// NewCostEstimator creates a new cost estimator
func NewCostEstimator(inputCostPerMillion, outputCostPerMillion float64) *CostEstimator {
	return &CostEstimator{
		inputCostPerMillion:  inputCostPerMillion,
		outputCostPerMillion: outputCostPerMillion,
	}
}

// EstimateCost estimates the cost for a prompt
func (ce *CostEstimator) EstimateCost(prompt string, estimatedOutputTokens int) float64 {
	inputTokens := estimateTokens(prompt)
	inputCost := float64(inputTokens) / 1000000 * ce.inputCostPerMillion
	outputCost := float64(estimatedOutputTokens) / 1000000 * ce.outputCostPerMillion
	return inputCost + outputCost
}

// EstimatePromptCost estimates just the input cost
func (ce *CostEstimator) EstimatePromptCost(prompt string) float64 {
	inputTokens := estimateTokens(prompt)
	return float64(inputTokens) / 1000000 * ce.inputCostPerMillion
}

// GetModelPricing returns pricing for different Claude models
func GetModelPricing(model string) (inputCost, outputCost float64) {
	// Pricing as of 2024 (per 1M tokens)
	pricing := map[string][2]float64{
		"claude-3-opus-20240229":    {15.0, 75.0},
		"claude-3-sonnet-20240229":  {3.0, 15.0},
		"claude-3-haiku-20240307":   {0.25, 1.25},
		"claude-sonnet-4-20250514":  {3.0, 15.0},
		"claude-3-5-sonnet-20241022": {3.0, 15.0},
	}

	if p, ok := pricing[model]; ok {
		return p[0], p[1]
	}

	// Default to Sonnet pricing
	return 3.0, 15.0
}
