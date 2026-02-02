package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/testforge/testforge/internal/services/scriptgen"
	"github.com/testforge/testforge/internal/services/testdesign"
)

func main() {
	// Parse flags
	testSuiteFile := flag.String("suite", "", "Path to test suite JSON file (from testdesign)")
	outputDir := flag.String("output", "./generated", "Output directory for generated scripts")
	baseURL := flag.String("base-url", "", "Base URL for tests (overrides suite config)")
	flag.Parse()

	if *testSuiteFile == "" {
		fmt.Println("Error: -suite flag is required")
		fmt.Println("Usage: scriptgen -suite test_suite.json -output ./generated")
		os.Exit(1)
	}

	// Load test suite
	suite, err := loadTestSuite(*testSuiteFile)
	if err != nil {
		fmt.Printf("Error loading test suite: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded test suite: %s\n", suite.Name)
	fmt.Printf("├── Features: %d\n", suite.Stats.TotalFeatures)
	fmt.Printf("├── Scenarios: %d\n", suite.Stats.TotalScenarios)
	fmt.Printf("├── Test Cases: %d\n", suite.Stats.TotalTestCases)
	fmt.Printf("└── Test Steps: %d\n\n", suite.Stats.TotalSteps)

	// Determine base URL
	url := *baseURL
	if url == "" {
		url = suite.GlobalConfig.BaseURL
	}
	if url == "" {
		// Try to extract from first test case
		for _, feature := range suite.Features {
			for _, scenario := range feature.Scenarios {
				for _, tc := range scenario.TestCases {
					if tc.TargetURL != "" {
						url = tc.TargetURL
						break
					}
				}
				if url != "" {
					break
				}
			}
			if url != "" {
				break
			}
		}
	}

	// Create generator
	config := scriptgen.GeneratorConfig{
		OutputDir:        *outputDir,
		TypeScriptStrict: true,
		IncludeComments:  true,
		GeneratePOM:      true,
		GenerateFixtures: true,
		BaseURL:          url,
		TestRunID:        suite.ID,
		TenantID:         suite.ProjectID,
		APIEndpoint:      "http://localhost:8081/api/v1",
	}

	generator := scriptgen.NewScriptGenerator(config)

	fmt.Println("Generating Playwright scripts...")
	startTime := time.Now()

	// Generate scripts
	project, err := generator.GenerateScripts(suite)
	if err != nil {
		fmt.Printf("Error generating scripts: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(startTime)

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Write all files
	filesWritten := 0
	for filename, content := range project.Files {
		fullPath := filepath.Join(*outputDir, filename)

		// Create parent directories
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", dir, err)
			continue
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			fmt.Printf("Error writing %s: %v\n", filename, err)
			continue
		}

		filesWritten++
	}

	// Print summary
	fmt.Println("\n=== SCRIPT GENERATION COMPLETE ===\n")
	fmt.Printf("📁 Output Directory: %s\n", *outputDir)
	fmt.Printf("📄 Files Generated: %d\n", filesWritten)
	fmt.Printf("🧪 Test Cases: %d\n", project.TestCount)
	fmt.Printf("📑 Page Objects: %d\n", project.PageCount)
	fmt.Printf("📝 Lines of Code: %d\n", project.LinesOfCode)
	fmt.Printf("⏱️  Duration: %s\n\n", duration.Round(time.Millisecond))

	// List generated files by category
	fmt.Println("Generated Files:")

	// Config files
	fmt.Println("\n📋 Configuration:")
	configFiles := []string{"playwright.config.ts", "package.json", "tsconfig.json", ".env.example", ".gitignore", "README.md"}
	for _, f := range configFiles {
		if _, ok := project.Files[f]; ok {
			fmt.Printf("   ├── %s\n", f)
		}
	}

	// Page Objects
	fmt.Println("\n📄 Page Objects:")
	for filename := range project.Files {
		if filepath.Dir(filename) == "pages" && filename != "pages/base.page.ts" {
			fmt.Printf("   ├── %s\n", filename)
		}
	}
	fmt.Printf("   └── pages/base.page.ts\n")

	// Utilities
	fmt.Println("\n🔧 Utilities:")
	for filename := range project.Files {
		if filepath.Dir(filename) == "utils" {
			fmt.Printf("   ├── %s\n", filename)
		}
	}

	// Fixtures
	fmt.Println("\n🎭 Fixtures:")
	for filename := range project.Files {
		if filepath.Dir(filename) == "fixtures" {
			fmt.Printf("   ├── %s\n", filename)
		}
	}

	// Test Files
	fmt.Println("\n🧪 Test Files:")
	testDirs := map[string][]string{
		"smoke":         {},
		"regression":    {},
		"e2e":           {},
		"accessibility": {},
		"security":      {},
	}
	for filename := range project.Files {
		dir := filepath.Dir(filename)
		if filepath.Dir(dir) == "tests" {
			testType := filepath.Base(dir)
			testDirs[testType] = append(testDirs[testType], filename)
		}
	}
	for testType, files := range testDirs {
		if len(files) > 0 {
			fmt.Printf("   tests/%s/\n", testType)
			for _, f := range files {
				fmt.Printf("      └── %s\n", filepath.Base(f))
			}
		}
	}

	// Instructions
	fmt.Println("\n🚀 Next Steps:")
	fmt.Printf("   cd %s\n", *outputDir)
	fmt.Println("   npm install")
	fmt.Println("   npx playwright install")
	fmt.Println("   npx playwright test --list")
	fmt.Println("   npm test")
}

func loadTestSuite(path string) (*testdesign.TestSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var suite testdesign.TestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	// Ensure stats are calculated
	if suite.Stats.TotalTestCases == 0 {
		suite.CalculateStats()
	}

	return &suite, nil
}
