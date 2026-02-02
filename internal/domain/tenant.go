package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tenant represents an organization using the platform
type Tenant struct {
	ID       uuid.UUID      `json:"id" db:"id"`
	Name     string         `json:"name" db:"name"`
	Slug     string         `json:"slug" db:"slug"`
	Plan     Plan           `json:"plan" db:"plan"`
	Settings TenantSettings `json:"settings" db:"settings"`
	Timestamps
}

// TenantSettings contains configurable options per tenant
type TenantSettings struct {
	MaxConcurrentRuns   int      `json:"max_concurrent_runs"`
	MaxTestCasesPerRun  int      `json:"max_test_cases_per_run"`
	RetentionDays       int      `json:"retention_days"`
	EnableSelfHealing   bool     `json:"enable_self_healing"`
	EnableVisualTesting bool     `json:"enable_visual_testing"`
	AllowedDomains      []string `json:"allowed_domains,omitempty"`
	WebhookSecret       string   `json:"webhook_secret,omitempty"`
	NotifyOnComplete    bool     `json:"notify_on_complete"`
	NotifyOnFailure     bool     `json:"notify_on_failure"`
}

// DefaultTenantSettings returns settings based on plan
func DefaultTenantSettings(plan Plan) TenantSettings {
	switch plan {
	case PlanEnterprise:
		return TenantSettings{
			MaxConcurrentRuns:   20,
			MaxTestCasesPerRun:  500,
			RetentionDays:       365,
			EnableSelfHealing:   true,
			EnableVisualTesting: true,
			NotifyOnComplete:    true,
			NotifyOnFailure:     true,
		}
	case PlanPro:
		return TenantSettings{
			MaxConcurrentRuns:   5,
			MaxTestCasesPerRun:  100,
			RetentionDays:       90,
			EnableSelfHealing:   true,
			EnableVisualTesting: true,
			NotifyOnComplete:    true,
			NotifyOnFailure:     true,
		}
	default: // PlanFree
		return TenantSettings{
			MaxConcurrentRuns:   1,
			MaxTestCasesPerRun:  20,
			RetentionDays:       7,
			EnableSelfHealing:   false,
			EnableVisualTesting: false,
			NotifyOnComplete:    false,
			NotifyOnFailure:     true,
		}
	}
}

// NewTenant creates a new tenant with defaults
func NewTenant(name, slug string, plan Plan) *Tenant {
	now := time.Now().UTC()
	return &Tenant{
		ID:       uuid.New(),
		Name:     name,
		Slug:     slug,
		Plan:     plan,
		Settings: DefaultTenantSettings(plan),
		Timestamps: Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

// TenantRepository defines data access for tenants
type TenantRepository interface {
	Create(ctx context.Context, tenant *Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	Update(ctx context.Context, tenant *Tenant) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, limit, offset int) ([]*Tenant, int, error)
	ExistsBySlug(ctx context.Context, slug string) (bool, error)
}

// CreateTenantInput for API requests
type CreateTenantInput struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Plan Plan   `json:"plan"`
}

// UpdateTenantInput for API requests
type UpdateTenantInput struct {
	Name     *string         `json:"name,omitempty"`
	Plan     *Plan           `json:"plan,omitempty"`
	Settings *TenantSettings `json:"settings,omitempty"`
}
