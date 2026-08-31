# SprintHub — Work Management App

## Original Problem Statement
Build a simply usable work management app that can handle sprint backlog and routine task, monitoring task status, task time tracking. Master data: organization, users, roles, project. User can switch task views between list, board (Kanban), Gantt chart, calendar, and workload views. User can know achievement and productivity.

## Architecture
- **Backend**: FastAPI + Motor(MongoDB) + JWT + bcrypt + httpx + Pillow (logo resize)
- **Frontend**: React + Tailwind + Shadcn UI + Phosphor Icons + Recharts + Sonner
- **Cron**: `.emergent/crons.yml` (weekly-digest Mon 08:00 UTC)

## Authorization
- Org roles: owner, admin, manager, member
- Project sub-roles: lead, member
- Owner/Admin: full org; Manager/Member: only projects they belong to (via `require_project_access` + `require_task_access`)

## Implemented Milestones
1. MVP — Auth (JWT+Google), Multi-org, Projects/Sprints/Tasks CRUD, 5 task views, Time tracking, Analytics
2. Team Activity — Live per-member dashboard
3. Burndown, Idle Alerts, Weekly Digest email, Task Comments with @mentions
4. Task assignee filter
5. Project Members + Task Visibility scoping
6. Organization editing (name, logo) — logo now supports file upload with server-side resize (Pillow → 256×256 WebP, stored as data URL) and clear
7. User Profile page (name, avatar URL, password change)
8. **Project delete with cascade** (sprints, tasks, comments, time-entries, active-timers, project-members)
9. **Project status** (planning/active/on_hold/archived) — Literal-validated on POST and PATCH, displayed as colored badge on project cards
10. **Sprint filters** — by project and by status (client-side filter over the accessible sprint set)

## Backlog / Next Priorities
- [ ] DELETE /orgs/{id}/members/{user_id} endpoint (currently cannot remove an org member)
- [ ] AlertDialog for confirms instead of window.confirm
- [ ] Constrain invite role to Literal (owner/admin/manager/member)
- [ ] Return 404 on PATCH unknown org instead of 403
- [ ] Real email invites (Resend)
- [ ] File upload for avatar (currently URL only)
- [ ] Split server.py (~1300 lines) into routers
- [ ] Wire Sprints.jsx to use `?project_id=&status=` params instead of client-side filter

## Test Credentials
See `/app/memory/test_credentials.md`
