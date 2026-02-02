package discovery

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/testforge/testforge/internal/domain"
)

// Crawler performs BFS crawling of a website
type Crawler struct {
	pw         *playwright.Playwright
	browser    playwright.Browser
	config     DiscoveryConfig
	baseURL    string
	baseDomain string
	extractor  *Extractor
	storage    StorageClient

	// State
	visited     map[string]bool
	visitedMu   sync.RWMutex
	results     map[string]*PageModel
	resultsMu   sync.RWMutex
	queue       chan CrawlTask
	queueClosed bool
	queueMu     sync.RWMutex
	startTime   time.Time

	// Authentication
	authCookies []playwright.Cookie

	// Callbacks
	onHeartbeat func(string)
	onProgress  func(int, int)
}

// StorageClient interface for uploading screenshots
type StorageClient interface {
	UploadScreenshot(ctx context.Context, bucket, key string, data []byte) (string, error)
}

// NewCrawler creates a new crawler instance
func NewCrawler(config DiscoveryConfig, storage StorageClient) (*Crawler, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("starting playwright: %w", err)
	}

	browserOpts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(config.Headless),
	}

	browser, err := pw.Chromium.Launch(browserOpts)
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("launching browser: %w", err)
	}

	return &Crawler{
		pw:        pw,
		browser:   browser,
		config:    config,
		extractor: NewExtractor(),
		storage:   storage,
		visited:   make(map[string]bool),
		results:   make(map[string]*PageModel),
		queue:     make(chan CrawlTask, 1000),
	}, nil
}

// SetHeartbeatCallback sets a callback for progress heartbeats
func (c *Crawler) SetHeartbeatCallback(fn func(string)) {
	c.onHeartbeat = fn
}

// SetProgressCallback sets a callback for progress updates
func (c *Crawler) SetProgressCallback(fn func(int, int)) {
	c.onProgress = fn
}

// Close cleans up crawler resources
func (c *Crawler) Close() error {
	if c.browser != nil {
		c.browser.Close()
	}
	if c.pw != nil {
		return c.pw.Stop()
	}
	return nil
}

// authenticate performs authentication before crawling
func (c *Crawler) authenticate(ctx context.Context, authConfig *domain.AuthConfig) error {
	// Create a browser context for authentication
	browserCtx, err := c.browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
	})
	if err != nil {
		return fmt.Errorf("creating browser context for auth: %w", err)
	}

	page, err := browserCtx.NewPage()
	if err != nil {
		browserCtx.Close()
		return fmt.Errorf("creating page for auth: %w", err)
	}

	// Perform authentication
	auth := NewAuthenticator(authConfig)
	if err := auth.Authenticate(ctx, browserCtx, page); err != nil {
		page.Close()
		browserCtx.Close()
		return err
	}

	// Store the authenticated context for later use
	// Note: In a real implementation, we'd need to share cookies/state
	// For now, we'll close this and let workers handle auth per-context

	// Get cookies from authenticated session
	cookies, err := browserCtx.Cookies()
	if err == nil && len(cookies) > 0 {
		// Store cookies for worker contexts
		c.authCookies = cookies
	}

	page.Close()
	browserCtx.Close()

	return nil
}

// Crawl performs BFS crawling starting from the given URL
func (c *Crawler) Crawl(ctx context.Context, startURL string) (*AppModel, error) {
	return c.CrawlWithAuth(ctx, startURL, nil)
}

// CrawlWithAuth performs BFS crawling with optional authentication
func (c *Crawler) CrawlWithAuth(ctx context.Context, startURL string, authConfig *domain.AuthConfig) (*AppModel, error) {
	c.startTime = time.Now()

	// Parse and validate start URL
	parsedURL, err := url.Parse(startURL)
	if err != nil {
		return nil, fmt.Errorf("parsing start URL: %w", err)
	}

	c.baseURL = startURL
	c.baseDomain = parsedURL.Host

	// Perform authentication if configured
	if authConfig != nil && authConfig.IsAuthenticated() {
		if err := c.authenticate(ctx, authConfig); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Normalize the start URL
	normalizedStart := c.normalizeURL(startURL)

	// Initialize BFS
	c.queue <- CrawlTask{
		URL:   normalizedStart,
		Depth: 0,
	}

	// Create worker pool
	var wg sync.WaitGroup
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start workers
	for i := 0; i < c.config.Concurrency; i++ {
		wg.Add(1)
		go c.worker(workerCtx, &wg)
	}

	// Process queue until done or limits reached
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		closeQueue := func() {
			c.queueMu.Lock()
			if !c.queueClosed {
				c.queueClosed = true
				close(c.queue)
			}
			c.queueMu.Unlock()
		}

		// Give initial crawl time to populate queue
		time.Sleep(2 * time.Second)

		emptyCount := 0

		for {
			select {
			case <-ctx.Done():
				closeQueue()
				return
			case <-ticker.C:
				// Check duration limit
				if time.Since(c.startTime) > c.config.MaxDuration {
					closeQueue()
					return
				}

				// Check page limit
				c.resultsMu.RLock()
				pageCount := len(c.results)
				c.resultsMu.RUnlock()

				if pageCount >= c.config.MaxPages {
					closeQueue()
					return
				}

				// Check if queue is empty - require multiple consecutive empty checks
				queueLen := len(c.queue)
				if queueLen == 0 && pageCount > 0 {
					emptyCount++
					// Require 3 consecutive empty checks (1.5 seconds of empty queue)
					if emptyCount >= 3 {
						closeQueue()
						return
					}
				} else {
					emptyCount = 0
				}
			}
		}
	}()

	// Wait for workers to complete
	wg.Wait()

	// Build AppModel from results
	return c.buildAppModel(), nil
}

// worker processes crawl tasks from the queue
func (c *Crawler) worker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// Panic recovery to prevent worker crashes from bringing down the crawler
	defer func() {
		if r := recover(); r != nil {
			// Log panic but don't crash - other workers can continue
			if c.onHeartbeat != nil {
				c.onHeartbeat(fmt.Sprintf("Worker recovered from panic: %v", r))
			}
		}
	}()

	// Create browser context for this worker
	browserCtx, err := c.browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 TestForge/1.0"),
	})
	if err != nil {
		return
	}
	defer browserCtx.Close()

	// Inject authentication cookies if available
	if len(c.authCookies) > 0 {
		var optionalCookies []playwright.OptionalCookie
		for _, cookie := range c.authCookies {
			optionalCookies = append(optionalCookies, playwright.OptionalCookie{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   playwright.String(cookie.Domain),
				Path:     playwright.String(cookie.Path),
				Secure:   playwright.Bool(cookie.Secure),
				HttpOnly: playwright.Bool(cookie.HttpOnly),
			})
		}
		browserCtx.AddCookies(optionalCookies)
	}

	page, err := browserCtx.NewPage()
	if err != nil {
		return
	}
	defer page.Close()

	for task := range c.queue {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check if already visited
		c.visitedMu.Lock()
		if c.visited[task.URL] {
			c.visitedMu.Unlock()
			continue
		}
		c.visited[task.URL] = true
		c.visitedMu.Unlock()

		// Check limits before processing
		c.resultsMu.RLock()
		if len(c.results) >= c.config.MaxPages {
			c.resultsMu.RUnlock()
			continue
		}
		c.resultsMu.RUnlock()

		if time.Since(c.startTime) > c.config.MaxDuration {
			continue
		}

		// Crawl the page
		result := c.crawlPage(ctx, page, task)

		if result.Error != nil {
			continue
		}

		// Store result
		c.resultsMu.Lock()
		c.results[task.URL] = result.Page
		c.resultsMu.Unlock()

		// Report progress
		if c.onHeartbeat != nil {
			c.onHeartbeat(fmt.Sprintf("Crawled: %s (depth: %d)", task.URL, task.Depth))
		}
		if c.onProgress != nil {
			c.resultsMu.RLock()
			c.onProgress(len(c.results), c.config.MaxPages)
			c.resultsMu.RUnlock()
		}

		// Add new links to queue if within depth limit
		if task.Depth < c.config.MaxDepth {
			for _, link := range result.NewLinks {
				c.visitedMu.RLock()
				visited := c.visited[link]
				c.visitedMu.RUnlock()

				if !visited {
					c.queueMu.RLock()
					closed := c.queueClosed
					c.queueMu.RUnlock()

					if closed {
						break
					}

					select {
					case c.queue <- CrawlTask{
						URL:       link,
						Depth:     task.Depth + 1,
						ParentURL: task.URL,
					}:
					default:
						// Queue full, skip
					}
				}
			}
		}
	}
}

// crawlPage crawls a single page and extracts its elements
func (c *Crawler) crawlPage(ctx context.Context, page playwright.Page, task CrawlTask) CrawlResult {
	result := CrawlResult{}
	startTime := time.Now()

	// Navigate to the page - use networkidle to wait for JS frameworks to render
	resp, err := page.Goto(task.URL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle, // Wait for network to settle (JS loaded)
		Timeout:   playwright.Float(float64(c.config.Timeout.Milliseconds())),
	})
	if err != nil {
		result.Error = fmt.Errorf("navigating to %s: %w", task.URL, err)
		return result
	}

	// Check response status
	if resp != nil && resp.Status() >= 400 {
		result.Error = fmt.Errorf("page returned status %d", resp.Status())
		return result
	}

	// Wait for page to fully settle - important for SPAs (React, Vue, Angular)
	// First wait for network idle
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10000), // 10 seconds for slow SPAs
	}); err != nil {
		// Log but don't fail - page may still be usable
		if c.onHeartbeat != nil {
			c.onHeartbeat(fmt.Sprintf("Warning: wait for networkidle failed for %s: %v", task.URL, err))
		}
	}

	// Additional wait for JavaScript frameworks to finish rendering
	// This catches late-rendering components in React/Vue/Angular apps
	page.WaitForTimeout(1500) // No error return - always succeeds

	loadTime := time.Since(startTime)

	// Extract page elements
	elements, err := c.extractor.ExtractPageElements(page, c.baseURL)
	if err != nil {
		result.Error = fmt.Errorf("extracting elements: %w", err)
		return result
	}

	// Get DOM content for hashing
	domContent, err := page.Content()
	if err != nil {
		result.Error = fmt.Errorf("getting page content: %w", err)
		return result
	}
	domHash := HashDOM(domContent)

	// Take screenshot
	var screenshots []string
	if c.storage != nil {
		screenshotData, err := page.Screenshot(playwright.PageScreenshotOptions{
			FullPage: playwright.Bool(true),
			Type:     playwright.ScreenshotTypeJpeg,
			Quality:  playwright.Int(80),
		})
		if err == nil {
			// Generate screenshot path
			screenshotKey := fmt.Sprintf("discovery/page_%s.jpg", domHash)
			uri, err := c.storage.UploadScreenshot(ctx, "testforge", screenshotKey, screenshotData)
			if err == nil {
				screenshots = append(screenshots, uri)
			}
		}
	}

	// Build PageModel
	pageModel := &PageModel{
		URL:         task.URL,
		Title:       elements.Title,
		Description: elements.MetaDesc,
		Depth:       task.Depth,
		Forms:       elements.Forms,
		Buttons:     elements.Buttons,
		Links:       elements.Links,
		Inputs:      elements.Inputs,
		Navigation:  elements.Navigation,
		Screenshots: screenshots,
		DOMHash:     domHash,
		LoadTime:    loadTime,
		HasAuth:     elements.HasAuth,
		PageType:    elements.PageType,
		CrawledAt:   time.Now(),
	}

	result.Page = pageModel

	// Collect internal links for further crawling
	for _, link := range elements.Links {
		if link.IsInternal && link.Href != "" {
			normalizedLink := c.normalizeURL(link.Href)
			if normalizedLink != "" && c.shouldCrawl(normalizedLink) {
				result.NewLinks = append(result.NewLinks, normalizedLink)
			}
		}
	}

	return result
}

// normalizeURL normalizes a URL for consistent comparison
func (c *Crawler) normalizeURL(rawURL string) string {
	if rawURL == "" || rawURL == "#" || strings.HasPrefix(rawURL, "#") {
		return ""
	}

	// Handle relative URLs
	if strings.HasPrefix(rawURL, "/") && !strings.HasPrefix(rawURL, "//") {
		parsedBase, _ := url.Parse(c.baseURL)
		rawURL = parsedBase.Scheme + "://" + parsedBase.Host + rawURL
	}

	// Handle protocol-relative URLs
	if strings.HasPrefix(rawURL, "//") {
		parsedBase, _ := url.Parse(c.baseURL)
		rawURL = parsedBase.Scheme + ":" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	// Remove fragments and normalize
	parsed.Fragment = ""
	parsed.RawQuery = "" // Also remove query params for deduplication

	return strings.TrimSuffix(parsed.String(), "/")
}

// shouldCrawl checks if a URL should be crawled
func (c *Crawler) shouldCrawl(urlStr string) bool {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Only crawl same domain
	if parsed.Host != c.baseDomain {
		return false
	}

	// Skip certain file types
	skipExtensions := []string{".pdf", ".jpg", ".jpeg", ".png", ".gif", ".svg", ".css", ".js", ".ico", ".woff", ".woff2", ".ttf", ".eot"}
	pathLower := strings.ToLower(parsed.Path)
	for _, ext := range skipExtensions {
		if strings.HasSuffix(pathLower, ext) {
			return false
		}
	}

	// Skip common non-page paths
	skipPaths := []string{"/cdn-cgi/", "/api/", "/_next/", "/__", "/static/", "/assets/"}
	for _, skip := range skipPaths {
		if strings.Contains(pathLower, skip) {
			return false
		}
	}

	return true
}

// buildAppModel builds the final AppModel from crawl results
func (c *Crawler) buildAppModel() *AppModel {
	c.resultsMu.RLock()
	defer c.resultsMu.RUnlock()

	appModel := &AppModel{
		BaseURL:      c.baseURL,
		Pages:        c.results,
		DiscoveredAt: time.Now(),
	}

	// Build sitemap
	for url, page := range c.results {
		appModel.SiteMap = append(appModel.SiteMap, SiteMapEntry{
			URL:   url,
			Title: page.Title,
			Depth: page.Depth,
		})
	}

	// Calculate stats
	var totalForms, totalButtons, totalLinks, totalInputs, totalScreenshots, maxDepth int
	for _, page := range c.results {
		totalForms += len(page.Forms)
		totalButtons += len(page.Buttons)
		totalLinks += len(page.Links)
		totalInputs += len(page.Inputs)
		totalScreenshots += len(page.Screenshots)
		if page.Depth > maxDepth {
			maxDepth = page.Depth
		}
	}

	appModel.Stats = DiscoveryStats{
		TotalPages:       len(c.results),
		TotalForms:       totalForms,
		TotalButtons:     totalButtons,
		TotalLinks:       totalLinks,
		TotalInputs:      totalInputs,
		TotalScreenshots: totalScreenshots,
		Duration:         time.Since(c.startTime),
		MaxDepthReached:  maxDepth,
	}

	// Detect business flows (simplified)
	appModel.Flows = c.detectFlows()

	return appModel
}

// detectFlows detects potential business flows from crawled pages
func (c *Crawler) detectFlows() []BusinessFlow {
	var flows []BusinessFlow

	// Look for auth flows
	for url, page := range c.results {
		if page.HasAuth {
			flow := BusinessFlow{
				ID:          fmt.Sprintf("flow-auth-%s", page.DOMHash[:8]),
				Name:        "User Authentication",
				Description: "Login/signup flow detected",
				Steps: []FlowStep{
					{Order: 1, Action: "navigate", URL: url},
				},
				EntryPoint: url,
				Type:       "auth",
				Priority:   "critical",
				Confidence: 0.9,
			}
			flows = append(flows, flow)
			break // Only add one auth flow
		}
	}

	// Look for forms (CRUD flows)
	for url, page := range c.results {
		for _, form := range page.Forms {
			if form.FormType != "login" && form.FormType != "signup" && form.FormType != "search" {
				flow := BusinessFlow{
					ID:          fmt.Sprintf("flow-form-%s", form.ID),
					Name:        fmt.Sprintf("%s Form Submission", strings.Title(form.FormType)),
					Description: fmt.Sprintf("Form submission flow: %s", form.SubmitText),
					Steps: []FlowStep{
						{Order: 1, Action: "navigate", URL: url},
						{Order: 2, Action: "fill", Selector: form.Selectors.BestSelector()},
						{Order: 3, Action: "submit"},
					},
					EntryPoint: url,
					Type:       "crud",
					Priority:   "high",
					Confidence: 0.7,
				}
				flows = append(flows, flow)
			}
		}
	}

	// Look for navigation flows
	if len(c.results) > 1 {
		var navSteps []FlowStep
		order := 1
		for url := range c.results {
			navSteps = append(navSteps, FlowStep{Order: order, Action: "navigate", URL: url})
			order++
		}
		flow := BusinessFlow{
			ID:          "flow-navigation",
			Name:        "Site Navigation",
			Description: "Navigate through main pages",
			Steps:       navSteps,
			EntryPoint:  c.baseURL,
			Type:        "navigation",
			Priority:    "medium",
			Confidence:  0.8,
		}
		flows = append(flows, flow)
	}

	return flows
}
