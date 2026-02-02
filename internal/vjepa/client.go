package vjepa

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/testforge/testforge/internal/vjepa/proto"
)

// Client wraps the V-JEPA gRPC client
type Client struct {
	conn   *grpc.ClientConn
	client pb.VJEPAServiceClient
	logger *zap.Logger
}

// ClientConfig contains configuration for the V-JEPA client
type ClientConfig struct {
	Address     string
	Timeout     time.Duration
	MaxMsgSize  int // Max message size in bytes
	Logger      *zap.Logger
}

// DefaultClientConfig returns sensible defaults
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Address:    "localhost:50051",
		Timeout:    30 * time.Second,
		MaxMsgSize: 50 * 1024 * 1024, // 50MB
	}
}

// NewClient creates a new V-JEPA client
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Logger == nil {
		cfg.Logger, _ = zap.NewDevelopment()
	}

	if cfg.MaxMsgSize == 0 {
		cfg.MaxMsgSize = 50 * 1024 * 1024
	}

	conn, err := grpc.Dial(cfg.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(cfg.MaxMsgSize),
			grpc.MaxCallSendMsgSize(cfg.MaxMsgSize),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to V-JEPA service at %s: %w", cfg.Address, err)
	}

	cfg.Logger.Info("Connected to V-JEPA service", zap.String("address", cfg.Address))

	return &Client{
		conn:   conn,
		client: pb.NewVJEPAServiceClient(conn),
		logger: cfg.Logger,
	}, nil
}

// Close closes the connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// CompareFrames compares two screenshots and returns semantic similarity
func (c *Client) CompareFrames(ctx context.Context, baseline, actual []byte, context string) (*CompareResult, error) {
	resp, err := c.client.CompareFrames(ctx, &pb.CompareFramesRequest{
		BaselineData: baseline,
		ActualData:   actual,
		Context:      context,
		Settings: &pb.CompareSettings{
			SimilarityThreshold: 0.85,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("CompareFrames failed: %w", err)
	}

	result := &CompareResult{
		SimilarityScore: resp.SimilarityScore,
		SemanticMatch:   resp.SemanticMatch,
		Confidence:      resp.Confidence,
		Analysis:        resp.Analysis,
		ChangedRegions:  make([]ChangedRegion, len(resp.ChangedRegions)),
	}

	for i, r := range resp.ChangedRegions {
		result.ChangedRegions[i] = ChangedRegion{
			X:            int(r.Region.X),
			Y:            int(r.Region.Y),
			Width:        int(r.Region.Width),
			Height:       int(r.Region.Height),
			ChangeType:   r.ChangeType,
			Significance: r.Significance,
			Description:  r.Description,
		}
	}

	c.logger.Debug("CompareFrames completed",
		zap.Float32("similarity", resp.SimilarityScore),
		zap.Bool("match", resp.SemanticMatch),
	)

	return result, nil
}

// CompareFramesByURI compares frames by their MinIO/S3 URIs
func (c *Client) CompareFramesByURI(ctx context.Context, baselineURI, actualURI, context string) (*CompareResult, error) {
	resp, err := c.client.CompareFrames(ctx, &pb.CompareFramesRequest{
		BaselineUri: baselineURI,
		ActualUri:   actualURI,
		Context:     context,
		Settings: &pb.CompareSettings{
			SimilarityThreshold: 0.85,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("CompareFramesByURI failed: %w", err)
	}

	return &CompareResult{
		SimilarityScore: resp.SimilarityScore,
		SemanticMatch:   resp.SemanticMatch,
		Confidence:      resp.Confidence,
		Analysis:        resp.Analysis,
	}, nil
}

// CompareFramesWithThreshold compares frames with custom similarity threshold
func (c *Client) CompareFramesWithThreshold(ctx context.Context, baseline, actual []byte, threshold float32, context string) (*CompareResult, error) {
	resp, err := c.client.CompareFrames(ctx, &pb.CompareFramesRequest{
		BaselineData: baseline,
		ActualData:   actual,
		Context:      context,
		Settings: &pb.CompareSettings{
			SimilarityThreshold: threshold,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("CompareFramesWithThreshold failed: %w", err)
	}

	result := &CompareResult{
		SimilarityScore: resp.SimilarityScore,
		SemanticMatch:   resp.SemanticMatch,
		Confidence:      resp.Confidence,
		Analysis:        resp.Analysis,
		ChangedRegions:  make([]ChangedRegion, len(resp.ChangedRegions)),
	}

	for i, r := range resp.ChangedRegions {
		result.ChangedRegions[i] = ChangedRegion{
			X:            int(r.Region.X),
			Y:            int(r.Region.Y),
			Width:        int(r.Region.Width),
			Height:       int(r.Region.Height),
			ChangeType:   r.ChangeType,
			Significance: r.Significance,
			Description:  r.Description,
		}
	}

	return result, nil
}

// DetectStability checks if UI is stable across multiple frames
func (c *Client) DetectStability(ctx context.Context, frames [][]byte) (*StabilityResult, error) {
	resp, err := c.client.DetectStability(ctx, &pb.DetectStabilityRequest{
		Frames:             frames,
		StabilityThreshold: 0.98,
		MinStableFrames:    3,
	})
	if err != nil {
		return nil, fmt.Errorf("DetectStability failed: %w", err)
	}

	c.logger.Debug("DetectStability completed",
		zap.Bool("stable", resp.IsStable),
		zap.Int32("stable_frame", resp.StableFrameIndex),
	)

	return &StabilityResult{
		IsStable:         resp.IsStable,
		StableFrameIndex: int(resp.StableFrameIndex),
		StabilityScore:   resp.StabilityScore,
		Analysis:         resp.Analysis,
	}, nil
}

// DetectStabilityByURIs checks stability using MinIO URIs
func (c *Client) DetectStabilityByURIs(ctx context.Context, frameURIs []string) (*StabilityResult, error) {
	resp, err := c.client.DetectStability(ctx, &pb.DetectStabilityRequest{
		FrameUris:          frameURIs,
		StabilityThreshold: 0.98,
		MinStableFrames:    3,
	})
	if err != nil {
		return nil, fmt.Errorf("DetectStabilityByURIs failed: %w", err)
	}

	return &StabilityResult{
		IsStable:         resp.IsStable,
		StableFrameIndex: int(resp.StableFrameIndex),
		StabilityScore:   resp.StabilityScore,
		Analysis:         resp.Analysis,
	}, nil
}

// GenerateEmbedding generates an embedding vector for an image
func (c *Client) GenerateEmbedding(ctx context.Context, imageData []byte) (*EmbeddingResult, error) {
	resp, err := c.client.GenerateEmbedding(ctx, &pb.GenerateEmbeddingRequest{
		ImageData: imageData,
		Normalize: true,
	})
	if err != nil {
		return nil, fmt.Errorf("GenerateEmbedding failed: %w", err)
	}

	return &EmbeddingResult{
		Embedding:    resp.Embedding,
		EmbeddingDim: int(resp.EmbeddingDim),
		ModelVersion: resp.ModelVersion,
	}, nil
}

// BatchCompare compares multiple frame pairs
func (c *Client) BatchCompare(ctx context.Context, pairs []FramePair) (*BatchCompareResult, error) {
	pbPairs := make([]*pb.FramePair, len(pairs))
	for i, p := range pairs {
		pbPairs[i] = &pb.FramePair{
			PairId:       p.PairID,
			BaselineData: p.BaselineData,
			BaselineUri:  p.BaselineURI,
			ActualData:   p.ActualData,
			ActualUri:    p.ActualURI,
			Context:      p.Context,
		}
	}

	resp, err := c.client.BatchCompare(ctx, &pb.BatchCompareRequest{
		Pairs: pbPairs,
		Settings: &pb.CompareSettings{
			SimilarityThreshold: 0.85,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("BatchCompare failed: %w", err)
	}

	results := make([]PairCompareResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = PairCompareResult{
			PairID: r.PairId,
			Result: CompareResult{
				SimilarityScore: r.Result.SimilarityScore,
				SemanticMatch:   r.Result.SemanticMatch,
				Confidence:      r.Result.Confidence,
				Analysis:        r.Result.Analysis,
			},
		}
	}

	c.logger.Debug("BatchCompare completed",
		zap.Int("pairs", len(pairs)),
		zap.Float32("avg_similarity", resp.AverageSimilarity),
		zap.Int32("matches", resp.Matches),
	)

	return &BatchCompareResult{
		Results:           results,
		AverageSimilarity: resp.AverageSimilarity,
		Matches:           int(resp.Matches),
		Mismatches:        int(resp.Mismatches),
	}, nil
}

// AnalyzeChange analyzes the visual change after an action
func (c *Client) AnalyzeChange(ctx context.Context, before, after []byte, action string) (*ChangeAnalysis, error) {
	resp, err := c.client.AnalyzeChange(ctx, &pb.AnalyzeChangeRequest{
		BeforeData:      before,
		AfterData:       after,
		ActionPerformed: action,
	})
	if err != nil {
		return nil, fmt.Errorf("AnalyzeChange failed: %w", err)
	}

	c.logger.Debug("AnalyzeChange completed",
		zap.String("action", action),
		zap.Bool("expected", resp.ExpectedChange),
	)

	return &ChangeAnalysis{
		Description:    resp.Description,
		Changes:        resp.Changes,
		ExpectedChange: resp.ExpectedChange,
		Confidence:     resp.Confidence,
	}, nil
}

// HealthCheck verifies the service is operational
func (c *Client) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	resp, err := c.client.HealthCheck(ctx, &pb.HealthCheckRequest{})
	if err != nil {
		return nil, fmt.Errorf("HealthCheck failed: %w", err)
	}

	return &HealthStatus{
		Healthy:        resp.Healthy,
		ModelLoaded:    resp.ModelLoaded,
		Device:         resp.Device,
		MemoryUsedMB:   resp.MemoryUsedMb,
		MemoryTotalMB:  resp.MemoryTotalMb,
		AvgInferenceMS: resp.AvgInferenceMs,
	}, nil
}

// Types

// CompareResult contains the result of comparing two frames
type CompareResult struct {
	SimilarityScore float32
	SemanticMatch   bool
	Confidence      float32
	Analysis        string
	ChangedRegions  []ChangedRegion
}

// ChangedRegion represents a region that changed between frames
type ChangedRegion struct {
	X, Y, Width, Height int
	ChangeType          string
	Significance        float32
	Description         string
}

// StabilityResult contains the result of stability detection
type StabilityResult struct {
	IsStable         bool
	StableFrameIndex int
	StabilityScore   float32
	Analysis         string
}

// EmbeddingResult contains a generated embedding
type EmbeddingResult struct {
	Embedding    []byte
	EmbeddingDim int
	ModelVersion string
}

// FramePair represents a pair of frames to compare
type FramePair struct {
	PairID       string
	BaselineData []byte
	BaselineURI  string
	ActualData   []byte
	ActualURI    string
	Context      string
}

// PairCompareResult contains the result for a single pair in batch comparison
type PairCompareResult struct {
	PairID string
	Result CompareResult
}

// BatchCompareResult contains results from batch comparison
type BatchCompareResult struct {
	Results           []PairCompareResult
	AverageSimilarity float32
	Matches           int
	Mismatches        int
}

// ChangeAnalysis contains the analysis of a visual change
type ChangeAnalysis struct {
	Description    string
	Changes        []string
	ExpectedChange bool
	Confidence     float32
}

// HealthStatus contains the service health information
type HealthStatus struct {
	Healthy        bool
	ModelLoaded    string
	Device         string
	MemoryUsedMB   int64
	MemoryTotalMB  int64
	AvgInferenceMS float32
}
