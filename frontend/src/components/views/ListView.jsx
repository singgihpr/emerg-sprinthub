import React, { useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Play, Trash, PencilSimple, CaretDown, FolderOpen } from "@phosphor-icons/react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";

const STATUS_STYLES = {
  todo: "bg-slate-100 text-slate-700",
  in_progress: "bg-blue-100 text-blue-700",
  review: "bg-amber-100 text-amber-700",
  done: "bg-emerald-100 text-emerald-700",
};
const STATUS_LABELS = { todo: "To Do", in_progress: "In Progress", review: "Review", done: "Done" };
const PRIO = {
  urgent: "bg-red-100 text-red-700",
  high: "bg-orange-100 text-orange-700",
  medium: "bg-slate-100 text-slate-600",
  low: "bg-slate-50 text-slate-500",
};

export default function ListView({ tasks, projects, members, onEdit, reload, groupByProject = false }) {
  const { currentOrg } = useOrg();
  const projectMap = Object.fromEntries(projects.map((p) => [p.project_id, p]));
  const memberMap = Object.fromEntries(members.map((m) => [m.user_id, m]));
  const [collapsed, setCollapsed] = useState({});

  const startTimer = async (task) => {
    await api.post(`/orgs/${currentOrg.org_id}/timer/start`, { task_id: task.task_id });
    window.dispatchEvent(new Event("timer-changed"));
  };
  const del = async (task) => {
    if (!window.confirm("Delete this task?")) return;
    await api.delete(`/orgs/${currentOrg.org_id}/tasks/${task.task_id}`);
    reload();
  };

  const TaskTable = ({ rows, showProjectCol = true }) => (
    <table className="w-full text-sm">
      <thead className="bg-slate-50 text-slate-500 text-xs uppercase tracking-wider">
        <tr>
          <th className="text-left px-4 py-3 font-medium">Task</th>
          {showProjectCol && <th className="text-left px-4 py-3 font-medium">Project</th>}
          <th className="text-left px-4 py-3 font-medium">Status</th>
          <th className="text-left px-4 py-3 font-medium">Priority</th>
          <th className="text-left px-4 py-3 font-medium">Assignee</th>
          <th className="text-left px-4 py-3 font-medium">Due</th>
          <th className="text-left px-4 py-3 font-medium">Time</th>
          <th className="text-right px-4 py-3 font-medium">Actions</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((t) => {
          const a = memberMap[t.assignee_id];
          const logged = t.logged_minutes || 0;
          return (
            <tr key={t.task_id} className="border-t border-slate-100 hover:bg-slate-50/50" data-testid={`task-row-${t.task_id}`}>
              <td className="px-4 py-3 font-medium text-slate-900">{t.title}</td>
              {showProjectCol && (
                <td className="px-4 py-3">
                  <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-100 text-slate-600">{projectMap[t.project_id]?.key || "—"}</span>
                </td>
              )}
              <td className="px-4 py-3">
                <Badge className={`${STATUS_STYLES[t.status]} border-0 font-normal`}>{STATUS_LABELS[t.status]}</Badge>
              </td>
              <td className="px-4 py-3">
                <Badge className={`${PRIO[t.priority]} border-0 font-normal capitalize`}>{t.priority}</Badge>
              </td>
              <td className="px-4 py-3">
                {a ? (
                  <div className="flex items-center gap-2">
                    <Avatar className="h-6 w-6"><AvatarImage src={a.picture} /><AvatarFallback className="text-[10px] bg-indigo-100 text-indigo-700">{(a.name || a.email).slice(0, 2).toUpperCase()}</AvatarFallback></Avatar>
                    <span className="text-slate-700 text-xs">{a.name || a.email}</span>
                  </div>
                ) : <span className="text-slate-400 text-xs">Unassigned</span>}
              </td>
              <td className="px-4 py-3 text-slate-600 text-xs">{t.due_date || "—"}</td>
              <td className="px-4 py-3 text-slate-600 text-xs font-mono">{Math.floor(logged / 60)}h {logged % 60}m</td>
              <td className="px-4 py-3">
                <div className="flex items-center justify-end gap-1">
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0 hover:bg-orange-50 hover:text-orange-600" onClick={() => startTimer(t)} data-testid={`start-timer-${t.task_id}`}><Play size={14} weight="fill" /></Button>
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0" onClick={() => onEdit(t)} data-testid={`edit-task-${t.task_id}`}><PencilSimple size={14} /></Button>
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0 hover:bg-red-50 hover:text-red-600" onClick={() => del(t)} data-testid={`delete-task-${t.task_id}`}><Trash size={14} /></Button>
                </div>
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );

  if (!groupByProject) {
    return (
      <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
        {tasks.length === 0 ? (
          <div className="text-center py-12 text-slate-500">No tasks. Create one to get started.</div>
        ) : (
          <TaskTable rows={tasks} />
        )}
      </div>
    );
  }

  // Grouped by project
  const groups = projects
    .map((p) => ({ project: p, rows: tasks.filter((t) => t.project_id === p.project_id) }))
    .filter((g) => g.rows.length > 0);
  const orphanRows = tasks.filter((t) => !projectMap[t.project_id]);
  if (orphanRows.length > 0) {
    groups.push({ project: { project_id: "__none__", name: "No project", key: "—", color: "#94A3B8" }, rows: orphanRows });
  }

  if (tasks.length === 0) {
    return (
      <div className="bg-white border border-slate-200 rounded-lg p-12 text-center text-slate-500 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
        No tasks. Create one to get started.
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {groups.map(({ project, rows }) => {
        const isOpen = !collapsed[project.project_id];
        return (
          <Collapsible key={project.project_id} open={isOpen} onOpenChange={(o) => setCollapsed((c) => ({ ...c, [project.project_id]: !o }))}>
            <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
              <CollapsibleTrigger asChild>
                <button className="w-full flex items-center gap-3 px-4 py-3 hover:bg-slate-50 transition-colors" data-testid={`project-group-${project.project_id}`}>
                  <CaretDown size={16} className={`text-slate-400 transition-transform ${isOpen ? "" : "-rotate-90"}`} />
                  <div className="w-7 h-7 rounded-md flex items-center justify-center" style={{ background: `${project.color}20`, color: project.color }}>
                    <FolderOpen size={16} weight="duotone" />
                  </div>
                  <span className="font-display font-semibold text-slate-900">{project.name}</span>
                  {project.key && project.key !== "—" && (
                    <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-slate-100 text-slate-600">{project.key}</span>
                  )}
                  <Badge variant="secondary" className="ml-auto text-xs">{rows.length}</Badge>
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <div className="border-t border-slate-100">
                  <TaskTable rows={rows} showProjectCol={false} />
                </div>
              </CollapsibleContent>
            </div>
          </Collapsible>
        );
      })}
    </div>
  );
}
