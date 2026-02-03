package billing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestPlanConstants(t *testing.T) {
	assert.Equal(t, Plan("free"), PlanFree)
	assert.Equal(t, Plan("pro"), PlanPro)
	assert.Equal(t, Plan("enterprise"), PlanEnterprise)
}

func TestDefaultPlans(t *testing.T) {
	t.Run("free plan", func(t *testing.T) {
		plan := DefaultPlans[PlanFree]
		assert.Equal(t, "Free", plan.Name)
		assert.Equal(t, int64(0), plan.MonthlyPrice)
		assert.Equal(t, int64(50), plan.TestRunsLimit)
		assert.Equal(t, int64(10000), plan.AITokensLimit)
		assert.Equal(t, int64(60), plan.SandboxMinutes)
		assert.Equal(t, 0, plan.TrialDays)
		assert.Contains(t, plan.Features, "basic_reports")
		assert.Contains(t, plan.Features, "email_support")
	})

	t.Run("pro plan", func(t *testing.T) {
		plan := DefaultPlans[PlanPro]
		assert.Equal(t, "Pro", plan.Name)
		assert.Equal(t, int64(9900), plan.MonthlyPrice)
		assert.Equal(t, int64(500), plan.TestRunsLimit)
		assert.Equal(t, int64(100000), plan.AITokensLimit)
		assert.Equal(t, int64(600), plan.SandboxMinutes)
		assert.Equal(t, 14, plan.TrialDays)
		assert.Contains(t, plan.Features, "self_healing")
		assert.Contains(t, plan.Features, "visual_ai")
	})

	t.Run("enterprise plan", func(t *testing.T) {
		plan := DefaultPlans[PlanEnterprise]
		assert.Equal(t, "Enterprise", plan.Name)
		assert.Equal(t, int64(-1), plan.TestRunsLimit)  // Unlimited
		assert.Equal(t, int64(-1), plan.AITokensLimit)  // Unlimited
		assert.Equal(t, int64(-1), plan.SandboxMinutes) // Unlimited
		assert.Equal(t, 30, plan.TrialDays)
		assert.Contains(t, plan.Features, "sso")
		assert.Contains(t, plan.Features, "audit_log")
		assert.Contains(t, plan.Features, "sla")
	})
}

func TestSubscription_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{name: "active status", status: "active", expected: true},
		{name: "trialing status", status: "trialing", expected: true},
		{name: "canceled status", status: "canceled", expected: false},
		{name: "past_due status", status: "past_due", expected: false},
		{name: "incomplete status", status: "incomplete", expected: false},
		{name: "empty status", status: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &Subscription{Status: tt.status}
			assert.Equal(t, tt.expected, sub.IsActive())
		})
	}
}

func TestSubscription_IsTrial(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{name: "trialing status", status: "trialing", expected: true},
		{name: "active status", status: "active", expected: false},
		{name: "canceled status", status: "canceled", expected: false},
		{name: "empty status", status: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := &Subscription{Status: tt.status}
			assert.Equal(t, tt.expected, sub.IsTrial())
		})
	}
}

func TestSubscription_GetPlanConfig(t *testing.T) {
	t.Run("returns free plan config", func(t *testing.T) {
		sub := &Subscription{Plan: PlanFree}
		config := sub.GetPlanConfig()
		assert.Equal(t, "Free", config.Name)
		assert.Equal(t, int64(50), config.TestRunsLimit)
	})

	t.Run("returns pro plan config", func(t *testing.T) {
		sub := &Subscription{Plan: PlanPro}
		config := sub.GetPlanConfig()
		assert.Equal(t, "Pro", config.Name)
		assert.Equal(t, int64(500), config.TestRunsLimit)
	})

	t.Run("returns enterprise plan config", func(t *testing.T) {
		sub := &Subscription{Plan: PlanEnterprise}
		config := sub.GetPlanConfig()
		assert.Equal(t, "Enterprise", config.Name)
		assert.Equal(t, int64(-1), config.TestRunsLimit)
	})

	t.Run("returns free plan for unknown plan", func(t *testing.T) {
		sub := &Subscription{Plan: Plan("unknown")}
		config := sub.GetPlanConfig()
		assert.Equal(t, "Free", config.Name)
	})
}

func TestSubscription_Fields(t *testing.T) {
	now := time.Now()
	periodEnd := now.AddDate(0, 1, 0)
	stripeCustomerID := "cus_123"
	stripeSubID := "sub_123"
	stripePriceID := "price_123"
	paymentMethodID := "pm_123"

	sub := Subscription{
		ID:                     uuid.New(),
		TenantID:               uuid.New(),
		StripeCustomerID:       &stripeCustomerID,
		StripeSubscriptionID:   &stripeSubID,
		StripePriceID:          &stripePriceID,
		Plan:                   PlanPro,
		Status:                 "active",
		BillingCycleAnchor:     &now,
		CurrentPeriodStart:     &now,
		CurrentPeriodEnd:       &periodEnd,
		TrialStart:             nil,
		TrialEnd:               nil,
		CancelAt:               nil,
		CanceledAt:             nil,
		CancelAtPeriodEnd:      false,
		DefaultPaymentMethodID: &paymentMethodID,
		Metadata:               json.RawMessage(`{"key":"value"}`),
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	assert.NotEqual(t, uuid.Nil, sub.ID)
	assert.NotEqual(t, uuid.Nil, sub.TenantID)
	assert.Equal(t, "cus_123", *sub.StripeCustomerID)
	assert.Equal(t, "sub_123", *sub.StripeSubscriptionID)
	assert.Equal(t, "price_123", *sub.StripePriceID)
	assert.Equal(t, PlanPro, sub.Plan)
	assert.Equal(t, "active", sub.Status)
	assert.NotNil(t, sub.CurrentPeriodStart)
	assert.NotNil(t, sub.CurrentPeriodEnd)
	assert.False(t, sub.CancelAtPeriodEnd)
	assert.Equal(t, "pm_123", *sub.DefaultPaymentMethodID)
}

func TestSubscription_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stripeCustomerID := "cus_123"

	sub := Subscription{
		ID:               uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:         uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		StripeCustomerID: &stripeCustomerID,
		Plan:             PlanPro,
		Status:           "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	data, err := json.Marshal(sub)
	assert.NoError(t, err)

	var parsed Subscription
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err)

	assert.Equal(t, sub.ID, parsed.ID)
	assert.Equal(t, sub.TenantID, parsed.TenantID)
	assert.Equal(t, "cus_123", *parsed.StripeCustomerID)
	assert.Equal(t, PlanPro, parsed.Plan)
	assert.Equal(t, "active", parsed.Status)
}

func TestNullableTime(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		result := nullableTime(nil)
		assert.Nil(t, result)
	})

	t.Run("valid timestamp returns time", func(t *testing.T) {
		ts := int64(1700000000)
		result := nullableTime(&ts)
		assert.NotNil(t, result)
		assert.Equal(t, time.Unix(1700000000, 0), *result)
	})

	t.Run("zero timestamp returns epoch", func(t *testing.T) {
		ts := int64(0)
		result := nullableTime(&ts)
		assert.NotNil(t, result)
		assert.Equal(t, time.Unix(0, 0), *result)
	})
}

func TestPlanConfig_Struct(t *testing.T) {
	config := PlanConfig{
		Name:           "Test Plan",
		StripePriceID:  "price_test",
		MonthlyPrice:   4900,
		TestRunsLimit:  100,
		AITokensLimit:  50000,
		SandboxMinutes: 120,
		Features:       []string{"feature1", "feature2"},
		TrialDays:      7,
	}

	assert.Equal(t, "Test Plan", config.Name)
	assert.Equal(t, "price_test", config.StripePriceID)
	assert.Equal(t, int64(4900), config.MonthlyPrice)
	assert.Equal(t, int64(100), config.TestRunsLimit)
	assert.Equal(t, int64(50000), config.AITokensLimit)
	assert.Equal(t, int64(120), config.SandboxMinutes)
	assert.Len(t, config.Features, 2)
	assert.Equal(t, 7, config.TrialDays)
}

func TestSubscription_TrialFields(t *testing.T) {
	now := time.Now()
	trialEnd := now.AddDate(0, 0, 14)

	sub := Subscription{
		Status:     "trialing",
		Plan:       PlanPro,
		TrialStart: &now,
		TrialEnd:   &trialEnd,
	}

	assert.True(t, sub.IsTrial())
	assert.True(t, sub.IsActive())
	assert.NotNil(t, sub.TrialStart)
	assert.NotNil(t, sub.TrialEnd)
}

func TestSubscription_CancellationFields(t *testing.T) {
	now := time.Now()
	cancelAt := now.AddDate(0, 1, 0)

	sub := Subscription{
		Status:            "active",
		CancelAtPeriodEnd: true,
		CancelAt:          &cancelAt,
		CanceledAt:        &now,
	}

	assert.True(t, sub.CancelAtPeriodEnd)
	assert.NotNil(t, sub.CancelAt)
	assert.NotNil(t, sub.CanceledAt)
}
