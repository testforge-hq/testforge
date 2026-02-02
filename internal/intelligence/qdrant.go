// Package intelligence provides AI-powered learning and suggestions
package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// QdrantConfig holds Qdrant configuration
type QdrantConfig struct {
	Host       string
	Port       int
	APIKey     string
	Collection string
	Dimension  int // Embedding dimension (e.g., 1536 for OpenAI, 1024 for Claude)
}

// QdrantClient provides access to Qdrant vector database
type QdrantClient struct {
	config     QdrantConfig
	httpClient *http.Client
	logger     *zap.Logger
	baseURL    string
}

// NewQdrantClient creates a new Qdrant client
func NewQdrantClient(config QdrantConfig, logger *zap.Logger) *QdrantClient {
	if config.Port == 0 {
		config.Port = 6333
	}
	if config.Collection == "" {
		config.Collection = "testforge_patterns"
	}
	if config.Dimension == 0 {
		config.Dimension = 1536 // Default to OpenAI embedding size
	}

	return &QdrantClient{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     logger,
		baseURL:    fmt.Sprintf("http://%s:%d", config.Host, config.Port),
	}
}

// VectorPoint represents a point in the vector space
type VectorPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// SearchResult represents a search result from Qdrant
type SearchResult struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

// InitCollection creates the collection if it doesn't exist
func (q *QdrantClient) InitCollection(ctx context.Context) error {
	// Check if collection exists
	exists, err := q.collectionExists(ctx)
	if err != nil {
		return fmt.Errorf("checking collection: %w", err)
	}

	if exists {
		q.logger.Debug("collection already exists", zap.String("collection", q.config.Collection))
		return nil
	}

	// Create collection
	payload := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     q.config.Dimension,
			"distance": "Cosine",
		},
		"optimizers_config": map[string]interface{}{
			"indexing_threshold": 10000,
		},
		"replication_factor": 1,
	}

	_, err = q.request(ctx, "PUT", fmt.Sprintf("/collections/%s", q.config.Collection), payload)
	if err != nil {
		return fmt.Errorf("creating collection: %w", err)
	}

	// Create payload indexes for filtering
	indexes := []struct {
		field     string
		fieldType string
	}{
		{"type", "keyword"},
		{"category", "keyword"},
		{"tenant_id", "keyword"},
		{"element_type", "keyword"},
		{"strategy", "keyword"},
	}

	for _, idx := range indexes {
		indexPayload := map[string]interface{}{
			"field_name":   idx.field,
			"field_schema": idx.fieldType,
		}
		_, err = q.request(ctx, "PUT", fmt.Sprintf("/collections/%s/index", q.config.Collection), indexPayload)
		if err != nil {
			q.logger.Warn("failed to create index", zap.String("field", idx.field), zap.Error(err))
		}
	}

	q.logger.Info("created Qdrant collection", zap.String("collection", q.config.Collection))
	return nil
}

// UpsertPattern stores or updates a pattern with its embedding
func (q *QdrantClient) UpsertPattern(ctx context.Context, patternID string, embedding []float32, metadata map[string]interface{}) error {
	payload := map[string]interface{}{
		"points": []map[string]interface{}{
			{
				"id":      patternID,
				"vector":  embedding,
				"payload": metadata,
			},
		},
	}

	_, err := q.request(ctx, "PUT", fmt.Sprintf("/collections/%s/points", q.config.Collection), payload)
	if err != nil {
		return fmt.Errorf("upserting pattern: %w", err)
	}

	return nil
}

// UpsertPatterns stores multiple patterns in batch
func (q *QdrantClient) UpsertPatterns(ctx context.Context, points []VectorPoint) error {
	payload := map[string]interface{}{
		"points": points,
	}

	_, err := q.request(ctx, "PUT", fmt.Sprintf("/collections/%s/points", q.config.Collection), payload)
	if err != nil {
		return fmt.Errorf("batch upserting patterns: %w", err)
	}

	return nil
}

// SearchSimilar finds similar patterns using vector similarity
func (q *QdrantClient) SearchSimilar(ctx context.Context, embedding []float32, filter map[string]interface{}, limit int) ([]SearchResult, error) {
	payload := map[string]interface{}{
		"vector":       embedding,
		"limit":        limit,
		"with_payload": true,
	}

	if filter != nil {
		payload["filter"] = filter
	}

	resp, err := q.request(ctx, "POST", fmt.Sprintf("/collections/%s/points/search", q.config.Collection), payload)
	if err != nil {
		return nil, fmt.Errorf("searching patterns: %w", err)
	}

	var result struct {
		Result []struct {
			ID      interface{}            `json:"id"`
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing search results: %w", err)
	}

	results := make([]SearchResult, 0, len(result.Result))
	for _, r := range result.Result {
		idStr := fmt.Sprintf("%v", r.ID)
		results = append(results, SearchResult{
			ID:      idStr,
			Score:   r.Score,
			Payload: r.Payload,
		})
	}

	return results, nil
}

// SearchByType finds patterns filtered by type
func (q *QdrantClient) SearchByType(ctx context.Context, embedding []float32, patternType string, limit int) ([]SearchResult, error) {
	filter := map[string]interface{}{
		"must": []map[string]interface{}{
			{
				"key":   "type",
				"match": map[string]interface{}{"value": patternType},
			},
		},
	}

	return q.SearchSimilar(ctx, embedding, filter, limit)
}

// SearchHealingPatterns finds similar healing patterns
func (q *QdrantClient) SearchHealingPatterns(ctx context.Context, embedding []float32, elementType string, limit int) ([]SearchResult, error) {
	filter := map[string]interface{}{
		"must": []map[string]interface{}{
			{
				"key":   "type",
				"match": map[string]interface{}{"value": "healing"},
			},
		},
	}

	if elementType != "" {
		filter["must"] = append(filter["must"].([]map[string]interface{}), map[string]interface{}{
			"key":   "element_type",
			"match": map[string]interface{}{"value": elementType},
		})
	}

	return q.SearchSimilar(ctx, embedding, filter, limit)
}

// SearchElementPatterns finds similar element patterns
func (q *QdrantClient) SearchElementPatterns(ctx context.Context, embedding []float32, industry string, limit int) ([]SearchResult, error) {
	filter := map[string]interface{}{
		"must": []map[string]interface{}{
			{
				"key":   "type",
				"match": map[string]interface{}{"value": "element"},
			},
		},
	}

	if industry != "" {
		filter["must"] = append(filter["must"].([]map[string]interface{}), map[string]interface{}{
			"key":   "category",
			"match": map[string]interface{}{"value": industry},
		})
	}

	return q.SearchSimilar(ctx, embedding, filter, limit)
}

// DeletePattern removes a pattern from the collection
func (q *QdrantClient) DeletePattern(ctx context.Context, patternID string) error {
	payload := map[string]interface{}{
		"points": []string{patternID},
	}

	_, err := q.request(ctx, "POST", fmt.Sprintf("/collections/%s/points/delete", q.config.Collection), payload)
	if err != nil {
		return fmt.Errorf("deleting pattern: %w", err)
	}

	return nil
}

// GetPattern retrieves a pattern by ID
func (q *QdrantClient) GetPattern(ctx context.Context, patternID string) (*SearchResult, error) {
	payload := map[string]interface{}{
		"ids":          []string{patternID},
		"with_payload": true,
		"with_vector":  false,
	}

	resp, err := q.request(ctx, "POST", fmt.Sprintf("/collections/%s/points", q.config.Collection), payload)
	if err != nil {
		return nil, fmt.Errorf("getting pattern: %w", err)
	}

	var result struct {
		Result []struct {
			ID      interface{}            `json:"id"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing result: %w", err)
	}

	if len(result.Result) == 0 {
		return nil, fmt.Errorf("pattern not found")
	}

	return &SearchResult{
		ID:      fmt.Sprintf("%v", result.Result[0].ID),
		Payload: result.Result[0].Payload,
	}, nil
}

// GetCollectionInfo returns information about the collection
func (q *QdrantClient) GetCollectionInfo(ctx context.Context) (map[string]interface{}, error) {
	resp, err := q.request(ctx, "GET", fmt.Sprintf("/collections/%s", q.config.Collection), nil)
	if err != nil {
		return nil, fmt.Errorf("getting collection info: %w", err)
	}

	var result struct {
		Result map[string]interface{} `json:"result"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing result: %w", err)
	}

	return result.Result, nil
}

// Health checks the Qdrant server health
func (q *QdrantClient) Health(ctx context.Context) error {
	_, err := q.request(ctx, "GET", "/healthz", nil)
	return err
}

// Private methods

func (q *QdrantClient) collectionExists(ctx context.Context) (bool, error) {
	resp, err := q.request(ctx, "GET", "/collections", nil)
	if err != nil {
		return false, err
	}

	var result struct {
		Result struct {
			Collections []struct {
				Name string `json:"name"`
			} `json:"collections"`
		} `json:"result"`
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return false, err
	}

	for _, c := range result.Result.Collections {
		if c.Name == q.config.Collection {
			return true, nil
		}
	}

	return false, nil
}

func (q *QdrantClient) request(ctx context.Context, method, path string, payload interface{}) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if q.config.APIKey != "" {
		req.Header.Set("api-key", q.config.APIKey)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("qdrant error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// SemanticPatternStore integrates Qdrant with the knowledge base
type SemanticPatternStore struct {
	qdrant   *QdrantClient
	kb       *KnowledgeBase
	embedder Embedder
	logger   *zap.Logger
}

// Embedder generates embeddings for text
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// NewSemanticPatternStore creates a new semantic pattern store
func NewSemanticPatternStore(qdrant *QdrantClient, kb *KnowledgeBase, embedder Embedder, logger *zap.Logger) *SemanticPatternStore {
	return &SemanticPatternStore{
		qdrant:   qdrant,
		kb:       kb,
		embedder: embedder,
		logger:   logger,
	}
}

// IndexPattern indexes a pattern with its semantic embedding
func (sps *SemanticPatternStore) IndexPattern(ctx context.Context, patternID uuid.UUID, patternType KnowledgeType, description string, metadata map[string]interface{}) error {
	// Generate embedding for the pattern description
	embedding, err := sps.embedder.Embed(ctx, description)
	if err != nil {
		return fmt.Errorf("generating embedding: %w", err)
	}

	// Add type to metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["type"] = string(patternType)
	metadata["pattern_id"] = patternID.String()

	// Store in Qdrant
	err = sps.qdrant.UpsertPattern(ctx, patternID.String(), embedding, metadata)
	if err != nil {
		return fmt.Errorf("storing in Qdrant: %w", err)
	}

	return nil
}

// FindSimilarPatterns finds semantically similar patterns
func (sps *SemanticPatternStore) FindSimilarPatterns(ctx context.Context, query string, patternType KnowledgeType, limit int) ([]SearchResult, error) {
	// Generate embedding for the query
	embedding, err := sps.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generating embedding: %w", err)
	}

	// Search Qdrant
	filter := map[string]interface{}{
		"must": []map[string]interface{}{
			{
				"key":   "type",
				"match": map[string]interface{}{"value": string(patternType)},
			},
		},
	}

	results, err := sps.qdrant.SearchSimilar(ctx, embedding, filter, limit)
	if err != nil {
		return nil, fmt.Errorf("searching Qdrant: %w", err)
	}

	return results, nil
}

// FindHealingStrategiesSemantic finds healing strategies using semantic search
func (sps *SemanticPatternStore) FindHealingStrategiesSemantic(ctx context.Context, selectorDescription string, elementType string, limit int) ([]HealingPattern, error) {
	// Generate query embedding
	embedding, err := sps.embedder.Embed(ctx, selectorDescription)
	if err != nil {
		return nil, fmt.Errorf("generating embedding: %w", err)
	}

	// Search for similar healing patterns
	results, err := sps.qdrant.SearchHealingPatterns(ctx, embedding, elementType, limit)
	if err != nil {
		return nil, fmt.Errorf("searching patterns: %w", err)
	}

	// Convert results to HealingPattern
	patterns := make([]HealingPattern, 0, len(results))
	for _, r := range results {
		pattern := HealingPattern{
			Strategy:    getString(r.Payload, "strategy"),
			ElementType: getString(r.Payload, "element_type"),
			SuccessRate: float64(r.Score),
		}

		if original, ok := r.Payload["original_selector"].(string); ok {
			pattern.OriginalSelector = original
		}
		if healed, ok := r.Payload["healed_selector"].(string); ok {
			pattern.HealedSelector = healed
		}

		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
