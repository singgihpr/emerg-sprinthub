-- Plans: Free/Pro/Enterprise with generous limits
CREATE TABLE plans (
    plan_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    stripe_price_id TEXT,
    limits JSONB NOT NULL DEFAULT '{"members": 5, "projects": 3, "storage_mb": 500, "api_calls_per_month": 10000}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Subscription state on organizations
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS plan_id TEXT REFERENCES plans(plan_id);
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS stripe_customer_id TEXT;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS stripe_subscription_id TEXT;
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS subscription_status TEXT DEFAULT 'active';

-- Quota tracking: current usage per org
CREATE TABLE org_quotas (
    org_id TEXT PRIMARY KEY REFERENCES organizations(org_id) ON DELETE CASCADE,
    current_members INTEGER NOT NULL DEFAULT 0,
    current_projects INTEGER NOT NULL DEFAULT 0,
    current_storage_bytes BIGINT NOT NULL DEFAULT 0,
    api_calls_this_month INTEGER NOT NULL DEFAULT 0,
    api_calls_reset_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Usage events for metering/overage billing
CREATE TABLE usage_events (
    event_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    event_type TEXT NOT NULL, -- 'api_call', 'storage_bytes', 'member_added', 'project_created'
    quantity BIGINT NOT NULL DEFAULT 1,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_usage_events_org_timestamp ON usage_events(org_id, timestamp);
CREATE INDEX idx_usage_events_type ON usage_events(event_type);
CREATE INDEX idx_org_quotas_org ON org_quotas(org_id);
