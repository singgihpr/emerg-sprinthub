# SprintHub — Work Management App

## Original Problem Statement
Build a simply usable work management app that can handle sprint backlog and routine task, monitoring task status, task time tracking. Master data: organization, users, roles, project. User can switch task views between list, board (Kanban), Gantt chart, calendar, and workload views. User can know achievement and productivity.

## Architecture
- **Backend**: FastAPI + Motor(MongoDB) + JWT + bcrypt + httpx (Resend email proxy)
- **Frontend**: React + Tailwind + Shadcn UI + Phosphor Icons + Recharts + Sonner
- **Cron**: `.emergent/crons.yml` (weekly-digest Mon 08:00 UTC)

## Authorization Model
- Org roles: owner, admin, manager, member
- Project sub-roles: lead, member
- Owner/Admin: full org access; Manager/Member: only projects they belong to
- Enforced via `require_project_access` + `require_task_access` on every project/task-scoped route

## Implemented Milestones
1. **MVP** — Auth (JWT+Google), Multi-org, Projects/Sprints/Tasks CRUD, 5 task views, Time tracking, Analytics
2. **Team Activity** — Live per-member dashboard
3. **Burndown, Idle Alerts, Weekly Digest email, Task Comments with @mentions**
4. **Task assignee filter**
5. **Project Members + Task Visibility scoping**
6. **Organization editing** — PATCH /orgs/{id} (name, logo) — owner/admin only, with Workspace Settings dialog in the sidebar
7. **User Profile** — /profile page with Details (name, avatar URL, email display) and Password (current+new+confirm) tabs. New endpoints: PATCH /auth/me, POST /auth/change-password

## Backlog / Next Priorities
- [ ] DELETE /projects/{id} endpoint + confirm dialog
- [ ] AlertDialog for confirms instead of window.confirm
- [ ] Real email invites for new org/project members
- [ ] File upload for avatar/logo (currently URL only)
- [ ] Split server.py (~1200 lines) into routers
- [ ] Styled color picker instead of native <input type=color>

## Test Credentials
See `/app/memory/test_credentials.md`
