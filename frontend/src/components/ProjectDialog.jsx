import React, { useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Trash, UserPlus, Crown } from "@phosphor-icons/react";
import { toast } from "sonner";
import { formatApiErrorDetail } from "@/lib/api";

export default function ProjectDialog({ open, onOpenChange, project, orgMembers, onSaved }) {
  const { currentOrg } = useOrg();
  const [tab, setTab] = useState("details");
  const [form, setForm] = useState({ name: "", description: "", color: "#4F46E5" });
  const [projectMembers, setProjectMembers] = useState([]);
  const [membersLoaded, setMembersLoaded] = useState(false);
  const [newMemberId, setNewMemberId] = useState("");
  const [newMemberRole, setNewMemberRole] = useState("member");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (project) {
      setForm({ name: project.name || "", description: project.description || "", color: project.color || "#4F46E5" });
    }
    setTab("details");
    setMembersLoaded(false);
  }, [project, open]);

  const loadMembers = useCallback(async () => {
    if (!project || !currentOrg) return;
    try {
      const { data } = await api.get(`/orgs/${currentOrg.org_id}/projects/${project.project_id}/members`);
      setProjectMembers(data);
    } catch (e) {
      toast.error(formatApiErrorDetail(e.response?.data?.detail) || "Failed to load members");
    } finally {
      setMembersLoaded(true);
    }
  }, [project, currentOrg]);

  useEffect(() => { if (open && tab === "members" && !membersLoaded) loadMembers(); }, [open, tab, membersLoaded, loadMembers]);

  const save = async () => {
    if (!project) return;
    setBusy(true);
    try {
      await api.patch(`/orgs/${currentOrg.org_id}/projects/${project.project_id}`, form);
      toast.success("Project updated");
      onSaved?.();
      onOpenChange(false);
    } catch (e) {
      toast.error(formatApiErrorDetail(e.response?.data?.detail) || "Failed to save");
    } finally { setBusy(false); }
  };

  const addMember = async () => {
    if (!newMemberId) return;
    setBusy(true);
    try {
      await api.post(`/orgs/${currentOrg.org_id}/projects/${project.project_id}/members`, { user_id: newMemberId, role: newMemberRole });
      toast.success("Member added");
      setNewMemberId(""); setNewMemberRole("member");
      await loadMembers();
      onSaved?.();
    } catch (e) {
      toast.error(formatApiErrorDetail(e.response?.data?.detail) || "Failed to add member");
    } finally { setBusy(false); }
  };

  const removeMember = async (userId) => {
    if (!window.confirm("Remove this member from the project?")) return;
    try {
      await api.delete(`/orgs/${currentOrg.org_id}/projects/${project.project_id}/members/${userId}`);
      toast.success("Member removed");
      await loadMembers();
      onSaved?.();
    } catch (e) {
      toast.error(formatApiErrorDetail(e.response?.data?.detail) || "Failed to remove member");
    }
  };

  const availableMembers = orgMembers.filter((om) => !projectMembers.some((pm) => pm.user_id === om.user_id));

  if (!project) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-3">
            <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-100 text-slate-600">{project.key}</span>
            <span>Edit project</span>
          </DialogTitle>
          <DialogDescription>Update project details and manage who can see its tasks.</DialogDescription>
        </DialogHeader>
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="grid grid-cols-2">
            <TabsTrigger value="details" data-testid="project-tab-details">Details</TabsTrigger>
            <TabsTrigger value="members" data-testid="project-tab-members">
              Members{membersLoaded ? ` (${projectMembers.length})` : ""}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="details" className="space-y-3 mt-4">
            <div>
              <Label>Name</Label>
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} data-testid="project-edit-name" className="mt-1.5" />
            </div>
            <div>
              <Label>Description</Label>
              <Textarea rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="mt-1.5" />
            </div>
            <div>
              <Label>Color</Label>
              <Input type="color" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} className="mt-1.5 h-10 w-24" />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
              <Button disabled={busy} className="bg-indigo-600 hover:bg-indigo-700" onClick={save} data-testid="project-save-changes-btn">Save changes</Button>
            </div>
          </TabsContent>

          <TabsContent value="members" className="mt-4 space-y-4">
            <div className="rounded-lg border border-slate-200 divide-y divide-slate-100">
              {!membersLoaded ? (
                <div className="p-6 text-center text-sm text-slate-500">Loading…</div>
              ) : projectMembers.length === 0 ? (
                <div className="p-6 text-center text-sm text-slate-500">No members yet.</div>
              ) : projectMembers.map((m) => (
                <div key={m.user_id} className="p-3 flex items-center gap-3" data-testid={`project-member-${m.user_id}`}>
                  <Avatar className="h-9 w-9">
                    <AvatarImage src={m.picture} />
                    <AvatarFallback className="bg-indigo-100 text-indigo-700 text-xs">{(m.name || m.email || "?").slice(0, 2).toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-slate-900 truncate">{m.name || m.email}</div>
                    <div className="text-xs text-slate-500 truncate">{m.email}</div>
                  </div>
                  <Badge variant="secondary" className="capitalize flex items-center gap-1">
                    {m.project_role === "lead" && <Crown size={12} weight="fill" className="text-amber-500" />}
                    {m.project_role}
                  </Badge>
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0 text-slate-400 hover:bg-red-50 hover:text-red-600" onClick={() => removeMember(m.user_id)} data-testid={`remove-project-member-${m.user_id}`}>
                    <Trash size={14} />
                  </Button>
                </div>
              ))}
            </div>

            <div className="border border-dashed border-slate-300 rounded-lg p-4 space-y-3">
              <div className="text-xs uppercase tracking-wider text-slate-500 font-medium">Add a member</div>
              <div className="flex flex-wrap gap-2">
                <Select value={newMemberId} onValueChange={setNewMemberId}>
                  <SelectTrigger className="flex-1 min-w-[220px]" data-testid="add-project-member-select">
                    <SelectValue placeholder="Choose an organization member" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableMembers.length === 0 ? (
                  <div className="p-2 text-sm text-slate-500">All org members are already in this project.</div>
                ) : availableMembers.map((m) => (
                  <SelectItem key={m.user_id} value={m.user_id}>
                    <div className="flex flex-col">
                      <span>{m.name || m.email}</span>
                      {m.name && m.email && m.name !== m.email && <span className="text-xs text-slate-500">{m.email}</span>}
                    </div>
                  </SelectItem>
                ))}
                  </SelectContent>
                </Select>
                <Select value={newMemberRole} onValueChange={setNewMemberRole}>
                  <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="member">Member</SelectItem>
                    <SelectItem value="lead">Lead</SelectItem>
                  </SelectContent>
                </Select>
                <Button disabled={!newMemberId || busy} className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={addMember} data-testid="add-project-member-btn">
                  <UserPlus size={14} /> Add
                </Button>
              </div>
              <p className="text-xs text-slate-500">Only project members and org admins/owners can see this project's tasks.</p>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
