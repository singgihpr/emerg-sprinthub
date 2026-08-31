"""Iteration 11: project delete cascade, project status field, sprint list filters."""
import os

import pytest
import requests
from dotenv import dotenv_values

frontend_env = dotenv_values("/app/frontend/.env")
base_url = os.environ.get("REACT_APP_BACKEND_URL") or frontend_env.get("REACT_APP_BACKEND_URL")
if not base_url:
    raise RuntimeError("REACT_APP_BACKEND_URL missing")
BASE_URL = base_url.rstrip("/")

ADMIN_EMAIL = "widiardhana@gmail.com"
ADMIN_PW = "Admin@1234"
MEMBER_EMAIL = "test_qa_member_1787903479@example.com"
MEMBER_PW = "Welcome@123"


def _login(email, password):
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json"})
    r = s.post(f"{BASE_URL}/api/auth/login", json={"email": email, "password": password})
    if r.status_code != 200:
        pytest.fail(f"Login failed for {email}: {r.status_code} {r.text[:300]}")
    s.headers.update({"Authorization": f"Bearer {r.json()['token']}"})
    return s


@pytest.fixture(scope="module")
def admin():
    return _login(ADMIN_EMAIL, ADMIN_PW)


@pytest.fixture(scope="module")
def member():
    return _login(MEMBER_EMAIL, MEMBER_PW)


@pytest.fixture(scope="module")
def org_id(admin):
    r = admin.get(f"{BASE_URL}/api/orgs")
    assert r.status_code == 200, r.text
    orgs = r.json()
    seeded = next((o for o in orgs if o["name"].startswith("Acme")), orgs[0])
    return seeded["org_id"]


def _mkproject(sess, org_id, suffix, **extra):
    body = {"name": f"TEST_QA {suffix}", "key": f"T{suffix[:4].upper()}", "description": "throwaway"}
    body.update(extra)
    r = sess.post(f"{BASE_URL}/api/orgs/{org_id}/projects", json=body)
    assert r.status_code == 200, r.text
    return r.json()


# ---------- project status on create / patch ----------
class TestProjectStatus:
    def test_create_defaults_to_active(self, admin, org_id):
        p = _mkproject(admin, org_id, "stdef1")
        try:
            assert p["status"] == "active"
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            got = next(x for x in listed if x["project_id"] == p["project_id"])
            assert got["status"] == "active"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{p['project_id']}")

    def test_create_accepts_explicit_status(self, admin, org_id):
        p = _mkproject(admin, org_id, "stexp1", status="planning")
        try:
            assert p["status"] == "planning"
            got = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in got if x["project_id"] == p["project_id"])["status"] == "planning"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{p['project_id']}")

    def test_create_rejects_invalid_status(self, admin, org_id):
        r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects",
                       json={"name": "TEST_QA badstatus", "key": "TBAD1", "status": "bogus"})
        if r.status_code == 200:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{r.json()['project_id']}")
        assert r.status_code in (400, 422), f"invalid status accepted on create: {r.status_code}"

    def test_patch_status_on_hold_persists(self, admin, org_id):
        p = _mkproject(admin, org_id, "stpatch")
        pid = p["project_id"]
        try:
            r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"status": "on_hold"})
            assert r.status_code == 200, r.text
            assert r.json()["status"] == "on_hold"
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in listed if x["project_id"] == pid)["status"] == "on_hold"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    @pytest.mark.parametrize("st", ["planning", "active", "on_hold", "archived"])
    def test_patch_all_valid_statuses(self, admin, org_id, st):
        p = _mkproject(admin, org_id, f"sv{st[:3]}")
        pid = p["project_id"]
        try:
            r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"status": st})
            assert r.status_code == 200, r.text
            assert r.json()["status"] == st
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_patch_invalid_status_rejected(self, admin, org_id):
        p = _mkproject(admin, org_id, "stinval")
        pid = p["project_id"]
        try:
            r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"status": "nope"})
            assert r.status_code in (400, 422), r.text
            # unchanged
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in listed if x["project_id"] == pid)["status"] == "active"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_seeded_projects_have_status(self, admin, org_id):
        listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
        web = next((p for p in listed if p["name"] == "Web Redesign"), None)
        assert web is not None, "seeded 'Web Redesign' project missing"
        assert web.get("status") == "active", f"backfill did not set status: {web.get('status')}"


# ---------- delete cascade + RBAC ----------
class TestProjectDelete:
    def test_delete_cascades_all_children(self, admin, org_id):
        p = _mkproject(admin, org_id, "casc")
        pid = p["project_id"]

        sr = admin.post(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                        json={"project_id": pid, "name": "TEST_QA sprint casc"})
        assert sr.status_code == 200, sr.text
        sid = sr.json()["sprint_id"]

        tr = admin.post(f"{BASE_URL}/api/orgs/{org_id}/tasks",
                        json={"project_id": pid, "sprint_id": sid, "title": "TEST_QA task casc"})
        assert tr.status_code == 200, tr.text
        tid = tr.json()["task_id"]

        cr = admin.post(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments", json={"body": "TEST_QA comment"})
        assert cr.status_code == 200, cr.text

        ter = admin.post(f"{BASE_URL}/api/orgs/{org_id}/time-entries",
                         json={"task_id": tid, "minutes": 30, "note": "TEST_QA entry"})
        assert ter.status_code == 200, ter.text
        eid = ter.json()["entry_id"]
        all_te = admin.get(f"{BASE_URL}/api/orgs/{org_id}/time-entries")
        assert all_te.status_code == 200 and eid in [x["entry_id"] for x in all_te.json()]

        # leave an ACTIVE timer running on the task
        st = admin.post(f"{BASE_URL}/api/orgs/{org_id}/timer/start", json={"task_id": tid})
        assert st.status_code == 200, st.text
        assert admin.get(f"{BASE_URL}/api/orgs/{org_id}/timer").json().get("task_id") == tid

        dr = admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")
        assert dr.status_code == 200, dr.text
        body = dr.json()
        assert body.get("ok") is True
        assert body.get("deleted_tasks") == 1, body

        # project gone
        assert pid not in [x["project_id"] for x in admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()]
        # sprint gone
        assert sid not in [x["sprint_id"] for x in admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints").json()]
        # task gone
        assert tid not in [x["task_id"] for x in admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()]
        # comments removed / task unreachable
        cr2 = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}/comments")
        assert cr2.status_code in (403, 404) or cr2.json() == [], f"{cr2.status_code} {cr2.text[:200]}"
        # time entry removed
        assert eid not in [x["entry_id"] for x in admin.get(f"{BASE_URL}/api/orgs/{org_id}/time-entries").json()]
        # no active timer left
        at = admin.get(f"{BASE_URL}/api/orgs/{org_id}/timer")
        assert at.status_code == 200
        assert not at.json() or at.json().get("task_id") != tid

    def test_delete_unknown_project_404(self, admin, org_id):
        r = admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/prj_does_not_exist")
        assert r.status_code == 404, r.text

    def test_member_cannot_delete(self, admin, member, org_id):
        p = _mkproject(admin, org_id, "rbacm")
        pid = p["project_id"]
        try:
            r = member.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")
            assert r.status_code == 403, r.text
            # still exists
            assert pid in [x["project_id"] for x in admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()]
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_manager_cannot_delete(self, admin, org_id):
        """Invite a throwaway manager and assert 403 on project delete."""
        import time
        email = f"test_qa_mgr_{int(time.time())}@example.com"
        inv = admin.post(f"{BASE_URL}/api/orgs/{org_id}/members",
                         json={"email": email, "name": "TEST_QA Manager", "role": "manager"})
        assert inv.status_code == 200, inv.text
        mgr = _login(email, "Welcome@123")
        orgs = mgr.get(f"{BASE_URL}/api/orgs").json()
        acme = next(o for o in orgs if o["org_id"] == org_id)
        assert acme["role"] == "manager", acme

        p = _mkproject(admin, org_id, "rbacmg")
        pid = p["project_id"]
        try:
            r = mgr.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")
            assert r.status_code == 403, f"manager was able to delete: {r.status_code} {r.text[:200]}"
            assert pid in [x["project_id"] for x in admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()]
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_unauthenticated_cannot_delete(self, admin, org_id):
        p = _mkproject(admin, org_id, "rbacan")
        pid = p["project_id"]
        try:
            r = requests.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")
            assert r.status_code in (401, 403), r.status_code
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_seed_project_still_intact(self, admin, org_id):
        projects = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
        web = next((p for p in projects if p["name"] == "Web Redesign"), None)
        assert web is not None, "seeded project was deleted!"
        tasks = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks", params={"project_id": web["project_id"]}).json()
        assert len(tasks) >= 5, f"seed tasks missing, only {len(tasks)}"


# ---------- sprint filters ----------
class TestSprintFilters:
    @pytest.fixture(scope="class")
    def fixture_data(self, admin, org_id):
        p = _mkproject(admin, org_id, "spflt")
        pid = p["project_id"]
        s = admin.post(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                       json={"project_id": pid, "name": "TEST_QA flt sprint"})
        assert s.status_code == 200, s.text
        yield {"pid": pid, "sid": s.json()["sprint_id"]}
        admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_no_params_returns_all(self, admin, org_id, fixture_data):
        r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints")
        assert r.status_code == 200, r.text
        ids = [x["sprint_id"] for x in r.json()]
        assert fixture_data["sid"] in ids

    def test_filter_by_project_id(self, admin, org_id, fixture_data):
        r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints", params={"project_id": fixture_data["pid"]})
        assert r.status_code == 200, r.text
        data = r.json()
        assert len(data) == 1
        assert data[0]["sprint_id"] == fixture_data["sid"]
        assert all(x["project_id"] == fixture_data["pid"] for x in data)

    def test_filter_by_status(self, admin, org_id, fixture_data):
        r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints", params={"status": "planned"})
        assert r.status_code == 200, r.text
        data = r.json()
        assert all(x["status"] == "planned" for x in data)
        assert fixture_data["sid"] in [x["sprint_id"] for x in data]

        r2 = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints", params={"status": "completed"})
        assert r2.status_code == 200
        assert fixture_data["sid"] not in [x["sprint_id"] for x in r2.json()]

    def test_filter_by_both(self, admin, org_id, fixture_data):
        r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                      params={"project_id": fixture_data["pid"], "status": "planned"})
        assert r.status_code == 200
        assert [x["sprint_id"] for x in r.json()] == [fixture_data["sid"]]

        r2 = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                       params={"project_id": fixture_data["pid"], "status": "active"})
        assert r2.status_code == 200
        assert r2.json() == []

    def test_unknown_project_id_returns_empty(self, admin, org_id):
        r = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints", params={"project_id": "prj_nope"})
        assert r.status_code == 200
        assert r.json() == []

    def test_member_restricted_project_returns_empty(self, member, org_id, fixture_data):
        """Member is not a project member of the throwaway project -> ACL must return []."""
        r = member.get(f"{BASE_URL}/api/orgs/{org_id}/sprints", params={"project_id": fixture_data["pid"]})
        assert r.status_code == 200, r.text
        assert r.json() == [], "ACL leak: member saw sprints of a project they cannot access"

    def test_member_unfiltered_list_excludes_restricted(self, member, org_id, fixture_data):
        r = member.get(f"{BASE_URL}/api/orgs/{org_id}/sprints")
        assert r.status_code == 200
        assert fixture_data["sid"] not in [x["sprint_id"] for x in r.json()]
