import React, { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { FolderOpen, Plus, PencilSimple } from "@phosphor-icons/react";
import ProjectDialog from "@/components/ProjectDialog";

export default function Projects() {
  const { currentOrg } = useOrg();
  const canManage = ["owner", "admin", "manager"].includes(currentOrg?.role);
  const [projects, setProjects] = useState([]);
  const [orgMembers, setOrgMembers] = useState([]);
  const [open, setOpen] = useState(false);
  const [editDialog, setEditDialog] = useState({ open: false, project: null });
  const [form, setForm] = useState({ name: "", key: "", description: "", color: "#4F46E5" });

  const load = async () => {
    if (!currentOrg) return;
    const [p, m] = await Promise.all([
      api.get(`/orgs/${currentOrg.org_id}/projects`),
      api.get(`/orgs/${currentOrg.org_id}/members`),
    ]);
    setProjects(p.data); setOrgMembers(m.data);
  };
  useEffect(() => { load(); }, [currentOrg]);

  const save = async () => {
    if (!form.name || !form.key) return;
    await api.post(`/orgs/${currentOrg.org_id}/projects`, form);
    setForm({ name: "", key: "", description: "", color: "#4F46E5" });
    setOpen(false); load();
  };

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Master Data</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Projects</h1>
          <p className="text-sm text-slate-500 mt-2">Manage projects and their members. Only owners/admins see every project.</p>
        </div>
        {canManage && (
          <Button className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={() => setOpen(true)} data-testid="new-project-btn">
            <Plus size={16} weight="bold" /> New project
          </Button>
        )}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {projects.map((p) => (
          <Card key={p.project_id} className="p-6 card-hover border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)] flex flex-col" data-testid={`project-card-${p.project_id}`}>
            <div className="flex items-start justify-between">
              <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ background: `${p.color}20`, color: p.color }}>
                <FolderOpen size={20} weight="duotone" />
              </div>
              <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-100 text-slate-600">{p.key}</span>
            </div>
            <h3 className="font-display font-semibold text-lg mt-4">{p.name}</h3>
            <p className="text-sm text-slate-500 mt-1 line-clamp-2 flex-1">{p.description || "No description"}</p>
            <div className="mt-4 pt-4 border-t border-slate-100 flex justify-end">
              {canManage ? (
                <Button variant="outline" size="sm" className="gap-2" onClick={() => setEditDialog({ open: true, project: p })} data-testid={`edit-project-${p.project_id}`}>
                  <PencilSimple size={14} /> Edit
                </Button>
              ) : (
                <span className="text-xs text-slate-400">View only</span>
              )}
            </div>
          </Card>
        ))}
        {projects.length === 0 && (
          <div className="col-span-full text-center py-12 text-slate-500">
            {canManage ? "No projects yet. Create one to get started." : "You don't belong to any project yet. Ask an admin to add you."}
          </div>
        )}
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Create project</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <div><Label>Name</Label><Input className="mt-1.5" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} data-testid="project-name-input" /></div>
            <div><Label>Key (short code, e.g., WEB)</Label><Input className="mt-1.5" value={form.key} onChange={(e) => setForm({ ...form, key: e.target.value.toUpperCase() })} maxLength={5} data-testid="project-key-input" /></div>
            <div><Label>Description</Label><Textarea className="mt-1.5" rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></div>
            <div><Label>Color</Label><Input type="color" className="mt-1.5 h-10 w-24" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} /></div>
            <Button className="w-full bg-indigo-600 hover:bg-indigo-700" onClick={save} data-testid="project-save-btn">Create</Button>
          </div>
        </DialogContent>
      </Dialog>

      <ProjectDialog
        open={editDialog.open}
        onOpenChange={(o) => setEditDialog({ open: o, project: o ? editDialog.project : null })}
        project={editDialog.project}
        orgMembers={orgMembers}
        onSaved={load}
      />
    </div>
  );
}
