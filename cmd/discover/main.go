package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/domain"
	"github.com/testforge/testforge/internal/services/discovery"
)

func main() {
	// Parse flags
	url := flag.String("url", "https://demo.playwright.dev/todomvc", "Target URL to discover")
	maxPages := flag.Int("pages", 10, "Maximum pages to crawl")
	maxDepth := flag.Int("depth", 2, "Maximum crawl depth")
	timeout := flag.Duration("timeout", 2*time.Minute, "Discovery timeout")
	output := flag.String("output", "", "Output file for JSON result (empty for stdout)")
	headless := flag.Bool("headless", true, "Run browser in headless mode")

	// Auth flags - credentials should come from environment variables for security
	loginURL := flag.String("login-url", "", "Login page URL for credential auth")
	usernameSelector := flag.String("username-selector", "#username", "CSS selector for username field")
	passwordSelector := flag.String("password-selector", "#password", "CSS selector for password field")
	submitSelector := flag.String("submit-selector", "button[type='submit']", "CSS selector for submit button")
	successIndicator := flag.String("success-indicator", "", "URL pattern or selector to verify login success")

	// Get credentials from environment variables (never from CLI args for security)
	username := os.Getenv("TESTFORGE_AUTH_USERNAME")
	password := os.Getenv("TESTFORGE_AUTH_PASSWORD")

	flag.Parse()

	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Printf("Starting discovery for: %s\n", *url)
	fmt.Printf("Max pages: %d, Max depth: %d, Timeout: %s\n", *maxPages, *maxDepth, *timeout)
	fmt.Println("---")

	// Configure discovery
	config := discovery.DiscoveryConfig{
		MaxPages:    *maxPages,
		MaxDuration: *timeout,
		MaxDepth:    *maxDepth,
		Concurrency: 2,
		Headless:    *headless,
		Timeout:     30 * time.Second,
	}

	// Build auth config if login URL is provided
	var authConfig *domain.AuthConfig
	if *loginURL != "" && username != "" && password != "" {
		fmt.Printf("Authentication enabled: %s\n", *loginURL)
		authConfig = &domain.AuthConfig{
			Type: domain.AuthTypeCredentials,
			Credentials: &domain.Credentials{
				LoginURL:         *loginURL,
				UsernameSelector: *usernameSelector,
				PasswordSelector: *passwordSelector,
				SubmitSelector:   *submitSelector,
				Username:         username,
				Password:         password,
				SuccessIndicator: *successIndicator,
				WaitAfterLogin:   2000,
			},
		}
	}

	// Create crawler (no storage for this test)
	crawler, err := discovery.NewCrawler(config, nil)
	if err != nil {
		fmt.Printf("Error creating crawler: %v\n", err)
		os.Exit(1)
	}
	defer crawler.Close()

	// Set progress callback
	crawler.SetHeartbeatCallback(func(msg string) {
		fmt.Printf("  [Progress] %s\n", msg)
	})

	// Run discovery
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	startTime := time.Now()
	appModel, err := crawler.CrawlWithAuth(ctx, *url, authConfig)
	if err != nil {
		fmt.Printf("Error crawling: %v\n", err)
		os.Exit(1)
	}
	duration := time.Since(startTime)

	// Print results
	fmt.Println("---")
	fmt.Println("Discovery Results:")
	fmt.Printf("├── Pages Found: %d\n", appModel.Stats.TotalPages)
	fmt.Printf("├── Forms: %d\n", appModel.Stats.TotalForms)
	fmt.Printf("├── Buttons: %d\n", appModel.Stats.TotalButtons)
	fmt.Printf("├── Links: %d\n", appModel.Stats.TotalLinks)
	fmt.Printf("├── Inputs: %d\n", appModel.Stats.TotalInputs)
	fmt.Printf("├── Screenshots: %d\n", appModel.Stats.TotalScreenshots)
	fmt.Printf("├── Flows Detected: %d\n", len(appModel.Flows))
	fmt.Printf("├── Max Depth Reached: %d\n", appModel.Stats.MaxDepthReached)
	fmt.Printf("└── Duration: %s\n", duration.Round(time.Millisecond))

	// Print pages
	fmt.Println("\nDiscovered Pages:")
	for url, page := range appModel.Pages {
		authStr := ""
		if page.HasAuth {
			authStr = " [AUTH]"
		}
		fmt.Printf("  - %s (%s)%s\n", url, page.PageType, authStr)
		fmt.Printf("    Title: %s\n", page.Title)
		fmt.Printf("    Forms: %d, Buttons: %d, Links: %d\n",
			len(page.Forms), len(page.Buttons), len(page.Links))
	}

	// Print flows
	if len(appModel.Flows) > 0 {
		fmt.Println("\nDetected Business Flows:")
		for _, flow := range appModel.Flows {
			fmt.Printf("  - %s (%s) [%s]\n", flow.Name, flow.Type, flow.Priority)
		}
	}

	// Output JSON if requested
	if *output != "" {
		jsonData, err := json.MarshalIndent(appModel, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*output, jsonData, 0644); err != nil {
			fmt.Printf("Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nJSON output saved to: %s\n", *output)
	}
}
