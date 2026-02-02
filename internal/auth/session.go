package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	// ErrInvalidSession is returned when the session is invalid
	ErrInvalidSession = errors.New("invalid session")

	// ErrSessionExpired is returned when the session has expired
	ErrSessionExpired = errors.New("session expired")

	// ErrInvalidSignature is returned when the JWT signature is invalid
	ErrInvalidSignature = errors.New("invalid signature")
)

// SessionConfig holds session configuration
type SessionConfig struct {
	Secret          []byte        // Secret for signing tokens
	AccessTokenTTL  time.Duration // Access token lifetime
	RefreshTokenTTL time.Duration // Refresh token lifetime
	Issuer          string        // Token issuer
}

// DefaultSessionConfig returns default session configuration
func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		Issuer:          "testforge",
	}
}

// Session represents a user session
type Session struct {
	ID           string     `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	TenantID     *uuid.UUID `json:"tenant_id,omitempty"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name,omitempty"`
	Permissions  []string   `json:"permissions,omitempty"`
	AuthProvider string     `json:"auth_provider"`
	IPAddress    string     `json:"ip_address,omitempty"`
	UserAgent    string     `json:"user_agent,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	LastActiveAt time.Time  `json:"last_active_at"`
}

// IsExpired returns true if the session has expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// TokenPair represents an access and refresh token pair
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"` // seconds
	ExpiresAt    time.Time `json:"expires_at"`
}

// JWTClaims represents the claims in a JWT
type JWTClaims struct {
	// Standard claims
	Subject   string `json:"sub"`           // User ID
	Issuer    string `json:"iss"`           // Token issuer
	Audience  string `json:"aud,omitempty"` // Token audience
	ExpiresAt int64  `json:"exp"`           // Expiration time (Unix)
	IssuedAt  int64  `json:"iat"`           // Issued at (Unix)
	NotBefore int64  `json:"nbf,omitempty"` // Not before (Unix)
	JWTID     string `json:"jti"`           // Unique token ID

	// Custom claims
	Email       string     `json:"email,omitempty"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	Permissions []string   `json:"permissions,omitempty"`
	SessionID   string     `json:"session_id,omitempty"`
	TokenType   string     `json:"token_type,omitempty"` // "access" or "refresh"
}

// SessionManager handles session creation and validation
type SessionManager struct {
	config SessionConfig
	redis  *redis.Client
}

// NewSessionManager creates a new session manager
func NewSessionManager(config SessionConfig, redisClient *redis.Client) (*SessionManager, error) {
	if len(config.Secret) == 0 {
		return nil, errors.New("session secret is required")
	}
	if config.AccessTokenTTL == 0 {
		config.AccessTokenTTL = DefaultSessionConfig().AccessTokenTTL
	}
	if config.RefreshTokenTTL == 0 {
		config.RefreshTokenTTL = DefaultSessionConfig().RefreshTokenTTL
	}
	if config.Issuer == "" {
		config.Issuer = "testforge"
	}

	return &SessionManager{
		config: config,
		redis:  redisClient,
	}, nil
}

// CreateSession creates a new session and returns tokens
func (m *SessionManager) CreateSession(ctx context.Context, userID uuid.UUID, email string, opts ...SessionOption) (*TokenPair, error) {
	session := &Session{
		ID:           generateSessionID(),
		UserID:       userID,
		Email:        email,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(m.config.RefreshTokenTTL),
		LastActiveAt: time.Now(),
	}

	// Apply options
	for _, opt := range opts {
		opt(session)
	}

	// Store session in Redis
	if err := m.storeSession(ctx, session); err != nil {
		return nil, fmt.Errorf("storing session: %w", err)
	}

	// Generate token pair
	return m.generateTokenPair(session)
}

// SessionOption is a functional option for session creation
type SessionOption func(*Session)

// WithTenant sets the tenant ID for the session
func WithTenant(tenantID uuid.UUID) SessionOption {
	return func(s *Session) {
		s.TenantID = &tenantID
	}
}

// WithPermissions sets the permissions for the session
func WithPermissions(perms []string) SessionOption {
	return func(s *Session) {
		s.Permissions = perms
	}
}

// WithAuthProvider sets the auth provider for the session
func WithAuthProvider(provider string) SessionOption {
	return func(s *Session) {
		s.AuthProvider = provider
	}
}

// WithClientInfo sets client info for the session
func WithClientInfo(ip, userAgent string) SessionOption {
	return func(s *Session) {
		s.IPAddress = ip
		s.UserAgent = userAgent
	}
}

// ValidateAccessToken validates an access token and returns the claims
func (m *SessionManager) ValidateAccessToken(ctx context.Context, token string) (*JWTClaims, error) {
	claims, err := m.parseToken(token)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != "access" {
		return nil, ErrInvalidSession
	}

	// Check expiration
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrSessionExpired
	}

	// Verify session still exists
	if claims.SessionID != "" {
		exists, err := m.sessionExists(ctx, claims.SessionID)
		if err != nil || !exists {
			return nil, ErrInvalidSession
		}
	}

	return claims, nil
}

// RefreshTokens refreshes the token pair using a refresh token
func (m *SessionManager) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := m.parseToken(refreshToken)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != "refresh" {
		return nil, ErrInvalidSession
	}

	// Check expiration
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrSessionExpired
	}

	// Get session from Redis
	session, err := m.getSession(ctx, claims.SessionID)
	if err != nil {
		return nil, ErrInvalidSession
	}

	// Update last active time
	session.LastActiveAt = time.Now()
	if err := m.storeSession(ctx, session); err != nil {
		return nil, fmt.Errorf("updating session: %w", err)
	}

	// Generate new token pair
	return m.generateTokenPair(session)
}

// RevokeSession revokes a session
func (m *SessionManager) RevokeSession(ctx context.Context, sessionID string) error {
	key := fmt.Sprintf("session:%s", sessionID)
	return m.redis.Del(ctx, key).Err()
}

// RevokeAllUserSessions revokes all sessions for a user
func (m *SessionManager) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	// This requires tracking sessions by user, which would need a secondary index
	// For now, sessions will expire naturally
	// A full implementation would use Redis SCAN or a user->sessions mapping
	return nil
}

// GetSession retrieves a session by ID
func (m *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	return m.getSession(ctx, sessionID)
}

// generateTokenPair creates a new access and refresh token pair
func (m *SessionManager) generateTokenPair(session *Session) (*TokenPair, error) {
	now := time.Now()
	accessExpires := now.Add(m.config.AccessTokenTTL)
	refreshExpires := now.Add(m.config.RefreshTokenTTL)

	// Access token claims
	accessClaims := &JWTClaims{
		Subject:     session.UserID.String(),
		Issuer:      m.config.Issuer,
		ExpiresAt:   accessExpires.Unix(),
		IssuedAt:    now.Unix(),
		JWTID:       generateTokenID(),
		Email:       session.Email,
		TenantID:    session.TenantID,
		Permissions: session.Permissions,
		SessionID:   session.ID,
		TokenType:   "access",
	}

	accessToken, err := m.signToken(accessClaims)
	if err != nil {
		return nil, fmt.Errorf("signing access token: %w", err)
	}

	// Refresh token claims (minimal)
	refreshClaims := &JWTClaims{
		Subject:   session.UserID.String(),
		Issuer:    m.config.Issuer,
		ExpiresAt: refreshExpires.Unix(),
		IssuedAt:  now.Unix(),
		JWTID:     generateTokenID(),
		SessionID: session.ID,
		TokenType: "refresh",
	}

	refreshToken, err := m.signToken(refreshClaims)
	if err != nil {
		return nil, fmt.Errorf("signing refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(m.config.AccessTokenTTL.Seconds()),
		ExpiresAt:    accessExpires,
	}, nil
}

// signToken creates a signed JWT token (simplified - use proper JWT library in production)
func (m *SessionManager) signToken(claims *JWTClaims) (string, error) {
	// Header
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	// Payload
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Signature
	message := header + "." + payload
	mac := hmac.New(sha256.New, m.config.Secret)
	mac.Write([]byte(message))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return message + "." + signature, nil
}

// parseToken parses and validates a JWT token
func (m *SessionManager) parseToken(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidSession
	}

	// Verify signature
	message := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, m.config.Secret)
	mac.Write([]byte(message))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, ErrInvalidSignature
	}

	// Decode payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidSession
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrInvalidSession
	}

	return &claims, nil
}

// storeSession stores a session in Redis
func (m *SessionManager) storeSession(ctx context.Context, session *Session) error {
	key := fmt.Sprintf("session:%s", session.ID)
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return m.redis.Set(ctx, key, data, time.Until(session.ExpiresAt)).Err()
}

// getSession retrieves a session from Redis
func (m *SessionManager) getSession(ctx context.Context, sessionID string) (*Session, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	data, err := m.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, ErrInvalidSession
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, ErrInvalidSession
	}

	return &session, nil
}

// sessionExists checks if a session exists
func (m *SessionManager) sessionExists(ctx context.Context, sessionID string) (bool, error) {
	key := fmt.Sprintf("session:%s", sessionID)
	exists, err := m.redis.Exists(ctx, key).Result()
	return exists > 0, err
}

// generateSessionID generates a random session ID
func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// generateTokenID generates a random token ID
func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
