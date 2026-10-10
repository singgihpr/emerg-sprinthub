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
go test ./...       # 13 passed
```

## Next: P2 / P3 — SaaS business + enterprise

- Stripe plans + per-tenant quotas/metering.
- SSO / SAML via WorkOS (buy, don't build).
- SCIM directory sync + fine-grained RBAC.
- GDPR erasure/export API; EU-residency cluster option for whales.
- SOC 2 process (Vanta/Drata), managed Postgres, pen test.
