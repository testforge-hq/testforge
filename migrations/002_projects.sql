-- Migration: Create projects table
-- Description: Testing projects within a tenant

CREATE TABLE IF NOT EXISTS projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    base_url VARCHAR(2048) NOT NULL,
    settings JSONB NOT NULL DEFAULT '{
        "auth_type": "none",
        "auth_config": {},
        "default_browser": "chromium",
        "default_viewport": "desktop",
        "viewport_width": 1920,
        "viewport_height": 1080,
        "default_timeout_ms": 30000,
        "retry_failed_tests": 2,
        "parallel_workers": 4,
        "capture_screenshots": true,
        "capture_video": true,
        "capture_trace": false,
        "max_crawl_depth": 5,
        "exclude_patterns": [],
        "include_patterns": [],
        "respect_robots_txt": true,
        "custom_headers": {},
        "custom_cookies": []
    }'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Indexes
CREATE INDEX idx_projects_tenant_id ON projects(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_name ON projects(tenant_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_projects_created_at ON projects(created_at);

-- Unique constraint: project name must be unique within a tenant
CREATE UNIQUE INDEX idx_projects_unique_name_per_tenant
    ON projects(tenant_id, name)
    WHERE deleted_at IS NULL;

-- Updated at trigger
CREATE TRIGGER update_projects_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Soft delete helper view
CREATE OR REPLACE VIEW active_projects AS
SELECT p.*
FROM projects p
JOIN active_tenants t ON p.tenant_id = t.id
WHERE p.deleted_at IS NULL;

COMMENT ON TABLE projects IS 'Testing projects within a tenant';
COMMENT ON COLUMN projects.base_url IS 'Root URL for the website being tested';
COMMENT ON COLUMN projects.settings IS 'Project-specific configuration as JSONB';
