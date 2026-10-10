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

## Done: P1 chunk 1 — auth + orgs/memberships/invites on Postgres + RLS

- **Per-request Postgres transaction middleware** (`pgTxMiddleware`) sets `app.current_user` and `app.current_org` so RLS policies fire.
- **Map-shaped query helpers** (`pgFindOne`, `pgFindMany`, `pgInsert`, `pgUpdate`, `pgDelete`) plus background variants for seed/cron.
- **Migrated to Postgres**: `users`, `refresh_tokens`, `organizations`, `memberships`, `invites`.
- **Still on Mongo**: `projects`, `project_members`, `sprints`, `tasks`, `recurring_tasks`, `time_entries`, `active_timers`, `comments`, `cron_runs`.
- **Seed** now writes admin/demo org/membership to Postgres; demo project/sprint/tasks still Mongo.
- **Tests** run against both ephemeral Postgres and Mongo; `getTestDatabaseURL()` reads `DATABASE_URL` or defaults to `localhost:5432`.

## Verified

```bash
cd backend
go vet ./...        # clean
go test ./...       # 13 passed
```

## Next: P1 chunk 2 — migrate remaining entities

1. `projects` + `project_members`
2. `sprints`
3. `tasks` + `recurring_tasks`
4. `time_entries` + `active_timers`
5. `comments` + analytics aggregations
6. `cron_runs`
7. Then remove Mongo connection, `initIndexes`, seed demo Mongo data, and the `mongo` docker-compose service.

P1 is the big lift; it is also the data-layer rewrite the codebase already needs for SaaS.

## Then: P2 / P3 — SaaS business + enterprise

- Stripe plans + per-tenant quotas/metering.
- SSO / SAML via WorkOS (buy, don't build).
- SCIM directory sync + fine-grained RBAC.
- GDPR erasure/export API; EU-residency cluster option for whales.
- SOC 2 process (Vanta/Drata), managed Postgres, pen test.
