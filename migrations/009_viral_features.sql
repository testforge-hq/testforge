-- Migration: 009_viral_features
-- Description: Add tables for viral growth features
-- Phase: 4.2-4.4 - Viral Growth Features

-- Create shareable reports table
CREATE TABLE IF NOT EXISTS shareable_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Share access
    share_code VARCHAR(20) NOT NULL UNIQUE,
    title VARCHAR(500) NOT NULL,
    description TEXT,

    -- Access control
    is_public BOOLEAN DEFAULT true,
    password_hash VARCHAR(100),
    expires_at TIMESTAMP WITH TIME ZONE,
    max_views INT,
    view_count INT DEFAULT 0,

    -- Cached report data for fast access
    report_data JSONB,

    -- Tracking
    created_by UUID NOT NULL REFERENCES users(id),
    last_viewed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for shareable reports
CREATE INDEX IF NOT EXISTS idx_shareable_reports_code ON shareable_reports(share_code);
CREATE INDEX IF NOT EXISTS idx_shareable_reports_project ON shareable_reports(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_shareable_reports_expires ON shareable_reports(expires_at) WHERE expires_at IS NOT NULL;

-- Create badge cache table
CREATE TABLE IF NOT EXISTS badge_cache (
    project_id UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL,  -- passing, failing, pending, unknown
    pass_rate FLOAT DEFAULT 0,
    test_count INT DEFAULT 0,
    svg_content TEXT,
    last_updated TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create GitHub integrations table
CREATE TABLE IF NOT EXISTS github_integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- GitHub connection
    installation_id BIGINT,
    owner VARCHAR(200) NOT NULL,
    repo VARCHAR(200) NOT NULL,

    -- Settings
    post_pr_comments BOOLEAN DEFAULT true,
    create_check_runs BOOLEAN DEFAULT true,
    auto_run_on_pr BOOLEAN DEFAULT false,
    branches TEXT[] DEFAULT ARRAY['main', 'master'],

    -- Tokens (encrypted)
    access_token_encrypted BYTEA,
    refresh_token_encrypted BYTEA,
    token_expires_at TIMESTAMP WITH TIME ZONE,

    -- Status
    is_active BOOLEAN DEFAULT true,
    last_sync_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    UNIQUE(project_id, owner, repo)
);

CREATE INDEX IF NOT EXISTS idx_github_integrations_project ON github_integrations(project_id);
CREATE INDEX IF NOT EXISTS idx_github_integrations_repo ON github_integrations(owner, repo);

-- Create webhook deliveries table (for tracking GitHub webhooks)
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id UUID NOT NULL REFERENCES github_integrations(id) ON DELETE CASCADE,

    -- Webhook info
    event_type VARCHAR(100) NOT NULL,  -- push, pull_request, etc.
    delivery_id VARCHAR(100),          -- GitHub's delivery ID
    payload JSONB,

    -- Processing status
    status VARCHAR(50) DEFAULT 'pending',  -- pending, processed, failed
    processed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT,
    retry_count INT DEFAULT 0,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_integration ON webhook_deliveries(integration_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status) WHERE status = 'pending';

-- Create PR comment history table
CREATE TABLE IF NOT EXISTS pr_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id UUID NOT NULL REFERENCES github_integrations(id) ON DELETE CASCADE,
    run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,

    -- PR info
    pr_number INT NOT NULL,
    commit_sha VARCHAR(40),
    github_comment_id BIGINT,

    -- Content
    comment_body TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pr_comments_integration ON pr_comments(integration_id, pr_number);
CREATE INDEX IF NOT EXISTS idx_pr_comments_run ON pr_comments(run_id);

-- Create referral tracking table
CREATE TABLE IF NOT EXISTS referrals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    referrer_tenant_id UUID NOT NULL REFERENCES tenants(id),

    -- Referral code
    code VARCHAR(20) NOT NULL UNIQUE,

    -- Tracking
    clicks INT DEFAULT 0,
    signups INT DEFAULT 0,
    conversions INT DEFAULT 0,  -- Paid conversions

    -- Rewards
    reward_type VARCHAR(50),  -- free_month, discount, credits
    reward_claimed BOOLEAN DEFAULT false,

    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_referrals_tenant ON referrals(referrer_tenant_id);
CREATE INDEX IF NOT EXISTS idx_referrals_code ON referrals(code);

-- Create referred users table
CREATE TABLE IF NOT EXISTS referred_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    referral_id UUID NOT NULL REFERENCES referrals(id),
    referred_tenant_id UUID NOT NULL REFERENCES tenants(id),

    -- Status
    signed_up_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    converted_at TIMESTAMP WITH TIME ZONE,  -- When they became paid

    UNIQUE(referral_id, referred_tenant_id)
);

-- Add triggers for updated_at
CREATE OR REPLACE FUNCTION update_github_integrations_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_github_integrations_updated_at ON github_integrations;
CREATE TRIGGER trigger_github_integrations_updated_at
    BEFORE UPDATE ON github_integrations
    FOR EACH ROW
    EXECUTE FUNCTION update_github_integrations_updated_at();

CREATE OR REPLACE FUNCTION update_pr_comments_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_pr_comments_updated_at ON pr_comments;
CREATE TRIGGER trigger_pr_comments_updated_at
    BEFORE UPDATE ON pr_comments
    FOR EACH ROW
    EXECUTE FUNCTION update_pr_comments_updated_at();

-- Function to update badge cache
CREATE OR REPLACE FUNCTION refresh_badge_cache(p_project_id UUID)
RETURNS VOID AS $$
DECLARE
    v_total INT;
    v_passed INT;
    v_status VARCHAR(20);
    v_pass_rate FLOAT;
BEGIN
    -- Get latest run stats
    SELECT
        COALESCE(SUM(total_tests), 0),
        COALESCE(SUM(passed_tests), 0)
    INTO v_total, v_passed
    FROM test_runs
    WHERE project_id = p_project_id
      AND status = 'completed'
      AND ended_at > NOW() - INTERVAL '30 days';

    -- Calculate pass rate
    IF v_total > 0 THEN
        v_pass_rate := v_passed::FLOAT / v_total;
        IF v_pass_rate >= 0.95 THEN
            v_status := 'passing';
        ELSIF v_pass_rate >= 0.50 THEN
            v_status := 'failing';
        ELSE
            v_status := 'failing';
        END IF;
    ELSE
        v_status := 'unknown';
        v_pass_rate := 0;
    END IF;

    -- Upsert badge cache
    INSERT INTO badge_cache (project_id, status, pass_rate, test_count, last_updated)
    VALUES (p_project_id, v_status, v_pass_rate, v_total, NOW())
    ON CONFLICT (project_id) DO UPDATE SET
        status = EXCLUDED.status,
        pass_rate = EXCLUDED.pass_rate,
        test_count = EXCLUDED.test_count,
        last_updated = NOW();
END;
$$ LANGUAGE plpgsql;

-- Trigger to refresh badge on test run completion
CREATE OR REPLACE FUNCTION trigger_refresh_badge()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status = 'completed' AND (OLD.status IS NULL OR OLD.status != 'completed') THEN
        PERFORM refresh_badge_cache(NEW.project_id);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_test_run_badge_refresh ON test_runs;
CREATE TRIGGER trigger_test_run_badge_refresh
    AFTER INSERT OR UPDATE ON test_runs
    FOR EACH ROW
    EXECUTE FUNCTION trigger_refresh_badge();

-- View for viral metrics
CREATE OR REPLACE VIEW viral_metrics AS
SELECT
    t.id as tenant_id,
    t.name as tenant_name,
    (SELECT COUNT(*) FROM shareable_reports sr WHERE sr.tenant_id = t.id) as shared_reports,
    (SELECT COALESCE(SUM(view_count), 0) FROM shareable_reports sr WHERE sr.tenant_id = t.id) as total_views,
    (SELECT COUNT(*) FROM github_integrations gi WHERE gi.tenant_id = t.id AND gi.is_active) as github_integrations,
    (SELECT COUNT(*) FROM referrals r WHERE r.referrer_tenant_id = t.id) as referral_codes,
    (SELECT COALESCE(SUM(signups), 0) FROM referrals r WHERE r.referrer_tenant_id = t.id) as referral_signups
FROM tenants t;

-- Add comments
COMMENT ON TABLE shareable_reports IS 'Public shareable test reports for viral distribution';
COMMENT ON TABLE badge_cache IS 'Cached badge data for fast SVG generation';
COMMENT ON TABLE github_integrations IS 'GitHub repository integrations for CI/CD';
COMMENT ON TABLE webhook_deliveries IS 'Track GitHub webhook deliveries';
COMMENT ON TABLE pr_comments IS 'History of PR comments posted';
COMMENT ON TABLE referrals IS 'Referral program tracking';
COMMENT ON TABLE referred_users IS 'Users who signed up via referral';
COMMENT ON FUNCTION refresh_badge_cache IS 'Updates badge cache for a project';
