import React, { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { FolderOpen, Plus } from "@phosphor-icons/react";

export default function Projects() {
  const { currentOrg } = useOrg();
  const [projects, setProjects] = useState([]);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: "", key: "", description: "", color: "#4F46E5" });

  const load = async () => {
    if (!currentOrg) return;
    const { data } = await api.get(`/orgs/${currentOrg.org_id}/projects`);
    setProjects(data);
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
        </div>
        <Button className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={() => setOpen(true)} data-testid="new-project-btn">
          <Plus size={16} weight="bold" /> New project
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {projects.map((p) => (
          <Card key={p.project_id} className="p-6 card-hover border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]" data-testid={`project-card-${p.project_id}`}>
            <div className="flex items-start justify-between">
              <div className="w-10 h-10 rounded-lg flex items-center justify-center" style={{ background: `${p.color}20`, color: p.color }}>
                <FolderOpen size={20} weight="duotone" />
              </div>
              <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-100 text-slate-600">{p.key}</span>
            </div>
            <h3 className="font-display font-semibold text-lg mt-4">{p.name}</h3>
            <p className="text-sm text-slate-500 mt-1 line-clamp-2">{p.description || "No description"}</p>
          </Card>
        ))}
        {projects.length === 0 && (
          <div className="col-span-full text-center py-12 text-slate-500">No projects yet. Create one to get started.</div>
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
    </div>
  );
}
