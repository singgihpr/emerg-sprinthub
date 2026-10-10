CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE roles (
    role_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    permissions JSONB NOT NULL DEFAULT '[]',
    is_default BOOLEAN NOT NULL DEFAULT false,
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, name)
);

CREATE TABLE role_assignments (
    assignment_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, user_id, role_id)
);

CREATE TABLE project_roles (
    project_role_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, name)
);

CREATE TABLE project_role_assignments (
    assignment_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES organizations(org_id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    project_role_id TEXT NOT NULL REFERENCES project_roles(project_role_id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, user_id, project_role_id)
);

CREATE INDEX idx_roles_org ON roles(org_id);
CREATE INDEX idx_role_assignments_org_user ON role_assignments(org_id, user_id);
CREATE INDEX idx_role_assignments_role ON role_assignments(role_id);
CREATE INDEX idx_project_roles_project ON project_roles(project_id);
CREATE INDEX idx_project_role_assignments_project_user ON project_role_assignments(project_id, user_id);

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE role_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_role_assignments ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_roles ON roles
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_role_assignments ON role_assignments
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_project_roles ON project_roles
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

CREATE POLICY tenant_isolation_project_role_assignments ON project_role_assignments
    USING (current_tenant() = '' OR org_id = current_tenant())
    WITH CHECK (current_tenant() = '' OR org_id = current_tenant());

DO $$
DECLARE
    org_rec RECORD;
    owner_role_id TEXT;
    admin_role_id TEXT;
    manager_role_id TEXT;
    member_role_id TEXT;
    viewer_role_id TEXT;
    member_rec RECORD;
    assignment_id TEXT;
BEGIN
    FOR org_rec IN SELECT org_id FROM organizations LOOP
        owner_role_id := 'role_' || encode(gen_random_bytes(16), 'hex');
        admin_role_id := 'role_' || encode(gen_random_bytes(16), 'hex');
        manager_role_id := 'role_' || encode(gen_random_bytes(16), 'hex');
        member_role_id := 'role_' || encode(gen_random_bytes(16), 'hex');
        viewer_role_id := 'role_' || encode(gen_random_bytes(16), 'hex');

        INSERT INTO roles (role_id, org_id, name, description, permissions, is_system) VALUES
            (owner_role_id, org_rec.org_id, 'Owner', 'Full access to everything', '["manage_org", "manage_billing", "invite_members", "manage_roles", "view_analytics", "create_project", "edit_project", "delete_project", "manage_project_members", "create_task", "edit_task", "delete_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"]'::jsonb, true),
            (admin_role_id, org_rec.org_id, 'Admin', 'Administrative access', '["manage_org", "invite_members", "manage_roles", "view_analytics", "create_project", "edit_project", "delete_project", "manage_project_members", "create_task", "edit_task", "delete_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"]'::jsonb, true),
            (manager_role_id, org_rec.org_id, 'Manager', 'Project management access', '["create_project", "edit_project", "manage_project_members", "create_task", "edit_task", "delete_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"]'::jsonb, true),
            (member_role_id, org_rec.org_id, 'Member', 'Standard access', '["create_task", "edit_task", "assign_task", "edit_comments", "create_time_entries", "view_time_entries"]'::jsonb, true),
            (viewer_role_id, org_rec.org_id, 'Viewer', 'Read-only access', '["view_time_entries"]'::jsonb, true);

        UPDATE roles SET is_default = true WHERE role_id = member_role_id;

        FOR member_rec IN SELECT user_id, role FROM memberships WHERE org_id = org_rec.org_id LOOP
            assignment_id := 'ra_' || encode(gen_random_bytes(16), 'hex');
            CASE member_rec.role
                WHEN 'owner' THEN INSERT INTO role_assignments (assignment_id, org_id, user_id, role_id) VALUES (assignment_id, org_rec.org_id, member_rec.user_id, owner_role_id);
                WHEN 'admin' THEN INSERT INTO role_assignments (assignment_id, org_id, user_id, role_id) VALUES (assignment_id, org_rec.org_id, member_rec.user_id, admin_role_id);
                WHEN 'manager' THEN INSERT INTO role_assignments (assignment_id, org_id, user_id, role_id) VALUES (assignment_id, org_rec.org_id, member_rec.user_id, manager_role_id);
                ELSE INSERT INTO role_assignments (assignment_id, org_id, user_id, role_id) VALUES (assignment_id, org_rec.org_id, member_rec.user_id, member_role_id);
            END CASE;
        END LOOP;
    END LOOP;
END $$;
