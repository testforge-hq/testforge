// Package admin provides admin dashboard functionality
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DashboardService provides admin dashboard data
type DashboardService struct {
	db     *sqlx.DB
	redis  *redis.Client
	logger *zap.Logger
}

// NewDashboardService creates a new dashboard service
func NewDashboardService(db *sqlx.DB, redis *redis.Client, logger *zap.Logger) *DashboardService {
	return &DashboardService{
		db:     db,
		redis:  redis,
		logger: logger,
	}
}

// OverviewStats provides high-level platform statistics
type OverviewStats struct {
	TotalTenants      int64   `json:"total_tenants"`
	ActiveTenants     int64   `json:"active_tenants"`      // Active in last 30 days
	TotalUsers        int64   `json:"total_users"`
	TotalProjects     int64   `json:"total_projects"`
	TotalTestRuns     int64   `json:"total_test_runs"`
	RunningTests      int64   `json:"running_tests"`
	TotalTestsToday   int64   `json:"total_tests_today"`
	PassRate          float64 `json:"pass_rate"`           // Overall pass rate
	MRR               float64 `json:"mrr"`                 // Monthly Recurring Revenue
	ARR               float64 `json:"arr"`                 // Annual Recurring Revenue
	TrialConversions  float64 `json:"trial_conversions"`   // Trial to paid conversion rate
	Churn             float64 `json:"churn"`               // Monthly churn rate
	LastUpdated       time.Time `json:"last_updated"`
}

// GetOverview returns high-level platform statistics
func (ds *DashboardService) GetOverview(ctx context.Context) (*OverviewStats, error) {
	// Try cache first
	if ds.redis != nil {
		cached, err := ds.redis.Get(ctx, "admin:overview").Bytes()
		if err == nil {
			var stats OverviewStats
			if json.Unmarshal(cached, &stats) == nil && time.Since(stats.LastUpdated) < 5*time.Minute {
				return &stats, nil
			}
		}
	}

	stats := &OverviewStats{LastUpdated: time.Now()}

	// Get tenant stats
	ds.db.GetContext(ctx, &stats.TotalTenants, `SELECT COUNT(*) FROM tenants`)
	ds.db.GetContext(ctx, &stats.ActiveTenants, `
		SELECT COUNT(DISTINCT tenant_id) FROM test_runs
		WHERE started_at > NOW() - INTERVAL '30 days'`)

	// Get user stats
	ds.db.GetContext(ctx, &stats.TotalUsers, `SELECT COUNT(*) FROM users`)

	// Get project stats
	ds.db.GetContext(ctx, &stats.TotalProjects, `SELECT COUNT(*) FROM projects`)

	// Get test run stats
	ds.db.GetContext(ctx, &stats.TotalTestRuns, `SELECT COUNT(*) FROM test_runs`)
	ds.db.GetContext(ctx, &stats.RunningTests, `
		SELECT COUNT(*) FROM test_runs WHERE status IN ('pending', 'running')`)
	ds.db.GetContext(ctx, &stats.TotalTestsToday, `
		SELECT COUNT(*) FROM test_runs WHERE started_at > CURRENT_DATE`)

	// Calculate pass rate
	var passed, total int64
	ds.db.GetContext(ctx, &passed, `
		SELECT COALESCE(SUM(passed_tests), 0) FROM test_runs
		WHERE started_at > NOW() - INTERVAL '30 days'`)
	ds.db.GetContext(ctx, &total, `
		SELECT COALESCE(SUM(total_tests), 0) FROM test_runs
		WHERE started_at > NOW() - INTERVAL '30 days'`)
	if total > 0 {
		stats.PassRate = float64(passed) / float64(total) * 100
	}

	// Get revenue stats
	ds.db.GetContext(ctx, &stats.MRR, `
		SELECT COALESCE(SUM(
			CASE plan
				WHEN 'pro' THEN 99
				WHEN 'enterprise' THEN 499
				ELSE 0
			END
		), 0) FROM subscriptions WHERE status = 'active'`)
	stats.ARR = stats.MRR * 12

	// Calculate trial conversion rate
	var trialSignups, trialConverted int64
	ds.db.GetContext(ctx, &trialSignups, `
		SELECT COUNT(*) FROM subscriptions WHERE trial_start IS NOT NULL`)
	ds.db.GetContext(ctx, &trialConverted, `
		SELECT COUNT(*) FROM subscriptions
		WHERE trial_start IS NOT NULL AND plan != 'free' AND status = 'active'`)
	if trialSignups > 0 {
		stats.TrialConversions = float64(trialConverted) / float64(trialSignups) * 100
	}

	// Cache the results
	if ds.redis != nil {
		data, _ := json.Marshal(stats)
		ds.redis.Set(ctx, "admin:overview", data, 5*time.Minute)
	}

	return stats, nil
}

// TenantDetails provides detailed tenant information
type TenantDetails struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	Name            string     `json:"name" db:"name"`
	Slug            string     `json:"slug" db:"slug"`
	Plan            string     `json:"plan" db:"plan"`
	Status          string     `json:"status" db:"status"`
	UserCount       int        `json:"user_count" db:"user_count"`
	ProjectCount    int        `json:"project_count" db:"project_count"`
	TestRunCount    int        `json:"test_run_count" db:"test_run_count"`
	TestRunsToday   int        `json:"test_runs_today" db:"test_runs_today"`
	AITokensUsed    int64      `json:"ai_tokens_used" db:"ai_tokens_used"`
	SandboxMinutes  int64      `json:"sandbox_minutes" db:"sandbox_minutes"`
	MonthlyCost     float64    `json:"monthly_cost" db:"monthly_cost"`
	LastActiveAt    *time.Time `json:"last_active_at" db:"last_active_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

// ListTenants returns a paginated list of tenants with details
func (ds *DashboardService) ListTenants(ctx context.Context, params ListParams) ([]TenantDetails, int64, error) {
	var total int64
	ds.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM tenants`)

	query := `
		SELECT
			t.id, t.name, t.slug,
			COALESCE(s.plan, 'free') as plan,
			COALESCE(s.status, 'active') as status,
			(SELECT COUNT(*) FROM users u JOIN tenant_memberships tm ON tm.user_id = u.id WHERE tm.tenant_id = t.id) as user_count,
			(SELECT COUNT(*) FROM projects WHERE tenant_id = t.id) as project_count,
			(SELECT COUNT(*) FROM test_runs WHERE tenant_id = t.id) as test_run_count,
			(SELECT COUNT(*) FROM test_runs WHERE tenant_id = t.id AND started_at > CURRENT_DATE) as test_runs_today,
			COALESCE((SELECT SUM(quantity) FROM usage_records WHERE tenant_id = t.id AND metric = 'ai_tokens' AND period_start > DATE_TRUNC('month', NOW())), 0) as ai_tokens_used,
			COALESCE((SELECT SUM(quantity) FROM usage_records WHERE tenant_id = t.id AND metric = 'sandbox_minutes' AND period_start > DATE_TRUNC('month', NOW())), 0) as sandbox_minutes,
			CASE COALESCE(s.plan, 'free')
				WHEN 'pro' THEN 99
				WHEN 'enterprise' THEN 499
				ELSE 0
			END as monthly_cost,
			(SELECT MAX(started_at) FROM test_runs WHERE tenant_id = t.id) as last_active_at,
			t.created_at
		FROM tenants t
		LEFT JOIN subscriptions s ON s.tenant_id = t.id
		ORDER BY t.created_at DESC
		LIMIT $1 OFFSET $2`

	var tenants []TenantDetails
	err := ds.db.SelectContext(ctx, &tenants, query, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, err
	}

	return tenants, total, nil
}

// ListParams holds pagination parameters
type ListParams struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Sort   string `json:"sort"`
	Order  string `json:"order"`
	Search string `json:"search"`
}

// UserDetails provides detailed user information
type UserDetails struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	Email        string     `json:"email" db:"email"`
	Name         string     `json:"name" db:"name"`
	TenantName   string     `json:"tenant_name" db:"tenant_name"`
	Role         string     `json:"role" db:"role"`
	Status       string     `json:"status" db:"status"`
	LastLoginAt  *time.Time `json:"last_login_at" db:"last_login_at"`
	LoginCount   int        `json:"login_count" db:"login_count"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// ListUsers returns a paginated list of users
func (ds *DashboardService) ListUsers(ctx context.Context, params ListParams) ([]UserDetails, int64, error) {
	var total int64
	ds.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM users`)

	query := `
		SELECT
			u.id, u.email, u.name,
			COALESCE(t.name, 'Unknown') as tenant_name,
			COALESCE(r.name, 'user') as role,
			CASE WHEN u.email_verified THEN 'active' ELSE 'pending' END as status,
			u.last_login_at,
			u.login_count,
			u.created_at
		FROM users u
		LEFT JOIN tenant_memberships tm ON tm.user_id = u.id
		LEFT JOIN tenants t ON t.id = tm.tenant_id
		LEFT JOIN roles r ON r.id = tm.role_id
		ORDER BY u.created_at DESC
		LIMIT $1 OFFSET $2`

	var users []UserDetails
	err := ds.db.SelectContext(ctx, &users, query, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// RunningTestInfo provides info about a currently running test
type RunningTestInfo struct {
	ID           uuid.UUID `json:"id" db:"id"`
	TenantName   string    `json:"tenant_name" db:"tenant_name"`
	ProjectName  string    `json:"project_name" db:"project_name"`
	Status       string    `json:"status" db:"status"`
	Progress     int       `json:"progress" db:"progress"`
	TotalTests   int       `json:"total_tests" db:"total_tests"`
	PassedTests  int       `json:"passed_tests" db:"passed_tests"`
	FailedTests  int       `json:"failed_tests" db:"failed_tests"`
	StartedAt    time.Time `json:"started_at" db:"started_at"`
	Duration     string    `json:"duration"`
}

// GetRunningTests returns currently running tests
func (ds *DashboardService) GetRunningTests(ctx context.Context) ([]RunningTestInfo, error) {
	query := `
		SELECT
			tr.id, t.name as tenant_name, p.name as project_name,
			tr.status, tr.progress,
			tr.total_tests, tr.passed_tests, tr.failed_tests,
			tr.started_at
		FROM test_runs tr
		JOIN projects p ON p.id = tr.project_id
		JOIN tenants t ON t.id = tr.tenant_id
		WHERE tr.status IN ('pending', 'running')
		ORDER BY tr.started_at DESC`

	var runs []RunningTestInfo
	err := ds.db.SelectContext(ctx, &runs, query)
	if err != nil {
		return nil, err
	}

	// Calculate duration
	for i := range runs {
		runs[i].Duration = time.Since(runs[i].StartedAt).Round(time.Second).String()
	}

	return runs, nil
}

// CostBreakdown provides cost analytics
type CostBreakdown struct {
	Date         string  `json:"date" db:"date"`
	AITokensCost float64 `json:"ai_tokens_cost"`
	SandboxCost  float64 `json:"sandbox_cost"`
	TotalCost    float64 `json:"total_cost"`
	Revenue      float64 `json:"revenue"`
	Margin       float64 `json:"margin"`
}

// GetCostAnalytics returns cost analytics for the past N days
func (ds *DashboardService) GetCostAnalytics(ctx context.Context, days int) ([]CostBreakdown, error) {
	query := `
		WITH daily_usage AS (
			SELECT
				DATE(created_at) as date,
				SUM(CASE WHEN metric = 'ai_tokens' THEN quantity ELSE 0 END) as ai_tokens,
				SUM(CASE WHEN metric = 'sandbox_minutes' THEN quantity ELSE 0 END) as sandbox_minutes
			FROM usage_records
			WHERE created_at > NOW() - INTERVAL '%d days'
			GROUP BY DATE(created_at)
		),
		daily_revenue AS (
			SELECT
				DATE(created_at) as date,
				SUM(
					CASE plan
						WHEN 'pro' THEN 99.0 / 30
						WHEN 'enterprise' THEN 499.0 / 30
						ELSE 0
					END
				) as revenue
			FROM subscriptions
			WHERE status = 'active' AND created_at > NOW() - INTERVAL '%d days'
			GROUP BY DATE(created_at)
		)
		SELECT
			du.date::text,
			(du.ai_tokens / 1000000.0) * 3.0 as ai_tokens_cost,
			(du.sandbox_minutes / 60.0) * 0.05 as sandbox_cost,
			((du.ai_tokens / 1000000.0) * 3.0) + ((du.sandbox_minutes / 60.0) * 0.05) as total_cost,
			COALESCE(dr.revenue, 0) as revenue
		FROM daily_usage du
		LEFT JOIN daily_revenue dr ON dr.date = du.date
		ORDER BY du.date DESC`

	var costs []CostBreakdown
	err := ds.db.SelectContext(ctx, &costs, fmt.Sprintf(query, days, days))
	if err != nil {
		return nil, err
	}

	// Calculate margins
	for i := range costs {
		if costs[i].Revenue > 0 {
			costs[i].Margin = ((costs[i].Revenue - costs[i].TotalCost) / costs[i].Revenue) * 100
		}
	}

	return costs, nil
}

// BillingOverview provides billing analytics
type BillingOverview struct {
	TotalMRR           float64 `json:"total_mrr"`
	TotalARR           float64 `json:"total_arr"`
	FreeTierCount      int     `json:"free_tier_count"`
	ProCount           int     `json:"pro_count"`
	EnterpriseCount    int     `json:"enterprise_count"`
	TrialCount         int     `json:"trial_count"`
	ChurnedThisMonth   int     `json:"churned_this_month"`
	NewSubscriptions   int     `json:"new_subscriptions"`
	UpgradesThisMonth  int     `json:"upgrades_this_month"`
}

// GetBillingOverview returns billing analytics
func (ds *DashboardService) GetBillingOverview(ctx context.Context) (*BillingOverview, error) {
	overview := &BillingOverview{}

	// Plan counts
	ds.db.GetContext(ctx, &overview.FreeTierCount, `
		SELECT COUNT(*) FROM subscriptions WHERE plan = 'free' AND status = 'active'`)
	ds.db.GetContext(ctx, &overview.ProCount, `
		SELECT COUNT(*) FROM subscriptions WHERE plan = 'pro' AND status = 'active'`)
	ds.db.GetContext(ctx, &overview.EnterpriseCount, `
		SELECT COUNT(*) FROM subscriptions WHERE plan = 'enterprise' AND status = 'active'`)
	ds.db.GetContext(ctx, &overview.TrialCount, `
		SELECT COUNT(*) FROM subscriptions WHERE status = 'trialing'`)

	// Calculate MRR
	overview.TotalMRR = float64(overview.ProCount)*99 + float64(overview.EnterpriseCount)*499
	overview.TotalARR = overview.TotalMRR * 12

	// This month's activity
	ds.db.GetContext(ctx, &overview.ChurnedThisMonth, `
		SELECT COUNT(*) FROM subscriptions
		WHERE canceled_at > DATE_TRUNC('month', NOW())`)
	ds.db.GetContext(ctx, &overview.NewSubscriptions, `
		SELECT COUNT(*) FROM subscriptions
		WHERE created_at > DATE_TRUNC('month', NOW()) AND plan != 'free'`)

	return overview, nil
}

// SystemHealth provides system health information
type SystemHealth struct {
	DatabaseStatus     string  `json:"database_status"`
	DatabaseLatencyMs  float64 `json:"database_latency_ms"`
	RedisStatus        string  `json:"redis_status"`
	RedisLatencyMs     float64 `json:"redis_latency_ms"`
	TemporalStatus     string  `json:"temporal_status"`
	WorkerCount        int     `json:"worker_count"`
	QueueDepth         int     `json:"queue_depth"`
	ErrorRate          float64 `json:"error_rate"`
	AvgResponseTimeMs  float64 `json:"avg_response_time_ms"`
}

// GetSystemHealth returns system health status
func (ds *DashboardService) GetSystemHealth(ctx context.Context) (*SystemHealth, error) {
	health := &SystemHealth{
		DatabaseStatus: "healthy",
		RedisStatus:    "healthy",
		TemporalStatus: "healthy",
	}

	// Check database
	start := time.Now()
	if err := ds.db.PingContext(ctx); err != nil {
		health.DatabaseStatus = "unhealthy"
	}
	health.DatabaseLatencyMs = float64(time.Since(start).Microseconds()) / 1000

	// Check Redis
	if ds.redis != nil {
		start = time.Now()
		if err := ds.redis.Ping(ctx).Err(); err != nil {
			health.RedisStatus = "unhealthy"
		}
		health.RedisLatencyMs = float64(time.Since(start).Microseconds()) / 1000
	}

	// Get error rate from recent requests
	var errors, total int64
	ds.db.GetContext(ctx, &total, `
		SELECT COUNT(*) FROM test_runs WHERE started_at > NOW() - INTERVAL '1 hour'`)
	ds.db.GetContext(ctx, &errors, `
		SELECT COUNT(*) FROM test_runs WHERE started_at > NOW() - INTERVAL '1 hour' AND status = 'error'`)
	if total > 0 {
		health.ErrorRate = float64(errors) / float64(total) * 100
	}

	return health, nil
}

// ActivityLogEntry represents an activity log entry
type ActivityLogEntry struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	TenantName   string                 `json:"tenant_name" db:"tenant_name"`
	UserEmail    string                 `json:"user_email" db:"user_email"`
	Action       string                 `json:"action" db:"action"`
	ResourceType string                 `json:"resource_type" db:"resource_type"`
	ResourceID   *uuid.UUID             `json:"resource_id" db:"resource_id"`
	Changes      map[string]interface{} `json:"changes"`
	IPAddress    string                 `json:"ip_address" db:"ip_address"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

// GetRecentActivity returns recent platform activity
func (ds *DashboardService) GetRecentActivity(ctx context.Context, limit int) ([]ActivityLogEntry, error) {
	query := `
		SELECT
			al.id,
			COALESCE(t.name, 'System') as tenant_name,
			COALESCE(u.email, 'system') as user_email,
			al.action, al.resource_type, al.resource_id,
			al.ip_address, al.created_at
		FROM audit_logs al
		LEFT JOIN tenants t ON t.id = al.tenant_id
		LEFT JOIN users u ON u.id = al.user_id
		ORDER BY al.created_at DESC
		LIMIT $1`

	var entries []ActivityLogEntry
	err := ds.db.SelectContext(ctx, &entries, query, limit)
	return entries, err
}

