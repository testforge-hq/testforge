package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRole_GetPermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions json.RawMessage
		want        []string
	}{
		{
			name:        "valid permissions",
			permissions: json.RawMessage(`["project:read", "run:create"]`),
			want:        []string{"project:read", "run:create"},
		},
		{
			name:        "empty permissions",
			permissions: json.RawMessage(`[]`),
			want:        []string{},
		},
		{
			name:        "invalid JSON",
			permissions: json.RawMessage(`not json`),
			want:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Role{Permissions: tt.permissions}
			got := r.GetPermissions()

			if len(got) != len(tt.want) {
				t.Errorf("GetPermissions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRole_HasPermission(t *testing.T) {
	role := &Role{
		Permissions: json.RawMessage(`["project:read", "run:*"]`),
	}

	tests := []struct {
		permission string
		want       bool
	}{
		{"project:read", true},
		{"project:write", false},
		{"run:create", true},
		{"run:delete", true},
		{"report:read", false},
	}

	for _, tt := range tests {
		t.Run(tt.permission, func(t *testing.T) {
			got := role.HasPermission(tt.permission)
			if got != tt.want {
				t.Errorf("HasPermission(%q) = %v, want %v", tt.permission, got, tt.want)
			}
		})
	}
}

func TestRole_DisplayNameOrName(t *testing.T) {
	displayName := "Admin Role"

	tests := []struct {
		name        string
		role        *Role
		want        string
	}{
		{
			name: "with display name",
			role: &Role{Name: "admin", DisplayName: &displayName},
			want: "Admin Role",
		},
		{
			name: "without display name",
			role: &Role{Name: "admin"},
			want: "admin",
		},
		{
			name: "with empty display name",
			role: &Role{Name: "admin", DisplayName: func() *string { s := ""; return &s }()},
			want: "admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.role.DisplayNameOrName()
			if got != tt.want {
				t.Errorf("DisplayNameOrName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckPermission(t *testing.T) {
	tests := []struct {
		name      string
		userPerms []string
		required  string
		want      bool
	}{
		{
			name:      "exact match",
			userPerms: []string{"project:read", "run:create"},
			required:  "project:read",
			want:      true,
		},
		{
			name:      "no match",
			userPerms: []string{"project:read"},
			required:  "project:write",
			want:      false,
		},
		{
			name:      "wildcard grants all",
			userPerms: []string{"*"},
			required:  "anything",
			want:      true,
		},
		{
			name:      "category wildcard matches",
			userPerms: []string{"project:*"},
			required:  "project:delete",
			want:      true,
		},
		{
			name:      "category wildcard doesn't match different category",
			userPerms: []string{"project:*"},
			required:  "run:create",
			want:      false,
		},
		{
			name:      "empty permissions",
			userPerms: []string{},
			required:  "project:read",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckPermission(tt.userPerms, tt.required)
			if got != tt.want {
				t.Errorf("CheckPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckAnyPermission(t *testing.T) {
	tests := []struct {
		name      string
		userPerms []string
		required  []string
		want      bool
	}{
		{
			name:      "has one of required",
			userPerms: []string{"project:read"},
			required:  []string{"project:read", "run:create"},
			want:      true,
		},
		{
			name:      "has none of required",
			userPerms: []string{"report:read"},
			required:  []string{"project:read", "run:create"},
			want:      false,
		},
		{
			name:      "empty required",
			userPerms: []string{"project:read"},
			required:  []string{},
			want:      false,
		},
		{
			name:      "wildcard matches any",
			userPerms: []string{"*"},
			required:  []string{"project:read", "run:create"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckAnyPermission(tt.userPerms, tt.required...)
			if got != tt.want {
				t.Errorf("CheckAnyPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckAllPermissions(t *testing.T) {
	tests := []struct {
		name      string
		userPerms []string
		required  []string
		want      bool
	}{
		{
			name:      "has all required",
			userPerms: []string{"project:read", "run:create", "report:read"},
			required:  []string{"project:read", "run:create"},
			want:      true,
		},
		{
			name:      "missing one required",
			userPerms: []string{"project:read"},
			required:  []string{"project:read", "run:create"},
			want:      false,
		},
		{
			name:      "empty required",
			userPerms: []string{"project:read"},
			required:  []string{},
			want:      true,
		},
		{
			name:      "wildcard matches all",
			userPerms: []string{"*"},
			required:  []string{"project:read", "run:create", "report:read"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckAllPermissions(tt.userPerms, tt.required...)
			if got != tt.want {
				t.Errorf("CheckAllPermissions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMergePermissions(t *testing.T) {
	tests := []struct {
		name     string
		permSets [][]string
		wantLen  int
	}{
		{
			name: "merge without duplicates",
			permSets: [][]string{
				{"project:read", "run:create"},
				{"report:read", "audit:read"},
			},
			wantLen: 4,
		},
		{
			name: "merge with duplicates",
			permSets: [][]string{
				{"project:read", "run:create"},
				{"project:read", "report:read"},
			},
			wantLen: 3,
		},
		{
			name:     "empty sets",
			permSets: [][]string{{}, {}},
			wantLen:  0,
		},
		{
			name: "single set",
			permSets: [][]string{
				{"project:read", "run:create"},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergePermissions(tt.permSets...)
			if len(got) != tt.wantLen {
				t.Errorf("MergePermissions() length = %v, want %v", len(got), tt.wantLen)
			}
		})
	}
}

func TestSystemRoleIDs(t *testing.T) {
	if RoleIDAdmin == uuid.Nil {
		t.Error("RoleIDAdmin should not be nil")
	}
	if RoleIDDeveloper == uuid.Nil {
		t.Error("RoleIDDeveloper should not be nil")
	}
	if RoleIDViewer == uuid.Nil {
		t.Error("RoleIDViewer should not be nil")
	}
	if RoleIDBillingAdmin == uuid.Nil {
		t.Error("RoleIDBillingAdmin should not be nil")
	}

	// Should all be different
	ids := []uuid.UUID{RoleIDAdmin, RoleIDDeveloper, RoleIDViewer, RoleIDBillingAdmin}
	seen := make(map[uuid.UUID]bool)
	for _, id := range ids {
		if seen[id] {
			t.Error("System role IDs should be unique")
		}
		seen[id] = true
	}
}

func TestRole_Fields(t *testing.T) {
	tenantID := uuid.New()
	displayName := "Test Role"
	description := "A test role"
	parentID := uuid.New()

	role := Role{
		ID:           uuid.New(),
		TenantID:     &tenantID,
		Name:         "test-role",
		DisplayName:  &displayName,
		Description:  &description,
		Permissions:  json.RawMessage(`["project:read"]`),
		IsSystem:     false,
		ParentRoleID: &parentID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if role.Name != "test-role" {
		t.Errorf("Name = %v, want %v", role.Name, "test-role")
	}
	if *role.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", *role.TenantID, tenantID)
	}
	if role.IsSystem {
		t.Error("IsSystem should be false")
	}
}
