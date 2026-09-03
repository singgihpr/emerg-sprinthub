import React, { useState, useEffect } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import TaskComments from "@/components/TaskComments";

const STATUS = [
  { v: "todo", l: "To Do" },
  { v: "in_progress", l: "In Progress" },
  { v: "review", l: "In Review" },
  { v: "done", l: "Done" },
];
const PRIORITY = ["low", "medium", "high", "urgent"];
const TYPE = ["task", "routine", "bug", "story"];

export default function TaskDialog({ open, onOpenChange, task, projects, members, sprints, onSaved }) {
  const { currentOrg } = useOrg();
  const [form, setForm] = useState({
    title: "", description: "", status: "todo", priority: "medium", type: "task",
    project_id: "", assignee_id: "", sprint_id: "", start_date: "", due_date: "", estimate_hours: 0, repeat: "none",
  });

  useEffect(() => {
    if (task) {
      setForm({
        title: task.title || "", description: task.description || "",
        status: task.status || "todo", priority: task.priority || "medium",
        type: task.type || "task", project_id: task.project_id || "",
        assignee_id: task.assignee_id || "", sprint_id: task.sprint_id || "",
        start_date: task.start_date || "", due_date: task.due_date || "",
        estimate_hours: task.estimate_hours || 0, repeat: task.repeat || "none",
      });
    } else {
      setForm({
        title: "", description: "", status: "todo", priority: "medium", type: "task",
        project_id: projects?.[0]?.project_id || "", assignee_id: "", sprint_id: "",
        start_date: "", due_date: "", estimate_hours: 0, repeat: "none",
      });
    }
  }, [task, open, projects]);

  const sprintOptions = [
    { value: "", label: "Backlog" },
    ...(sprints || []).filter((s) => s.project_id === form.project_id).map((s) => ({ value: s.sprint_id, label: s.name })),
  ];

  const save = async () => {
    if (!form.title || !form.project_id) return;
    const payload = {
      ...form,
      estimate_hours: parseFloat(form.estimate_hours) || 0,
      assignee_id: form.assignee_id || null,
      sprint_id: form.sprint_id || null,
      start_date: form.start_date || null,
      due_date: form.due_date || null,
    };
    if (task) {
      delete payload.repeat;
      await api.patch(`/orgs/${currentOrg.org_id}/tasks/${task.task_id}`, payload);
    } else {
      await api.post(`/orgs/${currentOrg.org_id}/tasks`, payload);
    }
    onSaved?.();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader><DialogTitle>{task ? "Edit task" : "New task"}</DialogTitle></DialogHeader>
        <div className="space-y-4 max-h-[75vh] overflow-y-auto pr-1">
          <div>
            <Label>Title</Label>
            <Input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} data-testid="task-title-input" className="mt-1.5" />
          </div>
          <div>
            <Label>Description</Label>
            <Textarea rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="mt-1.5" />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Project</Label>
              <Combobox value={form.project_id} onValueChange={(v) => setForm({ ...form, project_id: v, sprint_id: "" })}
                placeholder="Select project"
                options={(projects || []).map((p) => ({ value: p.project_id, label: p.name }))}
                className="mt-1.5" data-testid="task-project-select" />
            </div>
            <div>
              <Label>Sprint</Label>
              <Combobox value={form.sprint_id} onValueChange={(v) => setForm({ ...form, sprint_id: v })}
                placeholder="Backlog" options={sprintOptions} className="mt-1.5" data-testid="task-sprint-select" />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <Label>Status</Label>
              <Combobox value={form.status} onValueChange={(v) => setForm({ ...form, status: v })}
                options={STATUS.map((s) => ({ value: s.v, label: s.l }))} className="mt-1.5" />
            </div>
            <div>
              <Label>Priority</Label>
              <Combobox value={form.priority} onValueChange={(v) => setForm({ ...form, priority: v })}
                options={PRIORITY.map((p) => ({ value: p, label: p[0].toUpperCase() + p.slice(1) }))} className="mt-1.5" />
            </div>
            <div>
              <Label>Type</Label>
              <Combobox value={form.type} onValueChange={(v) => setForm({ ...form, type: v })}
                options={TYPE.map((t) => ({ value: t, label: t[0].toUpperCase() + t.slice(1) }))} className="mt-1.5" />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Assignee</Label>
              <Combobox value={form.assignee_id} onValueChange={(v) => setForm({ ...form, assignee_id: v })}
                placeholder="Unassigned"
                options={[{ value: "", label: "Unassigned" }, ...(members || []).map((m) => ({ value: m.user_id, label: m.name || m.email }))]}
                className="mt-1.5" />
            </div>
            <div>
              <Label>Estimate (hours)</Label>
              <Input type="number" step="0.5" value={form.estimate_hours} onChange={(e) => setForm({ ...form, estimate_hours: e.target.value })} className="mt-1.5" />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Start date</Label>
              <Input type="date" value={form.start_date} onChange={(e) => setForm({ ...form, start_date: e.target.value })} className="mt-1.5" />
            </div>
            <div>
              <Label>Due date</Label>
              <Input type="date" value={form.due_date} onChange={(e) => setForm({ ...form, due_date: e.target.value })} className="mt-1.5" />
            </div>
          </div>
          {!task && (
            <div>
              <Label>Repeat</Label>
              <Combobox value={form.repeat} onValueChange={(v) => setForm({ ...form, repeat: v })}
                options={[
                  { value: "none", label: "Does not repeat" },
                  { value: "daily", label: "Daily" },
                  { value: "weekly", label: "Weekly" },
                  { value: "monthly", label: "Monthly" },
                ]}
                className="mt-1.5" data-testid="task-repeat-select" />
              {form.repeat !== "none" && (
                <p className="text-xs text-slate-500 mt-1.5">A new task will be created automatically each {form.repeat === "daily" ? "day" : form.repeat === "weekly" ? "week" : "month"}, based on the start date.</p>
              )}
            </div>
          )}
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button className="bg-indigo-600 hover:bg-indigo-700" onClick={save} data-testid="task-save-btn">{task ? "Save changes" : "Create task"}</Button>
          </div>
          {task && <TaskComments taskId={task.task_id} members={members || []} />}
        </div>
      </DialogContent>
    </Dialog>
  );
}
