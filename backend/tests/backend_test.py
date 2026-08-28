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
    seeded = next((o for o in orgs if o["name"] == "Acme Corp"), orgs[0])
    return seeded["org_id"]


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


# ---------- Sprint Burndown (iteration 5) ----------
from datetime import date, datetime as _dt, timedelta as _td  # noqa: E402


class TestBurndown:
    state = {}

    def test_burndown_owner_200_and_shape(self, client, org_id):
        pr = client.get(f"{BASE_URL}/api/orgs/{org_id}/sprints")
        assert pr.status_code == 200, pr.text
        sprints = pr.json()
        assert sprints, "no sprints seeded"
        sp = sprints[0]
        TestBurndown.state["sprint_id"] = sp["sprint_id"]
        r = client.get(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sp['sprint_id']}/burndown")
        assert r.status_code == 200, r.text
        d = r.json()
        for k in ["sprint_id", "sprint_name", "start_date", "end_date", "total_tasks",
                  "completed_tasks", "total_estimate_hours", "series"]:
            assert k in d, f"missing field {k}: {d}"
        assert d["sprint_id"] == sp["sprint_id"]
        assert isinstance(d["series"], list) and d["series"]
        sd = _dt.fromisoformat(d["start_date"]).date()
        ed = _dt.fromisoformat(d["end_date"]).date()
        assert len(d["series"]) == (ed - sd).days + 1, \
            f"series len {len(d['series'])} != days {(ed - sd).days + 1}"
        TestBurndown.state["data"] = d

    def test_ideal_decreases_linearly_to_zero(self, client, org_id):
        d = TestBurndown.state.get("data")
        assert d, "prior test did not run"
        series = d["series"]
        total = d["total_estimate_hours"]
        assert abs(series[0]["ideal"] - total) < 0.02, f"first ideal {series[0]['ideal']} != total {total}"
        assert abs(series[-1]["ideal"]) < 0.02, f"last ideal should be 0, got {series[-1]['ideal']}"
        ideals = [p["ideal"] for p in series]
        assert all(ideals[i] >= ideals[i + 1] - 0.001 for i in range(len(ideals) - 1)), ideals
        # linearity: constant step
        if len(ideals) > 2:
            steps = [round(ideals[i] - ideals[i + 1], 2) for i in range(len(ideals) - 1)]
            assert max(steps) - min(steps) < 0.05, f"non-linear ideal steps {steps}"

    def test_actual_null_for_future_dates(self, client, org_id):
        d = TestBurndown.state.get("data")
        today = date.today()
        for p in d["series"]:
            pd = _dt.fromisoformat(p["date"]).date()
            if pd > today:
                assert p["actual"] is None, f"{p} should have null actual"
            else:
                assert isinstance(p["actual"], (int, float)), f"{p} actual should be numeric"

    def test_burndown_404_wrong_sprint(self, client, org_id):
        r = client.get(f"{BASE_URL}/api/orgs/{org_id}/sprints/sprint_nope123/burndown")
        assert r.status_code == 404, f"got {r.status_code}: {r.text[:200]}"

    def test_burndown_403_non_member_org(self, client):
        r = client.get(f"{BASE_URL}/api/orgs/org_doesnotexist999/sprints/s1/burndown")
        assert r.status_code == 403

    def test_burndown_401_unauthenticated(self, org_id):
        sid = TestBurndown.state.get("sprint_id", "s1")
        r = requests.get(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}/burndown")
        assert r.status_code == 401

    def test_burndown_403_for_member_role_user(self, client, org_id):
        """A user who IS a member of the org should be allowed (ensure_member allows any role)."""
        email = TestTeamActivity.state.get("member_email") or "test_qa_member_1787903479@example.com"
        s = requests.Session()
        lr = s.post(f"{BASE_URL}/api/auth/login", json={"email": email, "password": "Welcome@123"})
        if lr.status_code != 200:
            pytest.skip(f"member login unavailable: {lr.status_code}")
        tok = lr.json()["token"]
        sid = TestBurndown.state["sprint_id"]
        mr = requests.get(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}/burndown",
                          headers={"Authorization": f"Bearer {tok}"})
        assert mr.status_code == 200, f"org member should read burndown, got {mr.status_code}"


# ---------- Task Comments (iteration 5) ----------
class TestComments:
    state = {}

    @pytest.fixture(scope="class", autouse=True)
    def task(self, client, org_id):
        r = client.get(f"{BASE_URL}/api/orgs/{org_id}/tasks")
        assert r.status_code == 200, r.text
        tasks = r.json()
        assert tasks, "no tasks seeded"
        TestComments.state["task_id"] = tasks[0]["task_id"]
        return tasks[0]["task_id"]

    def test_create_comment_plain(self, client, org_id, creds):
        tid = TestComments.state["task_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments",
                        json={"body": "TEST_plain comment body"})
        assert r.status_code == 200, r.text
        d = r.json()
        assert d["body"] == "TEST_plain comment body"
        assert d["author_email"] == creds["email"]
        assert d["author_name"]
        assert d["mentions"] == [] and d["mentions_emails"] == []
        assert "comment_id" in d and "created_at" in d
        assert "_id" not in d
        TestComments.state["first_id"] = d["comment_id"]

    def test_create_comment_with_mention(self, client, org_id, creds):
        tid = TestComments.state["task_id"]
        member = TestTeamActivity.state.get("member_email") or "test_qa_member_1787903479@example.com"
        body = f"TEST_hey @{member} and @nobody_unknown@example.com please review"
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments", json={"body": body})
        assert r.status_code == 200, r.text
        d = r.json()
        assert member.lower() in d["mentions_emails"], d["mentions_emails"]
        assert "nobody_unknown@example.com" in d["mentions_emails"]
        assert len(d["mentions"]) == 1, f"only the existing user should resolve: {d['mentions']}"
        TestComments.state["second_id"] = d["comment_id"]

    def test_list_comments_sorted_and_enriched(self, client, org_id):
        tid = TestComments.state["task_id"]
        r = client.get(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments")
        assert r.status_code == 200, r.text
        items = r.json()
        assert isinstance(items, list) and len(items) >= 2
        ids = [c["comment_id"] for c in items]
        assert TestComments.state["first_id"] in ids
        assert TestComments.state["second_id"] in ids
        assert ids.index(TestComments.state["first_id"]) < ids.index(TestComments.state["second_id"])
        created = [c["created_at"] for c in items]
        assert created == sorted(created), "not sorted asc by created_at"
        for c in items:
            assert "_id" not in c
            assert c["author_email"], c
            assert c["author_name"], c
            assert "author_picture" in c

    def test_create_comment_404_missing_task(self, client, org_id):
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/tasks/task_nope999/comments",
                        json={"body": "TEST_x"})
        assert r.status_code == 404, f"got {r.status_code}: {r.text[:200]}"

    def test_comments_401_unauthenticated(self, org_id):
        tid = TestComments.state["task_id"]
        r = requests.get(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments")
        assert r.status_code == 401

    def test_comments_403_non_member(self, client):
        # iteration 7: require_task_access() resolves the task first, so an unknown
        # org+task now yields 404 (task not found) instead of 403. Both are acceptable
        # denials; no org data is leaked (real tasks in a foreign org still 403).
        r = client.get(f"{BASE_URL}/api/orgs/org_doesnotexist999/tasks/t1/comments")
        assert r.status_code in (403, 404), f"got {r.status_code}: {r.text[:200]}"

    def test_empty_body_validation(self, client, org_id):
        tid = TestComments.state["task_id"]
        r = client.post(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments", json={})
        assert r.status_code == 422, f"expected 422 got {r.status_code}"


# ---------- Weekly digest cron (iteration 5) ----------
class TestCronWeeklyDigest:
    def _secret(self):
        env = dotenv_values("/app/backend/.env")
        s = env.get("WEBHOOK_CRON_SECRET")
        if not s:
            pytest.fail("WEBHOOK_CRON_SECRET missing from /app/backend/.env")
        return s

    def test_no_auth_401(self):
        r = requests.post(f"{BASE_URL}/api/cron/weekly-digest")
        assert r.status_code == 401, f"got {r.status_code}: {r.text[:200]}"

    def test_wrong_secret_401(self):
        r = requests.post(f"{BASE_URL}/api/cron/weekly-digest",
                          headers={"Authorization": "Bearer nope", "X-Webhook-Id": "TEST_bad"})
        assert r.status_code == 401

    def test_queued_then_duplicate(self):
        secret = self._secret()
        wid = f"TEST_wh_{int(time.time())}"
        h = {"Authorization": f"Bearer {secret}", "X-Webhook-Id": wid}
        r1 = requests.post(f"{BASE_URL}/api/cron/weekly-digest", headers=h)
        assert r1.status_code == 200, r1.text
        assert r1.json() == {"ok": True, "queued": True}, r1.json()
        r2 = requests.post(f"{BASE_URL}/api/cron/weekly-digest", headers=h)
        assert r2.status_code == 200, r2.text
        assert r2.json() == {"ok": True, "duplicate": True}, r2.json()
        TestCronWeeklyDigest.wid = wid

    def test_cron_run_recorded_in_db(self):
        import asyncio
        from motor.motor_asyncio import AsyncIOMotorClient
        env = dotenv_values("/app/backend/.env")
        wid = getattr(TestCronWeeklyDigest, "wid", None)
        assert wid, "prior test did not run"

        async def check():
            cl = AsyncIOMotorClient(env["MONGO_URL"])
            doc = await cl[env["DB_NAME"]].cron_runs.find_one({"run_id": wid})
            cl.close()
            return doc
        doc = asyncio.get_event_loop().run_until_complete(check()) if False else asyncio.run(check())
        assert doc, f"no cron_runs record for {wid}"
        assert doc["name"] == "weekly-digest"
        assert doc["at"]

    def test_no_backend_exception_from_digest(self):
        """After queueing, backend log should not contain a digest traceback."""
        time.sleep(4)
        import subprocess
        out = subprocess.run(["tail", "-n", "120", "/var/log/supervisor/backend.err.log"],
                             capture_output=True, text=True).stdout
        bad = [ln for ln in out.splitlines() if "_build_and_send_digest" in ln or "Task exception was never retrieved" in ln]
        assert not bad, f"digest background task raised: {bad[:5]}"
