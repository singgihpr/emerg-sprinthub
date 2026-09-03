import React, { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Rocket, Plus, CalendarBlank, ChartLine, PencilSimple, FunnelSimple, Trash } from "@phosphor-icons/react";
import BurndownDialog from "@/components/BurndownDialog";
import ProjectDialog from "@/components/ProjectDialog";

const SPRINT_STATUSES = [
  { v: "planned", l: "Planned" },
  { v: "active", l: "Active" },
  { v: "completed", l: "Completed" },
];

export default function Sprints() {
  const { currentOrg } = useOrg();
  const canManage = ["owner", "admin", "manager"].includes(currentOrg?.role);
  const [sprints, setSprints] = useState([]);
  const [projects, setProjects] = useState([]);
  const [tasks, setTasks] = useState([]);
  const [orgMembers, setOrgMembers] = useState([]);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [burndown, setBurndown] = useState({ open: false, sprint: null });
  const [editProject, setEditProject] = useState({ open: false, project: null });
  const [form, setForm] = useState({ project_id: "", name: "", goal: "", start_date: "", end_date: "" });
  const [filterProject, setFilterProject] = useState("all");
  const [filterStatus, setFilterStatus] = useState("all");

  const load = async () => {
    if (!currentOrg) return;
    const [s, p, t, m] = await Promise.all([
      api.get(`/orgs/${currentOrg.org_id}/sprints`),
      api.get(`/orgs/${currentOrg.org_id}/projects`),
      api.get(`/orgs/${currentOrg.org_id}/tasks`),
      api.get(`/orgs/${currentOrg.org_id}/members`),
    ]);
    setSprints(s.data); setProjects(p.data); setTasks(t.data); setOrgMembers(m.data);
  };
  useEffect(() => { load(); }, [currentOrg]);

  const openCreate = () => {
    setEditing(null);
    setForm({ project_id: "", name: "", goal: "", start_date: "", end_date: "" });
    setOpen(true);
  };

  const openEdit = (s) => {
    setEditing(s);
    setForm({ project_id: s.project_id, name: s.name, goal: s.goal || "", start_date: s.start_date || "", end_date: s.end_date || "" });
    setOpen(true);
  };

  const save = async () => {
    if (!form.name || !form.project_id) return;
    if (editing) {
      await api.patch(`/orgs/${currentOrg.org_id}/sprints/${editing.sprint_id}`, form);
    } else {
      await api.post(`/orgs/${currentOrg.org_id}/sprints`, form);
    }
    setForm({ project_id: "", name: "", goal: "", start_date: "", end_date: "" });
    setEditing(null); setOpen(false); load();
  };

  const del = async (s) => {
    if (!window.confirm(`Delete sprint "${s.name}"? Its tasks will move to Backlog.`)) return;
    await api.delete(`/orgs/${currentOrg.org_id}/sprints/${s.sprint_id}`);
    load();
  };

  const changeStatus = async (sid, status) => {
    await api.patch(`/orgs/${currentOrg.org_id}/sprints/${sid}`, { status });
    load();
  };

  const sprintStats = (sid) => {
    const list = tasks.filter((t) => t.sprint_id === sid);
    const done = list.filter((t) => t.status === "done").length;
    return { total: list.length, done, progress: list.length ? (done / list.length) * 100 : 0 };
  };

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Delivery</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Sprints</h1>
        </div>
        {canManage && (
          <Button className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={openCreate} data-testid="new-sprint-btn">
            <Plus size={16} weight="bold" /> New sprint
          </Button>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <FunnelSimple size={16} className="text-slate-400" />
        <Combobox value={filterProject} onValueChange={setFilterProject} placeholder="Project"
          options={[{ value: "all", label: "All projects" }, ...projects.map((p) => ({ value: p.project_id, label: p.name }))]}
          className="w-56 bg-white" data-testid="sprint-filter-project" />
        <Combobox value={filterStatus} onValueChange={setFilterStatus} placeholder="Status"
          options={[{ value: "all", label: "All statuses" }, ...SPRINT_STATUSES.map((s) => ({ value: s.v, label: s.l }))]}
          className="w-44 bg-white" data-testid="sprint-filter-status" />
        {(filterProject !== "all" || filterStatus !== "all") && (
          <Button variant="ghost" size="sm" onClick={() => { setFilterProject("all"); setFilterStatus("all"); }} data-testid="sprint-filter-clear">
            Clear
          </Button>
        )}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {sprints
          .filter((s) => (filterProject === "all" || s.project_id === filterProject) && (filterStatus === "all" || s.status === filterStatus))
          .map((s) => {
          const st = sprintStats(s.sprint_id);
          const project = projects.find((p) => p.project_id === s.project_id);
          return (
            <Card key={s.sprint_id} className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)] card-hover" data-testid={`sprint-card-${s.sprint_id}`}>
              <div className="flex items-start justify-between gap-3">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-lg bg-indigo-100 text-indigo-700 flex items-center justify-center">
                    <Rocket size={20} weight="duotone" />
                  </div>
                  <div>
                    <h3 className="font-display font-semibold text-lg">{s.name}</h3>
                    {project ? (
                      <button
                        className="text-xs text-slate-500 hover:text-indigo-600 hover:underline flex items-center gap-1"
                        onClick={() => setEditProject({ open: true, project })}
                        data-testid={`sprint-project-link-${s.sprint_id}`}
                      >
                        <PencilSimple size={11} /> {project.name}
                      </button>
                    ) : (
                      <div className="text-xs text-slate-400">Restricted project</div>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <Badge variant="secondary" className="capitalize">{s.status}</Badge>
                  {canManage && (
                    <>
                      <Button variant="ghost" size="sm" className="h-7 w-7 p-0" onClick={() => openEdit(s)} data-testid={`edit-sprint-${s.sprint_id}`}>
                        <PencilSimple size={13} />
                      </Button>
                      <Button variant="ghost" size="sm" className="h-7 w-7 p-0 hover:bg-red-50 hover:text-red-600" onClick={() => del(s)} data-testid={`delete-sprint-${s.sprint_id}`}>
                        <Trash size={13} />
                      </Button>
                    </>
                  )}
                </div>
              </div>
              {canManage && (
                <div className="mt-3">
                  <Combobox value={s.status} onValueChange={(v) => changeStatus(s.sprint_id, v)} placeholder="Status"
                    options={SPRINT_STATUSES.map((opt) => ({ value: opt.v, label: opt.l }))}
                    className="w-40 h-8 text-xs bg-white" data-testid={`sprint-status-select-${s.sprint_id}`} />
                </div>
              )}
              {s.goal && <p className="text-sm text-slate-600 mt-4">{s.goal}</p>}
              <div className="flex items-center gap-3 mt-4 text-xs text-slate-500">
                <CalendarBlank size={14} />
                <span>{s.start_date || "—"} → {s.end_date || "—"}</span>
              </div>
              <div className="mt-4">
                <div className="flex justify-between text-xs mb-1.5">
                  <span className="text-slate-500">{st.done} / {st.total} tasks</span>
                  <span className="font-medium text-slate-700">{st.progress.toFixed(0)}%</span>
                </div>
                <div className="h-2 bg-slate-100 rounded-full overflow-hidden">
                  <div className="h-full bg-indigo-600 transition-all" style={{ width: `${st.progress}%` }} />
                </div>
              </div>
              <div className="mt-4 pt-4 border-t border-slate-100 flex justify-end">
                <Button variant="outline" size="sm" className="gap-2" onClick={() => setBurndown({ open: true, sprint: s })} data-testid={`burndown-btn-${s.sprint_id}`}>
                  <ChartLine size={14} /> View burndown
                </Button>
              </div>
            </Card>
          );
        })}
        {sprints.filter((s) => (filterProject === "all" || s.project_id === filterProject) && (filterStatus === "all" || s.status === filterStatus)).length === 0 && (
          <div className="col-span-full text-center py-12 text-slate-500">
            {sprints.length === 0 ? "No sprints yet." : "No sprints match these filters."}
          </div>
        )}
      </div>

      <Dialog open={open} onOpenChange={(o) => { setOpen(o); if (!o) setEditing(null); }}>
        <DialogContent>
          <DialogHeader><DialogTitle>{editing ? "Edit sprint" : "Create sprint"}</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <div><Label>Project</Label>
              <Combobox value={form.project_id} onValueChange={(v) => setForm({ ...form, project_id: v })}
                placeholder="Select project" disabled={!!editing}
                options={projects.map((p) => ({ value: p.project_id, label: p.name }))}
                className="mt-1.5" data-testid="sprint-project-select" />
            </div>
            <div><Label>Name</Label><Input className="mt-1.5" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} data-testid="sprint-name-input" /></div>
            <div><Label>Goal</Label><Textarea className="mt-1.5" rows={2} value={form.goal} onChange={(e) => setForm({ ...form, goal: e.target.value })} /></div>
            <div className="grid grid-cols-2 gap-3">
              <div><Label>Start</Label><Input type="date" className="mt-1.5" value={form.start_date} onChange={(e) => setForm({ ...form, start_date: e.target.value })} /></div>
              <div><Label>End</Label><Input type="date" className="mt-1.5" value={form.end_date} onChange={(e) => setForm({ ...form, end_date: e.target.value })} /></div>
            </div>
            <Button className="w-full bg-indigo-600 hover:bg-indigo-700" onClick={save} data-testid="sprint-save-btn">{editing ? "Save changes" : "Create sprint"}</Button>
          </div>
        </DialogContent>
      </Dialog>
      <BurndownDialog open={burndown.open} onOpenChange={(o) => setBurndown({ open: o, sprint: o ? burndown.sprint : null })} sprint={burndown.sprint} />
      <ProjectDialog
        open={editProject.open}
        onOpenChange={(o) => setEditProject({ open: o, project: o ? editProject.project : null })}
        project={editProject.project}
        orgMembers={orgMembers}
        onSaved={load}
      />
    </div>
  );
}
