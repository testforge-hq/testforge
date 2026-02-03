package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClaudeClient(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				APIKey: "test-api-key",
			},
			wantErr: false,
		},
		{
			name: "missing API key",
			config: Config{
				BaseURL: "https://api.anthropic.com",
			},
			wantErr: true,
		},
		{
			name: "custom config",
			config: Config{
				APIKey:       "test-api-key",
				Model:        "claude-3-opus-20240229",
				MaxTokens:    4096,
				RateLimitRPM: 100,
				CacheSize:    500,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClaudeClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClaudeClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClaudeClient() returned nil client")
			}
		})
	}
}

func TestClaudeClient_Complete_MockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version header")
		}

		// Return mock response
		resp := Response{
			ID:   "test-id",
			Type: "message",
			Role: "assistant",
			Content: []ContentBlock{
				{Type: "text", Text: "Hello! How can I help you today?"},
			},
			Model:      "claude-sonnet-4-20250514",
			StopReason: "end_turn",
			Usage: Usage{
				InputTokens:  10,
				OutputTokens: 8,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client with mock server
	client, err := NewClaudeClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewClaudeClient() error = %v", err)
	}

	// Make request
	ctx := context.Background()
	result, usage, err := client.Complete(ctx, "You are helpful", "Hello")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if result != "Hello! How can I help you today?" {
		t.Errorf("Complete() result = %q, want %q", result, "Hello! How can I help you today?")
	}

	if usage != nil {
		if usage.InputTokens != 10 {
			t.Errorf("InputTokens = %d, want 10", usage.InputTokens)
		}
		if usage.OutputTokens != 8 {
			t.Errorf("OutputTokens = %d, want 8", usage.OutputTokens)
		}
	}
}

func TestClaudeClient_Caching(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		resp := Response{
			ID:      "test-id",
			Content: []ContentBlock{{Type: "text", Text: "cached response"}},
			Usage:   Usage{InputTokens: 5, OutputTokens: 3},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		CacheSize: 100,
		CacheTTL:  time.Hour,
	})

	ctx := context.Background()

	// First request should hit server
	_, _, err := client.CompleteWithOptions(ctx, "system", "user", 0.3, true)
	if err != nil {
		t.Fatalf("first request error = %v", err)
	}
	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}

	// Second identical request should hit cache
	_, _, err = client.CompleteWithOptions(ctx, "system", "user", 0.3, true)
	if err != nil {
		t.Fatalf("second request error = %v", err)
	}
	if requestCount != 1 {
		t.Errorf("expected 1 request (cached), got %d", requestCount)
	}

	// Check metrics
	metrics := client.GetMetrics()
	if metrics.CacheHits != 1 {
		t.Errorf("CacheHits = %d, want 1", metrics.CacheHits)
	}
	if metrics.CacheMisses != 1 {
		t.Errorf("CacheMisses = %d, want 1", metrics.CacheMisses)
	}
}

func TestClaudeClient_CompleteJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Content: []ContentBlock{{
				Type: "text",
				Text: `{"name": "test", "value": 42}`,
			}},
			Usage: Usage{InputTokens: 10, OutputTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	var result struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	ctx := context.Background()
	_, err := client.CompleteJSON(ctx, "Return JSON", "Give me data", &result)
	if err != nil {
		t.Fatalf("CompleteJSON() error = %v", err)
	}

	if result.Name != "test" {
		t.Errorf("Name = %q, want %q", result.Name, "test")
	}
	if result.Value != 42 {
		t.Errorf("Value = %d, want 42", result.Value)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain JSON object",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON in code block",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON with surrounding text",
			input: "Here is the result: {\"key\": \"value\"} That's it.",
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON array",
			input: `[1, 2, 3]`,
			want:  `[1, 2, 3]`,
		},
		{
			name:  "nested JSON",
			input: `{"outer": {"inner": "value"}}`,
			want:  `{"outer": {"inner": "value"}}`,
		},
		{
			name:  "no JSON",
			input: "This is just plain text",
			want:  "",
		},
		{
			name:  "JSON with escaped quotes",
			input: `{"text": "He said \"hello\""}`,
			want:  `{"text": "He said \"hello\""}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLRUCache(t *testing.T) {
	cache := NewLRUCache(3, time.Hour)

	// Test basic set/get
	cache.Set("key1", []byte("value1"))
	if v, ok := cache.Get("key1"); !ok || string(v) != "value1" {
		t.Error("failed to get key1")
	}

	// Test LRU eviction
	cache.Set("key2", []byte("value2"))
	cache.Set("key3", []byte("value3"))
	cache.Set("key4", []byte("value4")) // Should evict key1

	if _, ok := cache.Get("key1"); ok {
		t.Error("key1 should have been evicted")
	}
	if _, ok := cache.Get("key2"); !ok {
		t.Error("key2 should still exist")
	}

	// Test size
	if cache.Size() != 3 {
		t.Errorf("Size() = %d, want 3", cache.Size())
	}
}

func TestLRUCache_TTL(t *testing.T) {
	cache := NewLRUCache(10, 100*time.Millisecond)

	cache.Set("key", []byte("value"))

	// Should get immediately
	if _, ok := cache.Get("key"); !ok {
		t.Error("should get key immediately")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not get after expiration
	if _, ok := cache.Get("key"); ok {
		t.Error("should not get expired key")
	}
}

func TestClaudeClient_CircuitBreaker(t *testing.T) {
	failCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:                 "test-key",
		BaseURL:                server.URL,
		CircuitBreakerEnabled:  true,
		CircuitBreakerTimeout:  100 * time.Millisecond,
		CircuitBreakerInterval: 100 * time.Millisecond,
		CircuitBreakerMinReqs:  3,
		FallbackEnabled:        false,
	})

	ctx := context.Background()

	// Make requests until circuit trips
	for i := 0; i < 5; i++ {
		_, _, err := client.CompleteWithOptions(ctx, "system", "user", 0.3, false)
		if err == nil {
			t.Errorf("request %d should have failed", i+1)
		}
	}

	// Circuit should be open now
	state := client.GetCircuitBreakerState()
	if state != "open" {
		t.Errorf("circuit state = %s, want open", state)
	}

	// Check health
	if client.IsHealthy() {
		t.Error("client should not be healthy when circuit is open")
	}
}

func TestClaudeClient_Metrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Content: []ContentBlock{{Type: "text", Text: "response"}},
			Usage:   Usage{InputTokens: 100, OutputTokens: 50},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	ctx := context.Background()

	// Make some requests
	for i := 0; i < 3; i++ {
		client.CompleteWithOptions(ctx, "system", "user"+string(rune(i)), 0.3, false)
	}

	metrics := client.GetMetrics()

	if metrics.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", metrics.TotalRequests)
	}
	if metrics.SuccessRequests != 3 {
		t.Errorf("SuccessRequests = %d, want 3", metrics.SuccessRequests)
	}
	if metrics.TotalTokensIn != 300 {
		t.Errorf("TotalTokensIn = %d, want 300", metrics.TotalTokensIn)
	}
	if metrics.TotalTokensOut != 150 {
		t.Errorf("TotalTokensOut = %d, want 150", metrics.TotalTokensOut)
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is..."},
		{"exact", 5, "exact"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncateString(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestClaudeClient_GetModel(t *testing.T) {
	client, err := NewClaudeClient(Config{
		APIKey: "test-key",
		Model:  "claude-3-opus-20240229",
	})
	if err != nil {
		t.Fatalf("NewClaudeClient() error = %v", err)
	}

	model := client.GetModel()
	if model != "claude-3-opus-20240229" {
		t.Errorf("GetModel() = %s, want claude-3-opus-20240229", model)
	}
}

func TestClaudeClient_GetModel_Default(t *testing.T) {
	client, err := NewClaudeClient(Config{
		APIKey: "test-key",
		// Model not set, should use default
	})
	if err != nil {
		t.Fatalf("NewClaudeClient() error = %v", err)
	}

	model := client.GetModel()
	// Should have default model
	if model == "" {
		t.Error("GetModel() should return default model, not empty string")
	}
}

func TestClaudeClient_GetCacheSize(t *testing.T) {
	client, err := NewClaudeClient(Config{
		APIKey:    "test-key",
		CacheSize: 500,
		CacheTTL:  time.Hour,
	})
	if err != nil {
		t.Fatalf("NewClaudeClient() error = %v", err)
	}

	// Initially cache should be empty
	size := client.GetCacheSize()
	if size != 0 {
		t.Errorf("GetCacheSize() = %d, want 0 for empty cache", size)
	}
}

func TestClaudeClient_GetCacheSize_WithEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Content: []ContentBlock{{Type: "text", Text: "response"}},
			Usage:   Usage{InputTokens: 10, OutputTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		CacheSize: 100,
		CacheTTL:  time.Hour,
	})

	ctx := context.Background()

	// Make requests that will be cached
	client.CompleteWithOptions(ctx, "system1", "prompt1", 0.3, true)
	client.CompleteWithOptions(ctx, "system2", "prompt2", 0.3, true)

	size := client.GetCacheSize()
	if size != 2 {
		t.Errorf("GetCacheSize() = %d, want 2", size)
	}
}

func TestClaudeClient_IsHealthy_Closed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Content: []ContentBlock{{Type: "text", Text: "response"}},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:                "test-key",
		BaseURL:               server.URL,
		CircuitBreakerEnabled: true,
	})

	// Circuit should be closed (healthy) initially
	if !client.IsHealthy() {
		t.Error("client should be healthy when circuit is closed")
	}

	state := client.GetCircuitBreakerState()
	if state != "closed" {
		t.Errorf("circuit state = %s, want closed", state)
	}
}

func TestClaudeClient_GetCircuitBreakerState_NoBreaker(t *testing.T) {
	client, _ := NewClaudeClient(Config{
		APIKey:                "test-key",
		CircuitBreakerEnabled: false,
	})

	state := client.GetCircuitBreakerState()
	if state != "disabled" {
		t.Errorf("GetCircuitBreakerState() = %s, want disabled", state)
	}
}

func TestClaudeClient_IsHealthy_NoBreaker(t *testing.T) {
	client, _ := NewClaudeClient(Config{
		APIKey:                "test-key",
		CircuitBreakerEnabled: false,
	})

	// Should always be healthy when circuit breaker is disabled
	if !client.IsHealthy() {
		t.Error("client should be healthy when circuit breaker is disabled")
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Model == "" {
		t.Error("Model should have default value")
	}
	if config.MaxTokens == 0 {
		t.Error("MaxTokens should have default value")
	}
	if config.BaseURL == "" {
		t.Error("BaseURL should have default value")
	}
	if config.Timeout == 0 {
		t.Error("Timeout should have default value")
	}
}

func TestLRUCache_MoveToEnd(t *testing.T) {
	cache := NewLRUCache(3, time.Hour)

	// Add entries
	cache.Set("key1", []byte("value1"))
	cache.Set("key2", []byte("value2"))
	cache.Set("key3", []byte("value3"))

	// Access key1 to move it to the end
	cache.Get("key1")

	// Add new entry - should evict key2 (oldest not accessed)
	cache.Set("key4", []byte("value4"))

	// key1 should still exist (was moved to end)
	if _, ok := cache.Get("key1"); !ok {
		t.Error("key1 should still exist after access")
	}

	// key2 should be evicted
	if _, ok := cache.Get("key2"); ok {
		t.Error("key2 should have been evicted")
	}
}

func TestClaudeClient_CompleteJSON_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Content: []ContentBlock{{
				Type: "text",
				Text: "This is not valid JSON",
			}},
			Usage: Usage{InputTokens: 10, OutputTokens: 5},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	var result struct {
		Name string `json:"name"`
	}

	ctx := context.Background()
	_, err := client.CompleteJSON(ctx, "Return JSON", "Give me data", &result)
	if err == nil {
		t.Error("CompleteJSON should return error for invalid JSON")
	}
}

func TestClaudeClient_CompleteWithOptions_CacheDisabled(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		resp := Response{
			Content: []ContentBlock{{Type: "text", Text: "response"}},
			Usage:   Usage{InputTokens: 5, OutputTokens: 3},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:    "test-key",
		BaseURL:   server.URL,
		CacheSize: 100,
		CacheTTL:  time.Hour,
	})

	ctx := context.Background()

	// Both requests should hit server when cache is disabled
	_, _, err := client.CompleteWithOptions(ctx, "system", "user", 0.3, false)
	if err != nil {
		t.Fatalf("first request error = %v", err)
	}

	_, _, err = client.CompleteWithOptions(ctx, "system", "user", 0.3, false)
	if err != nil {
		t.Fatalf("second request error = %v", err)
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests (cache disabled), got %d", requestCount)
	}
}

func TestClaudeClient_Error_Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"type":    "invalid_request_error",
				"message": "Bad request",
			},
		})
	}))
	defer server.Close()

	client, _ := NewClaudeClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	ctx := context.Background()
	_, _, err := client.CompleteWithOptions(ctx, "system", "user", 0.3, false)
	if err == nil {
		t.Error("CompleteWithOptions should return error for bad request")
	}
}

func TestExtractJSON_CodeBlockTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "json code block",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "generic code block",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLRUCache_Set_UpdateExisting(t *testing.T) {
	cache := NewLRUCache(10, time.Hour)

	// Set initial value
	cache.Set("key", []byte("value1"))

	// Update value
	cache.Set("key", []byte("value2"))

	// Should get updated value
	v, ok := cache.Get("key")
	if !ok {
		t.Error("should get updated key")
	}
	if string(v) != "value2" {
		t.Errorf("value = %q, want value2", string(v))
	}

	// Size should still be 1
	if cache.Size() != 1 {
		t.Errorf("Size() = %d, want 1", cache.Size())
	}
}
