package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/testforge/testforge/internal/llm"
	"github.com/testforge/testforge/internal/services/discovery"
	"github.com/testforge/testforge/internal/services/testdesign"
)

func main() {
	// Parse flags
	appModelFile := flag.String("app-model", "", "Path to app model JSON file from discovery")
	url := flag.String("url", "", "Target URL to discover first (if no app-model provided)")
	output := flag.String("output", "", "Output file for test suite JSON")
	maxPages := flag.Int("pages", 5, "Maximum pages to crawl for discovery")
	maxDepth := flag.Int("depth", 2, "Maximum crawl depth for discovery")
	flag.Parse()

	// Check for API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: ANTHROPIC_API_KEY environment variable is required")
		fmt.Println("Set it with: export ANTHROPIC_API_KEY=your-api-key")
		os.Exit(1)
	}

	var appModel *discovery.AppModel
	var err error

	// Either load app model from file or run discovery
	if *appModelFile != "" {
		appModel, err = loadAppModel(*appModelFile)
		if err != nil {
			fmt.Printf("Error loading app model: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Loaded app model with %d pages\n", len(appModel.Pages))
	} else if *url != "" {
		fmt.Printf("Running discovery on %s...\n", *url)
		appModel, err = runDiscovery(*url, *maxPages, *maxDepth)
		if err != nil {
			fmt.Printf("Error during discovery: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Discovery complete: %d pages, %d flows\n", len(appModel.Pages), len(appModel.Flows))
	} else {
		fmt.Println("Error: Either -app-model or -url is required")
		flag.Usage()
		os.Exit(1)
	}

	// Create Claude client
	cfg := llm.DefaultConfig()
	cfg.APIKey = apiKey
	client, err := llm.NewClaudeClient(cfg)
	if err != nil {
		fmt.Printf("Error creating Claude client: %v\n", err)
		os.Exit(1)
	}

	// Create architect
	config := testdesign.DefaultArchitectConfig()
	config.IncludeSecurityTests = true
	config.IncludeAccessibilityTests = true
	architect := testdesign.NewTestArchitect(client, config)

	// Prepare input
	input := testdesign.DesignInput{
		AppModel:    appModel,
		ProjectID:   "test-project",
		ProjectName: getProjectName(appModel),
		BaseURL:     getBaseURL(appModel),
		Roles:       []string{"anonymous", "user", "admin"},
		Environment: "test",
	}

	fmt.Println("---")
	fmt.Printf("Generating test suite for: %s\n", input.ProjectName)
	fmt.Printf("Base URL: %s\n", input.BaseURL)
	fmt.Printf("Pages to analyze: %d\n", len(appModel.Pages))
	fmt.Printf("Flows to test: %d\n", len(appModel.Flows))
	fmt.Println("---")

	// Generate test suite
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Println("Calling Claude API to generate tests...")
	startTime := time.Now()
	result, err := architect.DesignTestSuite(ctx, input)
	if err != nil {
		fmt.Printf("Error generating test suite: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Println("\n=== TEST SUITE GENERATION COMPLETE ===")

	fmt.Printf("Suite: %s\n", result.Suite.Name)
	fmt.Printf("Version: %s\n", result.Suite.Version)
	fmt.Printf("Generated: %s\n\n", result.Suite.Metadata.GeneratedAt.Format(time.RFC3339))

	// Stats
	fmt.Println("📊 Statistics:")
	fmt.Printf("├── Features: %d\n", result.Suite.Stats.TotalFeatures)
	fmt.Printf("├── Scenarios: %d\n", result.Suite.Stats.TotalScenarios)
	fmt.Printf("├── Test Cases: %d\n", result.Suite.Stats.TotalTestCases)
	fmt.Printf("├── Test Steps: %d\n", result.Suite.Stats.TotalSteps)
	fmt.Printf("├── Coverage Score: %.1f%%\n", result.Suite.Stats.CoverageScore)
	fmt.Printf("└── Est. Duration: %s\n\n", result.Suite.Stats.EstimatedDuration)

	// Coverage by type
	fmt.Println("📋 Coverage by Type:")
	for testType, count := range result.Suite.Stats.ByType {
		fmt.Printf("├── %s: %d\n", testType, count)
	}

	// Coverage by priority
	fmt.Println("\n🎯 Coverage by Priority:")
	for priority, count := range result.Suite.Stats.ByPriority {
		fmt.Printf("├── %s: %d\n", priority, count)
	}

	// Token usage and cost
	fmt.Println("\n💰 LLM Usage:")
	fmt.Printf("├── Chunks Processed: %d\n", result.Chunks)
	fmt.Printf("├── Input Tokens: %d\n", result.InputTokens)
	fmt.Printf("├── Output Tokens: %d\n", result.OutputTokens)
	fmt.Printf("├── Total Tokens: %d\n", result.TokensUsed)
	fmt.Printf("├── Estimated Cost: $%.4f\n", result.EstimatedCost)
	fmt.Printf("└── Generation Time: %s\n", result.GenerationTime.Round(time.Millisecond))

	// Validation warnings
	if len(result.ValidationWarns) > 0 {
		fmt.Println("\n⚠️  Validation Warnings:")
		for _, warn := range result.ValidationWarns {
			fmt.Printf("├── %s\n", warn)
		}
	}

	// List features
	fmt.Println("\n📁 Features Generated:")
	for i, feature := range result.Suite.Features {
		scenarioCount := len(feature.Scenarios)
		testCount := 0
		for _, s := range feature.Scenarios {
			testCount += len(s.TestCases)
		}
		fmt.Printf("%d. %s (%d scenarios, %d tests) [%s]\n",
			i+1, feature.Name, scenarioCount, testCount, feature.Priority)
	}

	// Sample test cases
	fmt.Println("\n📝 Sample Test Cases:")
	sampleCount := 0
	for _, feature := range result.Suite.Features {
		for _, scenario := range feature.Scenarios {
			for _, tc := range scenario.TestCases {
				if sampleCount >= 5 {
					break
				}
				fmt.Printf("\n  Test: %s\n", tc.Name)
				fmt.Printf("  Type: %s | Priority: %s | Category: %s\n", tc.Type, tc.Priority, tc.Category)
				if tc.Given != "" {
					fmt.Printf("  Given: %s\n", truncate(tc.Given, 80))
				}
				if tc.When != "" {
					fmt.Printf("  When: %s\n", truncate(tc.When, 80))
				}
				if tc.Then != "" {
					fmt.Printf("  Then: %s\n", truncate(tc.Then, 80))
				}
				fmt.Printf("  Steps: %d\n", len(tc.Steps))
				sampleCount++
			}
		}
	}

	// Output JSON if requested
	if *output != "" {
		jsonData, err := json.MarshalIndent(result.Suite, "", "  ")
		if err != nil {
			fmt.Printf("\nError marshaling JSON: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*output, jsonData, 0644); err != nil {
			fmt.Printf("\nError writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ Test suite saved to: %s\n", *output)
	}

	fmt.Printf("\n✅ Test design completed in %s\n", time.Since(startTime).Round(time.Millisecond))
}

func loadAppModel(path string) (*discovery.AppModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var appModel discovery.AppModel
	if err := json.Unmarshal(data, &appModel); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return &appModel, nil
}

func runDiscovery(targetURL string, maxPages, maxDepth int) (*discovery.AppModel, error) {
	config := discovery.DiscoveryConfig{
		MaxPages:    maxPages,
		MaxDepth:    maxDepth,
		MaxDuration: 2 * time.Minute,
		Headless:    true,
		Timeout:     30 * time.Second,
		Concurrency: 2,
	}

	crawler, err := discovery.NewCrawler(config, nil)
	if err != nil {
		return nil, fmt.Errorf("creating crawler: %w", err)
	}
	defer crawler.Close()

	ctx, cancel := context.WithTimeout(context.Background(), config.MaxDuration)
	defer cancel()

	return crawler.Crawl(ctx, targetURL)
}

func getProjectName(appModel *discovery.AppModel) string {
	for _, page := range appModel.Pages {
		if page.Title != "" && page.Title != "Untitled" {
			return page.Title
		}
	}
	return "Test Project"
}

func getBaseURL(appModel *discovery.AppModel) string {
	for _, page := range appModel.Pages {
		return page.URL
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
