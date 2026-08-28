"""Iteration 8: verify iteration-7 scoping fixes (create_task / create_sprint / list_time_entries)
in BOTH detached and re-attached states, plus adjacent write-path probes.
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
    block = re.search(r'(?is)Test Regular Member(.*?)##', CRED).group(1)
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
    data = {"org_id": org_id, "project_id": web["project_id"], "member_id": me,
            "all_projects": [p["project_id"] for p in projects]}
    yield data
    # always restore membership
    admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{web['project_id']}/members",
               json={"user_id": me, "role": "member"})


def _detach(admin, ctx):
    for pid in ctx["all_projects"]:
        admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{pid}/members/{ctx['member_id']}")


def _attach(admin, ctx):
    return admin.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members",
                      json={"user_id": ctx["member_id"], "role": "member"})


# ---------------- DETACHED state: 0 project memberships ----------------
class TestDetachedMember:
    @pytest.fixture(scope="class", autouse=True)
    def setup(self, admin, ctx):
        _detach(admin, ctx)
        yield

    def test_create_task_403(self, admin, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks",
                        json={"project_id": ctx["project_id"], "title": "TEST_i8_task"})
        if r.status_code == 200:
            admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{r.json()['task_id']}")
        assert r.status_code == 403, f"expected 403, got {r.status_code} {r.text[:200]}"

    def test_create_sprint_403(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/sprints",
                        json={"project_id": ctx["project_id"], "name": "TEST_i8_sprint"})
        assert r.status_code == 403, f"expected 403, got {r.status_code} {r.text[:200]}"

    def test_time_entries_empty(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/time-entries")
        assert r.status_code == 200
        assert r.json() == [], f"leak: {len(r.json())} entries"

    def test_projects_tasks_sprints_empty(self, member, ctx):
        o = ctx["org_id"]
        assert member.get(f"{BASE_URL}/api/orgs/{o}/projects").json() == []
        assert member.get(f"{BASE_URL}/api/orgs/{o}/tasks").json() == []
        assert member.get(f"{BASE_URL}/api/orgs/{o}/sprints").json() == []

    def test_analytics_zero(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/analytics")
        assert r.status_code == 200
        d = r.json()
        assert d.get("total_tasks") == 0, d

    def test_task_write_and_comments_403(self, admin, member, ctx):
        o = ctx["org_id"]
        tasks = admin.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        tid = next(t["task_id"] for t in tasks if t["project_id"] == ctx["project_id"])
        assert member.patch(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}", json={"status": "done"}).status_code == 403
        assert member.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}").status_code == 403
        assert member.get(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}/comments").status_code == 403
        assert member.post(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}/comments", json={"body": "TEST_i8"}).status_code == 403

    def test_project_members_roster_403(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members")
        assert r.status_code == 403, r.status_code

    def test_project_patch_403(self, member, ctx):
        r = member.patch(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}",
                         json={"name": "TEST_i8_rename"})
        assert r.status_code == 403, r.status_code

    def test_create_time_entry_on_inaccessible_task(self, admin, member, ctx):
        """Iteration 9 fix: POST /time-entries must call require_task_access -> 403."""
        o = ctx["org_id"]
        tasks = admin.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        tid = next(t["task_id"] for t in tasks if t["project_id"] == ctx["project_id"])
        r = member.post(f"{BASE_URL}/api/orgs/{o}/time-entries", json={"task_id": tid, "minutes": 7,
                                                                       "note": "TEST_i8_probe"})
        leaked = r.status_code == 200
        if leaked:
            # cleanup: remove entry + decrement logged minutes
            import pymongo
            cli = pymongo.MongoClient(os.environ.get("MONGO_URL") or dotenv_values("/app/backend/.env")["MONGO_URL"])
            dbn = dotenv_values("/app/backend/.env")["DB_NAME"]
            cli[dbn].time_entries.delete_one({"entry_id": r.json()["entry_id"]})
            cli[dbn].tasks.update_one({"task_id": tid}, {"$inc": {"logged_minutes": -7}})
        assert not leaked, "GAP: detached member logged time on an inaccessible task (200)"


# ---------------- RE-ATTACHED state ----------------
class TestReattachedMember:
    @pytest.fixture(scope="class", autouse=True)
    def setup(self, admin, ctx):
        _detach(admin, ctx)
        r = _attach(admin, ctx)
        assert r.status_code == 200, r.text[:200]
        yield

    def test_scoped_data_visible(self, member, ctx):
        o = ctx["org_id"]
        projects = member.get(f"{BASE_URL}/api/orgs/{o}/projects").json()
        assert [p["project_id"] for p in projects] == [ctx["project_id"]]
        tasks = member.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        assert len(tasks) > 0 and all(t["project_id"] == ctx["project_id"] for t in tasks)

    def test_time_entries_scoped_non_empty(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/time-entries")
        assert r.status_code == 200
        entries = r.json()
        assert len(entries) > 0, "expected scoped entries after re-attach"
        allowed = {t["task_id"] for t in member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks").json()}
        assert all(e["task_id"] in allowed for e in entries)
        assert all("_id" not in e for e in entries)

    def test_create_sprint_still_403_role(self, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/sprints",
                        json={"project_id": ctx["project_id"], "name": "TEST_i8_sprint2"})
        assert r.status_code == 403, f"member role should not create sprints, got {r.status_code}"

    def test_create_task_role_behaviour(self, admin, member, ctx):
        """Iteration 9 decision: project members MAY create tasks in their own projects (200)."""
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks",
                        json={"project_id": ctx["project_id"], "title": "TEST_i8_task2"})
        if r.status_code == 200:
            admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{r.json()['task_id']}")
        assert r.status_code == 200, (
            f"attached role=member should be able to create a task in an accessible project, got {r.status_code}"
        )

    def test_project_patch_still_403(self, member, ctx):
        r = member.patch(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}",
                         json={"name": "TEST_i8_rename"})
        assert r.status_code == 403

    def test_duplicate_member_add(self, admin, ctx):
        r = _attach(admin, ctx)
        assert r.status_code == 400
        assert "Already a project member" in r.json().get("detail", "")

    def test_comments_accessible(self, member, ctx):
        o = ctx["org_id"]
        tasks = member.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        tid = tasks[0]["task_id"]
        assert member.get(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}/comments").status_code == 200

    def test_project_members_roster_200(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members")
        assert r.status_code == 200
        assert any(x.get("user_id") == ctx["member_id"] for x in r.json())


# ---------------- Owner regression ----------------
class TestOwnerFullAccess:
    def test_owner_sees_everything(self, admin, ctx):
        o = ctx["org_id"]
        assert len(admin.get(f"{BASE_URL}/api/orgs/{o}/projects").json()) >= 2
        assert len(admin.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()) >= 5
        assert admin.get(f"{BASE_URL}/api/orgs/{o}/time-entries").status_code == 200
        assert len(admin.get(f"{BASE_URL}/api/orgs/{o}/time-entries").json()) > 0
        an = admin.get(f"{BASE_URL}/api/orgs/{o}/analytics").json()
        assert an["total_tasks"] >= 5

    def test_owner_task_crud(self, admin, ctx):
        o = ctx["org_id"]
        r = admin.post(f"{BASE_URL}/api/orgs/{o}/tasks",
                       json={"project_id": ctx["project_id"], "title": "TEST_i8_owner"})
        assert r.status_code == 200
        tid = r.json()["task_id"]
        p = admin.patch(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}", json={"status": "done"})
        assert p.status_code == 200 and p.json()["status"] == "done"
        g = admin.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        assert any(t["task_id"] == tid and t["status"] == "done" for t in g)
        assert admin.delete(f"{BASE_URL}/api/orgs/{o}/tasks/{tid}").status_code == 200
        g2 = admin.get(f"{BASE_URL}/api/orgs/{o}/tasks").json()
        assert not any(t["task_id"] == tid for t in g2)
