"""Delete leftover TEST_QA throwaway project(s) created by iteration-11 tests (cascade via API)."""
import os
import requests
from dotenv import dotenv_values

BASE_URL = dotenv_values("/app/frontend/.env")["REACT_APP_BACKEND_URL"].rstrip("/")
s = requests.Session()
r = s.post(f"{BASE_URL}/api/auth/login", json={"email": "widiardhana@gmail.com", "password": "Admin@1234"})
r.raise_for_status()
s.headers.update({"Authorization": f"Bearer {r.json()['token']}"})
org_id = next(o for o in s.get(f"{BASE_URL}/api/orgs").json() if o["name"].startswith("Acme"))["org_id"]
for p in s.get(f"{BASE_URL}/api/orgs/{org_id}/projects").json():
    if p["name"].startswith("TEST_QA"):
        d = s.delete(f"{BASE_URL}/api/orgs/{org_id}/projects/{p['project_id']}")
        print("deleted", p["name"], p["project_id"], d.status_code, d.text[:100])
print("done")
