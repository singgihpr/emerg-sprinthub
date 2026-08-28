import os
import re
import time
from pathlib import Path

import pytest
import requests
from dotenv import dotenv_values

frontend_env = dotenv_values("/app/frontend/.env")
base_url = os.environ.get("REACT_APP_BACKEND_URL") or frontend_env.get("REACT_APP_BACKEND_URL")
if not base_url:
    raise RuntimeError("REACT_APP_BACKEND_URL is missing")
BASE_URL = base_url.rstrip("/")


def _creds():
    p = Path("/app/memory/test_credentials.md")
    content = p.read_text(encoding="utf-8")
    e = re.search(r'(?im)^\s*(?:[-*]\s*)?(?:\*\*)?email(?:\*\*)?\s*:\s*`?([^`\s]+)', content)
    pw = re.search(r'(?im)^\s*(?:[-*]\s*)?(?:\*\*)?password(?:\*\*)?\s*:\s*`?([^`\s]+)', content)
    return {"email": e.group(1), "password": pw.group(1)}


@pytest.fixture(scope="module")
def creds():
    return _creds()


@pytest.fixture(scope="module")
def client(creds):
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json"})
    r = s.post(f"{BASE_URL}/api/auth/login", json=creds)
    if r.status_code != 200:
        pytest.fail(f"Login failed {r.status_code}: {r.text[:300]}")
    tok = r.json().get("token")
    assert tok
    s.headers.update({"Authorization": f"Bearer {tok}"})
    return s


@pytest.fixture(scope="module")
def org_id(client):
    r = client.get(f"{BASE_URL}/api/orgs")
    assert r.status_code == 200, r.text
    orgs = r.json()
    assert isinstance(orgs, list) and orgs
    return orgs[0]["org_id"]


# ---------- Auth ----------
class TestAuth:
    def test_login_sets_httponly_cookie(self, creds):
        r = requests.post(f"{BASE_URL}/api/auth/login", json=creds)
        assert r.status_code == 200
        data = r.json()
        assert data["email"] == creds["email"].lower()
        assert isinstance(data["token"], str) and len(data["token"]) > 10
        raw = r.headers.get("set-cookie", "")
        assert "access_token" in raw, f"no access_token cookie: {raw}"
        assert "HttpOnly" in raw

    def test_login_invalid(self, creds):
        r = requests.post(f"{BASE_URL}/api/auth/login",
                          json={"email": creds["email"], "password": "WrongPass!123"})
        assert r.status_code == 401

    def test_me_requires_auth(self):
        r = requests.get(f"{BASE_URL}/api/auth/me")
        assert r.status_code == 401

    def test_me_ok(self, client, creds):
        r = client.get(f"{BASE_URL}/api/auth/me")
        assert r.status_code == 200
        d = r.json()
        assert d["email"] == creds["email"].lower()
        assert "password_hash" not in d
        assert "_id" not in d


# ---------- Orgs / Projects / Sprints (POST bug-fix verification) ----------
class TestCreateFlows:
    created = {}

    def test_create_org(self, client):
        r = client.post(f"{BASE_URL}/api/orgs", json={"name": "TEST_Org_QA"})
        assert r.status_code == 200, r.text
        d = r.json()
        assert "_id" not in d
        assert d["name"] == "TEST_Org_QA"
        assert d["role"] == "owner"
        TestCreateFlows.created["org_id"] = d["org_id"]
        # verify persisted
        orgs = client.get(f"{BASE_URL}/api/orgs").json()
        assert d["org_id"] in [o["org_id"] for o in orgs]

    def test_new_org_has_empty_tasks(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.get(f"{BASE_URL}/api/orgs/{oid}/tasks")
        assert r.status_code == 200
        assert r.json() == []

    def test_create_project(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/projects",
                        json={"name": "TEST_Project", "key": "tqa", "description": "qa"})
        assert r.status_code == 200, r.text
        d = r.json()
        assert "_id" not in d
        assert d["key"] == "TQA"
        TestCreateFlows.created["project_id"] = d["project_id"]
        lst = client.get(f"{BASE_URL}/api/orgs/{oid}/projects").json()
        assert d["project_id"] in [p["project_id"] for p in lst]

    def test_create_sprint(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/sprints", json={
            "project_id": TestCreateFlows.created["project_id"],
            "name": "TEST_Sprint 1", "goal": "qa goal",
            "start_date": "2026-07-01", "end_date": "2026-07-14"})
        assert r.status_code == 200, r.text
        d = r.json()
        assert "_id" not in d
        assert d["status"] == "planned"
        TestCreateFlows.created["sprint_id"] = d["sprint_id"]
        lst = client.get(f"{BASE_URL}/api/orgs/{oid}/sprints").json()
        assert d["sprint_id"] in [s["sprint_id"] for s in lst]

    def test_create_task(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/tasks", json={
            "project_id": TestCreateFlows.created["project_id"],
            "title": "TEST_Task A", "status": "todo", "priority": "high",
            "estimate_hours": 2, "sprint_id": TestCreateFlows.created["sprint_id"]})
        assert r.status_code == 200, r.text
        d = r.json()
        assert "_id" not in d
        assert d["title"] == "TEST_Task A"
        assert d["logged_minutes"] == 0
        TestCreateFlows.created["task_id"] = d["task_id"]
        lst = client.get(f"{BASE_URL}/api/orgs/{oid}/tasks").json()
        assert d["task_id"] in [t["task_id"] for t in lst]

    def test_patch_task_status(self, client):
        oid = TestCreateFlows.created["org_id"]
        tid = TestCreateFlows.created["task_id"]
        r = client.patch(f"{BASE_URL}/api/orgs/{oid}/tasks/{tid}", json={"status": "done"})
        assert r.status_code == 200, r.text
        d = r.json()
        assert d["status"] == "done"
        assert d["completed_at"]
        assert "_id" not in d

    def test_patch_task_not_found(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.patch(f"{BASE_URL}/api/orgs/{oid}/tasks/tsk_doesnotexist", json={"status": "todo"})
        assert r.status_code == 404

    def test_create_time_entry_increments_logged(self, client):
        oid = TestCreateFlows.created["org_id"]
        tid = TestCreateFlows.created["task_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/time-entries",
                        json={"task_id": tid, "minutes": 30, "note": "TEST manual"})
        assert r.status_code == 200, r.text
        d = r.json()
        assert "_id" not in d
        assert d["minutes"] == 30
        tasks = client.get(f"{BASE_URL}/api/orgs/{oid}/tasks").json()
        t = next(x for x in tasks if x["task_id"] == tid)
        assert t["logged_minutes"] == 30
        entries = client.get(f"{BASE_URL}/api/orgs/{oid}/time-entries").json()
        assert d["entry_id"] in [e["entry_id"] for e in entries]

    def test_timer_start_stop(self, client):
        oid = TestCreateFlows.created["org_id"]
        tid = TestCreateFlows.created["task_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/timer/start", json={"task_id": tid})
        assert r.status_code == 200, r.text
        assert r.json()["task_id"] == tid
        g = client.get(f"{BASE_URL}/api/orgs/{oid}/timer")
        assert g.status_code == 200 and g.json().get("task_id") == tid
        time.sleep(2)
        s = client.post(f"{BASE_URL}/api/orgs/{oid}/timer/stop")
        assert s.status_code == 200, s.text
        d = s.json()
        assert "_id" not in d
        assert d["minutes"] >= 1
        assert client.get(f"{BASE_URL}/api/orgs/{oid}/timer").json() == {}
        tasks = client.get(f"{BASE_URL}/api/orgs/{oid}/tasks").json()
        t = next(x for x in tasks if x["task_id"] == tid)
        assert t["logged_minutes"] == 30 + d["minutes"]

    def test_timer_stop_no_active(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/timer/stop")
        assert r.status_code == 404

    def test_timer_start_missing_task_id(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/timer/start", json={})
        assert r.status_code == 400

    def test_invite_member(self, client):
        oid = TestCreateFlows.created["org_id"]
        email = f"test_qa_{int(time.time())}@example.com"
        r = client.post(f"{BASE_URL}/api/orgs/{oid}/members",
                        json={"email": email, "name": "TEST_Member", "role": "member"})
        assert r.status_code == 200, r.text
        assert r.json()["ok"] is True
        members = client.get(f"{BASE_URL}/api/orgs/{oid}/members").json()
        assert email in [m.get("email") for m in members]
        for m in members:
            assert "password_hash" not in m and "_id" not in m
        # duplicate invite
        dup = client.post(f"{BASE_URL}/api/orgs/{oid}/members",
                          json={"email": email, "name": "TEST_Member", "role": "member"})
        assert dup.status_code == 400

    def test_analytics(self, client):
        oid = TestCreateFlows.created["org_id"]
        r = client.get(f"{BASE_URL}/api/orgs/{oid}/analytics")
        assert r.status_code == 200, r.text
        d = r.json()
        assert d["total_tasks"] == 1
        assert d["completed_tasks"] == 1
        assert d["completion_rate"] == 100
        assert len(d["completed_series"]) == 7
        assert len(d["time_series"]) == 7
        assert d["total_logged_minutes"] >= 30

    def test_delete_task(self, client):
        oid = TestCreateFlows.created["org_id"]
        tid = TestCreateFlows.created["task_id"]
        r = client.delete(f"{BASE_URL}/api/orgs/{oid}/tasks/{tid}")
        assert r.status_code == 200
        tasks = client.get(f"{BASE_URL}/api/orgs/{oid}/tasks").json()
        assert tid not in [t["task_id"] for t in tasks]


# ---------- Access control ----------
class TestAccessControl:
    def test_tasks_unauthenticated(self, org_id):
        r = requests.get(f"{BASE_URL}/api/orgs/{org_id}/tasks")
        assert r.status_code == 401

    def test_non_member_org_forbidden(self, client):
        r = client.get(f"{BASE_URL}/api/orgs/org_nonexistent123/tasks")
        assert r.status_code == 403

    def test_invalid_token(self, org_id):
        r = requests.get(f"{BASE_URL}/api/orgs/{org_id}/tasks",
                         headers={"Authorization": "Bearer garbage.token.value"})
        assert r.status_code == 401

    def test_task_validation_error(self, client, org_id):
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/tasks", json={"title": "no project"})
        assert r.status_code == 422


# ---------- Seed data ----------
class TestSeedData:
    def test_seed_project_and_tasks(self, client, org_id):
        projects = client.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
        assert any(p["name"] == "Web Redesign" for p in projects)
        tasks = client.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
        assert len(tasks) >= 5
        for t in tasks:
            assert "_id" not in t


# ---------- Team Activity (new feature) ----------
class TestTeamActivity:
    state = {}

    def test_team_activity_owner_200_and_shape(self, client, org_id):
        r = client.get(f"{BASE_URL}/api/orgs/{org_id}/team-activity")
        assert r.status_code == 200, r.text
        d = r.json()
        assert "generated_at" in d and isinstance(d["generated_at"], str)
        assert isinstance(d["members"], list) and d["members"]
        for m in d["members"]:
            for k in ["user_id", "name", "email", "role", "active_timer", "active_count",
                      "in_progress_tasks", "estimate_hours", "logged_today_minutes",
                      "logged_week_minutes", "recent_activity"]:
                assert k in m, f"missing key {k}"
            assert "_id" not in m and "password_hash" not in m
            assert isinstance(m["active_count"], int)
            assert isinstance(m["in_progress_tasks"], list)
            assert isinstance(m["recent_activity"], list)
            assert isinstance(m["estimate_hours"], (int, float))
            assert isinstance(m["logged_today_minutes"], int)
            assert isinstance(m["logged_week_minutes"], int)
            for t in m["in_progress_tasks"]:
                for k in ["task_id", "title", "priority", "project_key", "due_date"]:
                    assert k in t
            for e in m["recent_activity"]:
                for k in ["task_id", "task_title", "minutes", "date", "note", "created_at"]:
                    assert k in e

    def test_active_timer_populated(self, client, org_id):
        tasks = client.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
        assert tasks
        tid = tasks[0]["task_id"]
        me = client.get(f"{BASE_URL}/api/auth/me").json()
        my_uid = me["user_id"]
        # ensure clean state
        client.post(f"{BASE_URL}/api/orgs/{org_id}/timer/stop")
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/timer/start", json={"task_id": tid})
        assert r.status_code == 200, r.text
        time.sleep(2)
        d = client.get(f"{BASE_URL}/api/orgs/{org_id}/team-activity").json()
        row = next(m for m in d["members"] if m["user_id"] == my_uid)
        at = row["active_timer"]
        assert at is not None, "active_timer not populated for running timer"
        assert at["task_id"] == tid
        assert at["task_title"] == tasks[0]["title"]
        assert at["elapsed_seconds"] >= 1
        assert isinstance(at["started_at"], str)
        assert "project_key" in at
        # timer holder sorted first
        assert d["members"][0]["active_timer"] is not None
        # cleanup
        client.post(f"{BASE_URL}/api/orgs/{org_id}/timer/stop")
        d2 = client.get(f"{BASE_URL}/api/orgs/{org_id}/team-activity").json()
        row2 = next(m for m in d2["members"] if m["user_id"] == my_uid)
        assert row2["active_timer"] is None

    def test_member_role_forbidden(self, client, org_id):
        email = f"test_qa_member_{int(time.time())}@example.com"
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/members",
                        json={"email": email, "name": "TEST_RegularMember", "role": "member"})
        assert r.status_code == 200, r.text
        TestTeamActivity.state["member_email"] = email
        s = requests.Session()
        lr = s.post(f"{BASE_URL}/api/auth/login", json={"email": email, "password": "Welcome@123"})
        assert lr.status_code == 200, lr.text
        tok = lr.json()["token"]
        mr = requests.get(f"{BASE_URL}/api/orgs/{org_id}/team-activity",
                          headers={"Authorization": f"Bearer {tok}"})
        assert mr.status_code == 403, f"expected 403 got {mr.status_code}: {mr.text[:200]}"

    def test_unauthenticated_401(self, org_id):
        r = requests.get(f"{BASE_URL}/api/orgs/{org_id}/team-activity")
        assert r.status_code == 401

    def test_non_member_org_403(self, client):
        r = client.get(f"{BASE_URL}/api/orgs/org_doesnotexist999/team-activity")
        assert r.status_code == 403
