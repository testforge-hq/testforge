// Package auth provides authentication functionality including OIDC/SSO
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidProvider is returned when the OIDC provider is not supported
	ErrInvalidProvider = errors.New("invalid or unsupported OIDC provider")

	// ErrInvalidState is returned when the state parameter doesn't match
	ErrInvalidState = errors.New("invalid state parameter")

	// ErrInvalidToken is returned when the token exchange fails
	ErrInvalidToken = errors.New("invalid or expired token")

	// ErrUserInfoFailed is returned when fetching user info fails
	ErrUserInfoFailed = errors.New("failed to fetch user info")
)

// OIDCProvider represents a supported OIDC provider
type OIDCProvider string

const (
	ProviderGoogle  OIDCProvider = "google"
	ProviderOkta    OIDCProvider = "okta"
	ProviderAzureAD OIDCProvider = "azure_ad"
	ProviderGitHub  OIDCProvider = "github"
)

// OIDCConfig holds configuration for an OIDC provider
type OIDCConfig struct {
	Provider     OIDCProvider
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string

	// Provider-specific URLs (auto-configured for well-known providers)
	AuthURL     string
	TokenURL    string
	UserInfoURL string

	// Okta-specific
	OktaDomain string

	// Azure AD-specific
	AzureTenantID string
}

// OIDCClient handles OIDC authentication flows
type OIDCClient struct {
	config OIDCConfig
	http   *http.Client
}

// OIDCUserInfo represents user information from an OIDC provider
type OIDCUserInfo struct {
	ID            string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}

// OIDCTokenResponse represents the token response from an OIDC provider
type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// NewOIDCClient creates a new OIDC client for the given provider
func NewOIDCClient(config OIDCConfig) (*OIDCClient, error) {
	if config.ClientID == "" || config.ClientSecret == "" {
		return nil, fmt.Errorf("client ID and secret are required")
	}

	// Configure provider-specific URLs
	switch config.Provider {
	case ProviderGoogle:
		config.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
		config.TokenURL = "https://oauth2.googleapis.com/token"
		config.UserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
		if len(config.Scopes) == 0 {
			config.Scopes = []string{"openid", "email", "profile"}
		}

	case ProviderOkta:
		if config.OktaDomain == "" {
			return nil, fmt.Errorf("Okta domain is required")
		}
		baseURL := fmt.Sprintf("https://%s", config.OktaDomain)
		config.AuthURL = baseURL + "/oauth2/default/v1/authorize"
		config.TokenURL = baseURL + "/oauth2/default/v1/token"
		config.UserInfoURL = baseURL + "/oauth2/default/v1/userinfo"
		if len(config.Scopes) == 0 {
			config.Scopes = []string{"openid", "email", "profile"}
		}

	case ProviderAzureAD:
		if config.AzureTenantID == "" {
			config.AzureTenantID = "common" // Multi-tenant
		}
		baseURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0", config.AzureTenantID)
		config.AuthURL = baseURL + "/authorize"
		config.TokenURL = baseURL + "/token"
		config.UserInfoURL = "https://graph.microsoft.com/oidc/userinfo"
		if len(config.Scopes) == 0 {
			config.Scopes = []string{"openid", "email", "profile"}
		}

	case ProviderGitHub:
		config.AuthURL = "https://github.com/login/oauth/authorize"
		config.TokenURL = "https://github.com/login/oauth/access_token"
		config.UserInfoURL = "https://api.github.com/user"
		if len(config.Scopes) == 0 {
			config.Scopes = []string{"user:email"}
		}

	default:
		// Custom provider - URLs must be provided
		if config.AuthURL == "" || config.TokenURL == "" || config.UserInfoURL == "" {
			return nil, ErrInvalidProvider
		}
	}

	return &OIDCClient{
		config: config,
		http:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// GenerateState generates a random state parameter for CSRF protection
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GetAuthURL returns the URL to redirect the user to for authentication
func (c *OIDCClient) GetAuthURL(state string, additionalParams map[string]string) string {
	params := url.Values{
		"client_id":     {c.config.ClientID},
		"redirect_uri":  {c.config.RedirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(c.config.Scopes, " ")},
		"state":         {state},
	}

	// Add additional params (e.g., nonce, prompt)
	for k, v := range additionalParams {
		params.Set(k, v)
	}

	return c.config.AuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens
func (c *OIDCClient) ExchangeCode(ctx context.Context, code string) (*OIDCTokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.config.RedirectURL},
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrInvalidToken, string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		// GitHub returns form-encoded response
		if c.config.Provider == ProviderGitHub {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return nil, fmt.Errorf("parsing response: %w", err)
			}
			tokenResp.AccessToken = values.Get("access_token")
			tokenResp.TokenType = values.Get("token_type")
		} else {
			return nil, fmt.Errorf("parsing token response: %w", err)
		}
	}

	return &tokenResp, nil
}

// GetUserInfo fetches user information using the access token
func (c *OIDCClient) GetUserInfo(ctx context.Context, accessToken string) (*OIDCUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.UserInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s", ErrUserInfoFailed, string(body))
	}

	var userInfo OIDCUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("parsing userinfo response: %w", err)
	}

	// GitHub-specific handling
	if c.config.Provider == ProviderGitHub {
		if userInfo.Email == "" {
			// Fetch email separately for GitHub
			email, err := c.fetchGitHubEmail(ctx, accessToken)
			if err == nil {
				userInfo.Email = email
			}
		}
		// GitHub uses 'id' instead of 'sub'
		var rawUser map[string]interface{}
		json.Unmarshal(body, &rawUser)
		if id, ok := rawUser["id"].(float64); ok {
			userInfo.ID = fmt.Sprintf("%d", int64(id))
		}
		if name, ok := rawUser["name"].(string); ok {
			userInfo.Name = name
		}
		if avatar, ok := rawUser["avatar_url"].(string); ok {
			userInfo.Picture = avatar
		}
	}

	return &userInfo, nil
}

// fetchGitHubEmail fetches the primary email for GitHub users
func (c *OIDCClient) fetchGitHubEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	return "", errors.New("no primary verified email found")
}

// Provider returns the OIDC provider type
func (c *OIDCClient) Provider() OIDCProvider {
	return c.config.Provider
}

// OIDCManager manages multiple OIDC providers
type OIDCManager struct {
	providers map[OIDCProvider]*OIDCClient
}

// NewOIDCManager creates a new OIDC manager
func NewOIDCManager() *OIDCManager {
	return &OIDCManager{
		providers: make(map[OIDCProvider]*OIDCClient),
	}
}

// RegisterProvider registers an OIDC provider
func (m *OIDCManager) RegisterProvider(config OIDCConfig) error {
	client, err := NewOIDCClient(config)
	if err != nil {
		return err
	}
	m.providers[config.Provider] = client
	return nil
}

// GetProvider returns the OIDC client for a provider
func (m *OIDCManager) GetProvider(provider OIDCProvider) (*OIDCClient, error) {
	client, ok := m.providers[provider]
	if !ok {
		return nil, ErrInvalidProvider
	}
	return client, nil
}

// ListProviders returns the list of registered providers
func (m *OIDCManager) ListProviders() []OIDCProvider {
	providers := make([]OIDCProvider, 0, len(m.providers))
	for p := range m.providers {
		providers = append(providers, p)
	}
	return providers
}

// OIDCState represents the state stored during OIDC flow
type OIDCState struct {
	ID          string       `json:"id"`
	Provider    OIDCProvider `json:"provider"`
	RedirectURL string       `json:"redirect_url"`
	TenantID    *uuid.UUID   `json:"tenant_id,omitempty"`
	InviteCode  string       `json:"invite_code,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
}

// IsExpired returns true if the state has expired
func (s *OIDCState) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}
