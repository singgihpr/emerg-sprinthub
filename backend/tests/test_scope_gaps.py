"""Iteration 7 gap probe: write/read paths that still only call ensure_member() and are NOT
project-scoped. Member is detached from every project first."""
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
        pytest.fail(f"login failed {email}: {r.status_code}")
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
    for p in projects:
        admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{p['project_id']}/members/{me}")
    yield {"org_id": org_id, "project_id": web["project_id"], "member_id": me}
    admin.post(f"{BASE_URL}/api/orgs/{org_id}/projects/{web['project_id']}/members",
               json={"user_id": me, "role": "member"})


class TestRemainingGaps:
    def test_create_task_in_inaccessible_project(self, admin, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks",
                        json={"project_id": ctx["project_id"], "title": "TEST_gap_create_task"})
        if r.status_code == 200:
            admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{r.json()['task_id']}")
        assert r.status_code == 403, f"GAP: member created a task in a project they don't belong to ({r.status_code})"

    def test_create_sprint_in_inaccessible_project(self, admin, member, ctx):
        r = member.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/sprints",
                        json={"project_id": ctx["project_id"], "name": "TEST_gap_sprint"})
        assert r.status_code == 403, f"GAP: member created a sprint in a project they don't belong to ({r.status_code})"

    def test_time_entries_not_leaked(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/time-entries")
        assert r.status_code == 200
        assert r.json() == [], f"GAP: member sees {len(r.json())} org time entries for inaccessible tasks"
