# SprintHub — Work Management App

## Original Problem Statement
Build a simply usable work management app that can handle sprint backlog and routine task, monitoring task status, task time tracking. Master data: organization, users, roles, project. User can switch task views between list, board (Kanban), Gantt chart, calendar, and workload views. User can know achievement and productivity.

## User Choices
- Auth: JWT + Emergent Google OAuth
- Multi-org with organization switcher
- All 5 views: List, Kanban, Gantt, Calendar, Workload
- Time tracking: Start/Stop timer + manual entry
- Design: Modern Professional light theme (indigo/slate)

## Architecture
- **Backend**: FastAPI + Motor(MongoDB) + JWT + bcrypt + httpx (Resend email)
- **Frontend**: React + Tailwind + Shadcn UI + Phosphor Icons + Recharts + Sonner toasts
- **Collections**: users, organizations, memberships, projects, project_members, sprints, tasks, time_entries, active_timers, user_sessions, comments, cron_runs
- **Scheduling**: Emergent-managed cron via `.emergent/crons.yml` (weekly-digest Mon 08:00 UTC)

## Authorization Model
- **Org roles**: owner, admin, manager, member
- **Project roles**: lead, member
- **Owner/Admin**: see & mutate everything in the org
- **Manager/Member**: see & mutate only projects they belong to
  - Enforced via `require_project_access(org_id, project_id, user)` and `require_task_access(org_id, task_id, user)` — applied on every project- or task-scoped route (list, PATCH, DELETE, comments, sprints, analytics, time-entries)

## Implemented
### Phase 1 (MVP)
- [x] JWT + Google OAuth
- [x] Multi-org, master data (Users/Projects/Sprints)
- [x] Tasks CRUD + 5 views (List/Kanban/Gantt/Calendar/Workload)
- [x] Time tracking (live timer widget + manual)
- [x] Analytics dashboard

### Phase 2 — Team Activity
- [x] Live per-member dashboard: active timer, in-progress tasks, capacity, hours

### Phase 3 — Extras (Feb 2026)
- [x] Sprint Burndown (Recharts ideal vs actual)
- [x] Idle Alerts (>2h timer badge, red variant)
- [x] Weekly Digest email cron (Resend, idempotent)
- [x] Task Comments with @mentions

### Phase 4 — Task Filter (Feb 2026)
- [x] Tasks page: assignee filter (All / Me / Unassigned / member) persisted in localStorage

### Phase 5 — Project Members + Task Visibility (Feb 2026)
- [x] Project editing (name/description/color) via new ProjectDialog on Projects page and Sprints page (click sprint's project link)
- [x] Project members CRUD (`project_members` collection) with lead/member sub-roles
- [x] Startup backfill: every seeded/existing project's creator becomes a project lead
- [x] Task visibility: owner/admin see everything; manager/member only see projects they belong to
- [x] `require_project_access` / `require_task_access` helpers applied to every project-scoped route (list, create, update, delete, comments, project-members, sprints list & create, analytics, time-entries)
- [x] Deterministic `/orgs` sort (created_at asc)
- [x] Toasts (sonner) on ProjectDialog save/add/remove
- [x] UI role gating: hide "New project", "New task", "New sprint" for role=member; per-card "View only" vs "Edit"; empty-state copy adapted

## Backlog / Next Priorities
- [ ] DELETE /projects/{id} endpoint + confirm dialog
- [ ] AlertDialog for confirms instead of window.confirm
- [ ] Constrain project role to Literal["member","lead"] and validate project exists on member add
- [ ] Return 404 from DELETE project-member when nothing was deleted
- [ ] Real email invites for new org members (Resend)
- [ ] Split server.py (~1160 lines) into routers per resource
- [ ] Styled color picker instead of native <input type=color>

## Test Credentials
See `/app/memory/test_credentials.md`
