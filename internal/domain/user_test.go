package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUser_IsLocked(t *testing.T) {
	tests := []struct {
		name        string
		lockedUntil *time.Time
		want        bool
	}{
		{
			name:        "not locked (nil)",
			lockedUntil: nil,
			want:        false,
		},
		{
			name:        "locked until future",
			lockedUntil: func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
			want:        true,
		},
		{
			name:        "lock expired",
			lockedUntil: func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{LockedUntil: tt.lockedUntil}
			got := u.IsLocked()
			if got != tt.want {
				t.Errorf("IsLocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUser_CanLogin(t *testing.T) {
	tests := []struct {
		name        string
		isActive    bool
		lockedUntil *time.Time
		want        bool
	}{
		{
			name:     "active and not locked",
			isActive: true,
			want:     true,
		},
		{
			name:     "inactive",
			isActive: false,
			want:     false,
		},
		{
			name:        "active but locked",
			isActive:    true,
			lockedUntil: func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
			want:        false,
		},
		{
			name:     "inactive and locked",
			isActive: false,
			lockedUntil: func() *time.Time { t := time.Now().Add(1 * time.Hour); return &t }(),
			want:     false,
		},
		{
			name:        "active with expired lock",
			isActive:    true,
			lockedUntil: func() *time.Time { t := time.Now().Add(-1 * time.Hour); return &t }(),
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{
				IsActive:    tt.isActive,
				LockedUntil: tt.lockedUntil,
			}
			got := u.CanLogin()
			if got != tt.want {
				t.Errorf("CanLogin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUser_DisplayNameOrEmail(t *testing.T) {
	displayName := "John Doe"

	tests := []struct {
		name        string
		user        *User
		want        string
	}{
		{
			name: "with display name",
			user: &User{Email: "john@example.com", DisplayName: &displayName},
			want: "John Doe",
		},
		{
			name: "without display name",
			user: &User{Email: "john@example.com"},
			want: "john@example.com",
		},
		{
			name: "with empty display name",
			user: &User{Email: "john@example.com", DisplayName: func() *string { s := ""; return &s }()},
			want: "john@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.user.DisplayNameOrEmail()
			if got != tt.want {
				t.Errorf("DisplayNameOrEmail() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUser_Fields(t *testing.T) {
	now := time.Now()
	displayName := "Test User"
	avatarURL := "https://example.com/avatar.png"
	passwordHash := "hashedpassword"
	mfaSecret := "secret"
	lastLoginIP := "192.168.1.1"
	authProviderID := "google-123"

	user := User{
		ID:                  uuid.New(),
		Email:               "test@example.com",
		EmailVerified:       true,
		EmailVerifiedAt:     &now,
		PasswordHash:        &passwordHash,
		AuthProvider:        AuthProviderGoogle,
		AuthProviderID:      &authProviderID,
		DisplayName:         &displayName,
		AvatarURL:           &avatarURL,
		MFAEnabled:          true,
		MFASecret:           &mfaSecret,
		LastLoginAt:         &now,
		LastLoginIP:         &lastLoginIP,
		FailedLoginAttempts: 2,
		IsActive:            true,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if user.Email != "test@example.com" {
		t.Errorf("Email = %v, want %v", user.Email, "test@example.com")
	}
	if !user.EmailVerified {
		t.Error("EmailVerified should be true")
	}
	if user.AuthProvider != AuthProviderGoogle {
		t.Errorf("AuthProvider = %v, want %v", user.AuthProvider, AuthProviderGoogle)
	}
	if !user.MFAEnabled {
		t.Error("MFAEnabled should be true")
	}
	if user.FailedLoginAttempts != 2 {
		t.Errorf("FailedLoginAttempts = %v, want %v", user.FailedLoginAttempts, 2)
	}
}

func TestAuthProvider(t *testing.T) {
	providers := []AuthProvider{
		AuthProviderLocal,
		AuthProviderGoogle,
		AuthProviderOkta,
		AuthProviderAzureAD,
		AuthProviderGitHub,
		AuthProviderSAML,
	}

	for _, p := range providers {
		if p == "" {
			t.Error("AuthProvider should not be empty")
		}
	}

	// Check specific values
	if AuthProviderLocal != "local" {
		t.Errorf("AuthProviderLocal = %v, want %v", AuthProviderLocal, "local")
	}
	if AuthProviderGoogle != "google" {
		t.Errorf("AuthProviderGoogle = %v, want %v", AuthProviderGoogle, "google")
	}
}

func TestCreateUserInput(t *testing.T) {
	password := "password123"
	displayName := "Test User"
	authProviderID := "google-123"

	input := CreateUserInput{
		Email:          "test@example.com",
		Password:       &password,
		DisplayName:    &displayName,
		AuthProvider:   AuthProviderLocal,
		AuthProviderID: &authProviderID,
	}

	if input.Email != "test@example.com" {
		t.Errorf("Email = %v, want %v", input.Email, "test@example.com")
	}
	if *input.Password != "password123" {
		t.Errorf("Password = %v, want %v", *input.Password, "password123")
	}
	if input.AuthProvider != AuthProviderLocal {
		t.Errorf("AuthProvider = %v, want %v", input.AuthProvider, AuthProviderLocal)
	}
}

func TestUpdateUserInput(t *testing.T) {
	displayName := "Updated Name"
	avatarURL := "https://example.com/new-avatar.png"

	input := UpdateUserInput{
		DisplayName: &displayName,
		AvatarURL:   &avatarURL,
	}

	if *input.DisplayName != "Updated Name" {
		t.Errorf("DisplayName = %v, want %v", *input.DisplayName, "Updated Name")
	}
	if *input.AvatarURL != "https://example.com/new-avatar.png" {
		t.Errorf("AvatarURL = %v, want %v", *input.AvatarURL, "https://example.com/new-avatar.png")
	}
}
