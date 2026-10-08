import React, { useEffect, useState } from "react";
import { toast } from "sonner";
import { api, formatApiErrorDetail } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { UserPlus } from "@phosphor-icons/react";

const ROLES = ["owner", "admin", "manager", "member"];

export default function Members() {
  const { currentOrg } = useOrg();
  const [members, setMembers] = useState([]);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ email: "", name: "", role: "member" });

  const load = async () => {
    if (!currentOrg) return;
    const { data } = await api.get(`/orgs/${currentOrg.org_id}/members`);
    setMembers(data);
  };
  useEffect(() => { load(); }, [currentOrg]);

  const invite = async () => {
    if (!form.email || !form.name) return;
    try {
      await api.post(`/orgs/${currentOrg.org_id}/members`, form);
      toast.success(`Invite sent to ${form.email}`);
      setForm({ email: "", name: "", role: "member" });
      setOpen(false); load();
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail));
    }
  };

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-start justify-between">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">People</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Members</h1>
        </div>
        <Button className="bg-indigo-600 hover:bg-indigo-700 gap-2" onClick={() => setOpen(true)} data-testid="invite-member-btn">
          <UserPlus size={16} weight="bold" /> Invite member
        </Button>
      </div>

      <Card className="border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)] overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-slate-50 text-slate-500 text-xs uppercase tracking-wider">
            <tr>
              <th className="text-left px-6 py-3 font-medium">Member</th>
              <th className="text-left px-6 py-3 font-medium">Role</th>
              <th className="text-left px-6 py-3 font-medium">Joined</th>
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.user_id} className="border-t border-slate-100 hover:bg-slate-50/50" data-testid={`member-row-${m.user_id}`}>
                <td className="px-6 py-4">
                  <div className="flex items-center gap-3">
                    <Avatar className="h-9 w-9">
                      <AvatarImage src={m.picture} />
                      <AvatarFallback className="bg-indigo-100 text-indigo-700 text-xs">{(m.name || m.email).slice(0, 2).toUpperCase()}</AvatarFallback>
                    </Avatar>
                    <div>
                      <div className="font-medium text-slate-900">{m.name || "—"}</div>
                      <div className="text-xs text-slate-500">{m.email}</div>
                    </div>
                  </div>
                </td>
                <td className="px-6 py-4">
                  <Badge variant="secondary" className="capitalize">{m.role}</Badge>
                  {m.invited && <Badge variant="outline" className="ml-2 text-amber-600 border-amber-300 bg-amber-50">Invited</Badge>}
                </td>
                <td className="px-6 py-4 text-slate-500 text-xs">{m.created_at?.slice(0, 10) || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Invite member</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <div><Label>Full name</Label><Input className="mt-1.5" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} data-testid="invite-name-input" /></div>
            <div><Label>Email</Label><Input className="mt-1.5" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} data-testid="invite-email-input" /></div>
            <div><Label>Role</Label>
              <Combobox value={form.role} onValueChange={(v) => setForm({ ...form, role: v })}
                options={ROLES.map((r) => ({ value: r, label: r[0].toUpperCase() + r.slice(1) }))}
                className="mt-1.5" />
            </div>
            <p className="text-xs text-slate-500">We'll email them an invite link to set their password. Registered users are added instantly.</p>
            <Button className="w-full bg-indigo-600 hover:bg-indigo-700" onClick={invite} data-testid="invite-submit-btn">Add member</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
