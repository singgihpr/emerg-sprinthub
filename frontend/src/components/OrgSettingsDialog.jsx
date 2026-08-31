import React, { useEffect, useRef, useState } from "react";
import { api, formatApiErrorDetail } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { toast } from "sonner";
import { UploadSimple, Trash } from "@phosphor-icons/react";

export default function OrgSettingsDialog({ open, onOpenChange }) {
  const { currentOrg, switchOrg, reload } = useOrg();
  const [name, setName] = useState("");
  const [logo, setLogo] = useState("");
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef(null);

  useEffect(() => {
    if (currentOrg) { setName(currentOrg.name || ""); setLogo(currentOrg.logo || ""); }
  }, [currentOrg, open]);

  const canEdit = currentOrg && ["owner", "admin"].includes(currentOrg.role);

  const save = async (e) => {
    e.preventDefault();
    if (!currentOrg) return;
    setBusy(true);
    try {
      const { data } = await api.patch(`/orgs/${currentOrg.org_id}`, { name });
      switchOrg(data);
      await reload();
      toast.success("Workspace updated");
      onOpenChange(false);
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail) || "Failed to save");
    } finally { setBusy(false); }
  };

  const uploadLogo = async (file) => {
    if (!file || !currentOrg) return;
    if (!file.type.startsWith("image/")) {
      toast.error("Please choose an image file");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      toast.error("Image must be under 5 MB");
      return;
    }
    setUploading(true);
    try {
      const fd = new FormData();
      fd.append("file", file);
      const { data } = await api.post(`/orgs/${currentOrg.org_id}/logo`, fd, {
        headers: { "Content-Type": "multipart/form-data" },
      });
      setLogo(data.logo);
      switchOrg(data);
      await reload();
      toast.success("Logo updated");
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail) || "Failed to upload");
    } finally { setUploading(false); }
  };

  const clearLogo = async () => {
    if (!currentOrg) return;
    setUploading(true);
    try {
      const { data } = await api.patch(`/orgs/${currentOrg.org_id}`, { logo: "" });
      setLogo("");
      switchOrg(data);
      await reload();
      toast.success("Logo removed");
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail) || "Failed to remove");
    } finally { setUploading(false); }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Workspace settings</DialogTitle>
          <DialogDescription>Rename your workspace and upload a logo (resized to 256×256).</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="flex items-center gap-2">
            <span className="text-xs uppercase tracking-wider text-slate-500 font-medium">Your role</span>
            <Badge variant="secondary" className="capitalize">{currentOrg?.role || "—"}</Badge>
          </div>

          <div>
            <Label>Logo</Label>
            <div className="mt-2 flex items-center gap-4">
              <div className="w-20 h-20 rounded-lg border border-slate-200 bg-slate-50 flex items-center justify-center overflow-hidden">
                {logo ? (
                  <img src={logo} alt="Logo preview" className="w-full h-full object-cover" data-testid="org-logo-preview" />
                ) : (
                  <span className="text-2xl font-display font-semibold text-slate-400">
                    {(name || "?").slice(0, 1).toUpperCase()}
                  </span>
                )}
              </div>
              <div className="flex flex-col gap-2">
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/*"
                  onChange={(e) => { const f = e.target.files?.[0]; if (f) uploadLogo(f); e.target.value = ""; }}
                  className="hidden"
                  data-testid="org-logo-file-input"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="gap-2"
                  disabled={!canEdit || uploading}
                  onClick={() => fileRef.current?.click()}
                  data-testid="org-logo-upload-btn"
                >
                  <UploadSimple size={14} /> {uploading ? "Uploading…" : logo ? "Replace" : "Upload logo"}
                </Button>
                {logo && canEdit && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="gap-2 text-red-600 hover:text-red-700 hover:bg-red-50"
                    disabled={uploading}
                    onClick={clearLogo}
                    data-testid="org-logo-remove-btn"
                  >
                    <Trash size={14} /> Remove
                  </Button>
                )}
                <p className="text-xs text-slate-400">PNG, JPG or WebP. Square recommended. Max 5 MB.</p>
              </div>
            </div>
          </div>

          <form onSubmit={save} className="space-y-4">
            <div>
              <Label>Workspace name</Label>
              <Input value={name} onChange={(e) => setName(e.target.value)} disabled={!canEdit} required data-testid="org-name-input" className="mt-1.5" />
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
        </div>
      </DialogContent>
    </Dialog>
  );
}
