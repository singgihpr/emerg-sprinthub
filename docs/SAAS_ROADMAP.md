# Sprint Hub — SaaS / Enterprise Roadmap

Branch: `feat/saas-postgres-rls`

## Done: P0 — Security hardening (Mongo backend)

- **CORS allowlist** via `ALLOWED_ORIGINS` env: credentialed wildcard-origin reflect closed.
- **Short-lived access tokens** (15 min) + **HttpOnly refresh-cookie rotation**: `/api/auth/refresh` issues a new refresh token and deletes the old one atomically. Logout and password change revoke refresh tokens.
- **Rate limiter** on `POST /api/auth/*` (10 req/min/IP) + Echo `Recover()` middleware.
- **Password policy** raised to 12 characters; registration now enforces it (invite-accept and change-password already did).
- **Seed gating**: admin + demo data only run when `SEED_DEMO=true` (default `true` for local dev, set `false` in production).
- **Frontend 401 refresh**: axios interceptor retries once through `/api/auth/refresh` after a 401.
- **Tests**: refresh rotation/revocation, CORS allowlist, password policy, rate limit.

## Done: P1 — Full Postgres migration + RLS

- **Per-request Postgres transaction middleware** (`pgTxMiddleware`) sets `app.current_user` and `app.current_org` so RLS policies fire on all tables.
- **Map-shaped query helpers** (`pgFindOne`, `pgFindMany`, `pgInsert`, `pgUpdate`, `pgDelete`) plus `pgExecQuery`/`pgExecRaw` for custom queries with IN clauses.
- **Background variants** (`pgPoolFind`, `pgPoolFindOne`, `pgPoolInsert`, `pgPoolUpdate`) for seed/cron/email jobs.
- **All entities on Postgres**: `users`, `refresh_tokens`, `organizations`, `memberships`, `invites`, `projects`, `project_members`, `sprints`, `tasks`, `recurring_tasks`, `time_entries`, `active_timers`, `comments`, `cron_runs`.
- **Mongo removed**: All Mongo connections, collections, helpers, and the `go.mongodb.org/mongo-driver` dependency removed.
- **Schema**: `tasks` table has `recurring_id`, `repeat`, `former_sprint_name` columns.
- **Seed** writes admin/demo org/membership/project/sprint/tasks to Postgres.
- **Tests** run against Postgres only; no Mongo dependency.

## Verified

```bash
cd backend
go vet ./...        # clean
go test ./...       # 20 passed
```

## Done: P4 — GDPR compliance (Indonesian PDPL, APAC, EU GDPR)

- **Data export API** (`GET /api/me/export`): returns JSON with user profile, memberships, tasks (created/assigned), comments, time entries, project memberships. Complies with GDPR Article 20 (data portability).
- **Data erasure API** (`DELETE /api/me`): deletes user and all related data via FK cascade. Blocks deletion if user owns any organization (must transfer ownership first). Complies with GDPR Article 17 (right to erasure).
- **Frontend Privacy tab**: Profile page now has a "Privacy" tab with "Export your data" button (downloads JSON) and "Delete account" button (with confirmation dialog).
- **Audit logging**: all GDPR actions (export, delete) logged to `audit_logs` table with IP, user agent, timestamps.

## Done: P5 — SOC 2 technical controls

- **Audit log table** (`audit_logs`): tracks security-relevant events (user.register, user.login, user.data_export, user.delete, etc.) with user_id, org_id, action, resource_type, resource_id, ip_address, user_agent, details (JSONB), created_at.
- **Audit log helper** (`auditLogRequest`): extracts request metadata (IP, user agent) and writes to audit_logs. Applied to auth handlers (register, login) and GDPR handlers.
- **Migration 000005**: creates audit_logs table with indexes on user_id, org_id, action, created_at.

## Next: P2 / P3 — SaaS business + enterprise

- Stripe plans + per-tenant quotas/metering.
- SSO / SAML via WorkOS (buy, don't build).
- SCIM directory sync + fine-grained RBAC.
- EU-residency cluster option for whales.
- SOC 2 process (Vanta/Drata), managed Postgres, pen test.
