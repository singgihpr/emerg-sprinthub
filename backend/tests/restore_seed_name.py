"""One-off helper: restore the seeded project name mutated by earlier test iterations."""
import os
import requests
from dotenv import dotenv_values

BASE_URL = (os.environ.get("REACT_APP_BACKEND_URL") or dotenv_values("/app/frontend/.env")["REACT_APP_BACKEND_URL"]).rstrip("/")
s = requests.Session()
r = s.post(f"{BASE_URL}/api/auth/login", json={"email": "widiardhana@gmail.com", "password": "Admin@1234"})
r.raise_for_status()
s.headers.update({"Authorization": f"Bearer {r.json()['token']}"})
orgs = s.get(f"{BASE_URL}/api/orgs").json()
org = next(o for o in orgs if o["name"].startswith("Acme"))
projects = s.get(f"{BASE_URL}/api/orgs/{org['org_id']}/projects").json()
print("before:", [(p["name"], p["key"]) for p in projects])
target = next((p for p in projects if p["key"] == "WEB"), None)
if target and target["name"] != "Web Redesign":
    resp = s.patch(f"{BASE_URL}/api/orgs/{org['org_id']}/projects/{target['project_id']}", json={"name": "Web Redesign"})
    print("restore:", resp.status_code, resp.text[:200])
print("after:", [(p["name"], p["key"]) for p in s.get(f"{BASE_URL}/api/orgs/{org['org_id']}/projects").json()])
