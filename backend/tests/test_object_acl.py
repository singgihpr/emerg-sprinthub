"""Object-level access control: can a plain 'member' who belongs to NO project reach
individual tasks/comments/project-members of an inaccessible project?"""
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
    block = re.search(r'(?is)Test Regular Member(.*?)##', CRED).group(1)
    e = re.search(r'(?im)^\s*[-*]\s*Email\s*:\s*`?([^`\s]+)', block).group(1)
    p = re.search(r'(?im)^\s*[-*]\s*Password\s*:\s*`?([^`\s]+)', block).group(1)
    return _login(e, p)


@pytest.fixture(scope="module")
def ctx(admin, member):
    org_id = _seeded_org(admin)
    me = member.get(f"{BASE_URL}/api/auth/me").json()["user_id"]
    projects = admin.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json()
    for p in projects:
        admin.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{p['project_id']}/members/{me}")
    tasks = admin.get(f"{BASE_URL}/api/orgs/{org_id}/tasks").json()
    task = next(t for t in tasks if t.get("project_id"))
    return {"org_id": org_id, "project_id": task["project_id"], "task_id": task["task_id"], "member_id": me}


class TestObjectLevelAcl:
    def test_member_cannot_patch_inaccessible_task(self, member, ctx):
        r = member.patch(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{ctx['task_id']}", json={"status": "done"})
        assert r.status_code in (403, 404), f"member was able to modify a task of a project they don't belong to ({r.status_code})"

    def test_member_cannot_read_inaccessible_task_comments(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{ctx['task_id']}/comments")
        assert r.status_code in (403, 404), f"comments readable ({r.status_code}, {len(r.json()) if r.ok else ''})"

    def test_member_cannot_list_members_of_inaccessible_project(self, member, ctx):
        r = member.get(f"{BASE_URL}/api/orgs/{ctx['org_id']}/projects/{ctx['project_id']}/members")
        assert r.status_code in (403, 404), f"project member roster readable ({r.status_code})"

    def test_member_cannot_delete_inaccessible_task(self, admin, member, ctx):
        # use a throwaway task so seeded data is untouched
        c = admin.post(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks",
                       json={"project_id": ctx["project_id"], "title": "TEST_acl_probe"})
        assert c.status_code == 200, c.text
        tid = c.json()["task_id"]
        try:
            r = member.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{tid}")
            assert r.status_code in (403, 404), \
                f"member deleted a task from an inaccessible project ({r.status_code})"
        finally:
            admin.delete(f"{BASE_URL}/api/orgs/{ctx['org_id']}/tasks/{tid}")
