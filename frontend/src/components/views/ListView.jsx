import React from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Play, Trash, PencilSimple } from "@phosphor-icons/react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";

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

export default function ListView({ tasks, projects, members, onEdit, reload }) {
  const { currentOrg } = useOrg();
  const projectMap = Object.fromEntries(projects.map((p) => [p.project_id, p]));
  const memberMap = Object.fromEntries(members.map((m) => [m.user_id, m]));

  const startTimer = async (task) => {
    await api.post(`/orgs/${currentOrg.org_id}/timer/start`, { task_id: task.task_id });
    window.dispatchEvent(new Event("timer-changed"));
  };
  const del = async (task) => {
    if (!window.confirm("Delete this task?")) return;
    await api.delete(`/orgs/${currentOrg.org_id}/tasks/${task.task_id}`);
    reload();
  };

  return (
    <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
      <table className="w-full text-sm">
        <thead className="bg-slate-50 text-slate-500 text-xs uppercase tracking-wider">
          <tr>
            <th className="text-left px-4 py-3 font-medium">Task</th>
            <th className="text-left px-4 py-3 font-medium">Project</th>
            <th className="text-left px-4 py-3 font-medium">Status</th>
            <th className="text-left px-4 py-3 font-medium">Priority</th>
            <th className="text-left px-4 py-3 font-medium">Assignee</th>
            <th className="text-left px-4 py-3 font-medium">Due</th>
            <th className="text-left px-4 py-3 font-medium">Time</th>
            <th className="text-right px-4 py-3 font-medium">Actions</th>
          </tr>
        </thead>
        <tbody>
          {tasks.map((t) => {
            const a = memberMap[t.assignee_id];
            const logged = t.logged_minutes || 0;
            return (
              <tr key={t.task_id} className="border-t border-slate-100 hover:bg-slate-50/50" data-testid={`task-row-${t.task_id}`}>
                <td className="px-4 py-3 font-medium text-slate-900">{t.title}</td>
                <td className="px-4 py-3">
                  <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-100 text-slate-600">{projectMap[t.project_id]?.key || "—"}</span>
                </td>
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
          {tasks.length === 0 && (
            <tr><td colSpan={8} className="text-center py-12 text-slate-500">No tasks. Create one to get started.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
