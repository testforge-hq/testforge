-- Migration: 004_api_keys
-- Description: Add API keys table with proper security features
-- Phase: 1.2 - Database API Key Validation

-- Create API keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Key identification
    name VARCHAR(255) NOT NULL,
    key_prefix VARCHAR(20) NOT NULL, -- First 8 chars of key for identification
    key_hash VARCHAR(64) NOT NULL,   -- SHA-256 hash of the full key

    -- Permissions and scopes
    scopes JSONB DEFAULT '["read", "write", "execute"]'::jsonb,

    -- Rate limiting (per-key limits, overrides tenant defaults)
    rate_limit_rpm INT DEFAULT NULL, -- NULL means use tenant default

    -- Lifecycle
    expires_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE,
    revoked_reason VARCHAR(500),

    -- Usage tracking
    last_used_at TIMESTAMP WITH TIME ZONE,
    last_used_ip INET,
    usage_count BIGINT DEFAULT 0,

    -- Metadata
    description TEXT,
    created_by UUID, -- Will reference users table when created
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    CONSTRAINT api_keys_key_hash_unique UNIQUE (key_hash),
    CONSTRAINT api_keys_name_tenant_unique UNIQUE (tenant_id, name)
);

-- Index for fast key lookup (most common query)
-- Note: Cannot use NOW() in index predicate as it's not IMMUTABLE
-- Expiration is checked at query time
CREATE INDEX IF NOT EXISTS idx_api_keys_hash_active
    ON api_keys(key_hash)
    WHERE revoked_at IS NULL;

-- Index for listing keys by tenant
CREATE INDEX IF NOT EXISTS idx_api_keys_tenant
    ON api_keys(tenant_id, created_at DESC);

-- Index for finding expired keys (cleanup job)
CREATE INDEX IF NOT EXISTS idx_api_keys_expires
    ON api_keys(expires_at)
    WHERE expires_at IS NOT NULL AND revoked_at IS NULL;

-- Add trigger for updated_at
CREATE OR REPLACE FUNCTION update_api_keys_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_api_keys_updated_at ON api_keys;
CREATE TRIGGER trigger_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION update_api_keys_updated_at();

-- Function to increment usage count (called asynchronously)
CREATE OR REPLACE FUNCTION increment_api_key_usage(
    p_key_hash VARCHAR(64),
    p_ip INET DEFAULT NULL
)
RETURNS VOID AS $$
BEGIN
    UPDATE api_keys
    SET
        usage_count = usage_count + 1,
        last_used_at = NOW(),
        last_used_ip = COALESCE(p_ip, last_used_ip)
    WHERE key_hash = p_key_hash;
END;
$$ LANGUAGE plpgsql;

-- View for active API keys (excludes sensitive hash)
CREATE OR REPLACE VIEW active_api_keys AS
SELECT
    id,
    tenant_id,
    name,
    key_prefix,
    scopes,
    rate_limit_rpm,
    expires_at,
    last_used_at,
    usage_count,
    description,
    created_at
FROM api_keys
WHERE revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW());

-- Add comment for documentation
COMMENT ON TABLE api_keys IS 'Stores API keys for tenant authentication. Keys are hashed with SHA-256 for security.';
COMMENT ON COLUMN api_keys.key_hash IS 'SHA-256 hash of the full API key. Never store the raw key.';
COMMENT ON COLUMN api_keys.key_prefix IS 'First 8 characters of the key for display purposes (e.g., tf_abc1****)';
COMMENT ON COLUMN api_keys.scopes IS 'JSON array of permitted scopes: read, write, execute, admin';
