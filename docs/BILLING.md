# Billing & Quotas

## Plans

Three tiers with generous limits:

| Plan       | Members | Projects | Storage | API Calls/Month |
|------------|---------|----------|---------|-----------------|
| **Free**       | 5       | 3        | 500 MB  | 10,000          |
| **Pro**        | 25      | 50       | 50 GB   | 100,000         |
| **Enterprise** | 1,000   | 1,000    | 500 GB  | 1,000,000       |

Plans are stored in the `plans` table with limits as JSONB. `stripe_price_id` links to Stripe for paid tiers (set via env/Stripe dashboard).

## Quota Enforcement

**Soft-cap model**: existing data over limit is preserved, but new creations are blocked until usage drops below the limit.

Example: org on Free plan (5 members) invites 2 more via legacy code path → 7 members. Downgrade from Pro to Free → still 7 members, but can't add new members until count drops to ≤4.

### Enforcement Points

- `POST /api/orgs/:org_id/projects` → checks `projects` quota
- `POST /api/orgs/:org_id/members` → checks `members` quota
- All `/api/orgs/:org_id/*` endpoints → increment `api_calls` quota

Returns `402 Payment Required` with message:
```
Quota exceeded: members limit is 5 (current: 5). Upgrade your plan.
```

### Tracking

`org_quotas` table maintains current counts per org:
- `current_members` — incremented on `inviteMember`, decremented on member removal
- `current_projects` — incremented on `createProject`, decremented on `deleteProject`
- `api_calls_this_month` — incremented by `usageMeteringMiddleware`, resets monthly
- `api_calls_reset_at` — timestamp for next reset

## Usage Metering

Every authenticated API call to an org-scoped endpoint logs a `usage_events` row:
```sql
INSERT INTO usage_events (event_id, org_id, event_type, quantity, timestamp)
VALUES ('evt_...', 'org_...', 'api_call', 1, now())
```

Event types:
- `api_call` — every authenticated request (via middleware)
- `member_added` — when a new member joins
- `project_created` — when a new project is created
- `storage_bytes` — (future) file upload sizes

Usage events enable:
1. **Overage billing** — charge for API calls beyond plan limit
2. **Dashboard analytics** — show usage trends per org
3. **Alerting** — notify admins approaching limits

## Stripe Integration

### Checkout Flow

1. Frontend calls `POST /api/billing/checkout` with `plan_id`
2. Backend creates Stripe Checkout session with `success_url` and `cancel_url`
3. User completes payment on Stripe-hosted page
4. Stripe sends `checkout.session.completed` webhook
5. Backend updates `organizations` with `stripe_customer_id`, `stripe_subscription_id`, `plan_id`

### Customer Portal

`POST /api/billing/portal` → returns Stripe Customer Portal URL for managing subscription, payment methods, invoices.

### Webhook Security

`POST /api/billing/webhook` verifies `Stripe-Signature` header using `STRIPE_WEBHOOK_SECRET`. Rejects unsigned/malformed requests with `400 Bad Request`.

Handled events:
- `checkout.session.completed` — activates subscription
- `customer.subscription.updated` — updates plan/status on upgrade/downgrade
- `customer.subscription.deleted` — downgrades to Free, sets `subscription_status = 'canceled'`

## Environment Variables

```env
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
APP_BASE_URL=https://sprinthub.example.com
```

All billing endpoints return `503 Service Unavailable` if Stripe credentials are missing (graceful degradation for self-hosted instances).

## Downgrade Path

When a subscription is canceled or expires:
1. Webhook sets `plan_id = 'free'` and `subscription_status = 'canceled'`
2. Existing data (members, projects) remains intact
3. New creations blocked until usage drops below Free plan limits
4. Org admins receive `402` with upgrade prompt

Soft-cap avoids data loss while enforcing plan limits on growth.
