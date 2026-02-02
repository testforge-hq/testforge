package discovery

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/testforge/testforge/internal/crypto"
	"github.com/testforge/testforge/internal/domain"
)

// Authenticator handles various authentication methods for crawling
type Authenticator struct {
	config        *domain.AuthConfig
	encryptionKey []byte
}

// NewAuthenticator creates a new authenticator with the given config
func NewAuthenticator(config *domain.AuthConfig) *Authenticator {
	return &Authenticator{
		config:        config,
		encryptionKey: crypto.DefaultKey(),
	}
}

// WithEncryptionKey sets a custom encryption key
func (a *Authenticator) WithEncryptionKey(key []byte) *Authenticator {
	a.encryptionKey = key
	return a
}

// Authenticate performs authentication based on the configured type
func (a *Authenticator) Authenticate(ctx context.Context, browserCtx playwright.BrowserContext, page playwright.Page) error {
	if a.config == nil || a.config.Type == domain.AuthTypeNone {
		return nil
	}

	switch a.config.Type {
	case domain.AuthTypeCredentials:
		return a.performLogin(ctx, page)
	case domain.AuthTypeCookie:
		return a.injectCookies(ctx, browserCtx)
	case domain.AuthTypeToken:
		return a.injectHeaders(ctx, browserCtx)
	case domain.AuthTypeBasic:
		return a.setupBasicAuth(ctx, browserCtx)
	default:
		return fmt.Errorf("unsupported auth type: %s", a.config.Type)
	}
}

// performLogin navigates to login page, fills credentials, and submits
func (a *Authenticator) performLogin(ctx context.Context, page playwright.Page) error {
	creds := a.config.Credentials
	if creds == nil {
		return fmt.Errorf("credentials config is required for credentials auth type")
	}

	// Decrypt credentials
	username, err := crypto.DecryptIfNotEmpty(creds.Username, a.encryptionKey)
	if err != nil {
		// If decryption fails, assume plaintext (for development)
		username = creds.Username
	}
	password, err := crypto.DecryptIfNotEmpty(creds.Password, a.encryptionKey)
	if err != nil {
		password = creds.Password
	}

	// Navigate to login page
	if _, err := page.Goto(creds.LoginURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return fmt.Errorf("navigating to login page: %w", err)
	}

	// Wait for the form to be ready
	if _, err := page.WaitForSelector(creds.UsernameSelector, playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000),
	}); err != nil {
		return fmt.Errorf("waiting for username field: %w", err)
	}

	// Fill username
	if err := page.Locator(creds.UsernameSelector).Fill(username); err != nil {
		return fmt.Errorf("filling username: %w", err)
	}

	// Fill password
	if err := page.Locator(creds.PasswordSelector).Fill(password); err != nil {
		return fmt.Errorf("filling password: %w", err)
	}

	// Click submit and wait for navigation
	submitLocator := page.Locator(creds.SubmitSelector)

	// Use Promise.all pattern to click and wait for navigation
	if err := submitLocator.Click(); err != nil {
		return fmt.Errorf("clicking submit: %w", err)
	}

	// Wait for navigation to complete
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateDomcontentloaded,
		Timeout: playwright.Float(10000),
	})

	// Wait after login
	waitTime := creds.WaitAfterLogin
	if waitTime <= 0 {
		waitTime = 3000
	}
	time.Sleep(time.Duration(waitTime) * time.Millisecond)

	// Verify login success if indicator is provided
	if creds.SuccessIndicator != "" {
		if err := a.verifyLoginSuccess(page, creds.SuccessIndicator); err != nil {
			return fmt.Errorf("login verification failed: %w", err)
		}
	}

	return nil
}

// verifyLoginSuccess checks if login was successful
func (a *Authenticator) verifyLoginSuccess(page playwright.Page, indicator string) error {
	// Check if indicator is a URL pattern
	if strings.HasPrefix(indicator, "http") || strings.HasPrefix(indicator, "/") {
		currentURL := page.URL()
		if strings.Contains(currentURL, indicator) {
			return nil
		}
		// Wait a bit and check again
		time.Sleep(1 * time.Second)
		currentURL = page.URL()
		if strings.Contains(currentURL, indicator) {
			return nil
		}
		return fmt.Errorf("expected URL to contain %s, got %s", indicator, currentURL)
	}

	// Assume it's a selector
	_, err := page.WaitForSelector(indicator, playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	})
	if err != nil {
		return fmt.Errorf("success indicator not found: %s", indicator)
	}

	return nil
}

// injectCookies sets cookies on the browser context
func (a *Authenticator) injectCookies(ctx context.Context, browserCtx playwright.BrowserContext) error {
	if len(a.config.Cookies) == 0 {
		return fmt.Errorf("no cookies configured")
	}

	var cookies []playwright.OptionalCookie
	for _, c := range a.config.Cookies {
		// Decrypt cookie value
		value, err := crypto.DecryptIfNotEmpty(c.Value, a.encryptionKey)
		if err != nil {
			value = c.Value // Fallback to plaintext
		}

		cookie := playwright.OptionalCookie{
			Name:     c.Name,
			Value:    value,
			Domain:   playwright.String(c.Domain),
			Path:     playwright.String(c.Path),
			Secure:   playwright.Bool(c.Secure),
			HttpOnly: playwright.Bool(c.HttpOnly),
		}

		if c.SameSite != "" {
			switch strings.ToLower(c.SameSite) {
			case "strict":
				cookie.SameSite = playwright.SameSiteAttributeStrict
			case "lax":
				cookie.SameSite = playwright.SameSiteAttributeLax
			case "none":
				cookie.SameSite = playwright.SameSiteAttributeNone
			}
		}

		cookies = append(cookies, cookie)
	}

	if err := browserCtx.AddCookies(cookies); err != nil {
		return fmt.Errorf("adding cookies: %w", err)
	}

	return nil
}

// injectHeaders sets up route interception to add custom headers
func (a *Authenticator) injectHeaders(ctx context.Context, browserCtx playwright.BrowserContext) error {
	if len(a.config.Headers) == 0 {
		return fmt.Errorf("no headers configured")
	}

	// Build headers map with decrypted values
	headers := make(map[string]string)
	for _, h := range a.config.Headers {
		value, err := crypto.DecryptIfNotEmpty(h.Value, a.encryptionKey)
		if err != nil {
			value = h.Value // Fallback to plaintext
		}
		headers[h.Name] = value
	}

	// Set extra HTTP headers on the context
	if err := browserCtx.SetExtraHTTPHeaders(headers); err != nil {
		return fmt.Errorf("setting extra headers: %w", err)
	}

	return nil
}

// setupBasicAuth configures HTTP basic authentication via Authorization header
func (a *Authenticator) setupBasicAuth(ctx context.Context, browserCtx playwright.BrowserContext) error {
	if a.config.BasicAuth == nil {
		return fmt.Errorf("basic auth config is required")
	}

	// Decrypt credentials
	username, err := crypto.DecryptIfNotEmpty(a.config.BasicAuth.Username, a.encryptionKey)
	if err != nil {
		username = a.config.BasicAuth.Username
	}
	password, err := crypto.DecryptIfNotEmpty(a.config.BasicAuth.Password, a.encryptionKey)
	if err != nil {
		password = a.config.BasicAuth.Password
	}

	// Encode credentials for Basic Auth header
	credentials := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	authHeader := "Basic " + credentials

	// Set Authorization header on all requests
	if err := browserCtx.SetExtraHTTPHeaders(map[string]string{
		"Authorization": authHeader,
	}); err != nil {
		return fmt.Errorf("setting basic auth header: %w", err)
	}

	return nil
}

// AuthResult contains the result of authentication
type AuthResult struct {
	Success bool
	Error   string
	Cookies []playwright.Cookie
}

// GetAuthCookies returns cookies after authentication (useful for debugging)
func (a *Authenticator) GetAuthCookies(browserCtx playwright.BrowserContext) ([]playwright.Cookie, error) {
	return browserCtx.Cookies()
}
