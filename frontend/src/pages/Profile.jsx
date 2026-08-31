import React, { useState } from "react";
import { api, formatApiErrorDetail } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { toast } from "sonner";
import { User, Key } from "@phosphor-icons/react";

export default function Profile() {
  const { user, setUser } = useAuth();
  const [name, setName] = useState(user?.name || "");
  const [picture, setPicture] = useState(user?.picture || "");
  const [savingProfile, setSavingProfile] = useState(false);
  const [pw, setPw] = useState({ current: "", next: "", confirm: "" });
  const [savingPw, setSavingPw] = useState(false);
  const initials = (user?.name || user?.email || "?").slice(0, 2).toUpperCase();

  const saveProfile = async (e) => {
    e.preventDefault();
    setSavingProfile(true);
    try {
      const { data } = await api.patch("/auth/me", { name, picture });
      setUser({ ...user, ...data });
      toast.success("Profile updated");
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail) || "Failed to save");
    } finally { setSavingProfile(false); }
  };

  const changePassword = async (e) => {
    e.preventDefault();
    if (pw.next !== pw.confirm) {
      toast.error("New password does not match confirmation");
      return;
    }
    if (pw.next.length < 6) {
      toast.error("New password must be at least 6 characters");
      return;
    }
    setSavingPw(true);
    try {
      await api.post("/auth/change-password", { current_password: pw.current, new_password: pw.next });
      setPw({ current: "", next: "", confirm: "" });
      toast.success("Password changed");
    } catch (err) {
      toast.error(formatApiErrorDetail(err.response?.data?.detail) || "Failed to change password");
    } finally { setSavingPw(false); }
  };

  return (
    <div className="p-8 space-y-6 max-w-3xl">
      <div>
        <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Account</div>
        <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Profile</h1>
        <p className="text-sm text-slate-500 mt-2">Update your personal information and password.</p>
      </div>

      <Tabs defaultValue="details">
        <TabsList>
          <TabsTrigger value="details" className="gap-2" data-testid="profile-tab-details"><User size={14} weight="duotone" /> Details</TabsTrigger>
          <TabsTrigger value="password" className="gap-2" data-testid="profile-tab-password"><Key size={14} weight="duotone" /> Password</TabsTrigger>
        </TabsList>

        <TabsContent value="details">
          <Card className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
            <form onSubmit={saveProfile} className="space-y-5">
              <div className="flex items-center gap-4">
                <Avatar className="h-16 w-16">
                  <AvatarImage src={picture} />
                  <AvatarFallback className="bg-indigo-100 text-indigo-700 text-lg">{initials}</AvatarFallback>
                </Avatar>
                <div className="text-xs text-slate-500 leading-relaxed">
                  Paste an image URL below to change your avatar. Leave blank to use initials.
                </div>
              </div>
              <div>
                <Label>Full name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} data-testid="profile-name-input" className="mt-1.5" />
              </div>
              <div>
                <Label>Email</Label>
                <Input value={user?.email || ""} disabled className="mt-1.5 bg-slate-50" />
                <p className="text-xs text-slate-400 mt-1">Email address can't be changed.</p>
              </div>
              <div>
                <Label>Avatar URL</Label>
                <Input value={picture} onChange={(e) => setPicture(e.target.value)} placeholder="https://…" data-testid="profile-picture-input" className="mt-1.5" />
              </div>
              <div className="flex justify-end">
                <Button type="submit" disabled={savingProfile} className="bg-indigo-600 hover:bg-indigo-700" data-testid="save-profile-btn">
                  {savingProfile ? "Saving…" : "Save changes"}
                </Button>
              </div>
            </form>
          </Card>
        </TabsContent>

        <TabsContent value="password">
          <Card className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
            <form onSubmit={changePassword} className="space-y-5">
              <div>
                <Label>Current password</Label>
                <Input type="password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} data-testid="current-password-input" className="mt-1.5" placeholder="Leave blank if you signed in with Google only" />
              </div>
              <div>
                <Label>New password</Label>
                <Input type="password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} required minLength={6} data-testid="new-password-input" className="mt-1.5" />
                <p className="text-xs text-slate-400 mt-1">At least 6 characters.</p>
              </div>
              <div>
                <Label>Confirm new password</Label>
                <Input type="password" value={pw.confirm} onChange={(e) => setPw({ ...pw, confirm: e.target.value })} required minLength={6} data-testid="confirm-password-input" className="mt-1.5" />
              </div>
              <div className="flex justify-end">
                <Button type="submit" disabled={savingPw || !pw.next} className="bg-indigo-600 hover:bg-indigo-700" data-testid="change-password-btn">
                  {savingPw ? "Changing…" : "Change password"}
                </Button>
              </div>
            </form>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
