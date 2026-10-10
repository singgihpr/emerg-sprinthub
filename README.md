# Sprint Hub

Project management app: projects, sprints, tasks, time tracking, and team analytics.

- **Backend:** Go (Echo) + MongoDB, in `backend/` (single-module API)
- **Frontend:** React 19 (Create React App + craco), shadcn/ui components, in `frontend/`

## Prerequisites

- Go 1.24+
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

cat > .env <<'EOF'
MONGO_URL=mongodb://localhost:27017
DB_NAME=sprinthub
JWT_SECRET=change-me-to-a-random-string
EOF

go run .
```

Optional `.env` vars:

| Var | Default | Purpose |
| --- | --- | --- |
| `ADMIN_EMAIL` | `widiardhana@gmail.com` | Seeded admin user email (only when `SEED_DEMO=true`) |
| `ADMIN_PASSWORD` | `Admin@1234` | Seeded admin password |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated allowlist for credentialed CORS |
| `SEED_DEMO` | `true` | Seed admin + demo org/projects on startup; set `false` in production |
| `RATE_LIMIT` | `on` | Per-IP rate limiter for `POST /api/auth/*`; set `off` to disable |
| `SMTP_*`, `EMAIL_FROM`, `EMAIL_FROM_NAME`, `APP_BASE_URL` | — | Transactional email (see `.env.example`) |
| `WEBHOOK_CRON_SECRET` | — | Bearer token for `/api/cron/*`; the built-in scheduler also runs these jobs |

On first startup the backend seeds the admin user, an "Acme Corp" org, and sample projects/sprints/tasks when `SEED_DEMO=true`. Health probe at http://localhost:8000/healthz. Access tokens expire in 15 minutes and are silently refreshed via `POST /api/auth/refresh` (HttpOnly cookie).

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

Seeded admin: `widiardhana@gmail.com` / `Admin@1234` (or your `ADMIN_EMAIL`/`ADMIN_PASSWORD` overrides). Email/password signup requires passwords of at least 12 characters.

## Tests

Go tests boot the router against a throwaway Mongo database (`sprinthub_gotest`) — MongoDB must be reachable on `localhost:27017`:

```bash
cd backend
go test ./...
```

## Project layout

```
backend/
  main.go            # config, routes, CORS, error handler
  db.go              # mongo client, collections, query/time helpers
  auth.go            # JWT, cookies, current-user lookup
  models.go          # request DTOs + validation
  handlers_*.go      # auth, orgs/members/invites, projects/sprints,
                     # tasks/time/timer, comments/analytics, cron
  email.go           # SMTP send, content scan, invite/digest templates
  scheduler.go       # built-in cron (weekly digest, recurring spawn)
  seed.go            # indexes + admin/demo seed + backfills
  main_test.go       # auth/ACL/logo/timer/cron/comment coverage
go.mod / go.sum
frontend/
  src/pages/         # Dashboard, Tasks, Projects, Sprints, Members,
                     # TeamActivity, Analytics, Profile
  src/components/    # dialogs, views (list/board/gantt/calendar/workload),
                     # ui/ (shadcn primitives)
  src/lib/api.js     # axios client, base URL from REACT_APP_BACKEND_URL
docs/                # PRD
```

## Known local-dev quirks

- Sprint/task dates are stored as plain `YYYY-MM-DD` strings; analytics dates are UTC.
- Invited members must accept their invite (email link) before they can log in — invited accounts have no password until then.
- Deleting a sprint moves its tasks to Backlog and tags them with the former sprint name.
