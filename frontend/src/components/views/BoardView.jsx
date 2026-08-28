import React from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";

const COLUMNS = [
  { key: "todo", label: "To Do", bar: "bg-slate-400" },
  { key: "in_progress", label: "In Progress", bar: "bg-blue-500" },
  { key: "review", label: "Review", bar: "bg-amber-500" },
  { key: "done", label: "Done", bar: "bg-emerald-500" },
];
const PRIO = {
  urgent: "border-l-red-500",
  high: "border-l-orange-500",
  medium: "border-l-slate-300",
  low: "border-l-slate-200",
};

export default function BoardView({ tasks, projects, members, onEdit, reload }) {
  const { currentOrg } = useOrg();
  const projectMap = Object.fromEntries(projects.map((p) => [p.project_id, p]));
  const memberMap = Object.fromEntries(members.map((m) => [m.user_id, m]));

  const move = async (task, status) => {
    if (task.status === status) return;
    await api.patch(`/orgs/${currentOrg.org_id}/tasks/${task.task_id}`, { status });
    reload();
  };

  const onDragStart = (e, task) => e.dataTransfer.setData("task_id", task.task_id);
  const onDrop = (e, status) => {
    e.preventDefault();
    const id = e.dataTransfer.getData("task_id");
    const task = tasks.find((t) => t.task_id === id);
    if (task) move(task, status);
  };

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
      {COLUMNS.map((col) => {
        const items = tasks.filter((t) => t.status === col.key);
        return (
          <div key={col.key} className="bg-slate-100/60 rounded-lg p-3 min-h-[500px]" onDragOver={(e) => e.preventDefault()} onDrop={(e) => onDrop(e, col.key)} data-testid={`board-col-${col.key}`}>
            <div className="flex items-center justify-between mb-3 px-2">
              <div className="flex items-center gap-2">
                <div className={`w-2 h-2 rounded-full ${col.bar}`} />
                <span className="text-sm font-medium text-slate-700">{col.label}</span>
                <span className="text-xs text-slate-400">{items.length}</span>
              </div>
            </div>
            <div className="space-y-2">
              {items.map((t) => {
                const a = memberMap[t.assignee_id];
                return (
                  <div
                    key={t.task_id}
                    draggable
                    onDragStart={(e) => onDragStart(e, t)}
                    onClick={() => onEdit(t)}
                    data-testid={`board-card-${t.task_id}`}
                    className={`bg-white rounded-lg border border-slate-200 p-3 cursor-pointer hover:shadow-md hover:-translate-y-0.5 transition-transform border-l-4 ${PRIO[t.priority]}`}
                  >
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-slate-100 text-slate-600">{projectMap[t.project_id]?.key || "—"}</span>
                      <Badge variant="secondary" className="text-[10px] capitalize">{t.type}</Badge>
                    </div>
                    <div className="text-sm font-medium text-slate-900 leading-snug">{t.title}</div>
                    <div className="flex items-center justify-between mt-3">
                      {a ? (
                        <Avatar className="h-6 w-6"><AvatarImage src={a.picture} /><AvatarFallback className="text-[10px] bg-indigo-100 text-indigo-700">{(a.name || a.email).slice(0, 2).toUpperCase()}</AvatarFallback></Avatar>
                      ) : <div className="h-6 w-6 rounded-full bg-slate-100" />}
                      <span className="text-[10px] text-slate-500 font-mono">{t.due_date?.slice(5) || ""}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
