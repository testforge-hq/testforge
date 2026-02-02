-- Migration: Create test_runs and test_cases tables
-- Description: Test execution tracking

-- Run status enum
CREATE TYPE run_status AS ENUM (
    'pending',
    'discovering',
    'designing',
    'automating',
    'executing',
    'healing',
    'reporting',
    'completed',
    'failed',
    'cancelled'
);

-- Test case status enum
CREATE TYPE testcase_status AS ENUM (
    'pending',
    'running',
    'passed',
    'failed',
    'skipped',
    'healed',
    'flaky'
);

-- Priority enum
CREATE TYPE priority_type AS ENUM ('low', 'medium', 'high', 'critical');

-- Create test_runs table
CREATE TABLE IF NOT EXISTS test_runs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status run_status NOT NULL DEFAULT 'pending',
    target_url VARCHAR(2048) NOT NULL,
    workflow_id VARCHAR(255) NOT NULL DEFAULT '',
    workflow_run_id VARCHAR(255) NOT NULL DEFAULT '',
    discovery_result JSONB,
    test_plan JSONB,
    summary JSONB,
    report_url VARCHAR(2048),
    triggered_by VARCHAR(255) NOT NULL DEFAULT 'api',
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Test runs indexes
CREATE INDEX idx_test_runs_tenant_id ON test_runs(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_runs_project_id ON test_runs(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_runs_status ON test_runs(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_runs_workflow_id ON test_runs(workflow_id) WHERE workflow_id != '';
CREATE INDEX idx_test_runs_created_at ON test_runs(created_at DESC);

-- Composite index for listing runs by tenant/project with status
CREATE INDEX idx_test_runs_tenant_status ON test_runs(tenant_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

-- Updated at trigger for test_runs
CREATE TRIGGER update_test_runs_updated_at
    BEFORE UPDATE ON test_runs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create test_cases table
CREATE TABLE IF NOT EXISTS test_cases (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    test_run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(500) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    category VARCHAR(100) NOT NULL DEFAULT 'functional',
    priority priority_type NOT NULL DEFAULT 'medium',
    status testcase_status NOT NULL DEFAULT 'pending',
    steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    script TEXT NOT NULL DEFAULT '',
    original_script TEXT,
    execution_result JSONB,
    healing_history JSONB DEFAULT '[]'::jsonb,
    retry_count INT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Test cases indexes
CREATE INDEX idx_test_cases_test_run_id ON test_cases(test_run_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_cases_tenant_id ON test_cases(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_cases_status ON test_cases(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_cases_priority ON test_cases(priority) WHERE deleted_at IS NULL;
CREATE INDEX idx_test_cases_category ON test_cases(category) WHERE deleted_at IS NULL;

-- Composite index for filtering failed tests
CREATE INDEX idx_test_cases_run_status ON test_cases(test_run_id, status)
    WHERE deleted_at IS NULL;

-- Updated at trigger for test_cases
CREATE TRIGGER update_test_cases_updated_at
    BEFORE UPDATE ON test_cases
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Create reports table
CREATE TABLE IF NOT EXISTS reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    test_run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL DEFAULT 'full',
    format VARCHAR(20) NOT NULL DEFAULT 'html',
    url VARCHAR(2048) NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    generated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Reports indexes
CREATE INDEX idx_reports_test_run_id ON reports(test_run_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_reports_tenant_id ON reports(tenant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_reports_expires_at ON reports(expires_at) WHERE expires_at IS NOT NULL AND deleted_at IS NULL;

-- Updated at trigger for reports
CREATE TRIGGER update_reports_updated_at
    BEFORE UPDATE ON reports
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Views for active records
CREATE OR REPLACE VIEW active_test_runs AS
SELECT tr.*
FROM test_runs tr
JOIN active_projects p ON tr.project_id = p.id
WHERE tr.deleted_at IS NULL;

CREATE OR REPLACE VIEW active_test_cases AS
SELECT tc.*
FROM test_cases tc
JOIN active_test_runs tr ON tc.test_run_id = tr.id
WHERE tc.deleted_at IS NULL;

-- Function to count active runs per tenant (for quota checking)
CREATE OR REPLACE FUNCTION count_active_runs(p_tenant_id UUID)
RETURNS INTEGER AS $$
DECLARE
    run_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO run_count
    FROM test_runs
    WHERE tenant_id = p_tenant_id
      AND deleted_at IS NULL
      AND status NOT IN ('completed', 'failed', 'cancelled');
    RETURN run_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON TABLE test_runs IS 'Individual test execution sessions';
COMMENT ON TABLE test_cases IS 'Individual test cases within a test run';
COMMENT ON TABLE reports IS 'Generated test reports';
COMMENT ON COLUMN test_runs.workflow_id IS 'Temporal workflow ID for tracking';
COMMENT ON COLUMN test_cases.healing_history IS 'History of self-healing attempts';
