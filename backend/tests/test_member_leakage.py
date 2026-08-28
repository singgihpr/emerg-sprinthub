"""Cross-check: which org-scoped endpoints still leak data to a plain 'member' who belongs to no project."""
import os
import re
from pathlib import Path

import pytest
import requests
from dotenv import dotenv_values

frontend_env = dotenv_values("/app/frontend/.env")
BASE_URL = (os.environ.get("REACT_APP_BACKEND_URL") or frontend_env["REACT_APP_BACKEND_URL"]).rstrip("/")
CRED = Path("/app/memory/test_credentials.md").read_text(encoding="utf-8")


def _seeded_org(admin):
    orgs = admin.get(f"{BASE_URL}/api/orgs").json()
    return next((o for o in orgs if o["name"] == "Acme Corp"), orgs[0])["org_id"]


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
def org_id(admin):
    return _seeded_org(admin)


@pytest.fixture(scope="module")
def member_no_projects(admin, member, org_id):
    """Ensure member is not in any project."""
    me = member.get(f"{BASE_URL}/api/auth/me").json()["user_id"]
    projects = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
    for p in projects:
        admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{p['project_id']}/members/{me}")
    assert member.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json() == []
    return me


class TestMemberLeakage:
    def test_tasks_empty(self, member, org_id, member_no_projects):
        r = member.get(f"{BASE_URL}/api/orgs/{org_id}/tasks")
        assert r.status_code == 200
        assert r.json() == []

    def test_sprints_are_filtered(self, member, org_id, member_no_projects):
        """A member with no projects should not see sprints of inaccessible projects."""
        r = member.get(f"{BASE_URL}/api/orgs/{org_id}/sprints")
        assert r.status_code == 200
        assert r.json() == [], f"LEAK: member sees {len(r.json())} sprints of projects they don't belong to"

    def test_dashboard_analytics_filtered(self, member, org_id, member_no_projects):
        r = member.get(f"{BASE_URL}/api/orgs/{org_id}/analytics")
        assert r.status_code in (200, 403), r.text
        if r.status_code == 200:
            data = r.json()
            total = data.get("total_tasks", data.get("totals", {}).get("total", 0))
            assert total == 0, f"LEAK: analytics reports {total} tasks for member with no projects: {data}"
