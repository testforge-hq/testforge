-- Migration: 005_rbac
-- Description: Add RBAC (Role-Based Access Control) with users, roles, and memberships
-- Phase: 2.1 - RBAC Implementation

-- Create auth provider enum
CREATE TYPE auth_provider AS ENUM ('local', 'google', 'okta', 'azure_ad', 'github', 'saml');

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity
    email VARCHAR(255) NOT NULL,
    email_verified BOOLEAN DEFAULT false,
    email_verified_at TIMESTAMP WITH TIME ZONE,

    -- Authentication
    password_hash VARCHAR(255), -- NULL for SSO users
    auth_provider auth_provider DEFAULT 'local',
    auth_provider_id VARCHAR(255), -- External provider user ID

    -- Profile
    display_name VARCHAR(255),
    avatar_url TEXT,

    -- Security
    mfa_enabled BOOLEAN DEFAULT false,
    mfa_secret VARCHAR(255),
    last_login_at TIMESTAMP WITH TIME ZONE,
    last_login_ip INET,
    failed_login_attempts INT DEFAULT 0,
    locked_until TIMESTAMP WITH TIME ZONE,

    -- Lifecycle
    is_active BOOLEAN DEFAULT true,
    deactivated_at TIMESTAMP WITH TIME ZONE,
    deactivated_reason TEXT,

    -- Metadata
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_provider_id_unique UNIQUE (auth_provider, auth_provider_id)
);

-- Indexes for users
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_users_provider ON users(auth_provider, auth_provider_id) WHERE is_active = true;

-- Create roles table
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE,

    -- Role definition
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(255),
    description TEXT,

    -- Permissions (JSON array of permission strings)
    permissions JSONB NOT NULL DEFAULT '[]',

    -- System role flag (cannot be modified or deleted)
    is_system BOOLEAN DEFAULT false,

    -- Hierarchy (for role inheritance)
    parent_role_id UUID REFERENCES roles(id) ON DELETE SET NULL,

    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    -- System roles have no tenant_id, tenant roles have tenant_id
    CONSTRAINT roles_name_unique UNIQUE NULLS NOT DISTINCT (tenant_id, name)
);

-- Indexes for roles
CREATE INDEX IF NOT EXISTS idx_roles_tenant ON roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_roles_system ON roles(is_system) WHERE is_system = true;

-- Create tenant memberships table (users belong to tenants with specific roles)
CREATE TABLE IF NOT EXISTS tenant_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,

    -- Status
    is_active BOOLEAN DEFAULT true,

    -- Invitation tracking
    invited_by UUID REFERENCES users(id),
    invited_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    accepted_at TIMESTAMP WITH TIME ZONE,

    -- Metadata
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    CONSTRAINT memberships_unique UNIQUE (user_id, tenant_id)
);

-- Indexes for memberships
CREATE INDEX IF NOT EXISTS idx_memberships_user ON tenant_memberships(user_id) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_memberships_tenant ON tenant_memberships(tenant_id) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_memberships_role ON tenant_memberships(role_id);

-- Insert system roles
INSERT INTO roles (id, name, display_name, description, permissions, is_system) VALUES
    ('00000000-0000-0000-0000-000000000001', 'admin', 'Administrator', 'Full access to all resources', '["*"]', true),
    ('00000000-0000-0000-0000-000000000002', 'developer', 'Developer', 'Create and manage projects and test runs', '["project:*", "run:*", "report:read", "api_key:read"]', true),
    ('00000000-0000-0000-0000-000000000003', 'viewer', 'Viewer', 'View-only access to projects and reports', '["project:read", "run:read", "report:read"]', true),
    ('00000000-0000-0000-0000-000000000004', 'billing_admin', 'Billing Administrator', 'Manage billing and subscriptions', '["billing:*", "tenant:read"]', true)
ON CONFLICT (tenant_id, name) DO NOTHING;

-- Add triggers for updated_at
CREATE OR REPLACE FUNCTION update_users_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_roles_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_memberships_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_users_updated_at ON users;
CREATE TRIGGER trigger_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_users_updated_at();

DROP TRIGGER IF EXISTS trigger_roles_updated_at ON roles;
CREATE TRIGGER trigger_roles_updated_at
    BEFORE UPDATE ON roles
    FOR EACH ROW
    EXECUTE FUNCTION update_roles_updated_at();

DROP TRIGGER IF EXISTS trigger_memberships_updated_at ON tenant_memberships;
CREATE TRIGGER trigger_memberships_updated_at
    BEFORE UPDATE ON tenant_memberships
    FOR EACH ROW
    EXECUTE FUNCTION update_memberships_updated_at();

-- Function to check if a user has a permission in a tenant
CREATE OR REPLACE FUNCTION user_has_permission(
    p_user_id UUID,
    p_tenant_id UUID,
    p_permission VARCHAR(255)
)
RETURNS BOOLEAN AS $$
DECLARE
    v_permissions JSONB;
    v_permission TEXT;
BEGIN
    -- Get user's permissions for the tenant
    SELECT r.permissions INTO v_permissions
    FROM tenant_memberships tm
    JOIN roles r ON r.id = tm.role_id
    WHERE tm.user_id = p_user_id
      AND tm.tenant_id = p_tenant_id
      AND tm.is_active = true;

    IF v_permissions IS NULL THEN
        RETURN false;
    END IF;

    -- Check for wildcard permission
    IF v_permissions ? '*' THEN
        RETURN true;
    END IF;

    -- Check for exact permission match
    IF v_permissions ? p_permission THEN
        RETURN true;
    END IF;

    -- Check for wildcard in permission category (e.g., 'project:*' matches 'project:read')
    FOR v_permission IN SELECT jsonb_array_elements_text(v_permissions)
    LOOP
        IF v_permission LIKE '%:*' THEN
            IF p_permission LIKE split_part(v_permission, ':', 1) || ':%' THEN
                RETURN true;
            END IF;
        END IF;
    END LOOP;

    RETURN false;
END;
$$ LANGUAGE plpgsql;

-- View for active memberships with role details
CREATE OR REPLACE VIEW user_memberships_view AS
SELECT
    tm.id,
    tm.user_id,
    u.email as user_email,
    u.display_name as user_name,
    tm.tenant_id,
    t.name as tenant_name,
    t.slug as tenant_slug,
    tm.role_id,
    r.name as role_name,
    r.display_name as role_display_name,
    r.permissions,
    tm.is_active,
    tm.accepted_at,
    tm.created_at
FROM tenant_memberships tm
JOIN users u ON u.id = tm.user_id
JOIN tenants t ON t.id = tm.tenant_id
JOIN roles r ON r.id = tm.role_id
WHERE tm.is_active = true
  AND u.is_active = true
  AND t.deleted_at IS NULL;

-- Add comments
COMMENT ON TABLE users IS 'Application users who can access tenants via memberships';
COMMENT ON TABLE roles IS 'Permission roles that can be assigned to users within tenants';
COMMENT ON TABLE tenant_memberships IS 'User membership in tenants with assigned roles';
COMMENT ON FUNCTION user_has_permission IS 'Check if a user has a specific permission within a tenant';
