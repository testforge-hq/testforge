-- Migration: 008_intelligence
-- Description: Add tables for AI learning and knowledge base
-- Phase: 4.1 - AI Learning Network Effects (THE MOAT)

-- Knowledge entry types enum
CREATE TYPE knowledge_type AS ENUM (
    'healing',
    'element',
    'flow',
    'error'
);

-- Create knowledge entries table (cross-tenant learning)
CREATE TABLE IF NOT EXISTS knowledge_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Classification
    type knowledge_type NOT NULL,
    category VARCHAR(100),           -- Industry or element type
    pattern VARCHAR(500) NOT NULL,   -- Anonymized pattern description
    pattern_hash VARCHAR(64) NOT NULL UNIQUE, -- SHA256 hash for deduplication

    -- Usage statistics
    success_count BIGINT DEFAULT 0,
    failure_count BIGINT DEFAULT 0,
    confidence FLOAT DEFAULT 0.0,    -- success_count / (success_count + failure_count)

    -- Pattern details (anonymized)
    metadata JSONB DEFAULT '{}',

    -- Contribution tracking (number of unique tenants)
    contributed_by INT DEFAULT 1,

    -- Timestamps
    last_used_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for knowledge entries
CREATE INDEX IF NOT EXISTS idx_knowledge_type ON knowledge_entries(type);
CREATE INDEX IF NOT EXISTS idx_knowledge_type_category ON knowledge_entries(type, category);
CREATE INDEX IF NOT EXISTS idx_knowledge_confidence ON knowledge_entries(confidence DESC) WHERE confidence > 0.5;
CREATE INDEX IF NOT EXISTS idx_knowledge_success ON knowledge_entries(success_count DESC);

-- Create knowledge contributions table (tracks which tenants contributed)
CREATE TABLE IF NOT EXISTS knowledge_contributions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pattern_hash VARCHAR(64) NOT NULL,
    tenant_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Ensure unique contributions per tenant per pattern
    CONSTRAINT unique_contribution UNIQUE (pattern_hash, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_contributions_pattern ON knowledge_contributions(pattern_hash);
CREATE INDEX IF NOT EXISTS idx_contributions_tenant ON knowledge_contributions(tenant_id);

-- Create flow templates table (pre-built templates for common flows)
CREATE TABLE IF NOT EXISTS flow_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Template info
    name VARCHAR(200) NOT NULL,
    description TEXT,
    industry VARCHAR(100),         -- e-commerce, saas, healthcare, etc.
    flow_type VARCHAR(100) NOT NULL, -- login, signup, checkout, search, etc.

    -- Template definition
    template JSONB NOT NULL,       -- Steps, variables, assertions

    -- Usage stats
    usage_count BIGINT DEFAULT 0,
    success_rate FLOAT DEFAULT 0.0,

    -- Metadata
    tags TEXT[],
    is_public BOOLEAN DEFAULT true,
    created_by UUID,               -- NULL for system templates

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_templates_industry ON flow_templates(industry);
CREATE INDEX IF NOT EXISTS idx_templates_flow ON flow_templates(flow_type);
CREATE INDEX IF NOT EXISTS idx_templates_usage ON flow_templates(usage_count DESC);

-- Create suggestions table (AI-generated suggestions)
CREATE TABLE IF NOT EXISTS suggestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Suggestion details
    type VARCHAR(50) NOT NULL,     -- test, healing, optimization, coverage, flow
    priority VARCHAR(20) NOT NULL, -- high, medium, low
    title VARCHAR(500) NOT NULL,
    description TEXT,
    rationale TEXT,

    -- Action to take
    action JSONB NOT NULL,

    -- Metrics
    confidence FLOAT DEFAULT 0.0,
    impact VARCHAR(500),

    -- Status
    dismissed BOOLEAN DEFAULT false,
    dismissed_at TIMESTAMP WITH TIME ZONE,
    applied BOOLEAN DEFAULT false,
    applied_at TIMESTAMP WITH TIME ZONE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_suggestions_project ON suggestions(project_id) WHERE NOT dismissed;
CREATE INDEX IF NOT EXISTS idx_suggestions_type ON suggestions(type);

-- Create suggestion feedback table (for learning from user choices)
CREATE TABLE IF NOT EXISTS suggestion_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    suggestion_id UUID NOT NULL REFERENCES suggestions(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Feedback
    accepted BOOLEAN NOT NULL,
    rejection_reason TEXT,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_feedback_suggestion ON suggestion_feedback(suggestion_id);

-- Insert default flow templates

-- E-commerce templates
INSERT INTO flow_templates (id, name, description, industry, flow_type, template, tags) VALUES
(gen_random_uuid(), 'Login Flow', 'Standard user login flow', 'e-commerce', 'login', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/login"},
        {"order": 2, "action": "fill", "target": "[name=\"email\"]", "value": "{{email}}"},
        {"order": 3, "action": "fill", "target": "[name=\"password\"]", "value": "{{password}}"},
        {"order": 4, "action": "click", "target": "button[type=\"submit\"]"},
        {"order": 5, "action": "wait", "target": "/dashboard|/account"}
    ],
    "variables": [
        {"name": "email", "type": "email", "required": true},
        {"name": "password", "type": "password", "required": true}
    ],
    "assertions": [
        {"type": "url", "expected": "/dashboard|/account"},
        {"type": "visible", "target": ".user-menu|.account-menu"}
    ]
}', ARRAY['auth', 'common']),

(gen_random_uuid(), 'Add to Cart Flow', 'Add product to shopping cart', 'e-commerce', 'add_to_cart', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/products"},
        {"order": 2, "action": "click", "target": ".product-card:first-child"},
        {"order": 3, "action": "wait", "target": ".product-detail"},
        {"order": 4, "action": "click", "target": "button:has-text(\"Add to Cart\")"},
        {"order": 5, "action": "wait", "target": ".cart-notification|.cart-count"}
    ],
    "variables": [],
    "assertions": [
        {"type": "visible", "target": ".cart-notification|.cart-count"},
        {"type": "text", "target": ".cart-count", "expected": ">0"}
    ]
}', ARRAY['shopping', 'core']),

(gen_random_uuid(), 'Checkout Flow', 'Complete checkout process', 'e-commerce', 'checkout', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/cart"},
        {"order": 2, "action": "click", "target": "button:has-text(\"Checkout\")"},
        {"order": 3, "action": "fill", "target": "[name=\"email\"]", "value": "{{email}}"},
        {"order": 4, "action": "fill", "target": "[name=\"address\"]", "value": "{{address}}"},
        {"order": 5, "action": "fill", "target": "[name=\"city\"]", "value": "{{city}}"},
        {"order": 6, "action": "fill", "target": "[name=\"zip\"]", "value": "{{zip}}"},
        {"order": 7, "action": "click", "target": "button:has-text(\"Continue\")"},
        {"order": 8, "action": "fill", "target": "[name=\"cardNumber\"]", "value": "{{card_number}}"},
        {"order": 9, "action": "fill", "target": "[name=\"expiry\"]", "value": "{{card_expiry}}"},
        {"order": 10, "action": "fill", "target": "[name=\"cvv\"]", "value": "{{card_cvv}}"},
        {"order": 11, "action": "click", "target": "button:has-text(\"Place Order\")"},
        {"order": 12, "action": "wait", "target": "/confirmation|/thank-you"}
    ],
    "variables": [
        {"name": "email", "type": "email", "required": true},
        {"name": "address", "type": "string", "required": true},
        {"name": "city", "type": "string", "required": true},
        {"name": "zip", "type": "string", "required": true},
        {"name": "card_number", "type": "string", "required": true, "default": "4242424242424242"},
        {"name": "card_expiry", "type": "string", "required": true, "default": "12/25"},
        {"name": "card_cvv", "type": "string", "required": true, "default": "123"}
    ],
    "assertions": [
        {"type": "url", "expected": "/confirmation|/thank-you"},
        {"type": "visible", "target": ".order-number|.confirmation-number"}
    ]
}', ARRAY['payment', 'core', 'critical']);

-- SaaS templates
INSERT INTO flow_templates (id, name, description, industry, flow_type, template, tags) VALUES
(gen_random_uuid(), 'User Signup', 'New user registration flow', 'saas', 'signup', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/signup"},
        {"order": 2, "action": "fill", "target": "[name=\"name\"]", "value": "{{name}}"},
        {"order": 3, "action": "fill", "target": "[name=\"email\"]", "value": "{{email}}"},
        {"order": 4, "action": "fill", "target": "[name=\"password\"]", "value": "{{password}}"},
        {"order": 5, "action": "fill", "target": "[name=\"confirm_password\"]", "value": "{{password}}"},
        {"order": 6, "action": "click", "target": "[type=\"checkbox\"][name=\"terms\"]"},
        {"order": 7, "action": "click", "target": "button[type=\"submit\"]"},
        {"order": 8, "action": "wait", "target": "/verify|/onboarding|/dashboard"}
    ],
    "variables": [
        {"name": "name", "type": "string", "required": true},
        {"name": "email", "type": "email", "required": true},
        {"name": "password", "type": "password", "required": true, "default": "Test123!@#"}
    ],
    "assertions": [
        {"type": "url", "expected": "/verify|/onboarding|/dashboard"}
    ]
}', ARRAY['auth', 'acquisition']),

(gen_random_uuid(), 'Dashboard Navigation', 'Navigate main dashboard sections', 'saas', 'navigation', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/dashboard"},
        {"order": 2, "action": "wait", "target": ".dashboard-loaded|.main-content"},
        {"order": 3, "action": "click", "target": "nav a:has-text(\"Settings\")"},
        {"order": 4, "action": "wait", "target": "/settings"},
        {"order": 5, "action": "click", "target": "nav a:has-text(\"Dashboard\")"},
        {"order": 6, "action": "wait", "target": "/dashboard"}
    ],
    "variables": [],
    "assertions": [
        {"type": "visible", "target": ".dashboard-content|.main-content"}
    ]
}', ARRAY['navigation', 'core']),

(gen_random_uuid(), 'Profile Update', 'Update user profile information', 'saas', 'profile', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/settings/profile"},
        {"order": 2, "action": "clear", "target": "[name=\"name\"]"},
        {"order": 3, "action": "fill", "target": "[name=\"name\"]", "value": "{{new_name}}"},
        {"order": 4, "action": "click", "target": "button:has-text(\"Save\")"},
        {"order": 5, "action": "wait", "target": ".success-message|.toast-success"}
    ],
    "variables": [
        {"name": "new_name", "type": "string", "required": true, "default": "Updated Name"}
    ],
    "assertions": [
        {"type": "visible", "target": ".success-message|.toast-success"}
    ]
}', ARRAY['settings', 'user']);

-- General templates
INSERT INTO flow_templates (id, name, description, industry, flow_type, template, tags) VALUES
(gen_random_uuid(), 'Search Flow', 'Basic search functionality', 'general', 'search', '{
    "steps": [
        {"order": 1, "action": "fill", "target": "input[type=\"search\"]|input[name=\"q\"]|.search-input", "value": "{{query}}"},
        {"order": 2, "action": "click", "target": "button[type=\"submit\"]|.search-button"},
        {"order": 3, "action": "wait", "target": ".search-results|.results"}
    ],
    "variables": [
        {"name": "query", "type": "string", "required": true, "default": "test search"}
    ],
    "assertions": [
        {"type": "visible", "target": ".search-results|.results"},
        {"type": "count", "target": ".result-item|.search-result", "expected": ">0"}
    ]
}', ARRAY['search', 'common']),

(gen_random_uuid(), 'Contact Form', 'Submit contact form', 'general', 'contact', '{
    "steps": [
        {"order": 1, "action": "navigate", "target": "/contact"},
        {"order": 2, "action": "fill", "target": "[name=\"name\"]", "value": "{{name}}"},
        {"order": 3, "action": "fill", "target": "[name=\"email\"]", "value": "{{email}}"},
        {"order": 4, "action": "fill", "target": "[name=\"message\"]|textarea", "value": "{{message}}"},
        {"order": 5, "action": "click", "target": "button[type=\"submit\"]"},
        {"order": 6, "action": "wait", "target": ".success|.thank-you"}
    ],
    "variables": [
        {"name": "name", "type": "string", "required": true, "default": "Test User"},
        {"name": "email", "type": "email", "required": true, "default": "test@example.com"},
        {"name": "message", "type": "string", "required": true, "default": "This is a test message."}
    ],
    "assertions": [
        {"type": "visible", "target": ".success|.thank-you|.confirmation"}
    ]
}', ARRAY['forms', 'common']);

-- Add updated_at trigger
CREATE OR REPLACE FUNCTION update_knowledge_entries_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_knowledge_entries_updated_at ON knowledge_entries;
CREATE TRIGGER trigger_knowledge_entries_updated_at
    BEFORE UPDATE ON knowledge_entries
    FOR EACH ROW
    EXECUTE FUNCTION update_knowledge_entries_updated_at();

CREATE OR REPLACE FUNCTION update_flow_templates_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_flow_templates_updated_at ON flow_templates;
CREATE TRIGGER trigger_flow_templates_updated_at
    BEFORE UPDATE ON flow_templates
    FOR EACH ROW
    EXECUTE FUNCTION update_flow_templates_updated_at();

-- Function to get top patterns by type
CREATE OR REPLACE FUNCTION get_top_patterns(
    p_type knowledge_type,
    p_limit INT DEFAULT 10
)
RETURNS TABLE (
    pattern VARCHAR,
    success_count BIGINT,
    confidence FLOAT,
    contributed_by INT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        k.pattern,
        k.success_count,
        k.confidence,
        k.contributed_by
    FROM knowledge_entries k
    WHERE k.type = p_type
      AND k.confidence >= 0.5
    ORDER BY k.success_count DESC, k.confidence DESC
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;

-- Function to calculate knowledge base stats
CREATE OR REPLACE FUNCTION get_knowledge_stats()
RETURNS TABLE (
    total_patterns BIGINT,
    healing_patterns BIGINT,
    element_patterns BIGINT,
    flow_patterns BIGINT,
    avg_confidence FLOAT,
    total_contributors BIGINT
) AS $$
BEGIN
    RETURN QUERY
    SELECT
        COUNT(*)::BIGINT as total_patterns,
        COUNT(*) FILTER (WHERE type = 'healing')::BIGINT as healing_patterns,
        COUNT(*) FILTER (WHERE type = 'element')::BIGINT as element_patterns,
        COUNT(*) FILTER (WHERE type = 'flow')::BIGINT as flow_patterns,
        AVG(confidence) as avg_confidence,
        (SELECT COUNT(DISTINCT tenant_id) FROM knowledge_contributions)::BIGINT as total_contributors
    FROM knowledge_entries;
END;
$$ LANGUAGE plpgsql;

-- Add comments
COMMENT ON TABLE knowledge_entries IS 'Cross-tenant learned patterns for AI-powered suggestions';
COMMENT ON TABLE knowledge_contributions IS 'Tracks which tenants contributed to patterns (anonymized)';
COMMENT ON TABLE flow_templates IS 'Pre-built test flow templates by industry';
COMMENT ON TABLE suggestions IS 'AI-generated suggestions for improving tests';
COMMENT ON TABLE suggestion_feedback IS 'User feedback on suggestions for learning';
COMMENT ON FUNCTION get_top_patterns IS 'Returns top performing patterns by type';
COMMENT ON FUNCTION get_knowledge_stats IS 'Returns overall knowledge base statistics';
