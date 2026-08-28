# SprintHub — Work Management App

## Original Problem Statement
Build a simply usable work management app that can handle sprint backlog and routine task, monitoring task status, task time tracking. Master data: organization, users, roles, project. User can switch task views between list, board (Kanban), Gantt chart, calendar, and workload views. User can know achievement and productivity.

## User Choices
- Auth: Both JWT and Emergent Google Auth
- Multi-org support with organization switcher
- All five views: List, Kanban, Gantt, Calendar, Workload
- Time tracking: Start/Stop timer + manual entry
- Design: Modern Professional light theme (indigo/slate)

## Architecture
- **Backend**: FastAPI + Motor(MongoDB) + JWT + bcrypt
- **Frontend**: React + Tailwind + Shadcn UI + Phosphor Icons + Recharts
- **Collections**: users, organizations, memberships, projects, sprints, tasks, time_entries, active_timers, user_sessions

## Implemented (Feb 2026)
- [x] JWT email/password auth + Emergent Google OAuth
- [x] Multi-org with org switcher & create-workspace
- [x] Master data: Users (with roles owner/admin/manager/member), Projects, Sprints
- [x] Tasks: create/update/delete, priority, status, type (task/routine/bug/story), estimate, dates
- [x] 5 views: List, Kanban (drag-and-drop), Gantt (timeline bars), Calendar (month view), Workload (capacity bars per user)
- [x] Time tracking: floating timer widget with live tick, manual entry via API, per-task logged minutes
- [x] Analytics dashboard: KPIs, weekly productivity, status pie, estimated vs logged
- [x] Seed data: Acme Corp org + Web Redesign project + Sprint 1 + 5 tasks
- [x] Test credentials at /app/memory/test_credentials.md

## Backlog / Next Priorities
- [ ] Add DialogDescription to dialogs for full a11y
- [ ] Replace native date picker with shadcn Calendar+Popover in TaskDialog
- [ ] Validate project_id/sprint_id belong to org on task create/update
- [ ] Enum/Literal constraints for status/priority/type
- [ ] Login brute-force lockout (5 fails / 15 min)
- [ ] Explicit CORS origins from env (currently `*`)
- [ ] Real email invites (Resend) instead of default password
- [ ] Bulk task ops, saved filters, task comments/attachments
- [ ] Burndown chart per sprint, velocity trend

## Test Credentials
See `/app/memory/test_credentials.md`
