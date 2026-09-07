# Sprint Hub

Project management app: projects, sprints, tasks, time tracking, and team analytics.

- **Backend:** FastAPI + MongoDB (motor), single-file API in `backend/server.py`
- **Frontend:** React 19 (Create React App + craco), shadcn/ui components, in `frontend/`

## Prerequisites

- Python 3.9+ (3.10+ recommended)
- Node.js 18+ and npm
- MongoDB running locally (or an Atlas connection string)

### MongoDB

Option A — Homebrew (macOS):

```bash
brew tap mongodb/brew
brew trust mongodb/brew        # required: tap is not auto-trusted
brew install mongodb-community
brew services start mongodb-community
```

Option B — Docker:

```bash
docker run -d --name mongo -p 27017:27017 mongo:7
```

## Backend setup

```bash
cd backend
python3 -m venv .venv && source .venv/bin/activate

# requirements.txt pins emergentintegrations (private Emergent.sh SDK, not on
# PyPI, unused by server.py) and misses httpx/Pillow — install around it:
grep -v emergentintegrations requirements.txt > /tmp/req-local.txt
pip install -r /tmp/req-local.txt httpx Pillow

cat > .env <<'EOF'
MONGO_URL=mongodb://localhost:27017
DB_NAME=sprinthub
JWT_SECRET=change-me-to-a-random-string
EOF

uvicorn server:app --reload --port 8000
```

Optional `.env` vars:

| Var | Default | Purpose |
| --- | --- | --- |
| `ADMIN_EMAIL` | `widiardhana@gmail.com` | Seeded admin user email |
| `ADMIN_PASSWORD` | `Admin@1234` | Seeded admin password |
| `EMERGENT_EMAIL_KEY` | — | Enables transactional email via Emergent proxy; skipped with a log warning if unset |

On first startup the backend seeds the admin user, an "Acme Corp" org, and sample projects/sprints/tasks. API docs at http://localhost:8000/docs.

## Frontend setup

```bash
cd frontend
# --legacy-peer-deps required: react-day-picker@8 declares React <=18 peers,
# app runs React 19 (works fine at runtime)
npm install --legacy-peer-deps

echo "REACT_APP_BACKEND_URL=http://localhost:8000" > .env
npm start
```

App at http://localhost:3000.

## Login

Seeded admin: `widiardhana@gmail.com` / `Admin@1234` (or your `ADMIN_EMAIL`/`ADMIN_PASSWORD` overrides). Email/password signup also available.

Google OAuth login does **not** work locally — it validates sessions against the Emergent preview environment.

## Tests

Backend integration tests run against a live server:

```bash
cd backend
source .venv/bin/activate
REACT_APP_BACKEND_URL=http://localhost:8000 python -m pytest
```

Notes:

- `pytest.ini` runs xdist with `-n 2 --dist loadscope`; use `-n 0` for serial (two suites race on fixture users under parallel runs).
- Some legacy test files hardcode `/app/frontend/.env` (Emergent container path); the `REACT_APP_BACKEND_URL` env var overrides it.
- Tests expect the seeded admin and a QA member (`test_qa_member_1787903479@example.com`). If missing, invite that email via the Members page (default password `Welcome@123`).

## Project layout

```
backend/
  server.py          # entire FastAPI app (auth, orgs, projects, sprints,
                     # tasks, timers, analytics)
  requirements.txt
  tests/             # pytest integration suites (hit live server)
frontend/
  src/pages/         # Dashboard, Tasks, Projects, Sprints, Members,
                     # TeamActivity, Analytics, Profile
  src/components/    # dialogs, views (list/board/gantt/calendar/workload),
                     # ui/ (shadcn primitives)
  src/lib/api.js     # axios client, base URL from REACT_APP_BACKEND_URL
memory/              # PRD
test_reports/        # QA iteration reports
```

## Known local-dev quirks

- Sprint/task dates are stored as plain `YYYY-MM-DD` strings; analytics dates are UTC.
- Invited members who never registered get default password `Welcome@123`.
- Deleting a sprint moves its tasks to Backlog and tags them with the former sprint name.
