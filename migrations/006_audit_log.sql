-- Migration: 006_audit_log
-- Description: Add audit logging table for tracking all user actions
-- Phase: 2.3 - Audit Logging

-- Create audit_logs table
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID,

    -- Action details
    action VARCHAR(100) NOT NULL, -- e.g., 'project.create', 'run.start', 'member.invite'
    resource_type VARCHAR(100) NOT NULL, -- e.g., 'project', 'testrun', 'user', 'api_key'
    resource_id UUID,

    -- Change tracking
    changes JSONB, -- {before: {...}, after: {...}}

    -- Request context
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(100),
    api_key_id UUID, -- If action was via API key

    -- Additional context
    metadata JSONB DEFAULT '{}',

    -- Timestamp (immutable)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for querying by tenant and time (most common query)
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_time
    ON audit_logs(tenant_id, created_at DESC);

-- Index for querying by user
CREATE INDEX IF NOT EXISTS idx_audit_logs_user
    ON audit_logs(user_id, created_at DESC)
    WHERE user_id IS NOT NULL;

-- Index for querying by resource
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
    ON audit_logs(resource_type, resource_id, created_at DESC);

-- Index for querying by action type
CREATE INDEX IF NOT EXISTS idx_audit_logs_action
    ON audit_logs(action, created_at DESC);

-- Index for querying by IP (security investigations)
CREATE INDEX IF NOT EXISTS idx_audit_logs_ip
    ON audit_logs(ip_address, created_at DESC)
    WHERE ip_address IS NOT NULL;

-- Partition by month for better performance (optional, for high-volume deployments)
-- CREATE TABLE audit_logs_2024_01 PARTITION OF audit_logs
--     FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');

-- Function to create audit log entry (for use in triggers or direct calls)
CREATE OR REPLACE FUNCTION create_audit_log(
    p_tenant_id UUID,
    p_user_id UUID,
    p_action VARCHAR(100),
    p_resource_type VARCHAR(100),
    p_resource_id UUID,
    p_changes JSONB DEFAULT NULL,
    p_ip_address INET DEFAULT NULL,
    p_user_agent TEXT DEFAULT NULL,
    p_request_id VARCHAR(100) DEFAULT NULL,
    p_api_key_id UUID DEFAULT NULL,
    p_metadata JSONB DEFAULT '{}'
)
RETURNS UUID AS $$
DECLARE
    v_log_id UUID;
BEGIN
    INSERT INTO audit_logs (
        tenant_id, user_id, action, resource_type, resource_id,
        changes, ip_address, user_agent, request_id, api_key_id, metadata
    ) VALUES (
        p_tenant_id, p_user_id, p_action, p_resource_type, p_resource_id,
        p_changes, p_ip_address, p_user_agent, p_request_id, p_api_key_id, p_metadata
    ) RETURNING id INTO v_log_id;

    RETURN v_log_id;
END;
$$ LANGUAGE plpgsql;

-- View for recent audit logs (last 24 hours)
CREATE OR REPLACE VIEW recent_audit_logs AS
SELECT
    al.*,
    u.email as user_email,
    u.display_name as user_name
FROM audit_logs al
LEFT JOIN users u ON u.id = al.user_id
WHERE al.created_at > NOW() - INTERVAL '24 hours'
ORDER BY al.created_at DESC;

-- View for audit log summary by tenant
CREATE OR REPLACE VIEW audit_log_summary AS
SELECT
    tenant_id,
    action,
    resource_type,
    COUNT(*) as count,
    DATE_TRUNC('hour', created_at) as hour
FROM audit_logs
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY tenant_id, action, resource_type, DATE_TRUNC('hour', created_at)
ORDER BY hour DESC, count DESC;

-- Add comments
COMMENT ON TABLE audit_logs IS 'Immutable audit log for tracking all user actions. Never delete or update rows.';
COMMENT ON COLUMN audit_logs.action IS 'Action performed, format: resource.verb (e.g., project.create, run.cancel)';
COMMENT ON COLUMN audit_logs.changes IS 'JSON object with before/after states for modifications';
COMMENT ON FUNCTION create_audit_log IS 'Helper function to create audit log entries with proper validation';

-- Create cleanup function for old logs (retention policy)
CREATE OR REPLACE FUNCTION cleanup_old_audit_logs(retention_days INT DEFAULT 90)
RETURNS BIGINT AS $$
DECLARE
    v_deleted BIGINT;
BEGIN
    DELETE FROM audit_logs
    WHERE created_at < NOW() - (retention_days || ' days')::INTERVAL;

    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RETURN v_deleted;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_old_audit_logs IS 'Cleanup function for removing audit logs older than retention period';
