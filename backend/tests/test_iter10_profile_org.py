"""Iteration 10: org update (PATCH /orgs/{id}), profile update (PATCH /auth/me),
change password (POST /auth/change-password)."""
import os
import re
from pathlib import Path

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


# ---------- PATCH /orgs/{org_id} ----------
class TestOrgUpdate:
    def test_owner_can_update_name_and_logo(self, admin, org_id):
        original = admin.get(f"{BASE_URL}/api/orgs").json()
        orig = next(o for o in original if o["org_id"] == org_id)
        try:
            payload = {"name": "TEST_Acme Renamed", "logo": "https://example.com/logo.png"}
            r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}", json=payload)
            assert r.status_code == 200, r.text
            data = r.json()
            assert data["name"] == payload["name"]
            assert data["logo"] == payload["logo"]
            assert data["role"] in ("owner", "admin")
            assert "_id" not in data
            # verify persistence via GET /orgs
            after = next(o for o in admin.get(f"{BASE_URL}/api/orgs").json() if o["org_id"] == org_id)
            assert after["name"] == payload["name"]
            assert after["logo"] == payload["logo"]
        finally:
            admin.patch(f"{BASE_URL}/api/orgs/{org_id}",
                        json={"name": orig["name"], "logo": orig.get("logo") or ""})

    def test_member_forbidden(self, member, org_id):
        r = member.patch(f"{BASE_URL}/api/orgs/{org_id}", json={"name": "TEST_hack"})
        assert r.status_code == 403, f"expected 403, got {r.status_code}: {r.text[:200]}"

    def test_empty_body_400(self, admin, org_id):
        r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}", json={})
        assert r.status_code == 400, f"expected 400, got {r.status_code}: {r.text[:200]}"

    def test_missing_org_404(self, admin):
        r = admin.patch(f"{BASE_URL}/api/orgs/org_does_not_exist_123", json={"name": "TEST_x"})
        assert r.status_code == 404, f"expected 404, got {r.status_code}: {r.text[:200]}"

    def test_unauthenticated_401(self, org_id):
        r = requests.patch(f"{BASE_URL}/api/orgs/{org_id}", json={"name": "TEST_x"})
        assert r.status_code in (401, 403), r.status_code

    def test_create_org_still_works(self, admin):
        r = admin.post(f"{BASE_URL}/api/orgs", json={"name": "TEST_QA Org iter10"})
        assert r.status_code == 200, r.text
        data = r.json()
        assert data["role"] == "owner"
        assert "_id" not in data
        new_org_id = data["org_id"]
        listed = admin.get(f"{BASE_URL}/api/orgs").json()
        assert any(o["org_id"] == new_org_id for o in listed)
        # newly created org is editable by its owner
        r2 = admin.patch(f"{BASE_URL}/api/orgs/{new_org_id}", json={"name": "TEST_QA Org iter10b"})
        assert r2.status_code == 200, r2.text
        assert r2.json()["name"] == "TEST_QA Org iter10b"


# ---------- PATCH /auth/me ----------
class TestProfileUpdate:
    def test_update_name_and_picture(self, admin):
        me = admin.get(f"{BASE_URL}/api/auth/me").json()
        orig_name, orig_pic = me.get("name"), me.get("picture")
        try:
            r = admin.patch(f"{BASE_URL}/api/auth/me",
                            json={"name": "TEST_Admin QA", "picture": "https://example.com/a.png"})
            assert r.status_code == 200, r.text
            data = r.json()
            assert data["name"] == "TEST_Admin QA"
            assert data["picture"] == "https://example.com/a.png"
            assert "password_hash" not in data
            assert "_id" not in data
            assert data["email"] == ADMIN_EMAIL
            again = admin.get(f"{BASE_URL}/api/auth/me").json()
            assert again["name"] == "TEST_Admin QA"
            assert again["picture"] == "https://example.com/a.png"
        finally:
            admin.patch(f"{BASE_URL}/api/auth/me",
                        json={"name": orig_name, "picture": orig_pic or ""})
            restored = admin.get(f"{BASE_URL}/api/auth/me").json()
            assert restored["name"] == orig_name

    def test_name_only_keeps_picture(self, admin):
        me = admin.get(f"{BASE_URL}/api/auth/me").json()
        orig_name = me.get("name")
        try:
            r = admin.patch(f"{BASE_URL}/api/auth/me", json={"name": "TEST_OnlyName"})
            assert r.status_code == 200, r.text
            assert r.json()["name"] == "TEST_OnlyName"
            assert r.json().get("picture") == me.get("picture")
        finally:
            admin.patch(f"{BASE_URL}/api/auth/me", json={"name": orig_name})

    def test_empty_body_400(self, admin):
        r = admin.patch(f"{BASE_URL}/api/auth/me", json={})
        assert r.status_code == 400, f"expected 400, got {r.status_code}: {r.text[:200]}"

    def test_unauthenticated(self):
        r = requests.patch(f"{BASE_URL}/api/auth/me", json={"name": "TEST_x"})
        assert r.status_code in (401, 403), r.status_code


# ---------- POST /auth/change-password ----------
@pytest.fixture(scope="module")
def throwaway_user():
    """Dedicated registered user so shared admin/member passwords are never touched."""
    import time
    email = f"test_qa_pw_{int(time.time())}@example.com"
    pw = "Welcome@123"
    s = requests.Session()
    s.headers.update({"Content-Type": "application/json"})
    r = s.post(f"{BASE_URL}/api/auth/register", json={"email": email, "password": pw, "name": "TEST_PW User"})
    if r.status_code != 200:
        pytest.fail(f"register failed {r.status_code}: {r.text[:300]}")
    s.headers.update({"Authorization": f"Bearer {r.json()['token']}"})
    return {"session": s, "email": email, "password": pw}


class TestChangePassword:
    """Uses a throwaway registered account so the admin/member passwords are untouched."""

    def test_wrong_current_password_400(self, throwaway_user):
        r = throwaway_user["session"].post(f"{BASE_URL}/api/auth/change-password",
                        json={"current_password": "TotallyWrong1!", "new_password": "Newpass@123"})
        assert r.status_code == 400, f"expected 400, got {r.status_code}: {r.text[:200]}"
        assert "incorrect" in str(r.json().get("detail", "")).lower()

    def test_short_new_password_422(self, throwaway_user):
        r = throwaway_user["session"].post(f"{BASE_URL}/api/auth/change-password",
                        json={"current_password": throwaway_user["password"], "new_password": "abc"})
        assert r.status_code == 422, f"expected 422, got {r.status_code}: {r.text[:200]}"

    def test_missing_current_password_400(self, throwaway_user):
        r = throwaway_user["session"].post(f"{BASE_URL}/api/auth/change-password",
                        json={"new_password": "Newpass@123"})
        assert r.status_code == 400, f"expected 400, got {r.status_code}: {r.text[:200]}"

    def test_change_then_restore(self, throwaway_user):
        member = throwaway_user["session"]
        MEMBER_EMAIL = throwaway_user["email"]
        MEMBER_PW = throwaway_user["password"]
        temp_pw = "TempPass@9876"
        r = member.post(f"{BASE_URL}/api/auth/change-password",
                        json={"current_password": MEMBER_PW, "new_password": temp_pw})
        assert r.status_code == 200, r.text
        assert r.json().get("ok") is True
        try:
            # old password rejected
            old = requests.post(f"{BASE_URL}/api/auth/login",
                                json={"email": MEMBER_EMAIL, "password": MEMBER_PW})
            assert old.status_code == 401, f"old password still works: {old.status_code}"
            # new password works
            new = requests.post(f"{BASE_URL}/api/auth/login",
                                json={"email": MEMBER_EMAIL, "password": temp_pw})
            assert new.status_code == 200, new.text
            assert new.json()["email"] == MEMBER_EMAIL
        finally:
            s = _login(MEMBER_EMAIL, temp_pw)
            rr = s.post(f"{BASE_URL}/api/auth/change-password",
                        json={"current_password": temp_pw, "new_password": MEMBER_PW})
            assert rr.status_code == 200, f"RESTORE FAILED: {rr.text[:300]}"
            back = requests.post(f"{BASE_URL}/api/auth/login",
                                 json={"email": MEMBER_EMAIL, "password": MEMBER_PW})
            assert back.status_code == 200, "Original member password not restored!"

    def test_admin_password_intact(self):
        r = requests.post(f"{BASE_URL}/api/auth/login",
                          json={"email": ADMIN_EMAIL, "password": ADMIN_PW})
        assert r.status_code == 200, "Admin password broken by tests"

    def test_unauthenticated(self):
        r = requests.post(f"{BASE_URL}/api/auth/change-password",
                          json={"current_password": "x", "new_password": "abcdef1"})
        assert r.status_code in (401, 403), r.status_code
