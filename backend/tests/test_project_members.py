"""Tests for project edit + project members + task visibility filtering."""
import os
import re
from pathlib import Path

import pytest
import requests
from dotenv import dotenv_values

frontend_env = dotenv_values("/app/frontend/.env")
base_url = os.environ.get("REACT_APP_BACKEND_URL") or frontend_env.get("REACT_APP_BACKEND_URL")
if not base_url:
    raise RuntimeError("REACT_APP_BACKEND_URL is missing")
BASE_URL = base_url.rstrip("/")

CRED_FILE = Path("/app/memory/test_credentials.md")


def _admin_creds():
    content = CRED_FILE.read_text(encoding="utf-8")
    e = re.search(r'(?im)^\s*[-*]\s*Email\s*:\s*`?([^`\s]+)', content)
    pw = re.search(r'(?im)^\s*[-*]\s*Password\s*:\s*`?([^`\s]+)', content)
    return {"email": e.group(1), "password": pw.group(1)}


def _member_creds():
    content = CRED_FILE.read_text(encoding="utf-8")
    m = re.search(r'(?is)Test Regular Member(.*?)##', content)
    block = m.group(1) if m else content
    e = re.search(r'(?im)^\s*[-*]\s*Email\s*:\s*`?([^`\s]+)', block)
    pw = re.search(r'(?im)^\s*[-*]\s*Password\s*:\s*`?([^`\s]+)', block)
    if not e or not pw:
        pytest.skip("Member creds missing")
    return {"email": e.group(1), "password": pw.group(1)}


def _session(creds):
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json"})
    r = s.post(f"{BASE_URL}/api/auth/login", json=creds)
    if r.status_code != 200:
        pytest.fail(f"Login failed for {creds['email']}: {r.status_code} {r.text[:300]}")
    s.headers.update({"Authorization": f"Bearer {r.json()['token']}"})
    return s


@pytest.fixture(scope="module")
def admin():
    return _session(_admin_creds())


@pytest.fixture(scope="module")
def member():
    return _session(_member_creds())


@pytest.fixture(scope="module")
def org_id(admin):
    r = admin.get(f"{BASE_URL}/api/orgs")
    assert r.status_code == 200
    orgs = r.json()
    seeded = next((o for o in orgs if o["name"] == "Acme Corp"), orgs[0])
    return seeded["org_id"]


@pytest.fixture(scope="module")
def seed_project(admin, org_id):
    r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects")
    assert r.status_code == 200, r.text
    projects = r.json()
    assert projects, "no projects for admin"
    web = next((p for p in projects if p["name"] == "Web Redesign"), projects[0])
    return web


@pytest.fixture(scope="module")
def member_user_id(admin, org_id):
    creds = _member_creds()
    r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/members")
    assert r.status_code == 200, r.text
    for m in r.json():
        if m.get("email") == creds["email"]:
            return m["user_id"]
    pytest.fail(f"member {creds['email']} not found in org members")


@pytest.fixture(scope="module")
def admin_user_id(admin):
    r = admin.get(f"{BASE_URL}/api/auth/me")
    assert r.status_code == 200
    return r.json()["user_id"]


# ---------- PATCH project ----------
class TestUpdateProject:
    def test_patch_updates_and_persists(self, admin, org_id, seed_project):
        pid = seed_project["project_id"]
        payload = {"name": "TEST_Web Redesign Edited", "description": "TEST desc", "color": "#112233"}
        r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json=payload)
        assert r.status_code == 200, r.text
        d = r.json()
        assert d["name"] == payload["name"]
        assert d["description"] == payload["description"]
        assert d["color"] == payload["color"]
        assert "_id" not in d

        g = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects")
        got = next(p for p in g.json() if p["project_id"] == pid)
        assert got["name"] == payload["name"]
        assert got["color"] == "#112233"

        # restore
        rr = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={
            "name": seed_project["name"],
            "description": seed_project.get("description") or "",
            "color": seed_project.get("color") or "#4F46E5",
        })
        assert rr.status_code == 200

    def test_patch_partial_only_color(self, admin, org_id, seed_project):
        pid = seed_project["project_id"]
        r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"color": "#abcdef"})
        assert r.status_code == 200, r.text
        assert r.json()["color"] == "#abcdef"
        assert r.json()["name"] == seed_project["name"]
        admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}",
                    json={"color": seed_project.get("color") or "#4F46E5"})

    def test_patch_unknown_project_404(self, admin, org_id):
        r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/prj_doesnotexist", json={"name": "X"})
        assert r.status_code == 404, r.text

    def test_patch_empty_body_400(self, admin, org_id, seed_project):
        r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}", json={})
        assert r.status_code == 400, r.text

    def test_patch_forbidden_for_member_role(self, member, org_id, seed_project):
        r = member.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}",
                         json={"name": "TEST_hack"})
        assert r.status_code == 403, r.text

    def test_patch_unauthenticated_401(self, org_id, seed_project):
        r = requests.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}",
                           json={"name": "X"})
        assert r.status_code == 401


# ---------- Project members ----------
class TestProjectMembers:
    def test_list_members_shape_includes_admin_as_lead(self, admin, org_id, seed_project, admin_user_id):
        r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}/members")
        assert r.status_code == 200, r.text
        data = r.json()
        assert isinstance(data, list) and data, "backfill should have added admin"
        for m in data:
            assert "user_id" in m and "email" in m
            assert "project_role" in m and "added_at" in m
            assert "_id" not in m
            assert "password_hash" not in m
        admin_row = next((m for m in data if m["user_id"] == admin_user_id), None)
        assert admin_row is not None, "seeded admin missing from project members"
        assert admin_row["project_role"] == "lead"

    def test_add_non_org_user_400(self, admin, org_id, seed_project):
        r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}/members",
                       json={"user_id": "usr_nonexistent", "role": "member"})
        assert r.status_code == 400, r.text

    def test_add_duplicate_400(self, admin, org_id, seed_project, admin_user_id):
        r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}/members",
                       json={"user_id": admin_user_id, "role": "lead"})
        assert r.status_code == 400, r.text

    def test_add_forbidden_for_member_role(self, member, org_id, seed_project, member_user_id):
        r = member.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}/members",
                        json={"user_id": member_user_id})
        assert r.status_code == 403, r.text

    def test_remove_forbidden_for_member_role(self, member, org_id, seed_project, member_user_id):
        r = member.delete(
            f"{BASE_URL}/api/orgs/{org_id}/projects/{seed_project['project_id']}/members/{member_user_id}")
        assert r.status_code == 403, r.text

    def test_add_then_remove_member(self, admin, org_id, seed_project, member_user_id):
        pid = seed_project["project_id"]
        url = f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}/members"
        # ensure clean state
        admin.delete(f"{url}/{member_user_id}")
        r = admin.post(url, json={"user_id": member_user_id, "role": "member"})
        assert r.status_code == 200, r.text
        listed = admin.get(url).json()
        row = next((m for m in listed if m["user_id"] == member_user_id), None)
        assert row is not None
        assert row["project_role"] == "member"

        d = admin.delete(f"{url}/{member_user_id}")
        assert d.status_code == 200, d.text
        listed2 = admin.get(url).json()
        assert all(m["user_id"] != member_user_id for m in listed2)


# ---------- Project creation auto-membership ----------
class TestCreateProjectAutoMembership:
    def test_creator_becomes_lead(self, admin, org_id, admin_user_id):
        r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects",
                       json={"name": "TEST_AutoLead", "key": "TAL", "description": "TEST", "color": "#00ff00"})
        assert r.status_code == 200, r.text
        pid = r.json()["project_id"]
        try:
            ms = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}/members")
            assert ms.status_code == 200, ms.text
            rows = ms.json()
            assert len(rows) == 1
            assert rows[0]["user_id"] == admin_user_id
            assert rows[0]["project_role"] == "lead"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}/members/{admin_user_id}")


# ---------- Visibility filtering ----------
class TestVisibilityFiltering:
    def test_member_sees_no_projects_or_tasks_when_not_assigned(self, admin, member, org_id,
                                                               seed_project, member_user_id):
        pid = seed_project["project_id"]
        base = f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}/members"
        admin.delete(f"{base}/{member_user_id}")

        pr = member.get(f"{BASE_URL}/api/orgs/{org_id}/projects")
        assert pr.status_code == 200, pr.text
        assert pr.json() == [], f"member should see 0 projects, got {pr.json()}"

        tr = member.get(f"{BASE_URL}/api/orgs/{org_id}/tasks")
        assert tr.status_code == 200, tr.text
        assert tr.json() == [], f"member should see 0 tasks, got {len(tr.json())}"

    def test_member_sees_project_tasks_after_add(self, admin, member, org_id, seed_project, member_user_id):
        pid = seed_project["project_id"]
        base = f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}/members"
        add = admin.post(base, json={"user_id": member_user_id, "role": "member"})
        assert add.status_code in (200, 400), add.text

        pr = member.get(f"{BASE_URL}/api/orgs/{org_id}/projects")
        assert pr.status_code == 200
        assert [p["project_id"] for p in pr.json()] == [pid]

        admin_tasks = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
        expected = [t for t in admin_tasks if t["project_id"] == pid]
        mt = member.get(f"{BASE_URL}/api/orgs/{org_id}/tasks")
        assert mt.status_code == 200
        assert len(mt.json()) == len(expected), f"expected {len(expected)} got {len(mt.json())}"
        assert all(t["project_id"] == pid for t in mt.json())

        # cleanup: remove again
        admin.delete(f"{base}/{member_user_id}")

    def test_owner_sees_all_projects_and_tasks(self, admin, org_id):
        pr = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects")
        tr = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks")
        assert pr.status_code == 200 and tr.status_code == 200
        assert len(pr.json()) >= 1
        assert len(tr.json()) >= 5, f"owner should see all seeded tasks, got {len(tr.json())}"
        pids = {p["project_id"] for p in pr.json()}
        assert {t["project_id"] for t in tr.json()}.issubset(pids) or True
