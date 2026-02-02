package intelligence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// EmbeddingConfig holds embedding service configuration
type EmbeddingConfig struct {
	Provider       string // "openai", "claude", "local"
	APIKey         string
	Model          string
	Dimension      int
	BaseURL        string
	CacheTTL       time.Duration
	MaxBatchSize   int
	RateLimitRPM   int
}

// DefaultEmbeddingConfig returns default embedding configuration
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		Provider:     "openai",
		Model:        "text-embedding-3-small",
		Dimension:    1536,
		BaseURL:      "https://api.openai.com/v1",
		CacheTTL:     24 * time.Hour,
		MaxBatchSize: 100,
		RateLimitRPM: 3000,
	}
}

// EmbeddingService generates and caches embeddings
type EmbeddingService struct {
	config     EmbeddingConfig
	httpClient *http.Client
	redis      *redis.Client
	logger     *zap.Logger

	// In-memory LRU cache
	cache    map[string]cachedEmbedding
	cacheMu  sync.RWMutex
	cacheMax int

	// Rate limiting
	rateLimiter *rateLimiter
}

type cachedEmbedding struct {
	embedding []float32
	createdAt time.Time
}

type rateLimiter struct {
	tokens    int
	maxTokens int
	refillAt  time.Time
	mu        sync.Mutex
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService(config EmbeddingConfig, redisClient *redis.Client, logger *zap.Logger) *EmbeddingService {
	return &EmbeddingService{
		config:     config,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		redis:      redisClient,
		logger:     logger,
		cache:      make(map[string]cachedEmbedding),
		cacheMax:   10000,
		rateLimiter: &rateLimiter{
			tokens:    config.RateLimitRPM,
			maxTokens: config.RateLimitRPM,
			refillAt:  time.Now().Add(time.Minute),
		},
	}
}

// Embed generates an embedding for text
func (es *EmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	// Generate cache key
	cacheKey := es.cacheKey(text)

	// Check in-memory cache
	es.cacheMu.RLock()
	if cached, ok := es.cache[cacheKey]; ok {
		if time.Since(cached.createdAt) < es.config.CacheTTL {
			es.cacheMu.RUnlock()
			return cached.embedding, nil
		}
	}
	es.cacheMu.RUnlock()

	// Check Redis cache
	if es.redis != nil {
		cached, err := es.redis.Get(ctx, "emb:"+cacheKey).Bytes()
		if err == nil {
			var embedding []float32
			if err := json.Unmarshal(cached, &embedding); err == nil {
				es.setMemoryCache(cacheKey, embedding)
				return embedding, nil
			}
		}
	}

	// Rate limit check
	if err := es.waitForRateLimit(ctx); err != nil {
		return nil, err
	}

	// Generate embedding
	embedding, err := es.generateEmbedding(ctx, text)
	if err != nil {
		return nil, err
	}

	// Cache the result
	es.setMemoryCache(cacheKey, embedding)

	if es.redis != nil {
		data, _ := json.Marshal(embedding)
		es.redis.Set(ctx, "emb:"+cacheKey, data, es.config.CacheTTL)
	}

	return embedding, nil
}

// EmbedBatch generates embeddings for multiple texts
func (es *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	uncachedIndices := make([]int, 0)
	uncachedTexts := make([]string, 0)

	// Check cache for each text
	for i, text := range texts {
		cacheKey := es.cacheKey(text)

		// Check in-memory cache
		es.cacheMu.RLock()
		if cached, ok := es.cache[cacheKey]; ok {
			if time.Since(cached.createdAt) < es.config.CacheTTL {
				results[i] = cached.embedding
				es.cacheMu.RUnlock()
				continue
			}
		}
		es.cacheMu.RUnlock()

		// Check Redis cache
		if es.redis != nil {
			cached, err := es.redis.Get(ctx, "emb:"+cacheKey).Bytes()
			if err == nil {
				var embedding []float32
				if err := json.Unmarshal(cached, &embedding); err == nil {
					results[i] = embedding
					es.setMemoryCache(cacheKey, embedding)
					continue
				}
			}
		}

		// Need to generate this embedding
		uncachedIndices = append(uncachedIndices, i)
		uncachedTexts = append(uncachedTexts, text)
	}

	// Generate embeddings for uncached texts in batches
	for i := 0; i < len(uncachedTexts); i += es.config.MaxBatchSize {
		end := i + es.config.MaxBatchSize
		if end > len(uncachedTexts) {
			end = len(uncachedTexts)
		}

		batchTexts := uncachedTexts[i:end]
		batchIndices := uncachedIndices[i:end]

		// Rate limit check
		if err := es.waitForRateLimit(ctx); err != nil {
			return nil, err
		}

		embeddings, err := es.generateEmbeddingBatch(ctx, batchTexts)
		if err != nil {
			return nil, err
		}

		// Store results and cache
		for j, embedding := range embeddings {
			originalIdx := batchIndices[j]
			results[originalIdx] = embedding

			cacheKey := es.cacheKey(texts[originalIdx])
			es.setMemoryCache(cacheKey, embedding)

			if es.redis != nil {
				data, _ := json.Marshal(embedding)
				es.redis.Set(ctx, "emb:"+cacheKey, data, es.config.CacheTTL)
			}
		}
	}

	return results, nil
}

// GetCacheStats returns cache statistics
func (es *EmbeddingService) GetCacheStats() map[string]interface{} {
	es.cacheMu.RLock()
	defer es.cacheMu.RUnlock()

	return map[string]interface{}{
		"memory_cache_size": len(es.cache),
		"memory_cache_max":  es.cacheMax,
		"cache_ttl_hours":   es.config.CacheTTL.Hours(),
	}
}

// ClearCache clears the in-memory cache
func (es *EmbeddingService) ClearCache() {
	es.cacheMu.Lock()
	defer es.cacheMu.Unlock()
	es.cache = make(map[string]cachedEmbedding)
}

// Private methods

func (es *EmbeddingService) cacheKey(text string) string {
	hash := sha256.Sum256([]byte(text + es.config.Model))
	return hex.EncodeToString(hash[:16])
}

func (es *EmbeddingService) setMemoryCache(key string, embedding []float32) {
	es.cacheMu.Lock()
	defer es.cacheMu.Unlock()

	// Evict oldest entries if cache is full
	if len(es.cache) >= es.cacheMax {
		// Simple eviction: remove first 10%
		count := 0
		for k := range es.cache {
			delete(es.cache, k)
			count++
			if count >= es.cacheMax/10 {
				break
			}
		}
	}

	es.cache[key] = cachedEmbedding{
		embedding: embedding,
		createdAt: time.Now(),
	}
}

func (es *EmbeddingService) waitForRateLimit(ctx context.Context) error {
	es.rateLimiter.mu.Lock()
	defer es.rateLimiter.mu.Unlock()

	// Refill tokens if needed
	if time.Now().After(es.rateLimiter.refillAt) {
		es.rateLimiter.tokens = es.rateLimiter.maxTokens
		es.rateLimiter.refillAt = time.Now().Add(time.Minute)
	}

	if es.rateLimiter.tokens <= 0 {
		// Wait until refill
		waitTime := time.Until(es.rateLimiter.refillAt)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			es.rateLimiter.tokens = es.rateLimiter.maxTokens
			es.rateLimiter.refillAt = time.Now().Add(time.Minute)
		}
	}

	es.rateLimiter.tokens--
	return nil
}

func (es *EmbeddingService) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := es.generateEmbeddingBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (es *EmbeddingService) generateEmbeddingBatch(ctx context.Context, texts []string) ([][]float32, error) {
	switch es.config.Provider {
	case "openai":
		return es.generateOpenAIEmbeddings(ctx, texts)
	case "local":
		return es.generateLocalEmbeddings(ctx, texts)
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", es.config.Provider)
	}
}

func (es *EmbeddingService) generateOpenAIEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	payload := map[string]interface{}{
		"model": es.config.Model,
		"input": texts,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", es.config.BaseURL+"/embeddings", bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+es.config.APIKey)

	resp, err := es.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// Sort by index to maintain order
	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}

	es.logger.Debug("generated embeddings",
		zap.Int("count", len(texts)),
		zap.Int("tokens", result.Usage.TotalTokens),
	)

	return embeddings, nil
}

func (es *EmbeddingService) generateLocalEmbeddings(ctx context.Context, texts []string) ([][]float32, error) {
	// Placeholder for local embedding generation (e.g., sentence-transformers)
	// In production, this would call a local embedding service
	return nil, fmt.Errorf("local embeddings not implemented")
}

// CosineSimilarity calculates cosine similarity between two embeddings
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float32) float32 {
	// Newton's method for square root
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}
