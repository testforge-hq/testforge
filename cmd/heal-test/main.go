package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/testforge/testforge/internal/services/healing"
)

func main() {
	// Parse flags
	apiKey := flag.String("api-key", os.Getenv("ANTHROPIC_API_KEY"), "Claude API key")
	model := flag.String("model", "claude-sonnet-4-20250514", "Claude model to use")
	vjepaEndpoint := flag.String("vjepa", "localhost:50051", "V-JEPA service endpoint")
	selector := flag.String("selector", "", "Failed selector to repair")
	errorMsg := flag.String("error", "", "Error message from test failure")
	pageURL := flag.String("url", "", "Page URL where failure occurred")
	htmlFile := flag.String("html", "", "File containing page HTML")
	testCode := flag.String("code", "", "Test code that failed")
	failedLine := flag.Int("line", 0, "Line number that failed")
	enableVisual := flag.Bool("visual", false, "Enable V-JEPA visual healing")
	verbose := flag.Bool("verbose", false, "Verbose output")

	flag.Parse()

	// Setup logger
	var logger *zap.Logger
	var err error
	if *verbose {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Check required flags
	if *apiKey == "" {
		fmt.Println("Error: Claude API key required. Set ANTHROPIC_API_KEY or use --api-key")
		flag.Usage()
		os.Exit(1)
	}

	// If no selector provided, run demo
	if *selector == "" {
		runDemo(logger, *apiKey, *model, *vjepaEndpoint, *enableVisual)
		return
	}

	// Read HTML from file if provided
	var pageHTML string
	if *htmlFile != "" {
		data, err := os.ReadFile(*htmlFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read HTML file: %v\n", err)
			os.Exit(1)
		}
		pageHTML = string(data)
	}

	// Create healing service
	config := healing.HealingConfig{
		ClaudeAPIKey:        *apiKey,
		ClaudeModel:         *model,
		ClaudeMaxTokens:     4096,
		VJEPAEndpoint:       *vjepaEndpoint,
		SimilarityThreshold: 0.85,
		MaxAttempts:         3,
		MaxHTMLSize:         100000,
		TimeoutSeconds:      60,
		MinConfidence:       0.7,
		EnableVisualHealing: *enableVisual,
		EnableSuggestions:   true,
	}

	service, err := healing.NewService(config, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create healing service: %v\n", err)
		os.Exit(1)
	}
	defer service.Close()

	// Create healing request
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	request := &healing.HealingRequest{
		TestName:     "manual-test",
		TestFile:     "test.spec.ts",
		Selector:     *selector,
		ErrorMessage: *errorMsg,
		PageHTML:     pageHTML,
		PageURL:      *pageURL,
		TestCode:     *testCode,
		FailedLine:   *failedLine,
	}

	// Detect failure type
	request.FailureType = healing.DetectFailureType(*errorMsg)

	fmt.Printf("🔧 Healing Request\n")
	fmt.Printf("   Selector: %s\n", *selector)
	fmt.Printf("   Error: %s\n", *errorMsg)
	fmt.Printf("   Failure Type: %s\n", request.FailureType)
	fmt.Println()

	// Attempt healing
	fmt.Println("⏳ Calling Claude for selector repair...")
	result, err := service.Heal(ctx, request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Healing failed: %v\n", err)
		os.Exit(1)
	}

	// Print result
	fmt.Println()
	fmt.Printf("📊 Healing Result\n")
	fmt.Printf("   Status: %s\n", result.Status)
	fmt.Printf("   Strategy: %s\n", result.Strategy)
	fmt.Printf("   Confidence: %.2f%%\n", result.Confidence*100)
	fmt.Printf("   Duration: %v\n", result.Duration)
	fmt.Println()

	if result.HealedSelector != "" {
		fmt.Printf("✅ Healed Selector:\n")
		fmt.Printf("   %s\n", result.HealedSelector)
		fmt.Println()
	}

	fmt.Printf("📝 Explanation:\n")
	fmt.Printf("   %s\n", result.Explanation)
	fmt.Println()

	if len(result.Suggestions) > 0 {
		fmt.Printf("💡 Suggestions:\n")
		for i, s := range result.Suggestions {
			fmt.Printf("   %d. [%.0f%%] %s\n", i+1, s.Confidence*100, s.Description)
			if s.Selector != "" {
				fmt.Printf("      Selector: %s\n", s.Selector)
			}
			if s.Code != "" {
				fmt.Printf("      Code: %s\n", s.Code)
			}
		}
	}

	if result.HealedCode != "" {
		fmt.Println()
		fmt.Printf("📄 Healed Code:\n")
		fmt.Println("```typescript")
		fmt.Println(result.HealedCode)
		fmt.Println("```")
	}
}

func runDemo(logger *zap.Logger, apiKey, model, vjepaEndpoint string, enableVisual bool) {
	fmt.Println("🧪 TestForge Self-Healing Demo")
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println()

	// Create healing service
	config := healing.HealingConfig{
		ClaudeAPIKey:        apiKey,
		ClaudeModel:         model,
		ClaudeMaxTokens:     4096,
		VJEPAEndpoint:       vjepaEndpoint,
		SimilarityThreshold: 0.85,
		MaxAttempts:         3,
		MaxHTMLSize:         100000,
		TimeoutSeconds:      60,
		MinConfidence:       0.7,
		EnableVisualHealing: enableVisual,
		EnableSuggestions:   true,
	}

	service, err := healing.NewService(config, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create healing service: %v\n", err)
		os.Exit(1)
	}
	defer service.Close()

	// Demo scenarios
	scenarios := []struct {
		name     string
		request  *healing.HealingRequest
	}{
		{
			name: "Selector Changed - Class Renamed",
			request: &healing.HealingRequest{
				TestName:     "login.spec.ts",
				TestFile:     "tests/login.spec.ts",
				Selector:     "button.btn-primary.login-btn",
				ErrorMessage: "Error: Timeout 30000ms exceeded.\nWaiting for selector \"button.btn-primary.login-btn\"\n=========================== logs ===========================\nwaiting for selector \"button.btn-primary.login-btn\"",
				PageHTML: `<!DOCTYPE html>
<html>
<head><title>Login</title></head>
<body>
  <div class="container">
    <form class="login-form" id="loginForm">
      <h1>Welcome Back</h1>
      <div class="form-group">
        <label for="email">Email</label>
        <input type="email" id="email" name="email" placeholder="Enter your email">
      </div>
      <div class="form-group">
        <label for="password">Password</label>
        <input type="password" id="password" name="password" placeholder="Enter your password">
      </div>
      <button type="submit" class="btn btn-primary submit-btn" data-testid="login-submit">
        Sign In
      </button>
      <a href="/forgot-password" class="forgot-link">Forgot password?</a>
    </form>
  </div>
</body>
</html>`,
				PageURL: "https://example.com/login",
				TestCode: `test('user can login', async ({ page }) => {
  await page.goto('/login');
  await page.fill('#email', 'user@example.com');
  await page.fill('#password', 'password123');
  await page.click('button.btn-primary.login-btn'); // Line 5
  await expect(page).toHaveURL('/dashboard');
});`,
				FailedLine: 5,
			},
		},
		{
			name: "Element Moved - ID Changed",
			request: &healing.HealingRequest{
				TestName:     "checkout.spec.ts",
				TestFile:     "tests/checkout.spec.ts",
				Selector:     "#add-to-cart-btn",
				ErrorMessage: "Error: locator.click: Error: element not found\n  selector \"#add-to-cart-btn\"",
				PageHTML: `<!DOCTYPE html>
<html>
<head><title>Product Page</title></head>
<body>
  <div class="product-container">
    <div class="product-info">
      <h1>Premium Widget</h1>
      <p class="price">$49.99</p>
      <p class="description">A fantastic widget for all your needs.</p>
    </div>
    <div class="product-actions">
      <select id="quantity" name="quantity">
        <option value="1">1</option>
        <option value="2">2</option>
        <option value="3">3</option>
      </select>
      <button
        type="button"
        id="btn-add-cart"
        class="btn btn-success add-cart-action"
        data-product-id="12345"
        aria-label="Add to shopping cart">
        Add to Cart
      </button>
      <button type="button" class="btn btn-outline wishlist-btn">
        Add to Wishlist
      </button>
    </div>
  </div>
</body>
</html>`,
				PageURL: "https://example.com/product/12345",
				TestCode: `test('add product to cart', async ({ page }) => {
  await page.goto('/product/12345');
  await page.selectOption('#quantity', '2');
  await page.click('#add-to-cart-btn'); // Line 4
  await expect(page.locator('.cart-count')).toHaveText('2');
});`,
				FailedLine: 4,
			},
		},
		{
			name: "Timeout - Slow Loading Element",
			request: &healing.HealingRequest{
				TestName:     "dashboard.spec.ts",
				TestFile:     "tests/dashboard.spec.ts",
				Selector:     ".dashboard-stats",
				ErrorMessage: "Error: Timeout 5000ms exceeded.\nCall log:\n  - waiting for selector \".dashboard-stats\"\n  - selector resolved to hidden element",
				PageHTML: `<!DOCTYPE html>
<html>
<head><title>Dashboard</title></head>
<body>
  <div class="dashboard">
    <header class="dashboard-header">
      <h1>Welcome, User</h1>
    </header>
    <div class="loading-placeholder" data-loading="true">
      Loading statistics...
    </div>
    <div class="dashboard-stats" style="display: none;" data-loaded="false">
      <div class="stat-card">Revenue: $10,000</div>
      <div class="stat-card">Orders: 150</div>
    </div>
  </div>
  <script>
    // Stats load after API call completes
    setTimeout(() => {
      document.querySelector('.loading-placeholder').style.display = 'none';
      document.querySelector('.dashboard-stats').style.display = 'block';
    }, 3000);
  </script>
</body>
</html>`,
				PageURL: "https://example.com/dashboard",
				TestCode: `test('dashboard shows statistics', async ({ page }) => {
  await page.goto('/dashboard');
  await page.waitForSelector('.dashboard-stats'); // Line 3
  await expect(page.locator('.stat-card').first()).toBeVisible();
});`,
				FailedLine: 3,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for i, scenario := range scenarios {
		fmt.Printf("\n📋 Scenario %d: %s\n", i+1, scenario.name)
		fmt.Println(string(make([]byte, 50)))

		// Detect failure type
		scenario.request.FailureType = healing.DetectFailureType(scenario.request.ErrorMessage)

		fmt.Printf("   Failed Selector: %s\n", scenario.request.Selector)
		fmt.Printf("   Failure Type: %s\n", scenario.request.FailureType)
		fmt.Println()

		fmt.Println("   ⏳ Calling Claude for analysis...")

		startTime := time.Now()
		result, err := service.Heal(ctx, scenario.request)
		elapsed := time.Since(startTime)

		if err != nil {
			fmt.Printf("   ❌ Error: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Printf("   📊 Result:\n")
		fmt.Printf("      Status: %s\n", result.Status)
		fmt.Printf("      Strategy: %s\n", result.Strategy)
		fmt.Printf("      Confidence: %.1f%%\n", result.Confidence*100)
		fmt.Printf("      Time: %v\n", elapsed)

		if result.HealedSelector != "" {
			fmt.Println()
			fmt.Printf("   ✅ Healed Selector:\n")
			fmt.Printf("      Before: %s\n", scenario.request.Selector)
			fmt.Printf("      After:  %s\n", result.HealedSelector)
		}

		fmt.Println()
		fmt.Printf("   📝 Explanation:\n")
		fmt.Printf("      %s\n", truncateString(result.Explanation, 200))

		if len(result.Suggestions) > 0 {
			fmt.Println()
			fmt.Printf("   💡 Alternatives:\n")
			for j, s := range result.Suggestions {
				if j >= 3 {
					fmt.Printf("      ... and %d more\n", len(result.Suggestions)-3)
					break
				}
				fmt.Printf("      %d. [%.0f%%] %s\n", j+1, s.Confidence*100, s.Description)
				if s.Selector != "" {
					fmt.Printf("         %s\n", s.Selector)
				}
			}
		}

		// Check for metadata
		if result.Metadata != nil {
			if changeType, ok := result.Metadata["change_type"]; ok {
				fmt.Printf("\n   🔍 Root Cause: %v\n", changeType)
			}
		}
	}

	fmt.Println()
	fmt.Println("=" + string(make([]byte, 50)))
	fmt.Println("✨ Demo Complete!")
	fmt.Println()
	fmt.Println("Usage for custom healing:")
	fmt.Println("  go run cmd/heal-test/main.go \\")
	fmt.Println("    --selector 'button.old-class' \\")
	fmt.Println("    --error 'selector not found' \\")
	fmt.Println("    --html page.html \\")
	fmt.Println("    --url 'https://example.com' \\")
	fmt.Println("    --code 'await page.click(\"button.old-class\");'")
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// For JSON output
type jsonResult struct {
	Status     string        `json:"status"`
	Strategy   string        `json:"strategy"`
	Confidence float64       `json:"confidence"`
	Duration   time.Duration `json:"duration"`
	Healed     struct {
		Selector string `json:"selector,omitempty"`
		Code     string `json:"code,omitempty"`
	} `json:"healed"`
	Explanation string `json:"explanation"`
	Suggestions []struct {
		Type       string  `json:"type"`
		Selector   string  `json:"selector,omitempty"`
		Code       string  `json:"code,omitempty"`
		Confidence float64 `json:"confidence"`
	} `json:"suggestions,omitempty"`
}

func outputJSON(result *healing.HealingResult) {
	jr := jsonResult{
		Status:      string(result.Status),
		Strategy:    string(result.Strategy),
		Confidence:  result.Confidence,
		Duration:    result.Duration,
		Explanation: result.Explanation,
	}
	jr.Healed.Selector = result.HealedSelector
	jr.Healed.Code = result.HealedCode

	for _, s := range result.Suggestions {
		jr.Suggestions = append(jr.Suggestions, struct {
			Type       string  `json:"type"`
			Selector   string  `json:"selector,omitempty"`
			Code       string  `json:"code,omitempty"`
			Confidence float64 `json:"confidence"`
		}{
			Type:       string(s.Strategy),
			Selector:   s.Selector,
			Code:       s.Code,
			Confidence: s.Confidence,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(jr)
}
