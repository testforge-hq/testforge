package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// Plan represents a subscription plan
type Plan string

const (
	PlanFree       Plan = "free"
	PlanPro        Plan = "pro"
	PlanEnterprise Plan = "enterprise"
)

// PlanConfig holds configuration for a plan
type PlanConfig struct {
	Name            string
	StripePriceID   string
	MonthlyPrice    int64 // cents
	TestRunsLimit   int64
	AITokensLimit   int64
	SandboxMinutes  int64
	Features        []string
	TrialDays       int
}

// DefaultPlans returns the default plan configurations
var DefaultPlans = map[Plan]PlanConfig{
	PlanFree: {
		Name:           "Free",
		MonthlyPrice:   0,
		TestRunsLimit:  50,
		AITokensLimit:  10000,
		SandboxMinutes: 60,
		Features:       []string{"basic_reports", "email_support"},
		TrialDays:      0,
	},
	PlanPro: {
		Name:           "Pro",
		MonthlyPrice:   9900, // $99
		TestRunsLimit:  500,
		AITokensLimit:  100000,
		SandboxMinutes: 600,
		Features:       []string{"basic_reports", "self_healing", "visual_ai", "priority_support"},
		TrialDays:      14,
	},
	PlanEnterprise: {
		Name:           "Enterprise",
		MonthlyPrice:   0, // Custom pricing
		TestRunsLimit:  -1, // Unlimited
		AITokensLimit:  -1,
		SandboxMinutes: -1,
		Features:       []string{"basic_reports", "self_healing", "visual_ai", "sso", "audit_log", "dedicated_support", "sla"},
		TrialDays:      30,
	},
}

// Subscription represents a subscription record
type Subscription struct {
	ID                     uuid.UUID  `db:"id" json:"id"`
	TenantID               uuid.UUID  `db:"tenant_id" json:"tenant_id"`
	StripeCustomerID       *string    `db:"stripe_customer_id" json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID   *string    `db:"stripe_subscription_id" json:"stripe_subscription_id,omitempty"`
	StripePriceID          *string    `db:"stripe_price_id" json:"stripe_price_id,omitempty"`
	Plan                   Plan       `db:"plan" json:"plan"`
	Status                 string     `db:"status" json:"status"`
	BillingCycleAnchor     *time.Time `db:"billing_cycle_anchor" json:"billing_cycle_anchor,omitempty"`
	CurrentPeriodStart     *time.Time `db:"current_period_start" json:"current_period_start,omitempty"`
	CurrentPeriodEnd       *time.Time `db:"current_period_end" json:"current_period_end,omitempty"`
	TrialStart             *time.Time `db:"trial_start" json:"trial_start,omitempty"`
	TrialEnd               *time.Time `db:"trial_end" json:"trial_end,omitempty"`
	CancelAt               *time.Time `db:"cancel_at" json:"cancel_at,omitempty"`
	CanceledAt             *time.Time `db:"canceled_at" json:"canceled_at,omitempty"`
	CancelAtPeriodEnd      bool       `db:"cancel_at_period_end" json:"cancel_at_period_end"`
	DefaultPaymentMethodID *string    `db:"default_payment_method_id" json:"default_payment_method_id,omitempty"`
	Metadata               json.RawMessage `db:"metadata" json:"metadata,omitempty"`
	CreatedAt              time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt              time.Time  `db:"updated_at" json:"updated_at"`
}

// IsActive returns true if the subscription is active
func (s *Subscription) IsActive() bool {
	return s.Status == "active" || s.Status == "trialing"
}

// IsTrial returns true if the subscription is in trial
func (s *Subscription) IsTrial() bool {
	return s.Status == "trialing"
}

// GetPlanConfig returns the plan configuration
func (s *Subscription) GetPlanConfig() PlanConfig {
	config, ok := DefaultPlans[s.Plan]
	if !ok {
		return DefaultPlans[PlanFree]
	}
	return config
}

// SubscriptionService handles subscription operations
type SubscriptionService struct {
	db     *sqlx.DB
	stripe *StripeClient
	logger *zap.Logger
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService(db *sqlx.DB, stripe *StripeClient, logger *zap.Logger) *SubscriptionService {
	return &SubscriptionService{
		db:     db,
		stripe: stripe,
		logger: logger,
	}
}

// GetByTenant retrieves the subscription for a tenant
func (s *SubscriptionService) GetByTenant(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	var sub Subscription
	err := s.db.GetContext(ctx, &sub, `
		SELECT * FROM subscriptions WHERE tenant_id = $1`, tenantID)

	if err == sql.ErrNoRows {
		// Create free tier subscription
		return s.createFreeTier(ctx, tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("getting subscription: %w", err)
	}

	return &sub, nil
}

// createFreeTier creates a free tier subscription for a tenant
func (s *SubscriptionService) createFreeTier(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	now := time.Now()
	periodEnd := now.AddDate(0, 1, 0)

	sub := &Subscription{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		Plan:               PlanFree,
		Status:             "active",
		CurrentPeriodStart: &now,
		CurrentPeriodEnd:   &periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, tenant_id, plan, status, current_period_start, current_period_end)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		sub.ID, sub.TenantID, sub.Plan, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd)

	if err != nil {
		return nil, fmt.Errorf("creating free tier: %w", err)
	}

	return sub, nil
}

// CreateStripeCustomer creates a Stripe customer for a tenant
func (s *SubscriptionService) CreateStripeCustomer(ctx context.Context, tenantID uuid.UUID, email, name string) (*Subscription, error) {
	sub, err := s.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if sub.StripeCustomerID != nil {
		return sub, nil // Already has customer
	}

	customer, err := s.stripe.CreateCustomer(ctx, email, name, tenantID)
	if err != nil {
		return nil, fmt.Errorf("creating Stripe customer: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE subscriptions SET stripe_customer_id = $1, updated_at = NOW()
		WHERE tenant_id = $2`, customer.ID, tenantID)

	if err != nil {
		return nil, fmt.Errorf("updating customer ID: %w", err)
	}

	sub.StripeCustomerID = &customer.ID
	return sub, nil
}

// Subscribe creates or updates a subscription
func (s *SubscriptionService) Subscribe(ctx context.Context, tenantID uuid.UUID, plan Plan, paymentMethodID string) (*Subscription, error) {
	sub, err := s.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if sub.StripeCustomerID == nil {
		return nil, fmt.Errorf("tenant has no Stripe customer")
	}

	planConfig, ok := DefaultPlans[plan]
	if !ok {
		return nil, fmt.Errorf("unknown plan: %s", plan)
	}

	// Create Stripe subscription
	opts := &SubscriptionOptions{
		PaymentMethodID: paymentMethodID,
		TrialDays:       planConfig.TrialDays,
		Metadata: map[string]string{
			"tenant_id": tenantID.String(),
			"plan":      string(plan),
		},
	}

	stripeSub, err := s.stripe.CreateSubscription(ctx, *sub.StripeCustomerID, planConfig.StripePriceID, opts)
	if err != nil {
		return nil, fmt.Errorf("creating Stripe subscription: %w", err)
	}

	// Update database
	_, err = s.db.ExecContext(ctx, `
		UPDATE subscriptions SET
			stripe_subscription_id = $1,
			stripe_price_id = $2,
			plan = $3,
			status = $4,
			current_period_start = $5,
			current_period_end = $6,
			trial_start = $7,
			trial_end = $8,
			default_payment_method_id = $9,
			updated_at = NOW()
		WHERE tenant_id = $10`,
		stripeSub.ID,
		planConfig.StripePriceID,
		plan,
		stripeSub.Status,
		time.Unix(stripeSub.CurrentPeriodStart, 0),
		time.Unix(stripeSub.CurrentPeriodEnd, 0),
		nullableTime(stripeSub.TrialStart),
		nullableTime(stripeSub.TrialEnd),
		paymentMethodID,
		tenantID,
	)

	if err != nil {
		return nil, fmt.Errorf("updating subscription: %w", err)
	}

	return s.GetByTenant(ctx, tenantID)
}

// Cancel cancels a subscription
func (s *SubscriptionService) Cancel(ctx context.Context, tenantID uuid.UUID, immediate bool) (*Subscription, error) {
	sub, err := s.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	if sub.StripeSubscriptionID == nil {
		// Free tier - just mark as canceled
		_, err = s.db.ExecContext(ctx, `
			UPDATE subscriptions SET status = 'canceled', canceled_at = NOW(), updated_at = NOW()
			WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return nil, err
		}
		return s.GetByTenant(ctx, tenantID)
	}

	stripeSub, err := s.stripe.CancelSubscription(ctx, *sub.StripeSubscriptionID, !immediate)
	if err != nil {
		return nil, fmt.Errorf("canceling Stripe subscription: %w", err)
	}

	// Update database
	_, err = s.db.ExecContext(ctx, `
		UPDATE subscriptions SET
			status = $1,
			cancel_at_period_end = $2,
			cancel_at = $3,
			canceled_at = $4,
			updated_at = NOW()
		WHERE tenant_id = $5`,
		stripeSub.Status,
		stripeSub.CancelAtPeriodEnd,
		nullableTime(stripeSub.CancelAt),
		nullableTime(stripeSub.CanceledAt),
		tenantID,
	)

	if err != nil {
		return nil, fmt.Errorf("updating subscription: %w", err)
	}

	return s.GetByTenant(ctx, tenantID)
}

// GetBillingPortalURL returns a URL to the Stripe billing portal
func (s *SubscriptionService) GetBillingPortalURL(ctx context.Context, tenantID uuid.UUID, returnURL string) (string, error) {
	sub, err := s.GetByTenant(ctx, tenantID)
	if err != nil {
		return "", err
	}

	if sub.StripeCustomerID == nil {
		return "", fmt.Errorf("tenant has no Stripe customer")
	}

	return s.stripe.CreateBillingPortalSession(ctx, *sub.StripeCustomerID, returnURL)
}

// CheckUsageLimit checks if a tenant has exceeded their usage limit
func (s *SubscriptionService) CheckUsageLimit(ctx context.Context, tenantID uuid.UUID, metric string, currentUsage int64) (bool, int64, error) {
	sub, err := s.GetByTenant(ctx, tenantID)
	if err != nil {
		return false, 0, err
	}

	config := sub.GetPlanConfig()

	var limit int64
	switch metric {
	case MetricTestRuns:
		limit = config.TestRunsLimit
	case MetricAITokens:
		limit = config.AITokensLimit
	case MetricSandboxMinutes:
		limit = config.SandboxMinutes
	default:
		return true, 0, nil
	}

	// -1 means unlimited
	if limit == -1 {
		return false, -1, nil
	}

	return currentUsage >= limit, limit, nil
}

// SyncFromStripe synchronizes subscription data from Stripe
func (s *SubscriptionService) SyncFromStripe(ctx context.Context, stripeSubscriptionID string) error {
	stripeSub, err := s.stripe.GetSubscription(ctx, stripeSubscriptionID)
	if err != nil {
		return fmt.Errorf("fetching Stripe subscription: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE subscriptions SET
			status = $1,
			current_period_start = $2,
			current_period_end = $3,
			cancel_at_period_end = $4,
			cancel_at = $5,
			canceled_at = $6,
			trial_start = $7,
			trial_end = $8,
			default_payment_method_id = $9,
			updated_at = NOW()
		WHERE stripe_subscription_id = $10`,
		stripeSub.Status,
		time.Unix(stripeSub.CurrentPeriodStart, 0),
		time.Unix(stripeSub.CurrentPeriodEnd, 0),
		stripeSub.CancelAtPeriodEnd,
		nullableTime(stripeSub.CancelAt),
		nullableTime(stripeSub.CanceledAt),
		nullableTime(stripeSub.TrialStart),
		nullableTime(stripeSub.TrialEnd),
		stripeSub.DefaultPaymentMethod,
		stripeSubscriptionID,
	)

	return err
}

func nullableTime(ts *int64) *time.Time {
	if ts == nil {
		return nil
	}
	t := time.Unix(*ts, 0)
	return &t
}
