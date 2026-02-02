-- Migration: 010_admin_dashboard
-- Description: Add tables for admin dashboard and system metrics
-- Phase: Admin Dashboard

-- Create admin users table (super admins)
CREATE TABLE IF NOT EXISTS admin_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'admin', -- admin, super_admin
    permissions JSONB DEFAULT '[]',

    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(user_id)
);

CREATE INDEX IF NOT EXISTS idx_admin_users_user ON admin_users(user_id);

-- Create system metrics table for historical data
CREATE TABLE IF NOT EXISTS system_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_name VARCHAR(100) NOT NULL,
    metric_value FLOAT NOT NULL,
    metric_type VARCHAR(20) NOT NULL, -- counter, gauge, histogram
    labels JSONB DEFAULT '{}',

    recorded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Partition by time for efficient queries
CREATE INDEX IF NOT EXISTS idx_system_metrics_name_time ON system_metrics(metric_name, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_system_metrics_time ON system_metrics(recorded_at DESC);

-- Create hourly aggregates table
CREATE TABLE IF NOT EXISTS metric_aggregates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_name VARCHAR(100) NOT NULL,
    hour_bucket TIMESTAMP WITH TIME ZONE NOT NULL,

    count BIGINT DEFAULT 0,
    sum FLOAT DEFAULT 0,
    min_val FLOAT,
    max_val FLOAT,
    avg_val FLOAT,
    p50 FLOAT,
    p90 FLOAT,
    p95 FLOAT,
    p99 FLOAT,

    labels JSONB DEFAULT '{}',

    UNIQUE(metric_name, hour_bucket, labels)
);

CREATE INDEX IF NOT EXISTS idx_metric_aggregates_name_hour ON metric_aggregates(metric_name, hour_bucket DESC);

-- Create admin audit log (separate from user audit for sensitive actions)
CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id UUID NOT NULL REFERENCES admin_users(id),

    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID,

    target_tenant_id UUID REFERENCES tenants(id),
    target_user_id UUID REFERENCES users(id),

    details JSONB DEFAULT '{}',
    ip_address INET,
    user_agent TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_admin ON admin_audit_logs(admin_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_action ON admin_audit_logs(action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_time ON admin_audit_logs(created_at DESC);

-- Create platform events table
CREATE TABLE IF NOT EXISTS platform_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) DEFAULT 'info', -- info, warning, error, critical

    tenant_id UUID REFERENCES tenants(id),
    user_id UUID REFERENCES users(id),

    message TEXT NOT NULL,
    details JSONB DEFAULT '{}',

    acknowledged_by UUID REFERENCES admin_users(id),
    acknowledged_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_platform_events_type ON platform_events(event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_platform_events_severity ON platform_events(severity, created_at DESC)
    WHERE severity IN ('error', 'critical');
CREATE INDEX IF NOT EXISTS idx_platform_events_unack ON platform_events(created_at DESC)
    WHERE acknowledged_at IS NULL;

-- Create feature flags table
CREATE TABLE IF NOT EXISTS feature_flags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,

    enabled BOOLEAN DEFAULT false,

    -- Targeting rules
    allowed_tenants UUID[] DEFAULT '{}',
    allowed_users UUID[] DEFAULT '{}',
    percentage_rollout INT DEFAULT 0, -- 0-100

    metadata JSONB DEFAULT '{}',

    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create rate limit overrides table
CREATE TABLE IF NOT EXISTS rate_limit_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,

    endpoint_pattern VARCHAR(200), -- e.g., "/api/v1/*" or NULL for all

    requests_per_minute INT,
    requests_per_hour INT,
    requests_per_day INT,

    reason TEXT,
    expires_at TIMESTAMP WITH TIME ZONE,

    created_by UUID REFERENCES admin_users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(tenant_id, endpoint_pattern)
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_overrides_tenant ON rate_limit_overrides(tenant_id);

-- Function to record a system metric
CREATE OR REPLACE FUNCTION record_metric(
    p_name VARCHAR(100),
    p_value FLOAT,
    p_type VARCHAR(20) DEFAULT 'gauge',
    p_labels JSONB DEFAULT '{}'
)
RETURNS VOID AS $$
BEGIN
    INSERT INTO system_metrics (metric_name, metric_value, metric_type, labels)
    VALUES (p_name, p_value, p_type, p_labels);
END;
$$ LANGUAGE plpgsql;

-- Function to aggregate metrics (run hourly via cron)
CREATE OR REPLACE FUNCTION aggregate_metrics()
RETURNS VOID AS $$
DECLARE
    v_hour_bucket TIMESTAMP WITH TIME ZONE;
BEGIN
    -- Aggregate previous hour's metrics
    v_hour_bucket := DATE_TRUNC('hour', NOW() - INTERVAL '1 hour');

    INSERT INTO metric_aggregates (
        metric_name, hour_bucket, count, sum, min_val, max_val, avg_val
    )
    SELECT
        metric_name,
        v_hour_bucket,
        COUNT(*),
        SUM(metric_value),
        MIN(metric_value),
        MAX(metric_value),
        AVG(metric_value)
    FROM system_metrics
    WHERE recorded_at >= v_hour_bucket
      AND recorded_at < v_hour_bucket + INTERVAL '1 hour'
    GROUP BY metric_name
    ON CONFLICT (metric_name, hour_bucket, labels) DO UPDATE SET
        count = EXCLUDED.count,
        sum = EXCLUDED.sum,
        min_val = EXCLUDED.min_val,
        max_val = EXCLUDED.max_val,
        avg_val = EXCLUDED.avg_val;

    -- Clean up old raw metrics (keep 24 hours)
    DELETE FROM system_metrics
    WHERE recorded_at < NOW() - INTERVAL '24 hours';
END;
$$ LANGUAGE plpgsql;

-- View for admin dashboard overview
CREATE OR REPLACE VIEW admin_dashboard_overview AS
SELECT
    (SELECT COUNT(*) FROM tenants) as total_tenants,
    (SELECT COUNT(DISTINCT tenant_id) FROM test_runs WHERE started_at > NOW() - INTERVAL '30 days') as active_tenants,
    (SELECT COUNT(*) FROM users) as total_users,
    (SELECT COUNT(*) FROM projects) as total_projects,
    (SELECT COUNT(*) FROM test_runs) as total_test_runs,
    (SELECT COUNT(*) FROM test_runs WHERE status IN ('pending', 'running')) as running_tests,
    (SELECT COALESCE(SUM(
        CASE plan
            WHEN 'pro' THEN 99
            WHEN 'enterprise' THEN 499
            ELSE 0
        END
    ), 0) FROM subscriptions WHERE status = 'active') as mrr,
    (SELECT COUNT(*) FROM platform_events WHERE severity IN ('error', 'critical') AND acknowledged_at IS NULL) as unack_alerts;

-- View for tenant health
CREATE OR REPLACE VIEW tenant_health AS
SELECT
    t.id as tenant_id,
    t.name as tenant_name,
    s.plan,
    s.status as subscription_status,
    (SELECT COUNT(*) FROM test_runs WHERE tenant_id = t.id AND started_at > NOW() - INTERVAL '7 days') as runs_last_week,
    (SELECT COUNT(*) FROM test_runs WHERE tenant_id = t.id AND status = 'error' AND started_at > NOW() - INTERVAL '7 days') as errors_last_week,
    (SELECT MAX(started_at) FROM test_runs WHERE tenant_id = t.id) as last_activity,
    CASE
        WHEN (SELECT MAX(started_at) FROM test_runs WHERE tenant_id = t.id) < NOW() - INTERVAL '30 days' THEN 'inactive'
        WHEN (SELECT COUNT(*) FROM test_runs WHERE tenant_id = t.id AND status = 'error' AND started_at > NOW() - INTERVAL '7 days') > 10 THEN 'unhealthy'
        ELSE 'healthy'
    END as health_status
FROM tenants t
LEFT JOIN subscriptions s ON s.tenant_id = t.id;

-- Insert default feature flags
INSERT INTO feature_flags (name, description, enabled) VALUES
    ('ai_self_healing', 'Enable AI-powered self-healing for tests', true),
    ('visual_testing', 'Enable visual regression testing', true),
    ('semantic_caching', 'Enable semantic caching for LLM responses', false),
    ('qdrant_integration', 'Enable Qdrant vector database integration', false),
    ('parallel_execution', 'Enable parallel test execution', true),
    ('beta_features', 'Enable beta features for selected tenants', false)
ON CONFLICT (name) DO NOTHING;

-- Add comments
COMMENT ON TABLE admin_users IS 'Platform administrators';
COMMENT ON TABLE system_metrics IS 'Raw system metrics for monitoring';
COMMENT ON TABLE metric_aggregates IS 'Hourly aggregated metrics';
COMMENT ON TABLE admin_audit_logs IS 'Audit log for admin actions';
COMMENT ON TABLE platform_events IS 'Platform-wide events and alerts';
COMMENT ON TABLE feature_flags IS 'Feature flag configuration';
COMMENT ON TABLE rate_limit_overrides IS 'Custom rate limits per tenant';
