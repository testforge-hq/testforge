package llm

import (
	"testing"
)

func TestDefaultCachedClientConfig(t *testing.T) {
	config := DefaultCachedClientConfig()

	// Check Claude config
	if config.ClaudeConfig.APIKey != "" {
		t.Error("default config should not have API key set")
	}

	// Check cost config
	if config.CostConfig.InputTokenCost != 3.0 {
		t.Errorf("InputTokenCost = %v, want %v", config.CostConfig.InputTokenCost, 3.0)
	}
	if config.CostConfig.OutputTokenCost != 15.0 {
		t.Errorf("OutputTokenCost = %v, want %v", config.CostConfig.OutputTokenCost, 15.0)
	}
	if config.CostConfig.DailyBudget != 100.0 {
		t.Errorf("DailyBudget = %v, want %v", config.CostConfig.DailyBudget, 100.0)
	}
	if config.CostConfig.AlertThreshold != 0.8 {
		t.Errorf("AlertThreshold = %v, want %v", config.CostConfig.AlertThreshold, 0.8)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantMin  int
		wantMax  int
	}{
		{
			name:    "empty string",
			text:    "",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "short text",
			text:    "Hello",
			wantMin: 1,
			wantMax: 2,
		},
		{
			name:    "medium text",
			text:    "Hello, how are you doing today?",
			wantMin: 5,
			wantMax: 10,
		},
		{
			name:    "longer text",
			text:    "The quick brown fox jumps over the lazy dog. This is a common pangram used in typography.",
			wantMin: 20,
			wantMax: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("estimateTokens() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestNewCostEstimator(t *testing.T) {
	ce := NewCostEstimator(3.0, 15.0)

	if ce == nil {
		t.Fatal("NewCostEstimator returned nil")
	}
	if ce.inputCostPerMillion != 3.0 {
		t.Errorf("inputCostPerMillion = %v, want %v", ce.inputCostPerMillion, 3.0)
	}
	if ce.outputCostPerMillion != 15.0 {
		t.Errorf("outputCostPerMillion = %v, want %v", ce.outputCostPerMillion, 15.0)
	}
}

func TestCostEstimator_EstimateCost(t *testing.T) {
	ce := NewCostEstimator(3.0, 15.0)

	tests := []struct {
		name                  string
		prompt                string
		estimatedOutputTokens int
		wantCostGT            float64
		wantCostLT            float64
	}{
		{
			name:                  "short prompt small output",
			prompt:                "Hello",
			estimatedOutputTokens: 10,
			wantCostGT:            0.0,
			wantCostLT:            0.001,
		},
		{
			name:                  "longer prompt larger output",
			prompt:                "This is a longer prompt that should have more tokens estimated for input.",
			estimatedOutputTokens: 1000,
			wantCostGT:            0.01,
			wantCostLT:            0.02,
		},
		{
			name:                  "empty prompt",
			prompt:                "",
			estimatedOutputTokens: 100,
			wantCostGT:            0.0,
			wantCostLT:            0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ce.EstimateCost(tt.prompt, tt.estimatedOutputTokens)
			if got < tt.wantCostGT || got > tt.wantCostLT {
				t.Errorf("EstimateCost() = %v, want between %v and %v", got, tt.wantCostGT, tt.wantCostLT)
			}
		})
	}
}

func TestCostEstimator_EstimatePromptCost(t *testing.T) {
	ce := NewCostEstimator(3.0, 15.0)

	tests := []struct {
		name       string
		prompt     string
		wantCostGT float64
		wantCostLT float64
	}{
		{
			name:       "short prompt",
			prompt:     "Hello",
			wantCostGT: 0.0,
			wantCostLT: 0.0001,
		},
		{
			name:       "medium prompt",
			prompt:     "This is a medium length prompt for testing purposes.",
			wantCostGT: 0.0,
			wantCostLT: 0.0001,
		},
		{
			name:       "empty prompt",
			prompt:     "",
			wantCostGT: 0.0,
			wantCostLT: 0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ce.EstimatePromptCost(tt.prompt)
			if got < tt.wantCostGT || got > tt.wantCostLT {
				t.Errorf("EstimatePromptCost() = %v, want between %v and %v", got, tt.wantCostGT, tt.wantCostLT)
			}
		})
	}
}

func TestGetModelPricing(t *testing.T) {
	tests := []struct {
		model           string
		wantInputCost   float64
		wantOutputCost  float64
	}{
		{
			model:          "claude-3-opus-20240229",
			wantInputCost:  15.0,
			wantOutputCost: 75.0,
		},
		{
			model:          "claude-3-sonnet-20240229",
			wantInputCost:  3.0,
			wantOutputCost: 15.0,
		},
		{
			model:          "claude-3-haiku-20240307",
			wantInputCost:  0.25,
			wantOutputCost: 1.25,
		},
		{
			model:          "claude-sonnet-4-20250514",
			wantInputCost:  3.0,
			wantOutputCost: 15.0,
		},
		{
			model:          "claude-3-5-sonnet-20241022",
			wantInputCost:  3.0,
			wantOutputCost: 15.0,
		},
		{
			model:          "unknown-model",
			wantInputCost:  3.0,  // defaults to Sonnet
			wantOutputCost: 15.0, // defaults to Sonnet
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			inputCost, outputCost := GetModelPricing(tt.model)
			if inputCost != tt.wantInputCost {
				t.Errorf("input cost = %v, want %v", inputCost, tt.wantInputCost)
			}
			if outputCost != tt.wantOutputCost {
				t.Errorf("output cost = %v, want %v", outputCost, tt.wantOutputCost)
			}
		})
	}
}

func TestCompletionOptions(t *testing.T) {
	opts := CompletionOptions{
		System:      "You are a helpful assistant",
		Temperature: 0.7,
		MaxTokens:   1000,
		UseCache:    true,
	}

	if opts.System != "You are a helpful assistant" {
		t.Errorf("System = %v, want 'You are a helpful assistant'", opts.System)
	}
	if opts.Temperature != 0.7 {
		t.Errorf("Temperature = %v, want 0.7", opts.Temperature)
	}
	if opts.MaxTokens != 1000 {
		t.Errorf("MaxTokens = %v, want 1000", opts.MaxTokens)
	}
	if !opts.UseCache {
		t.Error("UseCache should be true")
	}
}

func TestCachedClientConfig(t *testing.T) {
	config := CachedClientConfig{
		ClaudeConfig: Config{
			APIKey:  "test-key",
			Model:   "claude-3-sonnet-20240229",
		},
		CacheConfig: TokenCacheConfig{
			MemoryEnabled: true,
			MemoryMaxSize: 1000,
		},
		CostConfig: CostConfig{
			InputTokenCost:  3.0,
			OutputTokenCost: 15.0,
			DailyBudget:     50.0,
		},
	}

	if config.ClaudeConfig.APIKey != "test-key" {
		t.Errorf("APIKey = %v, want 'test-key'", config.ClaudeConfig.APIKey)
	}
	if config.ClaudeConfig.Model != "claude-3-sonnet-20240229" {
		t.Errorf("Model = %v, want 'claude-3-sonnet-20240229'", config.ClaudeConfig.Model)
	}
	if config.CacheConfig.MemoryMaxSize != 1000 {
		t.Errorf("MemoryMaxSize = %v, want 1000", config.CacheConfig.MemoryMaxSize)
	}
	if config.CostConfig.DailyBudget != 50.0 {
		t.Errorf("DailyBudget = %v, want 50.0", config.CostConfig.DailyBudget)
	}
}
