package llm

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestDefaultTokenCacheConfig(t *testing.T) {
	config := DefaultTokenCacheConfig()

	if !config.RedisEnabled {
		t.Error("RedisEnabled should be true by default")
	}
	if config.RedisTTL != 24*time.Hour {
		t.Errorf("RedisTTL = %v, want 24h", config.RedisTTL)
	}
	if !config.MemoryEnabled {
		t.Error("MemoryEnabled should be true by default")
	}
	if config.MemoryMaxSize != 1000 {
		t.Errorf("MemoryMaxSize = %v, want 1000", config.MemoryMaxSize)
	}
	if config.MemoryTTL != 1*time.Hour {
		t.Errorf("MemoryTTL = %v, want 1h", config.MemoryTTL)
	}
	if config.SemanticEnabled {
		t.Error("SemanticEnabled should be false by default")
	}
	if config.SemanticThreshold != 0.95 {
		t.Errorf("SemanticThreshold = %v, want 0.95", config.SemanticThreshold)
	}
	if !config.TrackCosts {
		t.Error("TrackCosts should be true by default")
	}
	if config.InputTokenCost != 0.003 {
		t.Errorf("InputTokenCost = %v, want 0.003", config.InputTokenCost)
	}
	if config.OutputTokenCost != 0.015 {
		t.Errorf("OutputTokenCost = %v, want 0.015", config.OutputTokenCost)
	}
}

func TestNewTokenCache(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
		MemoryTTL:     1 * time.Hour,
	}

	tc := NewTokenCache(config, nil, logger)
	if tc == nil {
		t.Fatal("NewTokenCache returned nil")
	}
	if tc.memCache == nil {
		t.Error("memCache should be initialized")
	}
	if tc.stats == nil {
		t.Error("stats should be initialized")
	}
}

func TestTokenCache_CacheKey(t *testing.T) {
	logger := zap.NewNop()
	tc := NewTokenCache(TokenCacheConfig{}, nil, logger)

	tests := []struct {
		name    string
		prompt  string
		model   string
		options map[string]interface{}
	}{
		{
			name:    "basic key",
			prompt:  "Hello world",
			model:   "claude-3-sonnet",
			options: nil,
		},
		{
			name:   "with temperature",
			prompt: "Hello world",
			model:  "claude-3-sonnet",
			options: map[string]interface{}{
				"temperature": 0.7,
			},
		},
		{
			name:   "with max_tokens",
			prompt: "Hello world",
			model:  "claude-3-sonnet",
			options: map[string]interface{}{
				"max_tokens": 1000,
			},
		},
		{
			name:   "with system",
			prompt: "Hello world",
			model:  "claude-3-sonnet",
			options: map[string]interface{}{
				"system": "You are helpful",
			},
		},
		{
			name:   "with all options",
			prompt: "Hello world",
			model:  "claude-3-sonnet",
			options: map[string]interface{}{
				"temperature": 0.5,
				"max_tokens":  500,
				"system":      "Be concise",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tc.CacheKey(tt.prompt, tt.model, tt.options)
			if key == "" {
				t.Error("CacheKey returned empty string")
			}
			if len(key) != 64 { // SHA256 hex = 64 chars
				t.Errorf("CacheKey length = %d, want 64", len(key))
			}
		})
	}

	// Same inputs should produce same key
	t.Run("deterministic", func(t *testing.T) {
		key1 := tc.CacheKey("test", "model", nil)
		key2 := tc.CacheKey("test", "model", nil)
		if key1 != key2 {
			t.Error("CacheKey should be deterministic")
		}
	})

	// Different inputs should produce different keys
	t.Run("different inputs", func(t *testing.T) {
		key1 := tc.CacheKey("test1", "model", nil)
		key2 := tc.CacheKey("test2", "model", nil)
		if key1 == key2 {
			t.Error("Different prompts should produce different keys")
		}
	})
}

func TestTokenCache_SetAndGet(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
		MemoryTTL:     1 * time.Hour,
		TrackCosts:    true,
	}
	tc := NewTokenCache(config, nil, logger)
	ctx := context.Background()

	t.Run("set and get", func(t *testing.T) {
		key := "test-key-1"
		response := "test response"
		tokens := TokenUsage{InputTokens: 10, OutputTokens: 20}

		tc.Set(ctx, key, response, tokens)

		got, gotTokens, found := tc.Get(ctx, key)
		if !found {
			t.Error("Get should find cached entry")
		}
		if got != response {
			t.Errorf("Get response = %v, want %v", got, response)
		}
		if gotTokens == nil {
			t.Fatal("Get tokens should not be nil")
		}
		if gotTokens.InputTokens != tokens.InputTokens {
			t.Errorf("InputTokens = %v, want %v", gotTokens.InputTokens, tokens.InputTokens)
		}
		if gotTokens.OutputTokens != tokens.OutputTokens {
			t.Errorf("OutputTokens = %v, want %v", gotTokens.OutputTokens, tokens.OutputTokens)
		}
	})

	t.Run("get missing key", func(t *testing.T) {
		_, _, found := tc.Get(ctx, "nonexistent-key")
		if found {
			t.Error("Get should not find nonexistent key")
		}
	})
}

func TestTokenCache_GetStats(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
		MemoryTTL:     1 * time.Hour,
		TrackCosts:    true,
		InputTokenCost:  0.003,
		OutputTokenCost: 0.015,
	}
	tc := NewTokenCache(config, nil, logger)
	ctx := context.Background()

	// Set and get to generate stats
	tc.Set(ctx, "key1", "response1", TokenUsage{InputTokens: 100, OutputTokens: 50})
	tc.Get(ctx, "key1") // hit
	tc.Get(ctx, "key2") // miss

	stats := tc.GetStats()

	if stats.TotalRequests != 2 {
		t.Errorf("TotalRequests = %v, want 2", stats.TotalRequests)
	}
	if stats.MemoryHits != 1 {
		t.Errorf("MemoryHits = %v, want 1", stats.MemoryHits)
	}
	if stats.MemoryMisses != 1 {
		t.Errorf("MemoryMisses = %v, want 1", stats.MemoryMisses)
	}
	if stats.TokensSaved <= 0 {
		t.Error("TokensSaved should be > 0 after cache hit")
	}
	if stats.CostSaved <= 0 {
		t.Error("CostSaved should be > 0 after cache hit")
	}
}

func TestTokenCache_ResetStats(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
	}
	tc := NewTokenCache(config, nil, logger)
	ctx := context.Background()

	// Generate some stats
	tc.Set(ctx, "key1", "response1", TokenUsage{InputTokens: 100, OutputTokens: 50})
	tc.Get(ctx, "key1")

	// Reset
	tc.ResetStats()

	stats := tc.GetStats()
	if stats.TotalRequests != 0 {
		t.Errorf("TotalRequests = %v, want 0 after reset", stats.TotalRequests)
	}
	if stats.MemoryHits != 0 {
		t.Errorf("MemoryHits = %v, want 0 after reset", stats.MemoryHits)
	}
}

func TestTokenCache_Clear(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
	}
	tc := NewTokenCache(config, nil, logger)
	ctx := context.Background()

	// Add entries
	tc.Set(ctx, "key1", "response1", TokenUsage{})
	tc.Set(ctx, "key2", "response2", TokenUsage{})

	// Clear
	err := tc.Clear(ctx)
	if err != nil {
		t.Errorf("Clear error = %v", err)
	}

	// Verify cleared
	_, _, found := tc.Get(ctx, "key1")
	if found {
		t.Error("Get should not find entry after Clear")
	}
}

func TestTokenCache_Eviction(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 5, // Small size to test eviction
		MemoryTTL:     1 * time.Hour,
	}
	tc := NewTokenCache(config, nil, logger)
	ctx := context.Background()

	// Fill cache beyond capacity
	for i := 0; i < 10; i++ {
		tc.Set(ctx, string(rune('a'+i)), "response", TokenUsage{})
	}

	// Cache should have evicted oldest entries
	tc.memCacheMu.RLock()
	size := len(tc.memCache)
	tc.memCacheMu.RUnlock()

	if size > config.MemoryMaxSize {
		t.Errorf("cache size = %d, should be <= %d", size, config.MemoryMaxSize)
	}
}

func TestTokenCache_Cleanup(t *testing.T) {
	logger := zap.NewNop()
	config := TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
		MemoryTTL:     1 * time.Millisecond, // Very short TTL
	}
	tc := NewTokenCache(config, nil, logger)
	ctx := context.Background()

	// Add entry
	tc.Set(ctx, "key1", "response1", TokenUsage{})

	// Wait for TTL to expire
	time.Sleep(5 * time.Millisecond)

	// Manually trigger cleanup
	tc.cleanup()

	// Entry should be expired
	_, _, found := tc.Get(ctx, "key1")
	if found {
		t.Error("Get should not find expired entry after cleanup")
	}
}

func TestTruncateForStorage(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "truncate",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateForStorage(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateForStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewCostTracker(t *testing.T) {
	logger := zap.NewNop()
	config := CostConfig{
		InputTokenCost:  3.0,
		OutputTokenCost: 15.0,
		DailyBudget:     100.0,
		AlertThreshold:  0.8,
	}

	ct := NewCostTracker(config, nil, logger)
	if ct == nil {
		t.Fatal("NewCostTracker returned nil")
	}
	if ct.config.InputTokenCost != 3.0 {
		t.Errorf("InputTokenCost = %v, want 3.0", ct.config.InputTokenCost)
	}
	if ct.dailyCosts == nil {
		t.Error("dailyCosts should be initialized")
	}
}

func TestCostTracker_RecordUsage(t *testing.T) {
	logger := zap.NewNop()
	config := CostConfig{
		InputTokenCost:  3.0,
		OutputTokenCost: 15.0,
		DailyBudget:     100.0,
		AlertThreshold:  0.8,
	}
	ct := NewCostTracker(config, nil, logger)
	ctx := context.Background()

	t.Run("record non-cached usage", func(t *testing.T) {
		ct.RecordUsage(ctx, 1000, 500, false)

		today := time.Now().Format("2006-01-02")
		ct.mu.Lock()
		daily := ct.dailyCosts[today]
		ct.mu.Unlock()

		if daily == nil {
			t.Fatal("daily cost should be recorded")
		}
		if daily.Requests != 1 {
			t.Errorf("Requests = %v, want 1", daily.Requests)
		}
		if daily.InputTokens != 1000 {
			t.Errorf("InputTokens = %v, want 1000", daily.InputTokens)
		}
		if daily.OutputTokens != 500 {
			t.Errorf("OutputTokens = %v, want 500", daily.OutputTokens)
		}
		if daily.TotalCost <= 0 {
			t.Error("TotalCost should be > 0")
		}
	})

	t.Run("record cached usage", func(t *testing.T) {
		ct.RecordUsage(ctx, 1000, 500, true) // cached = true

		today := time.Now().Format("2006-01-02")
		ct.mu.Lock()
		daily := ct.dailyCosts[today]
		ct.mu.Unlock()

		if daily.CacheHits != 1 {
			t.Errorf("CacheHits = %v, want 1", daily.CacheHits)
		}
		// Tokens should not be added for cached requests
		if daily.InputTokens != 1000 { // Still 1000 from previous test
			t.Errorf("InputTokens = %v, want 1000 (unchanged)", daily.InputTokens)
		}
	})
}

func TestCostTracker_GetDailyCost(t *testing.T) {
	logger := zap.NewNop()
	config := CostConfig{
		InputTokenCost:  3.0,
		OutputTokenCost: 15.0,
	}
	ct := NewCostTracker(config, nil, logger)
	ctx := context.Background()

	t.Run("get existing date", func(t *testing.T) {
		ct.RecordUsage(ctx, 1000, 500, false)
		today := time.Now().Format("2006-01-02")

		cost, err := ct.GetDailyCost(ctx, today)
		if err != nil {
			t.Errorf("GetDailyCost error = %v", err)
		}
		if cost == nil {
			t.Fatal("cost should not be nil")
		}
		if cost.Requests != 1 {
			t.Errorf("Requests = %v, want 1", cost.Requests)
		}
	})

	t.Run("get nonexistent date", func(t *testing.T) {
		_, err := ct.GetDailyCost(ctx, "1900-01-01")
		if err == nil {
			t.Error("GetDailyCost should return error for nonexistent date")
		}
	})
}

func TestCostTracker_GetMonthlyCost(t *testing.T) {
	logger := zap.NewNop()
	config := CostConfig{
		InputTokenCost:  3.0,
		OutputTokenCost: 15.0,
	}
	ct := NewCostTracker(config, nil, logger)
	ctx := context.Background()

	// Record some usage
	ct.RecordUsage(ctx, 1000, 500, false)
	ct.RecordUsage(ctx, 2000, 1000, false)

	cost, err := ct.GetMonthlyCost(ctx)
	if err != nil {
		t.Errorf("GetMonthlyCost error = %v", err)
	}
	if cost == nil {
		t.Fatal("cost should not be nil")
	}
	if cost.InputTokens != 3000 {
		t.Errorf("InputTokens = %v, want 3000", cost.InputTokens)
	}
	if cost.OutputTokens != 1500 {
		t.Errorf("OutputTokens = %v, want 1500", cost.OutputTokens)
	}
}

func TestCostTracker_IsOverBudget(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	t.Run("no budget set", func(t *testing.T) {
		config := CostConfig{
			DailyBudget: 0, // No budget
		}
		ct := NewCostTracker(config, nil, logger)

		if ct.IsOverBudget(ctx) {
			t.Error("IsOverBudget should return false when no budget is set")
		}
	})

	t.Run("under budget", func(t *testing.T) {
		config := CostConfig{
			InputTokenCost:  3.0,
			OutputTokenCost: 15.0,
			DailyBudget:     1000.0, // High budget
		}
		ct := NewCostTracker(config, nil, logger)
		ct.RecordUsage(ctx, 100, 50, false) // Small usage

		if ct.IsOverBudget(ctx) {
			t.Error("IsOverBudget should return false when under budget")
		}
	})

	t.Run("over budget", func(t *testing.T) {
		config := CostConfig{
			InputTokenCost:  3.0,
			OutputTokenCost: 15.0,
			DailyBudget:     0.00001, // Tiny budget
		}
		ct := NewCostTracker(config, nil, logger)
		ct.RecordUsage(ctx, 1000000, 500000, false) // Large usage

		if !ct.IsOverBudget(ctx) {
			t.Error("IsOverBudget should return true when over budget")
		}
	})
}

func TestNewPromptCache(t *testing.T) {
	logger := zap.NewNop()
	tc := NewTokenCache(TokenCacheConfig{MemoryEnabled: true, MemoryMaxSize: 100}, nil, logger)

	pc := NewPromptCache(tc, nil, nil, 0.95, logger)
	if pc == nil {
		t.Fatal("NewPromptCache returned nil")
	}
	if pc.tokenCache != tc {
		t.Error("tokenCache should be set")
	}
	if pc.threshold != 0.95 {
		t.Errorf("threshold = %v, want 0.95", pc.threshold)
	}
}

func TestPromptCache_GetSet(t *testing.T) {
	logger := zap.NewNop()
	tc := NewTokenCache(TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
		MemoryTTL:     1 * time.Hour,
	}, nil, logger)
	pc := NewPromptCache(tc, nil, nil, 0.95, logger)
	ctx := context.Background()

	prompt := "What is the capital of France?"
	model := "claude-3-sonnet"
	response := "The capital of France is Paris."
	tokens := TokenUsage{InputTokens: 10, OutputTokens: 8}

	// Set
	pc.Set(ctx, prompt, model, response, tokens)

	// Get (exact match)
	got, gotTokens, found := pc.Get(ctx, prompt, model)
	if !found {
		t.Error("Get should find exact match")
	}
	if got != response {
		t.Errorf("Get response = %v, want %v", got, response)
	}
	if gotTokens == nil {
		t.Error("Get tokens should not be nil")
	}
}

func TestPromptCache_GetNoMatch(t *testing.T) {
	logger := zap.NewNop()
	tc := NewTokenCache(TokenCacheConfig{
		MemoryEnabled: true,
		MemoryMaxSize: 100,
	}, nil, logger)
	pc := NewPromptCache(tc, nil, nil, 0.95, logger)
	ctx := context.Background()

	// Get without setting
	_, _, found := pc.Get(ctx, "unknown prompt", "model")
	if found {
		t.Error("Get should not find unknown prompt")
	}
}

func TestTokenUsage(t *testing.T) {
	usage := TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
		CachedTokens: 25,
	}

	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %v, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %v, want 50", usage.OutputTokens)
	}
	if usage.CachedTokens != 25 {
		t.Errorf("CachedTokens = %v, want 25", usage.CachedTokens)
	}
}

func TestCacheStats(t *testing.T) {
	stats := CacheStats{
		MemoryHits:    10,
		MemoryMisses:  5,
		RedisHits:     3,
		RedisMisses:   2,
		TotalRequests: 20,
		TokensSaved:   1000,
		CostSaved:     0.5,
		CacheHitRate:  0.65,
	}

	if stats.MemoryHits != 10 {
		t.Errorf("MemoryHits = %v, want 10", stats.MemoryHits)
	}
	if stats.CacheHitRate != 0.65 {
		t.Errorf("CacheHitRate = %v, want 0.65", stats.CacheHitRate)
	}
}

func TestDailyCost(t *testing.T) {
	daily := DailyCost{
		Date:         "2024-01-15",
		InputTokens:  10000,
		OutputTokens: 5000,
		TotalCost:    0.15,
		Requests:     100,
		CacheHits:    30,
	}

	if daily.Date != "2024-01-15" {
		t.Errorf("Date = %v, want 2024-01-15", daily.Date)
	}
	if daily.TotalCost != 0.15 {
		t.Errorf("TotalCost = %v, want 0.15", daily.TotalCost)
	}
}
