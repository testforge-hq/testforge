package domain

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// System role IDs (constant UUIDs for system roles)
var (
	RoleIDAdmin        = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	RoleIDDeveloper    = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	RoleIDViewer       = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	RoleIDBillingAdmin = uuid.MustParse("00000000-0000-0000-0000-000000000004")
)

// Permission constants
const (
	// Wildcard permissions
	PermissionAll = "*"

	// Project permissions
	PermissionProjectAll    = "project:*"
	PermissionProjectCreate = "project:create"
	PermissionProjectRead   = "project:read"
	PermissionProjectUpdate = "project:update"
	PermissionProjectDelete = "project:delete"

	// Test run permissions
	PermissionRunAll    = "run:*"
	PermissionRunCreate = "run:create"
	PermissionRunRead   = "run:read"
	PermissionRunCancel = "run:cancel"
	PermissionRunDelete = "run:delete"

	// Report permissions
	PermissionReportAll  = "report:*"
	PermissionReportRead = "report:read"

	// API key permissions
	PermissionAPIKeyAll    = "api_key:*"
	PermissionAPIKeyCreate = "api_key:create"
	PermissionAPIKeyRead   = "api_key:read"
	PermissionAPIKeyRevoke = "api_key:revoke"

	// Tenant permissions
	PermissionTenantAll    = "tenant:*"
	PermissionTenantRead   = "tenant:read"
	PermissionTenantUpdate = "tenant:update"
	PermissionTenantDelete = "tenant:delete"

	// User/member permissions
	PermissionMemberAll    = "member:*"
	PermissionMemberInvite = "member:invite"
	PermissionMemberRead   = "member:read"
	PermissionMemberUpdate = "member:update"
	PermissionMemberRemove = "member:remove"

	// Billing permissions
	PermissionBillingAll  = "billing:*"
	PermissionBillingRead = "billing:read"

	// Audit permissions
	PermissionAuditRead = "audit:read"
)

// Role represents a permission role
type Role struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	TenantID     *uuid.UUID      `json:"tenant_id,omitempty" db:"tenant_id"`
	Name         string          `json:"name" db:"name"`
	DisplayName  *string         `json:"display_name,omitempty" db:"display_name"`
	Description  *string         `json:"description,omitempty" db:"description"`
	Permissions  json.RawMessage `json:"permissions" db:"permissions"`
	IsSystem     bool            `json:"is_system" db:"is_system"`
	ParentRoleID *uuid.UUID      `json:"parent_role_id,omitempty" db:"parent_role_id"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// GetPermissions returns the permissions as a string slice
func (r *Role) GetPermissions() []string {
	var perms []string
	if err := json.Unmarshal(r.Permissions, &perms); err != nil {
		return nil
	}
	return perms
}

// HasPermission checks if the role has a specific permission
func (r *Role) HasPermission(permission string) bool {
	perms := r.GetPermissions()
	return CheckPermission(perms, permission)
}

// DisplayNameOrName returns the display name if set, otherwise the name
func (r *Role) DisplayNameOrName() string {
	if r.DisplayName != nil && *r.DisplayName != "" {
		return *r.DisplayName
	}
	return r.Name
}

// CreateRoleInput represents input for creating a role
type CreateRoleInput struct {
	Name        string    `json:"name" validate:"required,min=1,max=100"`
	DisplayName *string   `json:"display_name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Permissions []string  `json:"permissions" validate:"required"`
	TenantID    uuid.UUID `json:"tenant_id" validate:"required"`
}

// UpdateRoleInput represents input for updating a role
type UpdateRoleInput struct {
	DisplayName *string  `json:"display_name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// CheckPermission checks if a set of permissions grants access to a specific permission
func CheckPermission(userPerms []string, required string) bool {
	for _, perm := range userPerms {
		// Wildcard grants all permissions
		if perm == PermissionAll {
			return true
		}

		// Exact match
		if perm == required {
			return true
		}

		// Category wildcard (e.g., "project:*" matches "project:read")
		if strings.HasSuffix(perm, ":*") {
			category := strings.TrimSuffix(perm, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}
	}
	return false
}

// CheckAnyPermission checks if any of the required permissions are granted
func CheckAnyPermission(userPerms []string, required ...string) bool {
	for _, r := range required {
		if CheckPermission(userPerms, r) {
			return true
		}
	}
	return false
}

// CheckAllPermissions checks if all of the required permissions are granted
func CheckAllPermissions(userPerms []string, required ...string) bool {
	for _, r := range required {
		if !CheckPermission(userPerms, r) {
			return false
		}
	}
	return true
}

// MergePermissions combines multiple permission sets, removing duplicates
func MergePermissions(permSets ...[]string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, perms := range permSets {
		for _, p := range perms {
			if !seen[p] {
				seen[p] = true
				result = append(result, p)
			}
		}
	}

	return result
}
