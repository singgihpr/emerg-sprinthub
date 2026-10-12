# SprintHub - Complete Project Documentation

## Overview

SprintHub is a comprehensive project management application for teams to manage projects, sprints, tasks, time tracking, and analytics.

**Key Capabilities:**
- Multi-organization and multi-project management
- Sprint planning with burndown charts
- Task management (list, board, Gantt, calendar, workload views)
- Time tracking with real-time timer
- Team activity monitoring and analytics (CSV/PDF export)
- Role-based access control (RBAC) with custom roles
- Email invitations and member management
- Recurring tasks automation
- Audit logging (SOC 2), GDPR data export/deletion
- Stripe billing integration (optional)
- OpenID Connect SSO (optional)

---

## Tech Stack

| Layer | Technology |
|-------|-----------|
| **Backend** | Go 1.24+, Echo v4, PostgreSQL 16 |
| **Frontend** | React 19, craco, shadcn/ui, Tailwind CSS 3.4, Recharts |
| **Auth** | JWT (15 min access), HttpOnly refresh tokens, OIDC |
| **Infrastructure** | Docker, Docker Compose, nginx |
| **Email** | SMTP with transactional templates |
| **Billing** | Stripe (optional) |

---

## Data Structure

### Core Tables

**users** — `user_id, email (CITEXT UNIQUE), name, password_hash, picture, auth_provider, auth_provider_id, created_at`

**organizations** — `org_id, name, owner_id (FK users), logo, plan_id, stripe_customer_id, stripe_subscription_id, subscription_status, created_at`

**memberships** — `membership_id, org_id (FK), user_id (FK), role (owner/admin/manager/member/viewer), UNIQUE(org_id, user_id)`

**projects** — `project_id, org_id (FK), name, key (short code), description, color, status (planning/active/on_hold/archived), created_by (FK), created_at`

**project_members** — `project_member_id, org_id (FK), project_id (FK), user_id (FK), role, added_by (FK), UNIQUE(org_id, project_id, user_id)`

**sprints** — `sprint_id, org_id (FK), project_id (FK), name, goal, start_date, end_date, status (planned/active/completed), created_at`

**tasks** — `task_id, org_id (FK), project_id (FK), title, description, status (todo/in_progress/review/done), priority (low/medium/high/urgent), type (task/bug/feature), assignee_id (FK), sprint_id (FK nullable), start_date, due_date, estimate_hours, logged_minutes, recurring_id, repeat (none/daily/weekly/monthly), former_sprint_name, created_by (FK), created_at, updated_at, completed_at`

**recurring_tasks** — Template for tasks that repeat: `recurring_id, org_id (FK), project_id (FK), title, description, status, priority, type, assignee_id (FK), sprint_id (FK), estimate_hours, repeat, created_by (FK), created_at`

**time_entries** — `entry_id, org_id (FK), task_id (FK), user_id (FK), minutes, note, date, created_at`

**active_timers** — One per user per org: `org_id (FK), user_id (FK), task_id (FK), started_at, PK(org_id, user_id)`

**comments** — `comment_id, org_id (FK), task_id (FK), user_id (FK), body, mentions (JSONB), created_at`

**invites** — `invite_id, org_id (FK), email, name, token_hash, role, invited_by (FK), expires_at, used, created_at`

### RBAC Tables

**roles** — `role_id, org_id (FK), name, description, permissions (JSONB array), is_default, is_system, UNIQUE(org_id, name)`

**role_assignments** — `assignment_id, org_id (FK), user_id (FK), role_id (FK), UNIQUE(org_id, user_id, role_id)`

**project_roles** — `project_role_id, org_id (FK), project_id (FK), name, permissions (JSONB), is_system, UNIQUE(project_id, name)`

**project_role_assignments** — `assignment_id, org_id (FK), project_id (FK), user_id (FK), project_role_id (FK), UNIQUE(project_id, user_id, project_role_id)`

**Available Permissions:** `manage_org, manage_billing, invite_members, manage_roles, view_analytics, create_project, edit_project, delete_project, manage_project_members, create_task, edit_task, delete_task, assign_task, edit_comments, create_time_entries, view_time_entries`

### Billing Tables (Optional)

**plans** — `plan_id, name, stripe_price_id, limits (JSONB: members, projects, storage_mb, api_calls_per_month), created_at`

**org_quotas** — `org_id (PK FK), current_members, current_projects, current_storage_bytes, api_calls_this_month, api_calls_reset_at, updated_at`

**usage_events** — `event_id, org_id (FK), event_type, quantity, timestamp`

### Compliance Tables

**audit_logs** — `log_id, user_id, org_id, action, resource_type, resource_id, details (JSONB), ip_address, user_agent, created_at`

### Supporting Tables

**cron_runs** — `run_id, name, webhook_id, run_at, created_at`

**refresh_tokens** — `token_hash (PK), user_id (FK), expires_at, created_at`

### Security Features

- Row Level Security (RLS) on all tenant-scoped tables
- Tenant isolation via `current_tenant()` Postgres function
- User isolation on `memberships` and `refresh_tokens`

---

## User Flows

### 1. Authentication
- **Register:** email + name + password (min 12 chars) → JWT issued
- **Login:** email + password → access token (15 min) + refresh token (HttpOnly cookie)
- **Refresh:** automatic, seamless — no re-login needed
- **Logout:** refresh token invalidated, cookies cleared
- **OIDC SSO:** redirect to provider → callback → user linked/created → tokens issued

### 2. Organization Management
- **Create org:** user becomes owner → demo data seeded (if `SEED_DEMO=true`)
- **Switch org:** dropdown in header, persisted in localStorage
- **Invite members:** owner/admin enters email/name/role → email sent with invite link → invitee accepts → membership created

### 3. Project Management
- **Create project:** owner/admin/manager fills name/key/description/color/status
- **Add members:** project edit dialog → select org members → assign project roles
- **Access control:** owner/admin see all projects; others see only projects they belong to

### 4. Sprint Management
- **Create sprint:** select project, name, goal, start/end dates → status `planned`
- **Lifecycle:** planned → active → completed
- **Burndown chart:** ideal vs actual remaining work based on task estimates
- **Delete sprint:** tasks moved to Backlog, tagged with `former_sprint_name`

### 5. Task Management
- **Create task:** title, description, project, sprint (optional), priority, type, assignee, dates, estimate → status `todo`
- **Views:** list, board (Kanban), Gantt, calendar, workload
- **Status transitions:** todo → in_progress → review → done (flexible, can move backward)
- **Recurring tasks:** set `repeat` (daily/weekly/monthly) → cron spawns instances
- **Time tracking:** start timer → active_timers entry → stop timer → time_entries entry + logged_minutes updated
- **Comments:** add text, `@mention` users → mentions stored as JSONB array

### 6. Analytics
- **View:** last 7 days default, custom date range picker
- **Charts:** status breakdown (pie), completed over time (line), time logged (bar), estimate vs actual
- **Export CSV:** summary + status breakdown + daily + task details
- **Export PDF:** tables + charts as images via html2canvas

### 7. Team Activity
- Activity feed for all members: task CRUD, comments, time entries, sprint changes
- Filter by member, project, or date

### 8. Profile / GDPR
- **Update profile:** name, email, picture
- **Change password:** current password required, min 12 chars
- **Export data (GDPR):** JSON download, audit log entry `user.export`
- **Delete account (GDPR):** all data deleted/anonymized, audit log entry `user.delete`

### 9. Billing (Optional)
- **Subscribe:** select plan → Stripe Checkout → webhook updates subscription
- **Manage:** Stripe Customer Portal for payment/cancel
- **Quota enforcement:** API checks org_quotas before operations, 402 if exceeded

### 10. Cron Jobs
- **Weekly digest:** Monday 9 AM, aggregates completed tasks/time logged, emails members
- **Spawn recurring tasks:** daily midnight, checks templates, creates instances

---

## API Endpoints

### Authentication
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/register` | Create account |
| POST | `/api/auth/login` | Login |
| POST | `/api/auth/refresh` | Refresh access token |
| POST | `/api/auth/logout` | Logout |
| GET | `/api/auth/me` | Get current user |
| PATCH | `/api/auth/me` | Update profile |
| POST | `/api/auth/change-password` | Change password |
| GET | `/api/auth/oidc/login` | OIDC login redirect |
| GET | `/api/auth/oidc/callback` | OIDC callback |

### GDPR
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/me/export` | Export user data |
| DELETE | `/api/me` | Delete account |

### Organizations
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs` | List user's organizations |
| POST | `/api/orgs` | Create organization |
| PATCH | `/api/orgs/:org_id` | Update organization |
| POST | `/api/orgs/:org_id/logo` | Upload logo |
| GET | `/api/orgs/:org_id/members` | List members |
| POST | `/api/orgs/:org_id/members` | Invite member |

### RBAC
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/:org_id/permissions` | List all permissions |
| GET | `/api/orgs/:org_id/roles` | List roles |
| POST | `/api/orgs/:org_id/roles` | Create role |
| PATCH | `/api/orgs/:org_id/roles/:role_id` | Update role |
| DELETE | `/api/orgs/:org_id/roles/:role_id` | Delete role |
| POST | `/api/orgs/:org_id/members/:user_id/roles` | Assign role |
| DELETE | `/api/orgs/:org_id/members/:user_id/roles/:role_id` | Remove role |
| GET | `/api/orgs/:org_id/members/:user_id/roles` | Get user roles |

### Invites
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/invites/:token` | Get invite details |
| POST | `/api/invites/:token/accept` | Accept invite |

### Projects
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/:org_id/projects` | List projects |
| POST | `/api/orgs/:org_id/projects` | Create project |
| PATCH | `/api/orgs/:org_id/projects/:project_id` | Update project |
| DELETE | `/api/orgs/:org_id/projects/:project_id` | Delete project |
| GET | `/api/orgs/:org_id/projects/:project_id/members` | List project members |
| POST | `/api/orgs/:org_id/projects/:project_id/members` | Add project member |
| DELETE | `/api/orgs/:org_id/projects/:project_id/members/:user_id` | Remove project member |

### Sprints
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/:org_id/sprints` | List sprints |
| POST | `/api/orgs/:org_id/sprints` | Create sprint |
| PATCH | `/api/orgs/:org_id/sprints/:sprint_id` | Update sprint |
| DELETE | `/api/orgs/:org_id/sprints/:sprint_id` | Delete sprint |
| GET | `/api/orgs/:org_id/sprints/:sprint_id/burndown` | Get burndown data |

### Tasks
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/:org_id/tasks` | List tasks |
| POST | `/api/orgs/:org_id/tasks` | Create task |
| PATCH | `/api/orgs/:org_id/tasks/:task_id` | Update task |
| DELETE | `/api/orgs/:org_id/tasks/:task_id` | Delete task |
| GET | `/api/orgs/:org_id/recurring` | List recurring tasks |
| DELETE | `/api/orgs/:org_id/recurring/:recurring_id` | Stop recurring task |

### Time Tracking
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/:org_id/time-entries` | List time entries |
| POST | `/api/orgs/:org_id/time-entries` | Create time entry |
| GET | `/api/orgs/:org_id/timer` | Get active timer |
| POST | `/api/orgs/:org_id/timer/start` | Start timer |
| POST | `/api/orgs/:org_id/timer/stop` | Stop timer |

### Comments / Analytics / Billing / Cron
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/orgs/:org_id/tasks/:task_id/comments` | List comments |
| POST | `/api/orgs/:org_id/tasks/:task_id/comments` | Create comment |
| GET | `/api/orgs/:org_id/analytics` | Get analytics data |
| GET | `/api/orgs/:org_id/team-activity` | Get team activity |
| POST | `/api/billing/checkout` | Create Stripe checkout session |
| POST | `/api/billing/portal` | Create Stripe portal session |
| POST | `/api/billing/webhook` | Stripe webhook |
| POST | `/api/cron/weekly-digest` | Run weekly digest |
| POST | `/api/cron/spawn-recurring` | Spawn recurring tasks |

---

## Running Locally

### Option A: Docker Compose (Recommended)

**Prerequisites:** Docker Desktop + Docker Compose v2+

```bash
# 1. Clone repo
git clone <repo-url>
cd emerg-sprinthub

# 2. Create .env
cat > .env <<'EOF'
JWT_SECRET=your-random-secret-key-change-this
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=Admin@1234
SEED_DEMO=true
RATE_LIMIT=off
# Optional: SMTP_HOST, SMTP_USER, SMTP_PASS, EMAIL_FROM, APP_BASE_URL
# Optional: STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET
# Optional: OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET
EOF

# 3. Start all services
docker compose up -d

# 4. Verify
docker compose ps

# 5. Access
# Frontend: http://localhost:8030
# Backend API: http://localhost:8030/api (nginx proxy)

# 6. Logs
docker compose logs -f backend

# 7. Stop
docker compose down

# 8. Clean slate (remove volumes)
docker compose down -v
```

**Docker services:**
- `postgres`: PostgreSQL 16 (internal only, no port published)
- `backend`: Go API on port 8000
- `web`: React frontend via nginx on port 8030

### Option B: Local Development (Without Docker)

**Prerequisites:** Go 1.24+, Node.js 18+, npm, PostgreSQL 16

**1. Setup PostgreSQL:**
```bash
# Homebrew (macOS)
brew install postgresql@16
brew services start postgresql@16
createdb sprinthub
psql -d sprinthub -c "CREATE USER sprinthub WITH PASSWORD 'sprinthub';"
psql -d sprinthub -c "GRANT ALL PRIVILEGES ON DATABASE sprinthub TO sprinthub;"

# OR Docker
docker run -d --name postgres \
  -e POSTGRES_USER=sprinthub \
  -e POSTGRES_PASSWORD=sprinthub \
  -e POSTGRES_DB=sprinthub \
  -p 5432:5432 \
  postgres:16
```

**2. Backend:**
```bash
cd backend

cat > .env <<'EOF'
DATABASE_URL=postgres://sprinthub:sprinthub@localhost:5432/sprinthub?sslmode=disable
JWT_SECRET=your-random-secret-key
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=Admin@1234
SEED_DEMO=true
RATE_LIMIT=off
ALLOWED_ORIGINS=http://localhost:3000
EOF

go mod download
go run .
# Backend at http://localhost:8000
# Health: http://localhost:8000/healthz
```

**3. Frontend:**
```bash
cd frontend
npm install --legacy-peer-deps   # required for React 19
echo "REACT_APP_BACKEND_URL=http://localhost:8000" > .env
npm start
# Frontend at http://localhost:3000
```

**4. Login:**
- Email: `admin@example.com` (or your `ADMIN_EMAIL`)
- Password: `Admin@1234` (or your `ADMIN_PASSWORD`)

---

## Configuration

For detailed setup instructions for external services (Google SSO, SMTP providers, Stripe billing), see [CONFIG.md](CONFIG.md).

### Quick Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | **required** | JWT signing secret |
| `DATABASE_URL` | **required** | PostgreSQL connection string |
| `ADMIN_EMAIL` | `widiardhana@gmail.com` | Seeded admin email |
| `ADMIN_PASSWORD` | `Admin@1234` | Seeded admin password |
| `SEED_DEMO` | `true` | Seed admin + demo data on startup |
| `RATE_LIMIT` | `on` | Per-IP rate limiter for auth endpoints |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated CORS allowlist |
| `SMTP_HOST` | — | SMTP server host |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USER` | — | SMTP username |
| `SMTP_PASS` | — | SMTP password |
| `EMAIL_FROM` | `SMTP_USER` or `no-reply@localhost` | From email |
| `EMAIL_FROM_NAME` | `SprintHub` | From name |
| `APP_BASE_URL` | — | Base URL for email links and SSO redirects |
| `STRIPE_SECRET_KEY` | — | Stripe secret key |
| `STRIPE_WEBHOOK_SECRET` | — | Stripe webhook secret |
| `OIDC_ISSUER_URL` | — | OIDC issuer URL (e.g., `https://accounts.google.com`) |
| `OIDC_CLIENT_ID` | — | OIDC client ID |
| `OIDC_CLIENT_SECRET` | — | OIDC client secret |
| `WEBHOOK_CRON_SECRET` | — | Bearer token for `/api/cron/*` |

---

## Testing

```bash
# Backend tests (requires PostgreSQL at localhost:5432)
cd backend
go test ./...

# Frontend tests
cd frontend
npm test
```

---

## Known Issues

1. Sprint/task dates stored as `YYYY-MM-DD` strings; analytics dates are UTC
2. Invited members must accept invite (email link) before logging in — no password until then
3. Deleting sprint moves tasks to Backlog, tags with former sprint name
4. `react-day-picker@8` declares React <=18 peers; app runs React 19 (works fine)

---

## Project Structure

```
emerg-sprinthub/
├── backend/
│   ├── main.go              # Config, routes, CORS, error handler
│   ├── db.go                # ID/time helpers, value coercion
│   ├── pg.go                # PostgreSQL client, query helpers
│   ├── auth.go              # JWT, cookies, current-user lookup
│   ├── models.go            # Request DTOs and validation
│   ├── handlers_auth.go     # Authentication
│   ├── handlers_org.go      # Organizations
│   ├── handlers_project.go  # Projects
│   ├── handlers_task.go     # Tasks
│   ├── handlers_misc.go     # Comments, analytics, team activity
│   ├── handlers_billing.go  # Stripe billing
│   ├── handlers_rbac.go     # RBAC
│   ├── handlers_gdpr.go     # GDPR export/deletion
│   ├── handlers_oidc.go     # OIDC SSO
│   ├── handlers_cron.go     # Cron endpoints
│   ├── email.go             # SMTP, templates
│   ├── scheduler.go         # Built-in cron scheduler
│   ├── seed.go              # Indexes + admin/demo seed
│   ├── billing.go           # Billing logic
│   ├── password.go          # Password hashing
│   ├── migrations/          # SQL migrations (000001-000005)
│   ├── main_test.go         # Test suite
│   └── Dockerfile
├── frontend/
│   ├── src/
│   │   ├── pages/           # Dashboard, Tasks, Projects, Sprints,
│   │   │                    # Members, TeamActivity, Analytics, Roles,
│   │   │                    # Profile, Login, AcceptInvite
│   │   ├── components/      # AppShell, dialogs, views (list/board/gantt/
│   │   │                    # calendar/workload), ui/ (shadcn primitives)
│   │   ├── context/         # AuthContext, OrgContext
│   │   ├── lib/             # api.js (axios), analyticsExport.js
│   │   ├── App.js
│   │   └── App.css
│   ├── package.json
│   ├── Dockerfile
│   ├── nginx.conf
│   └── craco.config.js
├── docs/
│   ├── PRD.md
│   ├── BILLING.md
│   └── SAAS_ROADMAP.md
├── docker-compose.yml
└── README.md
```
