-- Migration: 007_billing
-- Description: Add billing tables for Stripe integration
-- Phase: 3.1 - Stripe Billing Integration

-- Subscription status enum
CREATE TYPE subscription_status AS ENUM (
    'trialing',
    'active',
    'past_due',
    'canceled',
    'unpaid',
    'incomplete',
    'incomplete_expired',
    'paused'
);

-- Create subscriptions table
CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL UNIQUE REFERENCES tenants(id) ON DELETE CASCADE,

    -- Stripe identifiers
    stripe_customer_id VARCHAR(100) UNIQUE,
    stripe_subscription_id VARCHAR(100) UNIQUE,
    stripe_price_id VARCHAR(100),

    -- Plan details
    plan plan_type DEFAULT 'free',
    status subscription_status DEFAULT 'active',

    -- Billing cycle
    billing_cycle_anchor TIMESTAMP WITH TIME ZONE,
    current_period_start TIMESTAMP WITH TIME ZONE,
    current_period_end TIMESTAMP WITH TIME ZONE,

    -- Trial
    trial_start TIMESTAMP WITH TIME ZONE,
    trial_end TIMESTAMP WITH TIME ZONE,

    -- Cancellation
    cancel_at TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE,
    cancel_at_period_end BOOLEAN DEFAULT false,

    -- Payment method
    default_payment_method_id VARCHAR(100),

    -- Metadata
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for subscriptions
CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe_customer ON subscriptions(stripe_customer_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_stripe_sub ON subscriptions(stripe_subscription_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status) WHERE status NOT IN ('canceled', 'incomplete_expired');

-- Create usage records table
CREATE TABLE IF NOT EXISTS usage_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Usage tracking
    metric VARCHAR(100) NOT NULL, -- test_runs, ai_tokens, sandbox_minutes
    quantity BIGINT NOT NULL DEFAULT 0,

    -- Billing period
    period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    period_end TIMESTAMP WITH TIME ZONE NOT NULL,

    -- Stripe reporting
    reported_to_stripe BOOLEAN DEFAULT false,
    stripe_usage_record_id VARCHAR(100),
    reported_at TIMESTAMP WITH TIME ZONE,

    -- Metadata
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Constraints
    CONSTRAINT usage_records_period_check CHECK (period_end > period_start)
);

-- Indexes for usage records
CREATE INDEX IF NOT EXISTS idx_usage_records_tenant_period ON usage_records(tenant_id, period_start, period_end);
CREATE INDEX IF NOT EXISTS idx_usage_records_unreported ON usage_records(tenant_id, metric) WHERE reported_to_stripe = false;

-- Create invoices table (for tracking Stripe invoices)
CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscription_id UUID REFERENCES subscriptions(id),

    -- Stripe identifiers
    stripe_invoice_id VARCHAR(100) UNIQUE,

    -- Invoice details
    number VARCHAR(100),
    status VARCHAR(50), -- draft, open, paid, void, uncollectible
    currency VARCHAR(3) DEFAULT 'usd',

    -- Amounts (in cents)
    subtotal BIGINT,
    tax BIGINT,
    total BIGINT,
    amount_due BIGINT,
    amount_paid BIGINT,
    amount_remaining BIGINT,

    -- Dates
    invoice_date TIMESTAMP WITH TIME ZONE,
    due_date TIMESTAMP WITH TIME ZONE,
    paid_at TIMESTAMP WITH TIME ZONE,
    period_start TIMESTAMP WITH TIME ZONE,
    period_end TIMESTAMP WITH TIME ZONE,

    -- URLs
    hosted_invoice_url TEXT,
    invoice_pdf TEXT,

    -- Metadata
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for invoices
CREATE INDEX IF NOT EXISTS idx_invoices_tenant ON invoices(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_invoices_stripe ON invoices(stripe_invoice_id);

-- Create payment methods table
CREATE TABLE IF NOT EXISTS payment_methods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,

    -- Stripe identifier
    stripe_payment_method_id VARCHAR(100) UNIQUE,

    -- Card details (non-sensitive)
    type VARCHAR(50), -- card, bank_account, etc.
    card_brand VARCHAR(50), -- visa, mastercard, etc.
    card_last4 VARCHAR(4),
    card_exp_month INT,
    card_exp_year INT,

    -- Status
    is_default BOOLEAN DEFAULT false,
    is_valid BOOLEAN DEFAULT true,

    -- Metadata
    billing_details JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for payment methods
CREATE INDEX IF NOT EXISTS idx_payment_methods_tenant ON payment_methods(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_methods_stripe ON payment_methods(stripe_payment_method_id);

-- Add triggers for updated_at
CREATE OR REPLACE FUNCTION update_subscriptions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_usage_records_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION update_invoices_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_subscriptions_updated_at ON subscriptions;
CREATE TRIGGER trigger_subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_subscriptions_updated_at();

DROP TRIGGER IF EXISTS trigger_usage_records_updated_at ON usage_records;
CREATE TRIGGER trigger_usage_records_updated_at
    BEFORE UPDATE ON usage_records
    FOR EACH ROW
    EXECUTE FUNCTION update_usage_records_updated_at();

DROP TRIGGER IF EXISTS trigger_invoices_updated_at ON invoices;
CREATE TRIGGER trigger_invoices_updated_at
    BEFORE UPDATE ON invoices
    FOR EACH ROW
    EXECUTE FUNCTION update_invoices_updated_at();

-- Function to get current usage for a tenant
CREATE OR REPLACE FUNCTION get_current_period_usage(
    p_tenant_id UUID,
    p_metric VARCHAR(100)
)
RETURNS BIGINT AS $$
DECLARE
    v_period_start TIMESTAMP WITH TIME ZONE;
    v_total BIGINT;
BEGIN
    -- Get current period start from subscription
    SELECT current_period_start INTO v_period_start
    FROM subscriptions
    WHERE tenant_id = p_tenant_id;

    -- If no subscription, use start of month
    IF v_period_start IS NULL THEN
        v_period_start := DATE_TRUNC('month', NOW());
    END IF;

    -- Sum usage for current period
    SELECT COALESCE(SUM(quantity), 0) INTO v_total
    FROM usage_records
    WHERE tenant_id = p_tenant_id
      AND metric = p_metric
      AND period_start >= v_period_start;

    RETURN v_total;
END;
$$ LANGUAGE plpgsql;

-- Function to record usage
CREATE OR REPLACE FUNCTION record_usage(
    p_tenant_id UUID,
    p_metric VARCHAR(100),
    p_quantity BIGINT DEFAULT 1
)
RETURNS UUID AS $$
DECLARE
    v_record_id UUID;
    v_period_start TIMESTAMP WITH TIME ZONE;
    v_period_end TIMESTAMP WITH TIME ZONE;
BEGIN
    -- Determine current billing period
    SELECT current_period_start, current_period_end INTO v_period_start, v_period_end
    FROM subscriptions
    WHERE tenant_id = p_tenant_id;

    -- Default to monthly period if no subscription
    IF v_period_start IS NULL THEN
        v_period_start := DATE_TRUNC('month', NOW());
        v_period_end := DATE_TRUNC('month', NOW()) + INTERVAL '1 month';
    END IF;

    -- Insert or update usage record
    INSERT INTO usage_records (tenant_id, metric, quantity, period_start, period_end)
    VALUES (p_tenant_id, p_metric, p_quantity, v_period_start, v_period_end)
    RETURNING id INTO v_record_id;

    RETURN v_record_id;
END;
$$ LANGUAGE plpgsql;

-- View for subscription summary
CREATE OR REPLACE VIEW subscription_summary AS
SELECT
    s.id,
    s.tenant_id,
    t.name as tenant_name,
    s.plan,
    s.status,
    s.current_period_start,
    s.current_period_end,
    s.trial_end,
    s.cancel_at_period_end,
    get_current_period_usage(s.tenant_id, 'test_runs') as test_runs_used,
    get_current_period_usage(s.tenant_id, 'ai_tokens') as ai_tokens_used,
    get_current_period_usage(s.tenant_id, 'sandbox_minutes') as sandbox_minutes_used
FROM subscriptions s
JOIN tenants t ON t.id = s.tenant_id;

-- Add comments
COMMENT ON TABLE subscriptions IS 'Stripe subscription records for tenants';
COMMENT ON TABLE usage_records IS 'Usage tracking for metered billing';
COMMENT ON TABLE invoices IS 'Stripe invoice records';
COMMENT ON TABLE payment_methods IS 'Stored payment methods from Stripe';
COMMENT ON FUNCTION get_current_period_usage IS 'Get current billing period usage for a metric';
COMMENT ON FUNCTION record_usage IS 'Record usage for metered billing';
