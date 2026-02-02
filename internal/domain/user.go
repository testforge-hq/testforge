package domain

import (
	"time"

	"github.com/google/uuid"
)

// AuthProvider represents the authentication provider
type AuthProvider string

const (
	AuthProviderLocal   AuthProvider = "local"
	AuthProviderGoogle  AuthProvider = "google"
	AuthProviderOkta    AuthProvider = "okta"
	AuthProviderAzureAD AuthProvider = "azure_ad"
	AuthProviderGitHub  AuthProvider = "github"
	AuthProviderSAML    AuthProvider = "saml"
)

// User represents an application user
type User struct {
	ID                  uuid.UUID    `json:"id" db:"id"`
	Email               string       `json:"email" db:"email"`
	EmailVerified       bool         `json:"email_verified" db:"email_verified"`
	EmailVerifiedAt     *time.Time   `json:"email_verified_at,omitempty" db:"email_verified_at"`
	PasswordHash        *string      `json:"-" db:"password_hash"` // Never expose
	AuthProvider        AuthProvider `json:"auth_provider" db:"auth_provider"`
	AuthProviderID      *string      `json:"auth_provider_id,omitempty" db:"auth_provider_id"`
	DisplayName         *string      `json:"display_name,omitempty" db:"display_name"`
	AvatarURL           *string      `json:"avatar_url,omitempty" db:"avatar_url"`
	MFAEnabled          bool         `json:"mfa_enabled" db:"mfa_enabled"`
	MFASecret           *string      `json:"-" db:"mfa_secret"` // Never expose
	LastLoginAt         *time.Time   `json:"last_login_at,omitempty" db:"last_login_at"`
	LastLoginIP         *string      `json:"last_login_ip,omitempty" db:"last_login_ip"`
	FailedLoginAttempts int          `json:"failed_login_attempts" db:"failed_login_attempts"`
	LockedUntil         *time.Time   `json:"locked_until,omitempty" db:"locked_until"`
	IsActive            bool         `json:"is_active" db:"is_active"`
	DeactivatedAt       *time.Time   `json:"deactivated_at,omitempty" db:"deactivated_at"`
	DeactivatedReason   *string      `json:"deactivated_reason,omitempty" db:"deactivated_reason"`
	Metadata            Metadata     `json:"metadata,omitempty" db:"metadata"`
	CreatedAt           time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at" db:"updated_at"`
}

// IsLocked returns true if the user account is currently locked
func (u *User) IsLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*u.LockedUntil)
}

// CanLogin returns true if the user can attempt to login
func (u *User) CanLogin() bool {
	return u.IsActive && !u.IsLocked()
}

// DisplayNameOrEmail returns the display name if set, otherwise the email
func (u *User) DisplayNameOrEmail() string {
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return u.Email
}

// CreateUserInput represents input for creating a user
type CreateUserInput struct {
	Email        string       `json:"email" validate:"required,email"`
	Password     *string      `json:"password,omitempty"` // Required for local auth
	DisplayName  *string      `json:"display_name,omitempty"`
	AuthProvider AuthProvider `json:"auth_provider"`
	AuthProviderID *string    `json:"auth_provider_id,omitempty"`
}

// UpdateUserInput represents input for updating a user
type UpdateUserInput struct {
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
}

// UserWithMemberships includes the user's tenant memberships
type UserWithMemberships struct {
	User
	Memberships []TenantMembership `json:"memberships"`
}

// TenantMembership represents a user's membership in a tenant
type TenantMembership struct {
	ID         uuid.UUID  `json:"id" db:"id"`
	UserID     uuid.UUID  `json:"user_id" db:"user_id"`
	TenantID   uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	RoleID     uuid.UUID  `json:"role_id" db:"role_id"`
	IsActive   bool       `json:"is_active" db:"is_active"`
	InvitedBy  *uuid.UUID `json:"invited_by,omitempty" db:"invited_by"`
	InvitedAt  time.Time  `json:"invited_at" db:"invited_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty" db:"accepted_at"`
	Metadata   Metadata   `json:"metadata,omitempty" db:"metadata"`
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`

	// Joined fields (when querying with related data)
	TenantName *string `json:"tenant_name,omitempty" db:"tenant_name"`
	TenantSlug *string `json:"tenant_slug,omitempty" db:"tenant_slug"`
	RoleName   *string `json:"role_name,omitempty" db:"role_name"`
}

// InviteUserInput represents input for inviting a user to a tenant
type InviteUserInput struct {
	Email    string    `json:"email" validate:"required,email"`
	RoleID   uuid.UUID `json:"role_id" validate:"required"`
	TenantID uuid.UUID `json:"tenant_id" validate:"required"`
}

// Metadata is a generic JSON metadata type
type Metadata map[string]interface{}
