"""Iteration 12 fix verification:
FIX 1 - POST /projects validates status via Pydantic Literal (422 for garbage).
FIX 2 - PATCH /projects validates status (422) AND enforces require_project_access
        (manager who is not a project member -> 403).
"""
import os
import time

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
VALID_STATUSES = ["planning", "active", "on_hold", "archived"]


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
    body = {"name": f"TEST_QA i12 {suffix}", "key": f"I12{suffix[:3].upper()}", "description": "throwaway"}
    body.update(extra)
    r = sess.post(f"{BASE_URL}/api/orgs/{org_id}/projects", json=body)
    assert r.status_code == 200, r.text
    return r.json()


@pytest.fixture(scope="module")
def manager(admin, org_id):
    """A manager-role org member who is NOT a member of any project."""
    email = f"test_qa_mgr_{int(time.time())}@example.com"
    r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/members",
                   json={"email": email, "name": "TEST_QA Manager i12", "role": "manager"})
    assert r.status_code == 200, r.text
    return _login(email, "Welcome@123")


# ---------------- FIX 1: status Literal on POST ----------------
class TestCreateStatusLiteral:
    def test_create_rejects_garbage_status_422(self, admin, org_id):
        r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects",
                       json={"name": "TEST_QA i12 bad", "key": "I12BAD", "status": "garbage"})
        if r.status_code == 200:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{r.json()['project_id']}")
            pytest.fail("garbage status accepted on create")
        assert r.status_code == 422, f"expected 422, got {r.status_code}: {r.text[:300]}"
        body = r.json()
        assert "detail" in body

    def test_create_garbage_status_not_persisted(self, admin, org_id):
        listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
        bad = [p for p in listed if p.get("status") not in VALID_STATUSES]
        assert bad == [], f"projects with invalid status present: {[(p['name'], p.get('status')) for p in bad]}"

    @pytest.mark.parametrize("st", VALID_STATUSES)
    def test_create_accepts_valid_status(self, admin, org_id, st):
        p = _mkproject(admin, org_id, f"c{st[:3]}", status=st)
        pid = p["project_id"]
        try:
            assert p["status"] == st
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in listed if x["project_id"] == pid)["status"] == st
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_create_omitted_status_defaults_active(self, admin, org_id):
        p = _mkproject(admin, org_id, "def")
        pid = p["project_id"]
        try:
            assert p["status"] == "active"
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in listed if x["project_id"] == pid)["status"] == "active"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")


# ---------------- FIX 2a: status Literal on PATCH ----------------
class TestPatchStatusLiteral:
    def test_patch_rejects_garbage_status_422(self, admin, org_id):
        p = _mkproject(admin, org_id, "pbad", status="planning")
        pid = p["project_id"]
        try:
            r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"status": "garbage"})
            assert r.status_code == 422, f"expected 422, got {r.status_code}: {r.text[:300]}"
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in listed if x["project_id"] == pid)["status"] == "planning"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    @pytest.mark.parametrize("st", VALID_STATUSES)
    def test_patch_accepts_valid_status(self, admin, org_id, st):
        p = _mkproject(admin, org_id, f"p{st[:3]}")
        pid = p["project_id"]
        try:
            r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"status": st})
            assert r.status_code == 200, r.text
            assert r.json()["status"] == st
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in listed if x["project_id"] == pid)["status"] == st
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")


# ---------------- FIX 2b: PATCH project ACL ----------------
class TestPatchProjectAcl:
    def test_manager_not_project_member_gets_403(self, admin, manager, org_id):
        p = _mkproject(admin, org_id, "acl1")
        pid = p["project_id"]
        try:
            # manager cannot even see it
            visible = manager.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert all(x["project_id"] != pid for x in visible), "manager should not see non-member project"
            r = manager.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}", json={"name": "TEST_QA hijacked"})
            assert r.status_code == 403, f"expected 403, got {r.status_code}: {r.text[:300]}"
            # unchanged
            got = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            assert next(x for x in got if x["project_id"] == pid)["name"] == p["name"]
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_manager_who_is_project_member_can_patch(self, admin, manager, org_id):
        me = manager.get(f"{BASE_URL}/api/auth/me").json()
        p = _mkproject(admin, org_id, "acl2")
        pid = p["project_id"]
        try:
            add = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}/members",
                             json={"user_id": me["user_id"], "role": "member"})
            assert add.status_code == 200, add.text
            r = manager.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}",
                              json={"name": "TEST_QA i12 acl2 renamed", "status": "on_hold"})
            assert r.status_code == 200, r.text
            assert r.json()["name"] == "TEST_QA i12 acl2 renamed"
            assert r.json()["status"] == "on_hold"
            listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
            got = next(x for x in listed if x["project_id"] == pid)
            assert got["name"] == "TEST_QA i12 acl2 renamed"
            assert got["status"] == "on_hold"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pid}")

    def test_org_member_project_member_still_403(self, admin, member, org_id):
        """member role is below manager -> 403 even though they are a project member."""
        listed = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
        web = next((p for p in listed if p["name"] == "Web Redesign"), None)
        assert web is not None
        r = member.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/{web['project_id']}",
                         json={"name": "TEST_QA nope"})
        assert r.status_code == 403, f"expected 403, got {r.status_code}: {r.text[:300]}"
        after = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
        assert next(x for x in after if x["project_id"] == web["project_id"])["name"] == "Web Redesign"

    def test_patch_unknown_project_404_or_403(self, admin, org_id):
        r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/prj_doesnotexist", json={"name": "x"})
        assert r.status_code in (403, 404), f"got {r.status_code}: {r.text[:200]}"

    def test_patch_anonymous_401(self, org_id):
        listed = requests.get(f"{BASE_URL}/api/orgs/{org_id}/projects")
        assert listed.status_code in (401, 403)
        r = requests.patch(f"{BASE_URL}/api/orgs/{org_id}/projects/prj_x", json={"name": "x"})
        assert r.status_code in (401, 403)
