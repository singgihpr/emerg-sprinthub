CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    user_id TEXT PRIMARY KEY,
    email CITEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    password_hash TEXT,
    picture TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE organizations (
    org_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users(user_id),
    logo TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    membership_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, user_id)
);

CREATE TABLE projects (
    project_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key TEXT NOT NULL,
    description TEXT,
    color TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_by TEXT REFERENCES users(user_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE project_members (
    project_member_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    added_by TEXT REFERENCES users(user_id),
    UNIQUE(org_id, project_id, user_id)
);

CREATE TABLE sprints (
    sprint_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT,
    start_date DATE,
    end_date DATE,
    status TEXT NOT NULL DEFAULT 'planned',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    task_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'todo',
    priority TEXT NOT NULL DEFAULT 'medium',
    type TEXT NOT NULL DEFAULT 'task',
    assignee_id TEXT REFERENCES users(user_id),
    sprint_id TEXT REFERENCES sprints(sprint_id),
    start_date DATE,
    due_date DATE,
    estimate_hours NUMERIC(8,2),
    logged_minutes INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(user_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE recurring_tasks (
    recurring_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'todo',
    priority TEXT NOT NULL DEFAULT 'medium',
    type TEXT NOT NULL DEFAULT 'task',
    assignee_id TEXT REFERENCES users(user_id),
    sprint_id TEXT REFERENCES sprints(sprint_id),
    estimate_hours NUMERIC(8,2),
    repeat TEXT NOT NULL DEFAULT 'none',
    created_by TEXT REFERENCES users(user_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE time_entries (
    entry_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    minutes INTEGER NOT NULL,
    note TEXT,
    date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE active_timers (
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id)
);

CREATE TABLE comments (
    comment_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    mentions JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invites (
    invite_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    email CITEXT NOT NULL,
    name TEXT,
    token_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    invited_by TEXT REFERENCES users(user_id),
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE cron_runs (
    run_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    webhook_id TEXT,
    run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for common query patterns
CREATE INDEX idx_tasks_org ON tasks(org_id);
CREATE INDEX idx_tasks_project ON tasks(project_id);
CREATE INDEX idx_tasks_sprint ON tasks(sprint_id);
CREATE INDEX idx_tasks_assignee ON tasks(assignee_id);
CREATE INDEX idx_projects_org ON projects(org_id);
CREATE INDEX idx_sprints_project ON sprints(project_id);
CREATE INDEX idx_sprints_org ON sprints(org_id);
CREATE INDEX idx_time_entries_org_date ON time_entries(org_id, date);
CREATE INDEX idx_comments_task ON comments(task_id);
CREATE INDEX idx_invites_org_email ON invites(org_id, email);
CREATE INDEX idx_memberships_user ON memberships(user_id);
CREATE INDEX idx_memberships_org ON memberships(org_id);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);

-- Tenant isolation helpers
CREATE OR REPLACE FUNCTION current_tenant() RETURNS TEXT AS $$
BEGIN
    RETURN current_setting('app.current_org', true);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION current_app_user() RETURNS TEXT AS $$
BEGIN
    RETURN current_setting('app.current_user', true);
END;
$$ LANGUAGE plpgsql;

-- Enable RLS
ALTER TABLE tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE sprints ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE comments ENABLE ROW LEVEL SECURITY;
ALTER TABLE invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE recurring_tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE refresh_tokens ENABLE ROW LEVEL SECURITY;

-- Org-scoped tables: visible only when current_org matches (or unset for maintenance queries)
CREATE POLICY tenant_isolation_tasks ON tasks
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_projects ON projects
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_sprints ON sprints
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_time_entries ON time_entries
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_comments ON comments
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_invites ON invites
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_recurring_tasks ON recurring_tasks
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_project_members ON project_members
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

-- User-scoped tables: a user only sees their own rows (or unset for maintenance)
CREATE POLICY user_isolation_memberships ON memberships
    USING (current_app_user() = '' OR user_id = current_app_user())
    WITH CHECK (current_app_user() = '' OR user_id = current_app_user());

CREATE POLICY user_isolation_refresh_tokens ON refresh_tokens
    USING (current_app_user() = '' OR user_id = current_app_user())
    WITH CHECK (current_app_user() = '' OR user_id = current_app_user());
