"""Iteration 9: final ACL hardening verification.

Covers:
- create_time_entry + start_timer now call require_task_access -> 403 for a detached member
- after re-attach: member can create tasks, time entries, start/stop timer (200)
- regression of iteration 6/7/8 ACL behaviour for a detached member
Self-restoring: member is re-attached to 'Web Redesign' at module teardown.
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
    org_id = org["org_id"]
    me = member.get(f"{BASE_URL}/api/auth/me").json()["user_id"]
    projects = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
    web = next((p for p in projects if p["name"] == "Web Redesign"), projects[0])
    tasks = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
    tid = next(t["task_id"] for t in tasks if t["project_id"] == web["project_id"])
    data = {"org_id": org_id, "project_id": web["project_id"], "member_id": me,
            "task_id": tid, "all_projects": [p["project_id"] for p in projects]}
    yield data
    admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{web['project_id']}/members",
               json={"user_id": me, "role": "member"})


def _detach(admin, ctx):
    for pid in ctx["all_projects"]:
        admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{pid}/members/{ctx['member_id']}")


def _attach(admin, ctx):
    return admin.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members",
                      json={"user_id": ctx["member_id"], "role": "member"})


def _mongo():
    import pymongo
    env = dotenv_values("/app/backend/.env")
    cli = pymongo.MongoClient(os.environ.get("MONGO_URL") or env["MONGO_URL"])
    return cli[env["DB_NAME"]]


# ---------------- DETACHED: the two newly fixed write paths ----------------
class TestDetachedWritePaths:
    @pytest.fixture(scope="class", autouse=True)
    def setup(self, admin, ctx):
        _detach(admin, ctx)
        yield

    def test_create_time_entry_403(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/time-entries",
                        json={"task_id": ctx["task_id"], "minutes": 7, "note": "TEST_i9_probe"})
        if r.status_code == 200:
            d = _mongo()
            d.time_entries.delete_one({"entry_id": r.json()["entry_id"]})
            d.tasks.update_one({"task_id": ctx["task_id"]}, {"$inc": {"logged_minutes": -7}})
        assert r.status_code == 403, f"expected 403, got {r.status_code} {r.text[:200]}"

    def test_time_entry_not_persisted(self, admin, ctx):
        entries = admin.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/time-entries").json()
        assert not any(e.get("note") == "TEST_i9_probe" for e in entries)

    def test_timer_start_403(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/timer/start",
                        json={"task_id": ctx["task_id"]})
        if r.status_code == 200:
            member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/timer/stop")
        assert r.status_code == 403, f"expected 403, got {r.status_code} {r.text[:200]}"

    def test_timer_start_missing_task_id_400(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/timer/start", json={})
        assert r.status_code == 400, r.status_code

    def test_time_entry_unknown_task_404(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/time-entries",
                        json={"task_id": "tsk_does_not_exist", "minutes": 5})
        assert r.status_code == 404, r.status_code

    # ---- iteration 6/7/8 regressions still hold while detached ----
    def test_lists_empty(self, member, ctx):
        o = ctx["org_id"]
        assert member.get(f"{BASE_URL}/api/orgs/{o}/projects").json() == []
        assert member.get(f"{BASE_URL}/api/orgs/{o}/tasks").json() == []
        assert member.get(f"{BASE_URL}/api/orgs/{o}/sprints").json() == []
        assert member.get(f"{BASE_URL}/api/orgs/{o}/time-entries").json() == []

    def test_analytics_zero(self, member, ctx):
        d = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/analytics").json()
        assert d.get("total_tasks") == 0, d

    def test_task_writes_and_comments_403(self, member, ctx):
        o, tid = ctx["org_id"], ctx["task_id"]
        assert member.patch(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}", json={"status": "done"}).status_code == 403
        assert member.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}").status_code == 403
        assert member.get(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}/comments").status_code == 403
        assert member.post(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}/comments", json={"body": "TEST_i9"}).status_code == 403

    def test_create_task_and_sprint_403(self, admin, member, ctx):
        o = ctx["org_id"]
        r = member.post(f"{BASE_URL}/api/orgs/{o}/tasks",
                        json={"project_id": ctx["project_id"], "title": "TEST_i9_task"})
        if r.status_code == 200:
            admin.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{r.json()['task_id']}")
        assert r.status_code == 403, r.status_code
        assert member.post(f"{BASE_URL}/api/orgs/{o}/sprints",
                           json={"project_id": ctx["project_id"], "name": "TEST_i9_sprint"}).status_code == 403

    def test_project_roster_and_patch_403(self, member, ctx):
        o, p = ctx["org_id"], ctx["project_id"]
        assert member.get(f"{BASE_URL}/api/orgs/{o}/projects/{p}/members").status_code == 403
        assert member.patch(f"{BASE_URL}/api/orgs/{o}/projects/{p}", json={"name": "TEST_i9_rename"}).status_code == 403


# ---------------- RE-ATTACHED: member happy path ----------------
class TestReattachedMemberWrites:
    @pytest.fixture(scope="class", autouse=True)
    def setup(self, admin, ctx):
        _detach(admin, ctx)
        assert _attach(admin, ctx).status_code == 200
        yield
        # cleanup any TEST_ artifacts created by this class
        d = _mongo()
        d.tasks.delete_many({"title": {"$regex": "^TEST_i9"}})
        d.time_entries.delete_many({"note": {"$regex": "^TEST_i9"}})

    def test_member_can_create_task(self, admin, member, ctx):
        o = ctx["org_id"]
        r = member.post(f"{BASE_URL}/api/orgs/{o}/tasks",
                        json={"project_id": ctx["project_id"], "title": "TEST_i9_member_task"})
        assert r.status_code == 200, f"{r.status_code} {r.text[:200]}"
        body = r.json()
        assert body["title"] == "TEST_i9_member_task"
        assert body["project_id"] == ctx["project_id"]
        assert "_id" not in body
        tid = body["task_id"]
        # verify persisted and visible to the member
        tasks = member.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        assert any(t["task_id"] == tid for t in tasks)
        admin.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}")

    def test_member_can_create_time_entry(self, member, ctx):
        o = ctx["org_id"]
        r = member.post(f"{BASE_URL}/api/orgs/{o}/time-entries",
                        json={"task_id": ctx["task_id"], "minutes": 6, "note": "TEST_i9_entry"})
        assert r.status_code == 200, f"{r.status_code} {r.text[:200]}"
        body = r.json()
        assert body["minutes"] == 6 and body["task_id"] == ctx["task_id"]
        assert "_id" not in body
        entries = member.get(f"{BASE_URL}/api/orgs/{o}/time-entries").json()
        assert any(e["entry_id"] == body["entry_id"] for e in entries)
        # cleanup
        d = _mongo()
        d.time_entries.delete_one({"entry_id": body["entry_id"]})
        d.tasks.update_one({"task_id": ctx["task_id"]}, {"$inc": {"logged_minutes": -6}})

    def test_member_timer_start_stop(self, member, ctx):
        o = ctx["org_id"]
        s = member.post(f"{BASE_URL}/api/orgs/{o}/timer/start", json={"task_id": ctx["task_id"]})
        assert s.status_code == 200, f"{s.status_code} {s.text[:200]}"
        assert s.json()["task_id"] == ctx["task_id"]
        active = member.get(f"{BASE_URL}/api/orgs/{o}/timer").json()
        assert active.get("task_id") == ctx["task_id"]
        st = member.post(f"{BASE_URL}/api/orgs/{o}/timer/stop")
        assert st.status_code == 200, f"{st.status_code} {st.text[:200]}"
        entry = st.json()
        assert member.get(f"{BASE_URL}/api/orgs/{o}/timer").json() == {}
        # cleanup the generated entry
        eid = entry.get("entry_id") or (entry.get("entry") or {}).get("entry_id")
        if eid:
            d = _mongo()
            te = d.time_entries.find_one({"entry_id": eid})
            d.time_entries.delete_one({"entry_id": eid})
            if te:
                d.tasks.update_one({"task_id": te["task_id"]}, {"$inc": {"logged_minutes": -te["minutes"]}})

    def test_member_cannot_touch_other_project_task(self, admin, member, ctx):
        """Task in a project the member is NOT a member of stays 403 for both write paths."""
        o = ctx["org_id"]
        other_pid = next((p for p in ctx["all_projects"] if p != ctx["project_id"]), None)
        if not other_pid:
            pytest.skip("only one project in the org")
        created = admin.post(f"{BASE_URL}/api/orgs/{o}/tasks",
                             json={"project_id": other_pid, "title": "TEST_i9_other_project"})
        assert created.status_code == 200, created.text[:200]
        other_tid = created.json()["task_id"]
        try:
            assert member.post(f"{BASE_URL}/api/orgs/{o}/time-entries",
                               json={"task_id": other_tid, "minutes": 3}).status_code == 403
            assert member.post(f"{BASE_URL}/api/orgs/{o}/timer/start",
                               json={"task_id": other_tid}).status_code == 403
            assert member.post(f"{BASE_URL}/api/orgs/{o}/tasks",
                               json={"project_id": other_pid, "title": "TEST_i9_other_create"}).status_code == 403
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{other_tid}")

    def test_scoped_reads(self, member, ctx):
        o = ctx["org_id"]
        projects = member.get(f"{BASE_URL}/api/orgs/{o}/projects").json()
        assert [p["project_id"] for p in projects] == [ctx["project_id"]]
        entries = member.get(f"{BASE_URL}/api/orgs/{o}/time-entries").json()
        allowed = {t["task_id"] for t in member.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()}
        assert all(e["task_id"] in allowed for e in entries)

    def test_sprint_create_still_role_blocked(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/sprints",
                        json={"project_id": ctx["project_id"], "name": "TEST_i9_sprint2"})
        assert r.status_code == 403, r.status_code


# ---------------- Owner regression ----------------
class TestOwnerRegression:
    def test_owner_writes(self, admin, ctx):
        o = ctx["org_id"]
        t = admin.post(f"{BASE_URL}/api/orgs/{o}/tasks",
                       json={"project_id": ctx["project_id"], "title": "TEST_i9_owner"})
        assert t.status_code == 200
        tid = t.json()["task_id"]
        te = admin.post(f"{BASE_URL}/api/orgs/{o}/time-entries", json={"task_id": tid, "minutes": 5})
        assert te.status_code == 200
        s = admin.post(f"{BASE_URL}/api/orgs/{o}/timer/start", json={"task_id": tid})
        assert s.status_code == 200
        assert admin.post(f"{BASE_URL}/api/orgs/{o}/timer/stop").status_code == 200
        assert admin.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}").status_code == 200
        d = _mongo()
        d.time_entries.delete_many({"task_id": tid})

    def test_owner_reads(self, admin, ctx):
        o = ctx["org_id"]
        assert len(admin.get(f"{BASE_URL}/api/orgs/{o}/projects").json()) >= 2
        assert len(admin.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()) >= 5
        an = admin.get(f"{BASE_URL}/api/orgs/{o}/analytics").json()
        assert an["total_tasks"] >= 5
