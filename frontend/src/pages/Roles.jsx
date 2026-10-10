import React, { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, formatApiErrorDetail } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Trash2, Plus, Edit2 } from "@phosphor-icons/react";

const PERMISSION_GROUPS = {
  "Organization": ["manage_org", "manage_billing", "invite_members", "manage_roles", "view_analytics"],
  "Projects": ["create_project", "edit_project", "delete_project", "manage_project_members"],
  "Tasks": ["create_task", "edit_task", "delete_task", "assign_task", "edit_comments"],
  "Time Tracking": ["create_time_entries", "view_time_entries"],
};

const PERMISSION_LABELS = {
  manage_org: "Manage organization settings",
  manage_billing: "Manage billing and subscriptions",
  invite_members: "Invite new members",
  manage_roles: "Manage roles and permissions",
  view_analytics: "View analytics and reports",
  create_project: "Create projects",
  edit_project: "Edit projects",
  delete_project: "Delete projects",
  manage_project_members: "Manage project members",
  create_task: "Create tasks",
  edit_task: "Edit tasks",
  delete_task: "Delete tasks",
  assign_task: "Assign tasks",
  edit_comments: "Edit comments",
  create_time_entries: "Create time entries",
  view_time_entries: "View time entries",
};

export default function Roles() {
  const { currentOrg } = useOrg();
  const [roles, setRoles] = useState([]);
  const [permissions, setPermissions] = useState([]);
  const [open, setOpen] = useState(false);
  const [editRole, setEditRole] = useState(null);
  const [form, setForm] = useState({ name: "", description: "", permissions: [] });

  const load = async () => {
    if (!currentOrg) return;
    try {
      const { data: rolesData } = await api.get(`/orgs/${currentOrg.org_id}/roles`);
      setRoles(rolesData);
      const { data: permData } = await api.get(`/orgs/${currentOrg.org_id}/permissions`);
      setPermissions(permData.permissions);
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail));
    }
  };

  useEffect(() => { load(); }, [currentOrg]);

  const openCreate = () => {
    setEditRole(null);
    setForm({ name: "", description: "", permissions: [] });
    setOpen(true);
  };

  const openEdit = (role) => {
    setEditRole(role);
    setForm({
      name: role.name,
      description: role.description || "",
      permissions: Array.isArray(role.permissions) ? role.permissions : [],
    });
    setOpen(true);
  };

  const save = async () => {
    if (!form.name) {
      toast.error("Role name is required");
      return;
    }
    try {
      if (editRole) {
        await api.patch(`/orgs/${currentOrg.org_id}/roles/${editRole.role_id}`, form);
        toast.success("Role updated");
      } else {
        await api.post(`/orgs/${currentOrg.org_id}/roles`, form);
        toast.success("Role created");
      }
      setOpen(false);
      load();
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail));
    }
  };

  const remove = async (role) => {
    if (role.is_system) {
      toast.error("Cannot delete system role");
      return;
    }
    if (!confirm(`Delete role "${role.name}"?`)) return;
    try {
      await api.delete(`/orgs/${currentOrg.org_id}/roles/${role.role_id}`);
      toast.success("Role deleted");
      load();
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail));
    }
  };

  const togglePermission = (perm) => {
    setForm(prev => ({
      ...prev,
      permissions: prev.permissions.includes(perm)
        ? prev.permissions.filter(p => p !== perm)
        : [...prev.permissions, perm],
    }));
  };

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Settings</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Roles & Permissions</h1>
        </div>
        <Button className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={openCreate}>
          <Plus size={16} weight="bold" /> Create role
        </Button>
      </div>

      <div className="grid gap-4">
        {roles.map((role) => (
          <Card key={role.role_id} className="border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)] p-6">
            <div className="flex items-start justify-between">
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <h3 className="font-semibold text-slate-900">{role.name}</h3>
                  {role.is_system && <Badge variant="outline" className="text-xs">System</Badge>}
                  {role.is_default && <Badge variant="secondary" className="text-xs">Default</Badge>}
                </div>
                {role.description && <p className="text-sm text-slate-600 mt-1">{role.description}</p>}
                <div className="flex flex-wrap gap-1 mt-3">
                  {Array.isArray(role.permissions) && role.permissions.slice(0, 5).map((perm) => (
                    <Badge key={perm} variant="secondary" className="text-xs">
                      {PERMISSION_LABELS[perm] || perm}
                    </Badge>
                  ))}
                  {Array.isArray(role.permissions) && role.permissions.length > 5 && (
                    <Badge variant="outline" className="text-xs">+{role.permissions.length - 5} more</Badge>
                  )}
                </div>
              </div>
              {!role.is_system && (
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" onClick={() => openEdit(role)}>
                    <Edit2 size={14} />
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => remove(role)} className="text-red-600 hover:text-red-700">
                    <Trash2 size={14} />
                  </Button>
                </div>
              )}
            </div>
          </Card>
        ))}
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{editRole ? "Edit role" : "Create role"}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div>
              <Label>Role name</Label>
              <Input className="mt-1.5" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </div>
            <div>
              <Label>Description</Label>
              <Input className="mt-1.5" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </div>
            <div>
              <Label className="mb-2 block">Permissions</Label>
              <div className="space-y-4">
                {Object.entries(PERMISSION_GROUPS).map(([group, perms]) => (
                  <div key={group}>
                    <div className="font-medium text-sm text-slate-700 mb-2">{group}</div>
                    <div className="space-y-2">
                      {perms.map((perm) => (
                        <div key={perm} className="flex items-center gap-2">
                          <Checkbox
                            id={perm}
                            checked={form.permissions.includes(perm)}
                            onCheckedChange={() => togglePermission(perm)}
                          />
                          <label htmlFor={perm} className="text-sm text-slate-600 cursor-pointer">
                            {PERMISSION_LABELS[perm] || perm}
                          </label>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
            <Button className="w-full bg-indigo-600 hover:bg-indigo-700" onClick={save}>
              {editRole ? "Update role" : "Create role"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
