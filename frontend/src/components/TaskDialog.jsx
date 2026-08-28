import React, { useState, useEffect } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";

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
    project_id: "", assignee_id: "", sprint_id: "", start_date: "", due_date: "", estimate_hours: 0,
  });

  useEffect(() => {
    if (task) {
      setForm({
        title: task.title || "", description: task.description || "",
        status: task.status || "todo", priority: task.priority || "medium",
        type: task.type || "task", project_id: task.project_id || "",
        assignee_id: task.assignee_id || "", sprint_id: task.sprint_id || "",
        start_date: task.start_date || "", due_date: task.due_date || "",
        estimate_hours: task.estimate_hours || 0,
      });
    } else {
      setForm({
        title: "", description: "", status: "todo", priority: "medium", type: "task",
        project_id: projects?.[0]?.project_id || "", assignee_id: "", sprint_id: "",
        start_date: "", due_date: "", estimate_hours: 0,
      });
    }
  }, [task, open, projects]);

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
        <div className="space-y-4">
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
              <Select value={form.project_id} onValueChange={(v) => setForm({ ...form, project_id: v })}>
                <SelectTrigger data-testid="task-project-select" className="mt-1.5"><SelectValue placeholder="Select project" /></SelectTrigger>
                <SelectContent>{projects?.map((p) => <SelectItem key={p.project_id} value={p.project_id}>{p.name}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div>
              <Label>Sprint</Label>
              <Select value={form.sprint_id || "none"} onValueChange={(v) => setForm({ ...form, sprint_id: v === "none" ? "" : v })}>
                <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">Backlog</SelectItem>
                  {sprints?.map((s) => <SelectItem key={s.sprint_id} value={s.sprint_id}>{s.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="grid grid-cols-3 gap-4">
            <div>
              <Label>Status</Label>
              <Select value={form.status} onValueChange={(v) => setForm({ ...form, status: v })}>
                <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
                <SelectContent>{STATUS.map((s) => <SelectItem key={s.v} value={s.v}>{s.l}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div>
              <Label>Priority</Label>
              <Select value={form.priority} onValueChange={(v) => setForm({ ...form, priority: v })}>
                <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
                <SelectContent>{PRIORITY.map((p) => <SelectItem key={p} value={p} className="capitalize">{p}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div>
              <Label>Type</Label>
              <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v })}>
                <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
                <SelectContent>{TYPE.map((t) => <SelectItem key={t} value={t} className="capitalize">{t}</SelectItem>)}</SelectContent>
              </Select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>Assignee</Label>
              <Select value={form.assignee_id || "none"} onValueChange={(v) => setForm({ ...form, assignee_id: v === "none" ? "" : v })}>
                <SelectTrigger className="mt-1.5"><SelectValue placeholder="Unassigned" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">Unassigned</SelectItem>
                  {members?.map((m) => <SelectItem key={m.user_id} value={m.user_id}>{m.name || m.email}</SelectItem>)}
                </SelectContent>
              </Select>
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
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button className="bg-indigo-600 hover:bg-indigo-700" onClick={save} data-testid="task-save-btn">{task ? "Save changes" : "Create task"}</Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
