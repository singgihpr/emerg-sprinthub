# Sprint Hub — SaaS / Enterprise Roadmap

Branch: `feat/saas-postgres-rls`

## Done: P0 — Security hardening (Mongo backend)

- **CORS allowlist** via `ALLOWED_ORIGINS` env: credentialed wildcard-origin reflect closed.
- **Short-lived access tokens** (15 min) + **HttpOnly refresh-cookie rotation**: `/api/auth/refresh` issues a new refresh token and deletes the old one atomically. Logout and password change revoke refresh tokens.
- **Rate limiter** on `POST /api/auth/*` (10 req/min/IP) + Echo `Recover()` middleware.
- **Password policy** raised to 12 characters; registration now enforces it (invite-accept and change-password already did).
- **Seed gating**: admin + demo data only run when `SEED_DEMO=true` (default `true` for local dev, set `false` in production).
- **Frontend 401 refresh**: axios interceptor retries once through `/api/auth/refresh` after a 401.
- **Tests**: refresh rotation/revocation, CORS allowlist, password policy, auth rate limit.

## Verified

```bash
cd backend
go vet ./...        # clean
go test ./...       # 13 passed
```

Frontend build verification: pending below.

## Next: P1 — Postgres + RLS migration

Why Postgres now: SOC 2 / GDPR + enterprise sales require **DB-enforced tenant isolation**. Mongo has no row-level security; every one of the ~45 query sites must remember `org_id`. RLS makes cross-tenant leaks structurally impossible.

1. **Typed data layer**: Go struct models + `sqlc` + `golang-migrate`. Mirror the 12 existing collections first, then replace `bson.M` loops.
2. **RLS**: `org_id` column on every tenant table; `SET app.current_org` per request; policies `USING (org_id = current_setting('app.current_org'))`. Connect as a non-superuser role.
3. **Repository layer**: single tenant-aware choke point; handlers lose direct collection access.
4. **Transactions**: invite accept, sprint delete, timer transitions become ACID.
5. **Audit log**: append-only `audit_events` table.
6. **Native dates**: `date` / `timestamptz` instead of string dates.
7. **Tests**: router-level suite against ephemeral Postgres (testcontainers or docker-compose service).
8. **Docker compose**: add Postgres, remove Mongo.

P1 is the big lift; it is also the data-layer rewrite the codebase already needs for SaaS.

## Then: P2 / P3 — SaaS business + enterprise

- Stripe plans + per-tenant quotas/metering.
- SSO / SAML via WorkOS (buy, don't build).
- SCIM directory sync + fine-grained RBAC.
- GDPR erasure/export API; EU-residency cluster option for whales.
- SOC 2 process (Vanta/Drata), managed Postgres, pen test.
