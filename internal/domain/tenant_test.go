package domain

import (
	"testing"
)

func TestNewTenant(t *testing.T) {
	tenant := NewTenant("Test Org", "test-org", PlanPro)

	if tenant.ID.String() == "" {
		t.Error("ID should not be empty")
	}
	if tenant.Name != "Test Org" {
		t.Errorf("Name = %q, want 'Test Org'", tenant.Name)
	}
	if tenant.Slug != "test-org" {
		t.Errorf("Slug = %q, want 'test-org'", tenant.Slug)
	}
	if tenant.Plan != PlanPro {
		t.Errorf("Plan = %v, want PlanPro", tenant.Plan)
	}
	if tenant.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if tenant.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
	if tenant.DeletedAt != nil {
		t.Error("DeletedAt should be nil")
	}
}

func TestDefaultTenantSettings(t *testing.T) {
	tests := []struct {
		plan              Plan
		maxConcurrentRuns int
		maxTestCases      int
		retentionDays     int
		selfHealing       bool
		visualTesting     bool
	}{
		{PlanFree, 1, 20, 7, false, false},
		{PlanPro, 5, 100, 90, true, true},
		{PlanEnterprise, 20, 500, 365, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.plan), func(t *testing.T) {
			settings := DefaultTenantSettings(tt.plan)

			if settings.MaxConcurrentRuns != tt.maxConcurrentRuns {
				t.Errorf("MaxConcurrentRuns = %d, want %d", settings.MaxConcurrentRuns, tt.maxConcurrentRuns)
			}
			if settings.MaxTestCasesPerRun != tt.maxTestCases {
				t.Errorf("MaxTestCasesPerRun = %d, want %d", settings.MaxTestCasesPerRun, tt.maxTestCases)
			}
			if settings.RetentionDays != tt.retentionDays {
				t.Errorf("RetentionDays = %d, want %d", settings.RetentionDays, tt.retentionDays)
			}
			if settings.EnableSelfHealing != tt.selfHealing {
				t.Errorf("EnableSelfHealing = %v, want %v", settings.EnableSelfHealing, tt.selfHealing)
			}
			if settings.EnableVisualTesting != tt.visualTesting {
				t.Errorf("EnableVisualTesting = %v, want %v", settings.EnableVisualTesting, tt.visualTesting)
			}
		})
	}
}

func TestNewTenant_DefaultSettings(t *testing.T) {
	// Free plan tenant should get free plan defaults
	tenant := NewTenant("Free User", "free-user", PlanFree)
	if tenant.Settings.MaxConcurrentRuns != 1 {
		t.Errorf("Free tenant MaxConcurrentRuns = %d, want 1", tenant.Settings.MaxConcurrentRuns)
	}

	// Pro plan tenant should get pro plan defaults
	proTenant := NewTenant("Pro User", "pro-user", PlanPro)
	if proTenant.Settings.MaxConcurrentRuns != 5 {
		t.Errorf("Pro tenant MaxConcurrentRuns = %d, want 5", proTenant.Settings.MaxConcurrentRuns)
	}

	// Enterprise plan tenant should get enterprise defaults
	entTenant := NewTenant("Enterprise User", "ent-user", PlanEnterprise)
	if entTenant.Settings.MaxConcurrentRuns != 20 {
		t.Errorf("Enterprise tenant MaxConcurrentRuns = %d, want 20", entTenant.Settings.MaxConcurrentRuns)
	}
}
