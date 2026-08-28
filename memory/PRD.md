# SprintHub — Work Management App

## Original Problem Statement
Build a simply usable work management app that can handle sprint backlog and routine task, monitoring task status, task time tracking. Master data: organization, users, roles, project. User can switch task views between list, board (Kanban), Gantt chart, calendar, and workload views. User can know achievement and productivity.

## User Choices
- Auth: JWT + Emergent Google OAuth (both)
- Multi-org with organization switcher
- All 5 views: List, Kanban, Gantt, Calendar, Workload
- Time tracking: Start/Stop timer + manual entry
- Design: Modern Professional light theme (indigo/slate)

## Architecture
- **Backend**: FastAPI + Motor(MongoDB) + JWT + bcrypt + httpx (Resend email)
- **Frontend**: React + Tailwind + Shadcn UI + Phosphor Icons + Recharts
- **Collections**: users, organizations, memberships, projects, sprints, tasks, time_entries, active_timers, user_sessions, comments, cron_runs
- **Scheduling**: Emergent-managed cron via `.emergent/crons.yml` (weekly-digest Mon 08:00 UTC)

## Implemented
### Phase 1 (MVP)
- [x] JWT email/password auth + Emergent Google OAuth
- [x] Multi-org with org switcher & create-workspace
- [x] Master data: Users (roles owner/admin/manager/member), Projects, Sprints
- [x] Tasks CRUD (title/description/status/priority/type/assignee/estimate/dates)
- [x] 5 views: List, Kanban (drag-and-drop), Gantt, Calendar, Workload
- [x] Time tracking: floating live timer widget + manual entries
- [x] Analytics dashboard: KPIs + weekly chart + status pie + estimate vs logged

### Phase 2 (Team Activity)
- [x] `GET /api/orgs/{id}/team-activity` (admin/manager/owner only)
- [x] `/team` page: live per-member panel — active timer, in-progress tasks, capacity, today/week hours, recent entries

### Phase 3 (Feb 2026)
- [x] **Sprint Burndown**: `GET /api/orgs/{id}/sprints/{sid}/burndown` returns ideal+actual series; "View burndown" dialog per sprint card with Recharts line chart
- [x] **Idle Alerts**: Team Activity KPI "Idle > 2h" + red variant of active-timer panel when elapsed > 2h
- [x] **Weekly Digest**: `POST /api/cron/weekly-digest` (Bearer WEBHOOK_CRON_SECRET, idempotent via X-Webhook-Id) queues background email to every admin/owner with last week's hours per member + overdue tasks. Uses Resend via Emergent proxy. Cron: `0 8 * * 1` UTC.
- [x] **Task Comments**: `GET/POST /api/orgs/{id}/tasks/{tid}/comments`. `@email` mentions parsed and stored. Inline mention suggestion picker with focus restore. Empty/blank/oversized bodies rejected.

## Backlog / Next Priorities
- [ ] DELETE /api/orgs/{id}/sprints/{sid}
- [ ] Sprint end-date-before-start-date validation
- [ ] Notify email on @mention (task comments)
- [ ] Clear completed_at when task status leaves "done"
- [ ] Use hmac.compare_digest for cron secret
- [ ] Split server.py (~1030 lines) into routers
- [ ] Add DialogDescription to dialogs for a11y
- [ ] Real email invites for new members (Resend)
- [ ] Burndown chart embedded in Dashboard for active sprints
- [ ] Task attachments, saved filters, bulk operations

## Test Credentials
See `/app/memory/test_credentials.md`
