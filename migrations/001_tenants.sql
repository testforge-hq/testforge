-- Migration: Create tenants table
-- Description: Multi-tenant organizations using the platform

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Plan enum type
CREATE TYPE plan_type AS ENUM ('free', 'pro', 'enterprise');

-- Create tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    plan plan_type NOT NULL DEFAULT 'free',
    settings JSONB NOT NULL DEFAULT '{
        "max_concurrent_runs": 1,
        "max_test_cases_per_run": 20,
        "retention_days": 7,
        "enable_self_healing": false,
        "enable_visual_testing": false,
        "allowed_domains": [],
        "webhook_secret": "",
        "notify_on_complete": false,
        "notify_on_failure": true
    }'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX idx_tenants_slug ON tenants(slug) WHERE deleted_at IS NULL;
CREATE INDEX idx_tenants_plan ON tenants(plan) WHERE deleted_at IS NULL;
CREATE INDEX idx_tenants_created_at ON tenants(created_at);

-- Updated at trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Soft delete helper view
CREATE OR REPLACE VIEW active_tenants AS
SELECT * FROM tenants WHERE deleted_at IS NULL;

COMMENT ON TABLE tenants IS 'Organizations using the TestForge platform';
COMMENT ON COLUMN tenants.slug IS 'URL-safe unique identifier for the tenant';
COMMENT ON COLUMN tenants.settings IS 'Tenant-specific configuration as JSONB';
