"""Iteration 11 — Org logo file-upload endpoint tests (POST /api/orgs/{org_id}/logo)."""
import base64
import io
import os

import pytest
import requests
from dotenv import dotenv_values
from PIL import Image

frontend_env = dotenv_values("/app/frontend/.env")
base_url = os.environ.get("REACT_APP_BACKEND_URL") or frontend_env.get("REACT_APP_BACKEND_URL")
if not base_url:
    raise RuntimeError("REACT_APP_BACKEND_URL missing")
BASE_URL = base_url.rstrip("/")
API = f"{BASE_URL}/api"

ADMIN = {"email": "widiardhana@gmail.com", "password": "Admin@1234"}
MEMBER = {"email": "test_qa_member_1787903479@example.com", "password": "Welcome@123"}


def _login(creds):
    s = requests.Session()
    r = s.post(f"{API}/auth/login", json=creds, timeout=30)
    if r.status_code != 200:
        pytest.fail(f"Login failed for {creds['email']}: {r.status_code} {r.text[:300]}")
    return s


@pytest.fixture(scope="module")
def admin():
    return _login(ADMIN)


@pytest.fixture(scope="module")
def member():
    return _login(MEMBER)


@pytest.fixture(scope="module")
def org_id(admin):
    r = admin.get(f"{API}/orgs", timeout=30)
    assert r.status_code == 200, r.text
    orgs = r.json()
    assert isinstance(orgs, list) and orgs
    return orgs[0]["org_id"]


def _png_bytes(w=600, h=300, color=(220, 30, 40, 255)):
    im = Image.new("RGBA", (w, h), color)
    # left half distinct so center-crop is verifiable
    for x in range(w // 2):
        for y in range(0, h, 40):
            im.putpixel((x, y), (10, 200, 90, 255))
    buf = io.BytesIO()
    im.save(buf, format="PNG")
    return buf.getvalue()


@pytest.fixture(scope="module", autouse=True)
def restore_logo(admin, org_id):
    """Restore workspace logo to cleared state after the module."""
    yield
    admin.patch(f"{API}/orgs/{org_id}", json={"logo": ""}, timeout=30)


class TestOrgLogoUpload:
    def test_upload_valid_png_returns_webp_256_square(self, admin, org_id):
        files = {"file": ("TEST_logo.png", _png_bytes(), "image/png")}
        r = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=60)
        assert r.status_code == 200, r.text
        data = r.json()
        assert "_id" not in data
        assert data["org_id"] == org_id
        assert isinstance(data.get("logo"), str)
        assert data["logo"].startswith("data:image/webp;base64,"), data["logo"][:60]
        raw = base64.b64decode(data["logo"].split(",", 1)[1])
        with Image.open(io.BytesIO(raw)) as im:
            assert im.format == "WEBP"
            assert im.size == (256, 256)

        # GET to verify persistence
        g = admin.get(f"{API}/orgs", timeout=30)
        assert g.status_code == 200
        org = next(o for o in g.json() if o["org_id"] == org_id)
        assert org["logo"] == data["logo"]

    def test_upload_non_square_jpeg_center_cropped(self, admin, org_id):
        im = Image.new("RGB", (400, 900), (12, 34, 56))
        buf = io.BytesIO()
        im.save(buf, format="JPEG")
        files = {"file": ("TEST_logo.jpg", buf.getvalue(), "image/jpeg")}
        r = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=60)
        assert r.status_code == 200, r.text
        raw = base64.b64decode(r.json()["logo"].split(",", 1)[1])
        with Image.open(io.BytesIO(raw)) as out:
            assert out.size == (256, 256)

    def test_reject_non_image_content_type(self, admin, org_id):
        files = {"file": ("TEST_note.txt", b"hello world", "text/plain")}
        r = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=30)
        assert r.status_code == 400, r.text
        assert "File must be an image" in r.text

    def test_reject_truncated_image(self, admin, org_id):
        header = b"\x89PNG\r\n\x1a\n" + b"\x00" * 8
        files = {"file": ("TEST_bad.png", header, "image/png")}
        r = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=30)
        assert r.status_code == 400, r.text
        assert "Invalid image file" in r.text

    def test_reject_empty_file(self, admin, org_id):
        files = {"file": ("TEST_empty.png", b"", "image/png")}
        r = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=30)
        assert r.status_code == 400, r.text
        assert "Empty file" in r.text

    def test_reject_too_large(self, admin, org_id):
        blob = b"\x89PNG\r\n\x1a\n" + os.urandom(5 * 1024 * 1024 + 1024)
        files = {"file": ("TEST_big.png", blob, "image/png")}
        r = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=120)
        assert r.status_code == 400, f"{r.status_code} {r.text[:300]}"
        assert "too large" in r.text.lower()

    def test_member_forbidden(self, member, org_id):
        files = {"file": ("TEST_logo.png", _png_bytes(100, 100), "image/png")}
        r = member.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=30)
        assert r.status_code == 403, f"{r.status_code} {r.text[:300]}"

    def test_unauthenticated_rejected(self, org_id):
        files = {"file": ("TEST_logo.png", _png_bytes(100, 100), "image/png")}
        r = requests.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=30)
        assert r.status_code in (401, 403), r.status_code

    def test_nonexistent_org_not_404_leak(self, admin):
        files = {"file": ("TEST_logo.png", _png_bytes(100, 100), "image/png")}
        r = admin.post(f"{API}/orgs/does-not-exist-xyz/logo", files=files, timeout=30)
        assert r.status_code in (403, 404), r.status_code

    def test_patch_clears_logo(self, admin, org_id):
        # ensure a logo exists first
        files = {"file": ("TEST_logo.png", _png_bytes(200, 200), "image/png")}
        up = admin.post(f"{API}/orgs/{org_id}/logo", files=files, timeout=60)
        assert up.status_code == 200
        r = admin.patch(f"{API}/orgs/{org_id}", json={"logo": ""}, timeout=30)
        assert r.status_code == 200, r.text
        assert r.json().get("logo") == ""
        g = admin.get(f"{API}/orgs", timeout=30)
        org = next(o for o in g.json() if o["org_id"] == org_id)
        assert org["logo"] == ""

    def test_patch_name_still_works(self, admin, org_id):
        g = admin.get(f"{API}/orgs", timeout=30)
        original = next(o for o in g.json() if o["org_id"] == org_id)["name"]
        try:
            r = admin.patch(f"{API}/orgs/{org_id}", json={"name": "TEST_Rename Co"}, timeout=30)
            assert r.status_code == 200, r.text
            assert r.json()["name"] == "TEST_Rename Co"
        finally:
            admin.patch(f"{API}/orgs/{org_id}", json={"name": original}, timeout=30)
