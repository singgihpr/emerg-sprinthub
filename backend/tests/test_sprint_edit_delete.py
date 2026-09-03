"""Sprint full-edit, delete (tasks move to backlog tagged with former sprint name)."""
import os

import pytest
import requests
from dotenv import dotenv_values

frontend_env_paths = [
    "/app/frontend/.env",
    os.path.join(os.path.dirname(__file__), "..", "..", "frontend", ".env"),
]
base_url = os.environ.get("REACT_APP_BACKEND_URL")
if not base_url:
    for p in frontend_env_paths:
        if os.path.exists(p):
            base_url = dotenv_values(p).get("REACT_APP_BACKEND_URL")
            break
if not base_url:
    raise RuntimeError("REACT_APP_BACKEND_URL missing")
BASE_URL = base_url.rstrip("/")

ADMIN_EMAIL = "widiardhana@gmail.com"
ADMIN_PW = "Admin@1234"


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
def org_id(admin):
    r = admin.get(f"{BASE_URL}/api/orgs")
    assert r.status_code == 200
    assert r.json(), "admin has no org"
    return r.json()[0]["org_id"]


@pytest.fixture(scope="module")
def project_id(admin, org_id):
    r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects",
                   json={"name": "Sprint Edit Delete Test", "key": "SED"})
    assert r.status_code == 200, r.text
    yield r.json()["project_id"]
    admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{r.json()['project_id']}")


def test_sprint_full_edit(admin, org_id, project_id):
    r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                   json={"project_id": project_id, "name": "S1", "goal": "g1",
                         "start_date": "2026-09-01", "end_date": "2026-09-14"})
    assert r.status_code == 200, r.text
    sid = r.json()["sprint_id"]

    r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}",
                    json={"name": "S1 renamed", "goal": "g2", "status": "active",
                          "start_date": "2026-09-02", "end_date": "2026-09-16"})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["name"] == "S1 renamed"
    assert body["goal"] == "g2"
    assert body["status"] == "active"
    assert body["start_date"] == "2026-09-02"
    assert body["end_date"] == "2026-09-16"

    # status-only patch (existing behavior) still works
    r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}", json={"status": "completed"})
    assert r.status_code == 200 and r.json()["status"] == "completed"

    # empty patch rejected
    r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}", json={})
    assert r.status_code == 400

    admin.delete(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}")


def test_sprint_delete_moves_tasks_to_backlog(admin, org_id, project_id):
    r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                   json={"project_id": project_id, "name": "Doomed Sprint"})
    assert r.status_code == 200, r.text
    sid = r.json()["sprint_id"]

    r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/tasks",
                   json={"project_id": project_id, "title": "tagged task", "sprint_id": sid})
    assert r.status_code == 200, r.text
    tid = r.json()["task_id"]

    r = admin.delete(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}")
    assert r.status_code == 200, r.text

    tasks = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
    task = next(t for t in tasks if t["task_id"] == tid)
    assert task["sprint_id"] is None
    assert task["former_sprint_name"] == "Doomed Sprint"

    # sprint gone
    sprints = admin.get(f"{BASE_URL}/api/orgs/{org_id}/sprints").json()
    assert all(s["sprint_id"] != sid for s in sprints)

    # deleting again -> 404
    r = admin.delete(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid}")
    assert r.status_code == 404

    # reassigning to a new sprint clears the tag
    r = admin.post(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                   json={"project_id": project_id, "name": "Fresh Sprint"})
    sid2 = r.json()["sprint_id"]
    r = admin.patch(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}", json={"sprint_id": sid2})
    assert r.status_code == 200, r.text
    assert r.json()["sprint_id"] == sid2
    assert r.json()["former_sprint_name"] is None

    admin.delete(f"{BASE_URL}/api/orgs/{org_id}/sprints/{sid2}")
    admin.delete(f"{BASE_URL}/api/orgs/{org_id}/tasks/{tid}")
