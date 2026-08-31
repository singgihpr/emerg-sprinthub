"""Seed/teardown a throwaway project + sprint for frontend sprint-filter testing.
Usage: python seed_iter11_ui.py seed|teardown
"""
import sys
import requests
from dotenv import dotenv_values

BASE_URL = dotenv_values("/app/frontend/.env")["REACT_APP_BACKEND_URL"].rstrip("/")
s = requests.Session()
r = s.post(f"{BASE_URL}/api/auth/login", json={"email": "widiardhana@gmail.com", "password": "Admin@1234"})
r.raise_for_status()
s.headers.update({"Authorization": f"Bearer {r.json()['token']}"})
org_id = next(o for o in s.get(f"{BASE_URL}/api/orgs").json() if o["name"].startswith("Acme"))["org_id"]

mode = sys.argv[1] if len(sys.argv) > 1 else "seed"
if mode == "seed":
    p = s.post(f"{BASE_URL}/api/orgs/{org_id}/projects",
               json={"name": "TEST_QA SprintFlt", "key": "TQSF", "status": "planning"})
    p.raise_for_status()
    pid = p.json()["project_id"]
    sp = s.post(f"{BASE_URL}/api/orgs/{org_id}/sprints",
                json={"project_id": pid, "name": "TEST_QA Filter Sprint", "start_date": "2026-07-01", "end_date": "2026-07-14"})
    sp.raise_for_status()
    print("seeded project", pid, "sprint", sp.json()["sprint_id"])
else:
    for pr in s.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json():
        if pr["name"].startswith("TEST_QA"):
            print("delete", pr["name"], s.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{pr['project_id']}").status_code)
print("sprints now:", [(x["name"], x["status"]) for x in s.get(f"{BASE_URL}/api/orgs/{org_id}/sprints").json()])
