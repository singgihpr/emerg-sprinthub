import React, { useEffect, useState } from "react";
import { api, formatApiErrorDetail } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toast } from "sonner";

export default function OrgSettingsDialog({ open, onOpenChange }) {
  const { currentOrg, switchOrg, reload } = useOrg();
  const [name, setName] = useState("");
  const [logo, setLogo] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (currentOrg) { setName(currentOrg.name || ""); setLogo(currentOrg.logo || ""); }
  }, [currentOrg, open]);

  const canEdit = currentOrg && ["owner", "admin"].includes(currentOrg.role);

  const save = async (e) => {
    e.preventDefault();
    if (!currentOrg) return;
    setBusy(true);
    try {
      const { data } = await api.patch(`/orgs/${currentOrg.org_id}`, { name, logo });
      switchOrg(data);
      await reload();
      toast.success("Workspace updated");
      onOpenChange(false);
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail) || "Failed to save");
    } finally { setBusy(false); }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Workspace settings</DialogTitle>
          <DialogDescription>Rename your workspace and set a logo URL.</DialogDescription>
        </DialogHeader>
        <form onSubmit={save} className="space-y-4">
          <div className="flex items-center gap-2">
            <span className="text-xs uppercase tracking-wider text-slate-500 font-medium">Your role</span>
            <Badge variant="secondary" className="capitalize">{currentOrg?.role || "—"}</Badge>
          </div>
          <div>
            <Label>Workspace name</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} disabled={!canEdit} required data-testid="org-name-input" className="mt-1.5" />
          </div>
          <div>
            <Label>Logo URL</Label>
            <Input value={logo} onChange={(e) => setLogo(e.target.value)} disabled={!canEdit} placeholder="https://…" data-testid="org-logo-input" className="mt-1.5" />
          </div>
          {!canEdit && <p className="text-xs text-amber-600">Only workspace owners and admins can edit these settings.</p>}
          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Close</Button>
            {canEdit && (
              <Button type="submit" disabled={busy} className="bg-indigo-600 hover:bg-indigo-700" data-testid="save-org-btn">
                {busy ? "Saving…" : "Save changes"}
              </Button>
            )}
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
