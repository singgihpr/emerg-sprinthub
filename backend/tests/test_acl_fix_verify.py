"""Iteration 7: verify the project-scoping ACL fixes end-to-end.

Flow:
 1. Remove the member from every project -> all project-scoped endpoints must 403 / return 0.
 2. Re-add the member to 'Web Redesign' -> the same endpoints must return 200 / valid data.
 3. GET /api/orgs must be deterministically sorted by created_at asc.
"""
import os
import re
from pathlib import Path

import pytest
import requests
from dotenv import dotenv_values

frontend_env = dotenv_values("/app/frontend/.env")
BASE_URL = (os.environ.get("REACT_APP_BACKEND_URL") or frontend_env["REACT_APP_BACKEND_URL"]).rstrip("/")
CRED = Path("/app/memory/test_credentials.md").read_text(encoding="utf-8")


def _login(email, pw):
    s = requests.Session()
    r = s.post(f"{BASE_URL}/api/auth/login", json={"email": email, "password": pw})
    if r.status_code != 200:
        pytest.fail(f"login failed {email}: {r.status_code} {r.text[:200]}")
    s.headers.update({"Authorization": f"Bearer {r.json()['token']}", "Content-Type": "application/json"})
    return s


@pytest.fixture(scope="module")
def admin():
    e = re.search(r'(?im)^\s*[-*]\s*Email\s*:\s*`?([^`\s]+)', CRED).group(1)
    p = re.search(r'(?im)^\s*[-*]\s*Password\s*:\s*`?([^`\s]+)', CRED).group(1)
    return _login(e, p)


@pytest.fixture(scope="module")
def member():
    block = re.search(r'(?is)Regular Member(.*?)##', CRED).group(1)
    e = re.search(r'(?im)^\s*[-*]\s*Email\s*:\s*`?([^`\s]+)', block).group(1)
    p = re.search(r'(?im)^\s*[-*]\s*Password\s*:\s*`?([^`\s]+)', block).group(1)
    return _login(e, p)


@pytest.fixture(scope="module")
def ctx(admin, member):
    orgs = admin.get(f"{BASE_URL}/api/orgs").json()
    org = next((o for o in orgs if o["name"] == "Acme Corp"), orgs[0])
    assert org["role"] == "owner", f"admin role in Acme Corp is {org['role']}"
    org_id = org["org_id"]
    me = member.get(f"{BASE_URL}/api/auth/me").json()["user_id"]
    projects = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
    web = next((p for p in projects if p["name"] == "Web Redesign"), projects[0])
    tasks = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
    task = next(t for t in tasks if t.get("project_id") == web["project_id"])
    return {"org_id": org_id, "project_id": web["project_id"], "task_id": task["task_id"],
            "member_id": me, "all_projects": projects}


@pytest.fixture(scope="module")
def detached(admin, ctx):
    """Member belongs to no project."""
    for p in ctx["all_projects"]:
        admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{p['project_id']}/members/{ctx['member_id']}")
    yield
    # restore: re-add to Web Redesign
    admin.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members",
               json={"user_id": ctx["member_id"], "role": "member"})


# --- Phase 1: member with zero project memberships ---
class TestZeroProjectMember:
    def test_projects_and_tasks_empty(self, member, ctx, detached):
        assert member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects").json() == []
        assert member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks").json() == []

    def test_patch_task_forbidden(self, member, ctx, detached):
        r = member.patch(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{ctx['task_id']}", json={"status": "done"})
        assert r.status_code == 403, f"expected 403, got {r.status_code} {r.text[:200]}"

    def test_delete_task_forbidden(self, admin, member, ctx, detached):
        c = admin.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks",
                       json={"project_id": ctx["project_id"], "title": "TEST_acl_probe_del"})
        assert c.status_code == 200, c.text
        tid = c.json()["task_id"]
        try:
            r = member.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{tid}")
            assert r.status_code == 403, f"expected 403, got {r.status_code}"
            # task still exists
            assert any(t["task_id"] == tid for t in admin.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks").json())
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{tid}")

    def test_get_comments_forbidden(self, member, ctx, detached):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{ctx['task_id']}/comments")
        assert r.status_code == 403, f"expected 403, got {r.status_code}"

    def test_post_comment_forbidden(self, member, ctx, detached):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{ctx['task_id']}/comments",
                        json={"body": "TEST_acl should not be allowed"})
        assert r.status_code == 403, f"expected 403, got {r.status_code}"

    def test_project_members_forbidden(self, member, ctx, detached):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members")
        assert r.status_code == 403, f"expected 403, got {r.status_code}"

    def test_sprints_empty(self, member, ctx, detached):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/sprints")
        assert r.status_code == 200
        assert r.json() == [], f"LEAK: {len(r.json())} sprints visible"

    def test_analytics_zeroed(self, member, ctx, detached):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/analytics")
        assert r.status_code == 200
        d = r.json()
        assert d["total_tasks"] == 0, f"LEAK: total_tasks={d['total_tasks']}"
        assert d["total_logged_minutes"] == 0, f"LEAK: logged={d['total_logged_minutes']}"
        assert d["completion_rate"] == 0
        assert sum(p["minutes"] for p in d["time_series"]) == 0
        assert sum(p["completed"] for p in d["completed_series"]) == 0


# --- Phase 2: member re-added to Web Redesign ---
@pytest.fixture(scope="module")
def attached(admin, ctx, detached):
    r = admin.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members",
                   json={"user_id": ctx["member_id"], "role": "member"})
    assert r.status_code == 200, r.text
    return r.json()


class TestReAddedMember:
    def test_projects_and_tasks_visible(self, member, ctx, attached):
        prjs = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects").json()
        assert [p["project_id"] for p in prjs] == [ctx["project_id"]]
        tasks = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks").json()
        assert len(tasks) >= 1
        assert all(t["project_id"] == ctx["project_id"] for t in tasks)

    def test_project_members_visible(self, member, ctx, attached):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members")
        assert r.status_code == 200, r.text
        assert any(u.get("user_id") == ctx["member_id"] for u in r.json())

    def test_comments_read_and_write(self, member, ctx, attached):
        base = f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{ctx['task_id']}/comments"
        g = member.get(base)
        assert g.status_code == 200, g.text
        p = member.post(base, json={"body": "TEST_acl allowed comment"})
        assert p.status_code == 200, p.text
        cid = p.json().get("comment_id")
        assert cid
        g2 = member.get(base)
        assert any(c["comment_id"] == cid for c in g2.json())
        # no DELETE /comments endpoint exists; TEST_ comment cleaned up out-of-band

    def test_patch_task_allowed(self, member, admin, ctx, attached):
        tid = ctx["task_id"]
        original = next(t for t in admin.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks").json()
                        if t["task_id"] == tid)["status"]
        try:
            r = member.patch(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{tid}", json={"status": "in_progress"})
            assert r.status_code == 200, r.text
            assert r.json()["status"] == "in_progress"
        finally:
            admin.patch(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{tid}", json={"status": original})

    def test_sprints_visible(self, member, ctx, attached):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/sprints")
        assert r.status_code == 200
        assert len(r.json()) >= 1
        assert all(s["project_id"] == ctx["project_id"] for s in r.json())

    def test_analytics_non_zero(self, member, ctx, attached):
        d = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/analytics").json()
        assert d["total_tasks"] >= 1, d


# --- Phase 3: deterministic org sort ---
class TestOrgSort:
    def test_orgs_sorted_by_created_at(self, admin):
        first = admin.get(f"{BASE_URL}/api/orgs")
        assert first.status_code == 200
        ids = [o["org_id"] for o in first.json()]
        created = [o["created_at"] for o in first.json()]
        assert created == sorted(created), f"not sorted asc: {created}"
        for _ in range(3):
            assert [o["org_id"] for o in admin.get(f"{BASE_URL}/api/orgs").json()] == ids


# --- Owner regression ---
class TestOwnerSeesAll:
    def test_owner_full_visibility(self, admin, ctx):
        org_id = ctx["org_id"]
        assert len(admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()) >= len(ctx["all_projects"])
        assert len(admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()) >= 5
        assert admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints").status_code == 200
        a = admin.get(f"{BASE_URL}/api/orgs/{org_id}/analytics").json()
        assert a["total_tasks"] >= 5
        assert admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects/{ctx['project_id']}/members").status_code == 200
        assert admin.get(f"{BASE_URL}/api/orgs/{org_id}/team-activity").status_code == 200
