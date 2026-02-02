package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/storage"
)

// Service orchestrates the discovery process
type Service struct {
	storage *storage.MinIOClient
	logger  *zap.Logger
	config  DiscoveryConfig
}

// NewService creates a new discovery service
func NewService(storage *storage.MinIOClient, logger *zap.Logger) *Service {
	return &Service{
		storage: storage,
		logger:  logger,
		config:  DefaultConfig(),
	}
}

// WithConfig returns a new service with the given config
func (s *Service) WithConfig(config DiscoveryConfig) *Service {
	return &Service{
		storage: s.storage,
		logger:  s.logger,
		config:  config,
	}
}

// DiscoveryInput contains input for the discovery process
type DiscoveryInput struct {
	TestRunID     uuid.UUID
	TenantID      uuid.UUID
	TargetURL     string
	MaxCrawlDepth int
	MaxPages      int
	Timeout       time.Duration
	AuthConfig    *domain.AuthConfig
}

// DiscoveryOutput contains the result of discovery
type DiscoveryOutput struct {
	AppModel      *AppModel
	AppModelURI   string
	PagesFound    int
	FormsFound    int
	FlowsDetected int
	Duration      time.Duration
}

// HeartbeatFunc is called periodically during discovery
type HeartbeatFunc func(status string)

// Discover performs website discovery and returns the app model
func (s *Service) Discover(ctx context.Context, input DiscoveryInput, heartbeat HeartbeatFunc) (*DiscoveryOutput, error) {
	startTime := time.Now()

	s.logger.Info("Starting discovery",
		zap.String("test_run_id", input.TestRunID.String()),
		zap.String("target_url", input.TargetURL),
		zap.Int("max_depth", input.MaxCrawlDepth),
	)

	// Build config from input
	config := s.config
	if input.MaxCrawlDepth > 0 {
		config.MaxDepth = input.MaxCrawlDepth
	}
	if input.MaxPages > 0 {
		config.MaxPages = input.MaxPages
	}
	if input.Timeout > 0 {
		config.MaxDuration = input.Timeout
	}

	// Create crawler
	crawler, err := NewCrawler(config, s.storage)
	if err != nil {
		return nil, fmt.Errorf("creating crawler: %w", err)
	}
	defer crawler.Close()

	// Set up heartbeat
	if heartbeat != nil {
		crawler.SetHeartbeatCallback(func(msg string) {
			heartbeat(msg)
		})
		crawler.SetProgressCallback(func(current, total int) {
			heartbeat(fmt.Sprintf("Progress: %d/%d pages", current, total))
		})
	}

	// Perform crawl with optional authentication
	appModel, err := crawler.CrawlWithAuth(ctx, input.TargetURL, input.AuthConfig)
	if err != nil {
		return nil, fmt.Errorf("crawling: %w", err)
	}

	// Upload app model to storage
	var appModelURI string
	if s.storage != nil {
		appModelJSON, err := json.MarshalIndent(appModel, "", "  ")
		if err == nil {
			key := fmt.Sprintf("%s/%s/discovery/app_model.json",
				input.TenantID.String(),
				input.TestRunID.String(),
			)
			appModelURI, err = s.storage.UploadJSON(ctx, key, appModelJSON)
			if err != nil {
				s.logger.Warn("Failed to upload app model", zap.Error(err))
			}
		}
	}

	duration := time.Since(startTime)

	output := &DiscoveryOutput{
		AppModel:      appModel,
		AppModelURI:   appModelURI,
		PagesFound:    appModel.Stats.TotalPages,
		FormsFound:    appModel.Stats.TotalForms,
		FlowsDetected: len(appModel.Flows),
		Duration:      duration,
	}

	s.logger.Info("Discovery completed",
		zap.String("test_run_id", input.TestRunID.String()),
		zap.Int("pages_found", output.PagesFound),
		zap.Int("forms_found", output.FormsFound),
		zap.Int("flows_detected", output.FlowsDetected),
		zap.Duration("duration", duration),
	)

	return output, nil
}

// DiscoverWithoutStorage performs discovery without uploading to storage
// Useful for testing or when storage is not available
func (s *Service) DiscoverWithoutStorage(ctx context.Context, targetURL string, config DiscoveryConfig) (*AppModel, error) {
	crawler, err := NewCrawler(config, nil)
	if err != nil {
		return nil, fmt.Errorf("creating crawler: %w", err)
	}
	defer crawler.Close()

	return crawler.Crawl(ctx, targetURL)
}
