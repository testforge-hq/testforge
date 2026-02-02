package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TokenCacheConfig holds cache configuration
type TokenCacheConfig struct {
	// Redis configuration
	RedisEnabled bool
	RedisTTL     time.Duration

	// Memory cache configuration
	MemoryEnabled   bool
	MemoryMaxSize   int
	MemoryTTL       time.Duration

	// Semantic cache (for similar prompts)
	SemanticEnabled    bool
	SemanticThreshold  float32 // Minimum similarity score (0.0-1.0)

	// Cost tracking
	TrackCosts       bool
	InputTokenCost   float64 // Cost per 1K input tokens
	OutputTokenCost  float64 // Cost per 1K output tokens
}

// DefaultTokenCacheConfig returns default cache configuration
func DefaultTokenCacheConfig() TokenCacheConfig {
	return TokenCacheConfig{
		RedisEnabled:      true,
		RedisTTL:          24 * time.Hour,
		MemoryEnabled:     true,
		MemoryMaxSize:     1000,
		MemoryTTL:         1 * time.Hour,
		SemanticEnabled:   false,
		SemanticThreshold: 0.95,
		TrackCosts:        true,
		InputTokenCost:    0.003,  // $3 per 1M input tokens
		OutputTokenCost:   0.015,  // $15 per 1M output tokens
	}
}

// TokenCache provides multi-layer caching for LLM responses
type TokenCache struct {
	config TokenCacheConfig
	redis  *redis.Client
	logger *zap.Logger

	// In-memory cache with LRU eviction
	memCache   map[string]*tokenCacheEntry
	memCacheMu sync.RWMutex
	lruList    []string // Simple LRU tracking

	// Statistics
	stats   *CacheStats
	statsMu sync.Mutex
}

type tokenCacheEntry struct {
	response    string
	tokens      TokenUsage
	createdAt   time.Time
	hitCount    int
}

// TokenUsage tracks token usage
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

// CacheStats tracks cache statistics
type CacheStats struct {
	MemoryHits     int64   `json:"memory_hits"`
	MemoryMisses   int64   `json:"memory_misses"`
	RedisHits      int64   `json:"redis_hits"`
	RedisMisses    int64   `json:"redis_misses"`
	TotalRequests  int64   `json:"total_requests"`
	TokensSaved    int64   `json:"tokens_saved"`
	CostSaved      float64 `json:"cost_saved"`
	CacheHitRate   float64 `json:"cache_hit_rate"`
	LastUpdated    time.Time `json:"last_updated"`
}

// NewTokenCache creates a new token cache
func NewTokenCache(config TokenCacheConfig, redisClient *redis.Client, logger *zap.Logger) *TokenCache {
	tc := &TokenCache{
		config:   config,
		redis:    redisClient,
		logger:   logger,
		memCache: make(map[string]*tokenCacheEntry),
		lruList:  make([]string, 0, config.MemoryMaxSize),
		stats:    &CacheStats{},
	}

	// Start background cleanup
	if config.MemoryEnabled {
		go tc.cleanupLoop()
	}

	return tc
}

// CacheKey generates a cache key for a prompt
func (tc *TokenCache) CacheKey(prompt string, model string, options map[string]interface{}) string {
	// Include relevant options in the key
	keyData := map[string]interface{}{
		"prompt": prompt,
		"model":  model,
	}

	// Only include options that affect output
	if temp, ok := options["temperature"]; ok {
		keyData["temperature"] = temp
	}
	if maxTokens, ok := options["max_tokens"]; ok {
		keyData["max_tokens"] = maxTokens
	}
	if systemPrompt, ok := options["system"]; ok {
		keyData["system"] = systemPrompt
	}

	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Get retrieves a cached response
func (tc *TokenCache) Get(ctx context.Context, key string) (string, *TokenUsage, bool) {
	tc.statsMu.Lock()
	tc.stats.TotalRequests++
	tc.statsMu.Unlock()

	// Check memory cache first
	if tc.config.MemoryEnabled {
		tc.memCacheMu.RLock()
		if entry, ok := tc.memCache[key]; ok {
			if time.Since(entry.createdAt) < tc.config.MemoryTTL {
				entry.hitCount++
				tc.memCacheMu.RUnlock()

				tc.recordHit(true, entry.tokens)
				return entry.response, &entry.tokens, true
			}
		}
		tc.memCacheMu.RUnlock()

		tc.statsMu.Lock()
		tc.stats.MemoryMisses++
		tc.statsMu.Unlock()
	}

	// Check Redis cache
	if tc.config.RedisEnabled && tc.redis != nil {
		data, err := tc.redis.Get(ctx, "llm:"+key).Bytes()
		if err == nil {
			var cached struct {
				Response string     `json:"response"`
				Tokens   TokenUsage `json:"tokens"`
			}
			if err := json.Unmarshal(data, &cached); err == nil {
				// Promote to memory cache
				tc.setMemoryCache(key, cached.Response, cached.Tokens)

				tc.recordHit(false, cached.Tokens)
				return cached.Response, &cached.Tokens, true
			}
		}

		tc.statsMu.Lock()
		tc.stats.RedisMisses++
		tc.statsMu.Unlock()
	}

	return "", nil, false
}

// Set stores a response in cache
func (tc *TokenCache) Set(ctx context.Context, key string, response string, tokens TokenUsage) {
	// Store in memory cache
	if tc.config.MemoryEnabled {
		tc.setMemoryCache(key, response, tokens)
	}

	// Store in Redis cache
	if tc.config.RedisEnabled && tc.redis != nil {
		data, _ := json.Marshal(map[string]interface{}{
			"response": response,
			"tokens":   tokens,
		})
		tc.redis.Set(ctx, "llm:"+key, data, tc.config.RedisTTL)
	}
}

// GetStats returns cache statistics
func (tc *TokenCache) GetStats() CacheStats {
	tc.statsMu.Lock()
	defer tc.statsMu.Unlock()

	stats := *tc.stats
	if stats.TotalRequests > 0 {
		stats.CacheHitRate = float64(stats.MemoryHits+stats.RedisHits) / float64(stats.TotalRequests)
	}
	stats.LastUpdated = time.Now()

	return stats
}

// ResetStats resets cache statistics
func (tc *TokenCache) ResetStats() {
	tc.statsMu.Lock()
	defer tc.statsMu.Unlock()
	tc.stats = &CacheStats{}
}

// Clear clears all caches
func (tc *TokenCache) Clear(ctx context.Context) error {
	// Clear memory cache
	tc.memCacheMu.Lock()
	tc.memCache = make(map[string]*tokenCacheEntry)
	tc.lruList = make([]string, 0, tc.config.MemoryMaxSize)
	tc.memCacheMu.Unlock()

	// Clear Redis cache (keys with llm: prefix)
	if tc.redis != nil {
		iter := tc.redis.Scan(ctx, 0, "llm:*", 100).Iterator()
		for iter.Next(ctx) {
			tc.redis.Del(ctx, iter.Val())
		}
		return iter.Err()
	}

	return nil
}

// Private methods

func (tc *TokenCache) setMemoryCache(key string, response string, tokens TokenUsage) {
	tc.memCacheMu.Lock()
	defer tc.memCacheMu.Unlock()

	// Check if we need to evict
	if len(tc.memCache) >= tc.config.MemoryMaxSize {
		tc.evictOldest()
	}

	tc.memCache[key] = &tokenCacheEntry{
		response:  response,
		tokens:    tokens,
		createdAt: time.Now(),
		hitCount:  0,
	}
	tc.lruList = append(tc.lruList, key)
}

func (tc *TokenCache) evictOldest() {
	// Remove oldest entries (first 10%)
	toRemove := tc.config.MemoryMaxSize / 10
	if toRemove < 1 {
		toRemove = 1
	}

	for i := 0; i < toRemove && len(tc.lruList) > 0; i++ {
		key := tc.lruList[0]
		tc.lruList = tc.lruList[1:]
		delete(tc.memCache, key)
	}
}

func (tc *TokenCache) recordHit(memory bool, tokens TokenUsage) {
	tc.statsMu.Lock()
	defer tc.statsMu.Unlock()

	if memory {
		tc.stats.MemoryHits++
	} else {
		tc.stats.RedisHits++
	}

	// Calculate tokens and cost saved
	totalTokens := int64(tokens.InputTokens + tokens.OutputTokens)
	tc.stats.TokensSaved += totalTokens

	if tc.config.TrackCosts {
		inputCost := float64(tokens.InputTokens) / 1000 * tc.config.InputTokenCost
		outputCost := float64(tokens.OutputTokens) / 1000 * tc.config.OutputTokenCost
		tc.stats.CostSaved += inputCost + outputCost
	}
}

func (tc *TokenCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		tc.cleanup()
	}
}

func (tc *TokenCache) cleanup() {
	tc.memCacheMu.Lock()
	defer tc.memCacheMu.Unlock()

	now := time.Now()
	newLRU := make([]string, 0, len(tc.lruList))

	for _, key := range tc.lruList {
		if entry, ok := tc.memCache[key]; ok {
			if now.Sub(entry.createdAt) > tc.config.MemoryTTL {
				delete(tc.memCache, key)
			} else {
				newLRU = append(newLRU, key)
			}
		}
	}

	tc.lruList = newLRU
}

// PromptCache provides semantic caching for similar prompts
type PromptCache struct {
	tokenCache *TokenCache
	embedder   Embedder
	qdrant     QdrantClient
	logger     *zap.Logger
	threshold  float32
}

// Embedder interface for generating embeddings
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// QdrantClient interface for vector search
type QdrantClient interface {
	SearchSimilar(ctx context.Context, embedding []float32, filter map[string]interface{}, limit int) ([]SearchResult, error)
	UpsertPattern(ctx context.Context, id string, embedding []float32, metadata map[string]interface{}) error
}

// SearchResult from Qdrant
type SearchResult struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

// NewPromptCache creates a semantic prompt cache
func NewPromptCache(tokenCache *TokenCache, embedder Embedder, qdrant QdrantClient, threshold float32, logger *zap.Logger) *PromptCache {
	return &PromptCache{
		tokenCache: tokenCache,
		embedder:   embedder,
		qdrant:     qdrant,
		logger:     logger,
		threshold:  threshold,
	}
}

// Get retrieves a semantically similar cached response
func (pc *PromptCache) Get(ctx context.Context, prompt string, model string) (string, *TokenUsage, bool) {
	// First try exact match
	key := pc.tokenCache.CacheKey(prompt, model, nil)
	if response, tokens, found := pc.tokenCache.Get(ctx, key); found {
		return response, tokens, true
	}

	// Try semantic search
	if pc.embedder != nil && pc.qdrant != nil {
		embedding, err := pc.embedder.Embed(ctx, prompt)
		if err != nil {
			pc.logger.Debug("failed to generate embedding", zap.Error(err))
			return "", nil, false
		}

		results, err := pc.qdrant.SearchSimilar(ctx, embedding, map[string]interface{}{
			"must": []map[string]interface{}{
				{"key": "model", "match": map[string]interface{}{"value": model}},
			},
		}, 1)
		if err != nil {
			pc.logger.Debug("semantic search failed", zap.Error(err))
			return "", nil, false
		}

		if len(results) > 0 && results[0].Score >= pc.threshold {
			// Found similar prompt
			if cachedKey, ok := results[0].Payload["cache_key"].(string); ok {
				if response, tokens, found := pc.tokenCache.Get(ctx, cachedKey); found {
					pc.logger.Debug("semantic cache hit",
						zap.Float32("similarity", results[0].Score),
					)
					return response, tokens, true
				}
			}
		}
	}

	return "", nil, false
}

// Set stores a response with its semantic embedding
func (pc *PromptCache) Set(ctx context.Context, prompt string, model string, response string, tokens TokenUsage) {
	key := pc.tokenCache.CacheKey(prompt, model, nil)
	pc.tokenCache.Set(ctx, key, response, tokens)

	// Store semantic embedding
	if pc.embedder != nil && pc.qdrant != nil {
		embedding, err := pc.embedder.Embed(ctx, prompt)
		if err != nil {
			pc.logger.Debug("failed to generate embedding for cache", zap.Error(err))
			return
		}

		err = pc.qdrant.UpsertPattern(ctx, key, embedding, map[string]interface{}{
			"cache_key": key,
			"model":     model,
			"prompt":    truncateForStorage(prompt, 500),
		})
		if err != nil {
			pc.logger.Debug("failed to store semantic embedding", zap.Error(err))
		}
	}
}

func truncateForStorage(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// CostTracker tracks LLM costs
type CostTracker struct {
	config CostConfig
	redis  *redis.Client
	logger *zap.Logger
	mu     sync.Mutex

	// Daily costs
	dailyCosts map[string]*DailyCost
}

// CostConfig holds cost tracking configuration
type CostConfig struct {
	InputTokenCost  float64 // Cost per 1M input tokens
	OutputTokenCost float64 // Cost per 1M output tokens
	DailyBudget     float64 // Daily budget limit
	AlertThreshold  float64 // Percentage of budget to trigger alert
}

// DailyCost tracks daily cost
type DailyCost struct {
	Date         string  `json:"date"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalCost    float64 `json:"total_cost"`
	Requests     int64   `json:"requests"`
	CacheHits    int64   `json:"cache_hits"`
}

// NewCostTracker creates a new cost tracker
func NewCostTracker(config CostConfig, redis *redis.Client, logger *zap.Logger) *CostTracker {
	return &CostTracker{
		config:     config,
		redis:      redis,
		logger:     logger,
		dailyCosts: make(map[string]*DailyCost),
	}
}

// RecordUsage records token usage and cost
func (ct *CostTracker) RecordUsage(ctx context.Context, inputTokens, outputTokens int, cached bool) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	daily, ok := ct.dailyCosts[today]
	if !ok {
		daily = &DailyCost{Date: today}
		ct.dailyCosts[today] = daily
	}

	daily.Requests++
	if cached {
		daily.CacheHits++
	} else {
		daily.InputTokens += int64(inputTokens)
		daily.OutputTokens += int64(outputTokens)

		inputCost := float64(inputTokens) / 1000000 * ct.config.InputTokenCost
		outputCost := float64(outputTokens) / 1000000 * ct.config.OutputTokenCost
		daily.TotalCost += inputCost + outputCost
	}

	// Check budget
	if ct.config.DailyBudget > 0 && daily.TotalCost >= ct.config.DailyBudget*ct.config.AlertThreshold {
		ct.logger.Warn("approaching daily budget",
			zap.Float64("current_cost", daily.TotalCost),
			zap.Float64("budget", ct.config.DailyBudget),
		)
	}

	// Persist to Redis
	if ct.redis != nil {
		data, _ := json.Marshal(daily)
		ct.redis.Set(ctx, "cost:"+today, data, 48*time.Hour)
	}
}

// GetDailyCost returns cost for a specific date
func (ct *CostTracker) GetDailyCost(ctx context.Context, date string) (*DailyCost, error) {
	ct.mu.Lock()
	if daily, ok := ct.dailyCosts[date]; ok {
		ct.mu.Unlock()
		return daily, nil
	}
	ct.mu.Unlock()

	// Try Redis
	if ct.redis != nil {
		data, err := ct.redis.Get(ctx, "cost:"+date).Bytes()
		if err == nil {
			var daily DailyCost
			if err := json.Unmarshal(data, &daily); err == nil {
				return &daily, nil
			}
		}
	}

	return nil, fmt.Errorf("no cost data for %s", date)
}

// GetMonthlyCost returns cost for the current month
func (ct *CostTracker) GetMonthlyCost(ctx context.Context) (*DailyCost, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	total := &DailyCost{Date: startOfMonth.Format("2006-01")}

	for d := startOfMonth; !d.After(now); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		if daily, err := ct.GetDailyCost(ctx, dateStr); err == nil {
			total.InputTokens += daily.InputTokens
			total.OutputTokens += daily.OutputTokens
			total.TotalCost += daily.TotalCost
			total.Requests += daily.Requests
			total.CacheHits += daily.CacheHits
		}
	}

	return total, nil
}

// IsOverBudget checks if daily budget is exceeded
func (ct *CostTracker) IsOverBudget(ctx context.Context) bool {
	if ct.config.DailyBudget <= 0 {
		return false
	}

	today := time.Now().Format("2006-01-02")
	daily, err := ct.GetDailyCost(ctx, today)
	if err != nil {
		return false
	}

	return daily.TotalCost >= ct.config.DailyBudget
}
