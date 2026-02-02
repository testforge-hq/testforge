// Package rbac provides role-based access control functionality
package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/testforge/testforge/internal/domain"
)

// Enforcer handles permission checking with caching
type Enforcer struct {
	roleRepo       RoleRepository
	membershipRepo MembershipRepository
	cache          *redis.Client
	cacheTTL       time.Duration
	localCache     sync.Map // Local in-memory cache for hot paths
}

// RoleRepository defines the interface for role data access
type RoleRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Role, error)
	GetByName(ctx context.Context, tenantID *uuid.UUID, name string) (*domain.Role, error)
	GetSystemRoles(ctx context.Context) ([]*domain.Role, error)
}

// MembershipRepository defines the interface for membership data access
type MembershipRepository interface {
	GetByUserAndTenant(ctx context.Context, userID, tenantID uuid.UUID) (*domain.TenantMembership, error)
	GetUserRoleInTenant(ctx context.Context, userID, tenantID uuid.UUID) (*domain.Role, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*domain.TenantMembership, error)
}

// EnforcerConfig holds configuration for the enforcer
type EnforcerConfig struct {
	CacheTTL time.Duration
}

// NewEnforcer creates a new RBAC enforcer
func NewEnforcer(roleRepo RoleRepository, membershipRepo MembershipRepository, cache *redis.Client, cfg EnforcerConfig) *Enforcer {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}

	return &Enforcer{
		roleRepo:       roleRepo,
		membershipRepo: membershipRepo,
		cache:          cache,
		cacheTTL:       cfg.CacheTTL,
	}
}

// cachedPermissions represents cached user permissions
type cachedPermissions struct {
	Permissions []string  `json:"permissions"`
	RoleID      uuid.UUID `json:"role_id"`
	ExpiresAt   int64     `json:"expires_at"`
}

// CheckPermission verifies if a user has a specific permission in a tenant
func (e *Enforcer) CheckPermission(ctx context.Context, userID, tenantID uuid.UUID, permission string) (bool, error) {
	perms, err := e.GetUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return false, err
	}

	return domain.CheckPermission(perms, permission), nil
}

// CheckAnyPermission verifies if a user has any of the specified permissions
func (e *Enforcer) CheckAnyPermission(ctx context.Context, userID, tenantID uuid.UUID, permissions ...string) (bool, error) {
	perms, err := e.GetUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return false, err
	}

	return domain.CheckAnyPermission(perms, permissions...), nil
}

// CheckAllPermissions verifies if a user has all of the specified permissions
func (e *Enforcer) CheckAllPermissions(ctx context.Context, userID, tenantID uuid.UUID, permissions ...string) (bool, error) {
	perms, err := e.GetUserPermissions(ctx, userID, tenantID)
	if err != nil {
		return false, err
	}

	return domain.CheckAllPermissions(perms, permissions...), nil
}

// GetUserPermissions retrieves all permissions for a user in a tenant
func (e *Enforcer) GetUserPermissions(ctx context.Context, userID, tenantID uuid.UUID) ([]string, error) {
	cacheKey := fmt.Sprintf("rbac:perms:%s:%s", userID, tenantID)

	// Try local cache first (for hot paths)
	if cached, ok := e.localCache.Load(cacheKey); ok {
		cp := cached.(*cachedPermissions)
		if time.Now().Unix() < cp.ExpiresAt {
			return cp.Permissions, nil
		}
		e.localCache.Delete(cacheKey)
	}

	// Try Redis cache
	if e.cache != nil {
		data, err := e.cache.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var cp cachedPermissions
			if json.Unmarshal(data, &cp) == nil && time.Now().Unix() < cp.ExpiresAt {
				// Store in local cache
				e.localCache.Store(cacheKey, &cp)
				return cp.Permissions, nil
			}
		}
	}

	// Query database
	role, err := e.membershipRepo.GetUserRoleInTenant(ctx, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting user role: %w", err)
	}
	if role == nil {
		return nil, nil // User has no role in this tenant
	}

	perms := role.GetPermissions()

	// Cache the result
	cp := &cachedPermissions{
		Permissions: perms,
		RoleID:      role.ID,
		ExpiresAt:   time.Now().Add(e.cacheTTL).Unix(),
	}

	// Store in local cache
	e.localCache.Store(cacheKey, cp)

	// Store in Redis
	if e.cache != nil {
		if data, err := json.Marshal(cp); err == nil {
			e.cache.Set(ctx, cacheKey, data, e.cacheTTL)
		}
	}

	return perms, nil
}

// GetUserRole retrieves the user's role in a tenant
func (e *Enforcer) GetUserRole(ctx context.Context, userID, tenantID uuid.UUID) (*domain.Role, error) {
	return e.membershipRepo.GetUserRoleInTenant(ctx, userID, tenantID)
}

// InvalidateUserPermissions removes cached permissions for a user
func (e *Enforcer) InvalidateUserPermissions(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) error {
	if tenantID != nil {
		// Invalidate specific tenant
		cacheKey := fmt.Sprintf("rbac:perms:%s:%s", userID, *tenantID)
		e.localCache.Delete(cacheKey)
		if e.cache != nil {
			e.cache.Del(ctx, cacheKey)
		}
	} else {
		// Invalidate all tenants for user
		// This requires listing all memberships
		memberships, err := e.membershipRepo.ListByUser(ctx, userID)
		if err != nil {
			return err
		}
		for _, m := range memberships {
			cacheKey := fmt.Sprintf("rbac:perms:%s:%s", userID, m.TenantID)
			e.localCache.Delete(cacheKey)
			if e.cache != nil {
				e.cache.Del(ctx, cacheKey)
			}
		}
	}
	return nil
}

// IsAdmin checks if a user has admin role in a tenant
func (e *Enforcer) IsAdmin(ctx context.Context, userID, tenantID uuid.UUID) (bool, error) {
	return e.CheckPermission(ctx, userID, tenantID, domain.PermissionAll)
}

// CanManageMembers checks if a user can manage tenant members
func (e *Enforcer) CanManageMembers(ctx context.Context, userID, tenantID uuid.UUID) (bool, error) {
	return e.CheckAnyPermission(ctx, userID, tenantID,
		domain.PermissionAll,
		domain.PermissionMemberAll,
		domain.PermissionMemberInvite,
	)
}

// CanManageProjects checks if a user can create/update/delete projects
func (e *Enforcer) CanManageProjects(ctx context.Context, userID, tenantID uuid.UUID) (bool, error) {
	return e.CheckAnyPermission(ctx, userID, tenantID,
		domain.PermissionAll,
		domain.PermissionProjectAll,
	)
}

// CanViewProjects checks if a user can view projects
func (e *Enforcer) CanViewProjects(ctx context.Context, userID, tenantID uuid.UUID) (bool, error) {
	return e.CheckAnyPermission(ctx, userID, tenantID,
		domain.PermissionAll,
		domain.PermissionProjectAll,
		domain.PermissionProjectRead,
	)
}

// CanRunTests checks if a user can create test runs
func (e *Enforcer) CanRunTests(ctx context.Context, userID, tenantID uuid.UUID) (bool, error) {
	return e.CheckAnyPermission(ctx, userID, tenantID,
		domain.PermissionAll,
		domain.PermissionRunAll,
		domain.PermissionRunCreate,
	)
}

// CanViewBilling checks if a user can view billing information
func (e *Enforcer) CanViewBilling(ctx context.Context, userID, tenantID uuid.UUID) (bool, error) {
	return e.CheckAnyPermission(ctx, userID, tenantID,
		domain.PermissionAll,
		domain.PermissionBillingAll,
		domain.PermissionBillingRead,
	)
}

// PermissionDeniedError represents a permission denied error
type PermissionDeniedError struct {
	UserID     uuid.UUID
	TenantID   uuid.UUID
	Permission string
}

func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf("permission denied: user %s does not have permission %s in tenant %s",
		e.UserID, e.Permission, e.TenantID)
}

// RequirePermission returns an error if the user doesn't have the permission
func (e *Enforcer) RequirePermission(ctx context.Context, userID, tenantID uuid.UUID, permission string) error {
	has, err := e.CheckPermission(ctx, userID, tenantID, permission)
	if err != nil {
		return err
	}
	if !has {
		return &PermissionDeniedError{
			UserID:     userID,
			TenantID:   tenantID,
			Permission: permission,
		}
	}
	return nil
}
