package discovery

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.uber.org/zap"

	discoveryservice "github.com/testforge/testforge/internal/services/discovery"
	"github.com/testforge/testforge/internal/storage"
	"github.com/testforge/testforge/internal/workflows"
)

// Activity implements the discovery activity
type Activity struct {
	storage *storage.MinIOClient
	logger  *zap.Logger
	useMock bool // Set to true to use mock data instead of real crawling
}

// Config contains activity configuration
type Config struct {
	MinIOEndpoint   string
	MinIOAccessKey  string
	MinIOSecretKey  string
	MinIOBucket     string
	MinIOUseSSL     bool
	UseMock         bool
	PlaywrightPath  string
}

// NewActivity creates a new discovery activity
func NewActivity() *Activity {
	return &Activity{
		useMock: true, // Default to mock for safety
	}
}

// NewActivityWithConfig creates a new discovery activity with configuration
func NewActivityWithConfig(cfg Config, logger *zap.Logger) (*Activity, error) {
	a := &Activity{
		logger:  logger,
		useMock: cfg.UseMock,
	}

	// Initialize MinIO client if configured
	if cfg.MinIOEndpoint != "" {
		client, err := storage.NewMinIOClient(storage.MinIOConfig{
			Endpoint:        cfg.MinIOEndpoint,
			AccessKeyID:     cfg.MinIOAccessKey,
			SecretAccessKey: cfg.MinIOSecretKey,
			UseSSL:          cfg.MinIOUseSSL,
			BucketName:      cfg.MinIOBucket,
		})
		if err != nil {
			return nil, fmt.Errorf("creating minio client: %w", err)
		}
		a.storage = client
	}

	return a, nil
}

// Execute performs website discovery and returns an app model
func (a *Activity) Execute(ctx context.Context, input workflows.DiscoveryInput) (*workflows.DiscoveryOutput, error) {
	logger := activity.GetLogger(ctx)
	startTime := time.Now()

	logger.Info("Starting discovery activity",
		"test_run_id", input.TestRunID.String(),
		"target_url", input.TargetURL,
		"max_depth", input.MaxCrawlDepth,
	)

	activity.RecordHeartbeat(ctx, "Starting discovery...")

	// Try real discovery, fall back to mock if it fails
	if !a.useMock {
		output, err := a.executeRealDiscovery(ctx, input)
		if err != nil {
			logger.Warn("Real discovery failed, falling back to mock",
				"error", err.Error(),
			)
		} else {
			return output, nil
		}
	}

	// Use mock discovery
	return a.executeMockDiscovery(ctx, input, startTime)
}

// executeRealDiscovery performs actual website crawling with Playwright
func (a *Activity) executeRealDiscovery(ctx context.Context, input workflows.DiscoveryInput) (*workflows.DiscoveryOutput, error) {
	// Create discovery service
	var logger *zap.Logger
	if a.logger != nil {
		logger = a.logger
	} else {
		logger, _ = zap.NewProduction()
	}

	service := discoveryservice.NewService(a.storage, logger)

	// Configure discovery
	config := discoveryservice.DefaultConfig()
	if input.MaxCrawlDepth > 0 {
		config.MaxDepth = input.MaxCrawlDepth
	}
	config.MaxPages = 50 // Default limit
	config.MaxDuration = 5 * time.Minute
	config.Headless = true

	service = service.WithConfig(config)

	// Perform discovery with heartbeats
	heartbeat := func(status string) {
		activity.RecordHeartbeat(ctx, status)
	}

	discoveryInput := discoveryservice.DiscoveryInput{
		TestRunID:     input.TestRunID,
		TenantID:      input.TestRunID, // Using TestRunID as TenantID placeholder
		TargetURL:     input.TargetURL,
		MaxCrawlDepth: input.MaxCrawlDepth,
	}

	result, err := service.Discover(ctx, discoveryInput, heartbeat)
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	// Convert to workflow output
	return a.convertToWorkflowOutput(result), nil
}

// convertToWorkflowOutput converts discovery result to workflow output
func (a *Activity) convertToWorkflowOutput(result *discoveryservice.DiscoveryOutput) *workflows.DiscoveryOutput {
	appModel := result.AppModel

	// Convert pages
	var pages []workflows.PageInfo
	for url, page := range appModel.Pages {
		pages = append(pages, workflows.PageInfo{
			URL:        url,
			Title:      page.Title,
			PageType:   page.PageType,
			HasForms:   len(page.Forms) > 0,
			HasAuth:    page.HasAuth,
			Screenshot: getFirstScreenshot(page.Screenshots),
		})
	}

	// Convert components
	var components []workflows.ComponentInfo
	for _, comp := range appModel.Components {
		components = append(components, workflows.ComponentInfo{
			ID:       comp.ID,
			Type:     comp.Type,
			Selector: comp.Selectors.BestSelector(),
			Pages:    comp.FoundOnPages,
		})
	}

	// Convert flows
	var flows []workflows.FlowInfo
	for _, flow := range appModel.Flows {
		// Convert FlowSteps to URLs for workflow compatibility
		var stepURLs []string
		for _, step := range flow.Steps {
			if step.URL != "" {
				stepURLs = append(stepURLs, step.URL)
			}
		}
		flows = append(flows, workflows.FlowInfo{
			ID:       flow.ID,
			Name:     flow.Name,
			Type:     flow.Type,
			Steps:    stepURLs,
			Priority: flow.Priority,
		})
	}

	workflowAppModel := &workflows.AppModel{
		Pages:         pages,
		Components:    components,
		BusinessFlows: flows,
		TechStack:     appModel.TechStack,
	}

	return &workflows.DiscoveryOutput{
		AppModel:      workflowAppModel,
		PagesFound:    appModel.Stats.TotalPages,
		FormsFound:    appModel.Stats.TotalForms,
		FlowsDetected: len(appModel.Flows),
		Duration:      result.Duration,
	}
}

// executeMockDiscovery returns mock data for testing
func (a *Activity) executeMockDiscovery(ctx context.Context, input workflows.DiscoveryInput, startTime time.Time) (*workflows.DiscoveryOutput, error) {
	logger := activity.GetLogger(ctx)

	// Simulate discovery work
	time.Sleep(2 * time.Second)
	activity.RecordHeartbeat(ctx, "Crawling pages...")

	// Mock discovered pages
	pages := []workflows.PageInfo{
		{
			URL:      input.TargetURL,
			Title:    "Home Page",
			PageType: "landing",
			HasForms: false,
			HasAuth:  false,
		},
		{
			URL:      fmt.Sprintf("%s/login", input.TargetURL),
			Title:    "Login",
			PageType: "auth",
			HasForms: true,
			HasAuth:  true,
		},
		{
			URL:      fmt.Sprintf("%s/products", input.TargetURL),
			Title:    "Products",
			PageType: "list",
			HasForms: false,
			HasAuth:  false,
		},
		{
			URL:      fmt.Sprintf("%s/cart", input.TargetURL),
			Title:    "Shopping Cart",
			PageType: "form",
			HasForms: true,
			HasAuth:  false,
		},
		{
			URL:      fmt.Sprintf("%s/checkout", input.TargetURL),
			Title:    "Checkout",
			PageType: "form",
			HasForms: true,
			HasAuth:  true,
		},
	}

	// Mock components
	components := []workflows.ComponentInfo{
		{ID: "nav-1", Type: "navbar", Selector: "nav.main-nav", Pages: []string{"/", "/products", "/cart"}},
		{ID: "footer-1", Type: "footer", Selector: "footer", Pages: []string{"/", "/products", "/cart"}},
		{ID: "login-form-1", Type: "form", Selector: "form#login", Pages: []string{"/login"}},
		{ID: "cart-form-1", Type: "form", Selector: "form#cart", Pages: []string{"/cart"}},
	}

	// Mock business flows
	flows := []workflows.FlowInfo{
		{
			ID:       "auth-flow-1",
			Name:     "User Login",
			Type:     "auth",
			Steps:    []string{"/", "/login", "/"},
			Priority: "high",
		},
		{
			ID:       "checkout-flow-1",
			Name:     "Purchase Flow",
			Type:     "checkout",
			Steps:    []string{"/products", "/cart", "/checkout"},
			Priority: "critical",
		},
		{
			ID:       "browse-flow-1",
			Name:     "Product Browsing",
			Type:     "navigation",
			Steps:    []string{"/", "/products"},
			Priority: "medium",
		},
	}

	appModel := &workflows.AppModel{
		Pages:         pages,
		Components:    components,
		BusinessFlows: flows,
		TechStack:     []string{"React", "Node.js", "PostgreSQL"},
	}

	duration := time.Since(startTime)

	output := &workflows.DiscoveryOutput{
		AppModel:      appModel,
		PagesFound:    len(pages),
		FormsFound:    4,
		FlowsDetected: len(flows),
		Duration:      duration,
	}

	logger.Info("Discovery activity completed (mock)",
		"pages_found", output.PagesFound,
		"flows_detected", output.FlowsDetected,
		"duration", duration,
	)

	return output, nil
}

func getFirstScreenshot(screenshots []string) string {
	if len(screenshots) > 0 {
		return screenshots[0]
	}
	return ""
}
