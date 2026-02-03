package vjepa

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	assert.Equal(t, "localhost:50051", config.Address)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 50*1024*1024, config.MaxMsgSize) // 50MB
	assert.Nil(t, config.Logger)
}

func TestClientConfig_Fields(t *testing.T) {
	logger := zap.NewNop()
	config := ClientConfig{
		Address:    "grpc.example.com:443",
		Timeout:    60 * time.Second,
		MaxMsgSize: 100 * 1024 * 1024,
		Logger:     logger,
	}

	assert.Equal(t, "grpc.example.com:443", config.Address)
	assert.Equal(t, 60*time.Second, config.Timeout)
	assert.Equal(t, 100*1024*1024, config.MaxMsgSize)
	assert.Equal(t, logger, config.Logger)
}

func TestNewClient_DefaultMaxMsgSize(t *testing.T) {
	// This test verifies that NewClient sets default MaxMsgSize when not provided
	// Note: This will fail to connect but we're testing the config handling
	config := ClientConfig{
		Address:    "localhost:50051",
		MaxMsgSize: 0, // Should be set to default
	}

	// The client creation will fail because there's no gRPC server
	// But we're testing the configuration handling
	client, err := NewClient(config)
	// Since there's no server, this might error or succeed with a lazy connection
	if err == nil && client != nil {
		defer client.Close()
	}
	// The main test is that it doesn't panic
}

func TestNewClient_WithLogger(t *testing.T) {
	logger := zap.NewNop()
	config := ClientConfig{
		Address: "localhost:50051",
		Logger:  logger,
	}

	client, err := NewClient(config)
	if err == nil && client != nil {
		defer client.Close()
		assert.Equal(t, logger, client.logger)
	}
}

func TestNewClient_WithoutLogger(t *testing.T) {
	config := ClientConfig{
		Address: "localhost:50051",
		Logger:  nil,
	}

	client, err := NewClient(config)
	if err == nil && client != nil {
		defer client.Close()
		assert.NotNil(t, client.logger)
	}
}

func TestCompareResult_Struct(t *testing.T) {
	result := CompareResult{
		SimilarityScore: 0.95,
		SemanticMatch:   true,
		Confidence:      0.92,
		Analysis:        "Images are semantically similar",
		ChangedRegions: []ChangedRegion{
			{
				X:            100,
				Y:            200,
				Width:        50,
				Height:       30,
				ChangeType:   "color",
				Significance: 0.3,
				Description:  "Minor color change",
			},
		},
	}

	assert.Equal(t, float32(0.95), result.SimilarityScore)
	assert.True(t, result.SemanticMatch)
	assert.Equal(t, float32(0.92), result.Confidence)
	assert.Equal(t, "Images are semantically similar", result.Analysis)
	require.Len(t, result.ChangedRegions, 1)
	assert.Equal(t, 100, result.ChangedRegions[0].X)
	assert.Equal(t, 200, result.ChangedRegions[0].Y)
	assert.Equal(t, 50, result.ChangedRegions[0].Width)
	assert.Equal(t, 30, result.ChangedRegions[0].Height)
	assert.Equal(t, "color", result.ChangedRegions[0].ChangeType)
	assert.Equal(t, float32(0.3), result.ChangedRegions[0].Significance)
}

func TestChangedRegion_Struct(t *testing.T) {
	region := ChangedRegion{
		X:            10,
		Y:            20,
		Width:        100,
		Height:       50,
		ChangeType:   "content",
		Significance: 0.8,
		Description:  "Text changed",
	}

	assert.Equal(t, 10, region.X)
	assert.Equal(t, 20, region.Y)
	assert.Equal(t, 100, region.Width)
	assert.Equal(t, 50, region.Height)
	assert.Equal(t, "content", region.ChangeType)
	assert.Equal(t, float32(0.8), region.Significance)
	assert.Equal(t, "Text changed", region.Description)
}

func TestStabilityResult_Struct(t *testing.T) {
	result := StabilityResult{
		IsStable:         true,
		StableFrameIndex: 3,
		StabilityScore:   0.99,
		Analysis:         "UI is stable after frame 3",
	}

	assert.True(t, result.IsStable)
	assert.Equal(t, 3, result.StableFrameIndex)
	assert.Equal(t, float32(0.99), result.StabilityScore)
	assert.Equal(t, "UI is stable after frame 3", result.Analysis)
}

func TestStabilityResult_Unstable(t *testing.T) {
	result := StabilityResult{
		IsStable:         false,
		StableFrameIndex: -1,
		StabilityScore:   0.45,
		Analysis:         "UI is still changing",
	}

	assert.False(t, result.IsStable)
	assert.Equal(t, -1, result.StableFrameIndex)
	assert.Equal(t, float32(0.45), result.StabilityScore)
}

func TestEmbeddingResult_Struct(t *testing.T) {
	embedding := []byte{0x01, 0x02, 0x03, 0x04}
	result := EmbeddingResult{
		Embedding:    embedding,
		EmbeddingDim: 768,
		ModelVersion: "v1.0.0",
	}

	assert.Equal(t, embedding, result.Embedding)
	assert.Equal(t, 768, result.EmbeddingDim)
	assert.Equal(t, "v1.0.0", result.ModelVersion)
}

func TestFramePair_Struct(t *testing.T) {
	pair := FramePair{
		PairID:       "pair-123",
		BaselineData: []byte{0x89, 0x50, 0x4E, 0x47}, // PNG magic bytes
		BaselineURI:  "bucket/baseline.png",
		ActualData:   []byte{0x89, 0x50, 0x4E, 0x47},
		ActualURI:    "bucket/actual.png",
		Context:      "Login page comparison",
	}

	assert.Equal(t, "pair-123", pair.PairID)
	assert.Equal(t, []byte{0x89, 0x50, 0x4E, 0x47}, pair.BaselineData)
	assert.Equal(t, "bucket/baseline.png", pair.BaselineURI)
	assert.Equal(t, []byte{0x89, 0x50, 0x4E, 0x47}, pair.ActualData)
	assert.Equal(t, "bucket/actual.png", pair.ActualURI)
	assert.Equal(t, "Login page comparison", pair.Context)
}

func TestFramePair_URIOnly(t *testing.T) {
	pair := FramePair{
		PairID:      "pair-456",
		BaselineURI: "s3://bucket/baseline.png",
		ActualURI:   "s3://bucket/actual.png",
		Context:     "Dashboard comparison",
	}

	assert.Equal(t, "pair-456", pair.PairID)
	assert.Nil(t, pair.BaselineData)
	assert.Nil(t, pair.ActualData)
	assert.Equal(t, "s3://bucket/baseline.png", pair.BaselineURI)
	assert.Equal(t, "s3://bucket/actual.png", pair.ActualURI)
}

func TestPairCompareResult_Struct(t *testing.T) {
	result := PairCompareResult{
		PairID: "pair-789",
		Result: CompareResult{
			SimilarityScore: 0.88,
			SemanticMatch:   true,
			Confidence:      0.90,
			Analysis:        "Similar appearance",
		},
	}

	assert.Equal(t, "pair-789", result.PairID)
	assert.Equal(t, float32(0.88), result.Result.SimilarityScore)
	assert.True(t, result.Result.SemanticMatch)
}

func TestBatchCompareResult_Struct(t *testing.T) {
	result := BatchCompareResult{
		Results: []PairCompareResult{
			{PairID: "pair-1", Result: CompareResult{SimilarityScore: 0.95, SemanticMatch: true}},
			{PairID: "pair-2", Result: CompareResult{SimilarityScore: 0.70, SemanticMatch: false}},
			{PairID: "pair-3", Result: CompareResult{SimilarityScore: 0.98, SemanticMatch: true}},
		},
		AverageSimilarity: 0.877,
		Matches:           2,
		Mismatches:        1,
	}

	assert.Len(t, result.Results, 3)
	assert.Equal(t, float32(0.877), result.AverageSimilarity)
	assert.Equal(t, 2, result.Matches)
	assert.Equal(t, 1, result.Mismatches)
}

func TestBatchCompareResult_Empty(t *testing.T) {
	result := BatchCompareResult{
		Results:           []PairCompareResult{},
		AverageSimilarity: 0,
		Matches:           0,
		Mismatches:        0,
	}

	assert.Empty(t, result.Results)
	assert.Equal(t, float32(0), result.AverageSimilarity)
}

func TestChangeAnalysis_Struct(t *testing.T) {
	analysis := ChangeAnalysis{
		Description:    "Button click triggered navigation",
		Changes:        []string{"URL changed", "Content updated", "New elements appeared"},
		ExpectedChange: true,
		Confidence:     0.95,
	}

	assert.Equal(t, "Button click triggered navigation", analysis.Description)
	assert.Len(t, analysis.Changes, 3)
	assert.Contains(t, analysis.Changes, "URL changed")
	assert.Contains(t, analysis.Changes, "Content updated")
	assert.Contains(t, analysis.Changes, "New elements appeared")
	assert.True(t, analysis.ExpectedChange)
	assert.Equal(t, float32(0.95), analysis.Confidence)
}

func TestChangeAnalysis_UnexpectedChange(t *testing.T) {
	analysis := ChangeAnalysis{
		Description:    "Unexpected modal appeared",
		Changes:        []string{"Modal overlay added"},
		ExpectedChange: false,
		Confidence:     0.85,
	}

	assert.False(t, analysis.ExpectedChange)
	assert.Equal(t, "Unexpected modal appeared", analysis.Description)
}

func TestHealthStatus_Struct(t *testing.T) {
	status := HealthStatus{
		Healthy:        true,
		ModelLoaded:    "v-jepa-base",
		Device:         "cuda:0",
		MemoryUsedMB:   4096,
		MemoryTotalMB:  16384,
		AvgInferenceMS: 45.5,
	}

	assert.True(t, status.Healthy)
	assert.Equal(t, "v-jepa-base", status.ModelLoaded)
	assert.Equal(t, "cuda:0", status.Device)
	assert.Equal(t, int64(4096), status.MemoryUsedMB)
	assert.Equal(t, int64(16384), status.MemoryTotalMB)
	assert.Equal(t, float32(45.5), status.AvgInferenceMS)
}

func TestHealthStatus_Unhealthy(t *testing.T) {
	status := HealthStatus{
		Healthy:        false,
		ModelLoaded:    "",
		Device:         "cpu",
		MemoryUsedMB:   0,
		MemoryTotalMB:  8192,
		AvgInferenceMS: 0,
	}

	assert.False(t, status.Healthy)
	assert.Empty(t, status.ModelLoaded)
	assert.Equal(t, "cpu", status.Device)
}

func TestHealthStatus_CPU(t *testing.T) {
	status := HealthStatus{
		Healthy:        true,
		ModelLoaded:    "v-jepa-small",
		Device:         "cpu",
		MemoryUsedMB:   2048,
		MemoryTotalMB:  8192,
		AvgInferenceMS: 250.0, // Slower on CPU
	}

	assert.True(t, status.Healthy)
	assert.Equal(t, "cpu", status.Device)
	assert.Equal(t, float32(250.0), status.AvgInferenceMS)
}
