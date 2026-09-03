import React, { useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { useAuth } from "@/context/AuthContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Combobox } from "@/components/ui/combobox";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Plus, MagnifyingGlass, ListBullets, Kanban, ChartBar, CalendarBlank, Users, User, FolderOpen, CircleDashed, Rocket } from "@phosphor-icons/react";
import TaskDialog from "@/components/TaskDialog";
import ListView from "@/components/views/ListView";
import BoardView from "@/components/views/BoardView";
import GanttView from "@/components/views/GanttView";
import CalendarView from "@/components/views/CalendarView";
import WorkloadView from "@/components/views/WorkloadView";

const VIEWS = [
  { key: "list", label: "List", icon: ListBullets },
  { key: "board", label: "Board", icon: Kanban },
  { key: "gantt", label: "Gantt", icon: ChartBar },
  { key: "calendar", label: "Calendar", icon: CalendarBlank },
  { key: "workload", label: "Workload", icon: Users },
];

const TASK_STATUSES = [
  { v: "todo", l: "To Do" },
  { v: "in_progress", l: "In Progress" },
  { v: "review", l: "Review" },
  { v: "done", l: "Done" },
];

export default function Tasks() {
  const { currentOrg } = useOrg();
  const { user } = useAuth();
  const [view, setView] = useState("list");
  const [tasks, setTasks] = useState([]);
  const [projects, setProjects] = useState([]);
  const [members, setMembers] = useState([]);
  const [sprints, setSprints] = useState([]);
  const [query, setQuery] = useState("");
  const [assignee, setAssignee] = useState(() => localStorage.getItem("task_assignee_filter") || "all");
  const [projectFilter, setProjectFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [sprintFilter, setSprintFilter] = useState("all");
  const [groupByProject, setGroupByProject] = useState(false);
  const [dialog, setDialog] = useState({ open: false, task: null });

  const load = useCallback(async () => {
    if (!currentOrg) return;
    const [t, p, m, s] = await Promise.all([
      api.get(`/orgs/${currentOrg.org_id}/tasks`),
      api.get(`/orgs/${currentOrg.org_id}/projects`),
      api.get(`/orgs/${currentOrg.org_id}/members`),
      api.get(`/orgs/${currentOrg.org_id}/sprints`),
    ]);
    setTasks(t.data); setProjects(p.data); setMembers(m.data); setSprints(s.data);
  }, [currentOrg]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    const h = () => load();
    window.addEventListener("data-changed", h);
    return () => window.removeEventListener("data-changed", h);
  }, [load]);

  useEffect(() => { localStorage.setItem("task_assignee_filter", assignee); }, [assignee]);

  const filtered = tasks.filter((t) => {
    if (query && !t.title.toLowerCase().includes(query.toLowerCase())) return false;
    if (projectFilter !== "all" && t.project_id !== projectFilter) return false;
    if (statusFilter !== "all" && t.status !== statusFilter) return false;
    if (sprintFilter === "backlog" && t.sprint_id) return false;
    if (sprintFilter !== "all" && sprintFilter !== "backlog" && t.sprint_id !== sprintFilter) return false;
    if (assignee === "all") return true;
    if (assignee === "me") return t.assignee_id === user?.user_id;
    if (assignee === "unassigned") return !t.assignee_id;
    return t.assignee_id === assignee;
  });

  const props = { tasks: filtered, projects, members, sprints, onEdit: (task) => setDialog({ open: true, task }), reload: load };
  const listProps = { ...props, groupByProject, statusFilter };

  const projectOptions = [{ value: "all", label: "All projects" }, ...projects.map((p) => ({ value: p.project_id, label: p.name }))];
  const statusOptions = [{ value: "all", label: "All statuses" }, ...TASK_STATUSES.map((s) => ({ value: s.v, label: s.l }))];
  const sprintOptions = [
    { value: "all", label: "All sprints" },
    { value: "backlog", label: "Backlog" },
    ...sprints.map((s) => ({ value: s.sprint_id, label: s.name })),
  ];
  const assigneeOptions = [
    { value: "all", label: "All assignees" },
    { value: "me", label: "Assigned to me" },
    { value: "unassigned", label: "Unassigned" },
    ...members.filter((m) => m.user_id !== user?.user_id).map((m) => ({ value: m.user_id, label: m.name || m.email })),
  ];

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Work</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Tasks</h1>
          <p className="text-sm text-slate-500 mt-2">Sprint backlog & routine work — pick your view.</p>
        </div>
        {projects.length > 0 && (
          <Button className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={() => setDialog({ open: true, task: null })} data-testid="new-task-btn">
            <Plus size={16} weight="bold" /> New task
          </Button>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-3 justify-between">
        <Tabs value={view} onValueChange={setView}>
          <TabsList className="bg-white border border-slate-200 h-10 p-1">
            {VIEWS.map((v) => (
              <TabsTrigger key={v.key} value={v.key} data-testid={`view-${v.key}`} className="gap-2 data-[state=active]:bg-indigo-600 data-[state=active]:text-white">
                <v.icon size={15} weight="duotone" />
                {v.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="flex items-center gap-2">
          {view === "list" && (
            <div className="flex items-center gap-2 mr-1 px-3 h-10 rounded-md border border-slate-200 bg-white" data-testid="group-by-project-wrap">
              <Switch id="group-by-project" checked={groupByProject} onCheckedChange={setGroupByProject} data-testid="group-by-project-toggle" />
              <Label htmlFor="group-by-project" className="text-sm text-slate-600 cursor-pointer whitespace-nowrap">Group by project</Label>
            </div>
          )}
          <Combobox value={projectFilter} onValueChange={setProjectFilter} options={projectOptions}
            icon={<FolderOpen size={14} className="mr-1 text-slate-400 shrink-0" />}
            className="h-10 w-48 bg-white" data-testid="project-filter" />
          <Combobox value={statusFilter} onValueChange={setStatusFilter} options={statusOptions}
            icon={<CircleDashed size={14} className="mr-1 text-slate-400 shrink-0" />}
            className="h-10 w-44 bg-white" data-testid="status-filter" />
          <Combobox value={sprintFilter} onValueChange={setSprintFilter} options={sprintOptions}
            icon={<Rocket size={14} className="mr-1 text-slate-400 shrink-0" />}
            className="h-10 w-48 bg-white" data-testid="sprint-filter" />
          <Combobox value={assignee} onValueChange={setAssignee} options={assigneeOptions}
            icon={<User size={14} className="mr-1 text-slate-400 shrink-0" />}
            className="h-10 w-52 bg-white" data-testid="assignee-filter" />
          <div className="relative">
            <MagnifyingGlass size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search tasks…" className="pl-9 h-10 w-64" data-testid="task-search" />
          </div>
        </div>
      </div>

      <div data-testid={`view-content-${view}`}>
        {view === "list" && <ListView {...listProps} />}
        {view === "board" && <BoardView {...props} />}
        {view === "gantt" && <GanttView {...props} />}
        {view === "calendar" && <CalendarView {...props} />}
        {view === "workload" && <WorkloadView {...props} />}
      </div>

      <TaskDialog
        open={dialog.open} onOpenChange={(o) => setDialog({ open: o, task: o ? dialog.task : null })}
        task={dialog.task} projects={projects} members={members} sprints={sprints}
        onSaved={load}
      />
    </div>
  );
}
