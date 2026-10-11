-- SOC 2 audit log: tracks security-relevant events (auth, data mutations)
CREATE TABLE audit_logs (
    log_id TEXT PRIMARY KEY,
    user_id TEXT,
    org_id TEXT,
    action TEXT NOT NULL, -- 'user.login', 'user.export', 'user.delete', 'task.create', etc.
    resource_type TEXT, -- 'user', 'task', 'project', 'org', etc.
    resource_id TEXT,
    details JSONB, -- additional context (before/after values, metadata)
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_org ON audit_logs(org_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at);
