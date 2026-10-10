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

## In progress: P1 — Postgres + RLS foundation

- **Schema**: `backend/migrations/000001_init_schema.*.sql` — 14 tables mirroring collections, indexes, RLS policies.
- **Connection**: `backend/pg.go` — pgx pool + `golang-migrate` runner, optional `DATABASE_URL`.
- **Infra**: `docker-compose.yml` adds `postgres:16`; `.env.example` + `README.md` updated.

## Verified

```bash
cd backend
go vet ./...        # clean
go test ./...       # 13 passed (Mongo tests)
```

Postgres migration applied verification: run with `DATABASE_URL` set (see below).

## Next: P1 continued — data-layer cutover

Why Postgres now: SOC 2 / GDPR + enterprise sales require **DB-enforced tenant isolation**. Mongo has no row-level security; every one of the ~45 query sites must remember `org_id`. RLS makes cross-tenant leaks structurally impossible.

1. **Per-request Postgres transaction middleware** that sets `app.current_user` and `app.current_org` so RLS policies fire.
2. **Repository/query helpers** for Postgres returning map-shaped rows to keep handler churn low.
3. **Migrate handler groups one at a time**: auth → orgs/members → projects → sprints → tasks → time/timer → comments/analytics/cron.
4. **Audit log**: append-only `audit_events` table.
5. **Tests**: router-level suite against ephemeral Postgres (testcontainers or docker-compose service).
6. **Remove Mongo**: once all handlers use Postgres, drop Mongo connection + seed + docker-compose service.

P1 is the big lift; it is also the data-layer rewrite the codebase already needs for SaaS.

## Then: P2 / P3 — SaaS business + enterprise

- Stripe plans + per-tenant quotas/metering.
- SSO / SAML via WorkOS (buy, don't build).
- SCIM directory sync + fine-grained RBAC.
- GDPR erasure/export API; EU-residency cluster option for whales.
- SOC 2 process (Vanta/Drata), managed Postgres, pen test.
