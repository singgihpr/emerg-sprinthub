import React, { useEffect, useState } from "react";
import { useParams, useNavigate, Navigate } from "react-router-dom";
import { api, formatApiErrorDetail } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { Kanban } from "@phosphor-icons/react";
import { INVITE } from "@/constants/testIds";

export default function AcceptInvite() {
  const { token } = useParams();
  const navigate = useNavigate();
  const { user, setUser } = useAuth();
  const [info, setInfo] = useState(null);
  const [loadError, setLoadError] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.get(`/invites/${token}`)
      .then(({ data }) => setInfo(data))
      .catch((err) => setLoadError(formatApiErrorDetail(err.response?.data?.detail)));
  }, [token]);

  if (user && user.user_id) return <Navigate to="/dashboard" replace />;

  const submit = async (e) => {
    e.preventDefault();
    if (password !== confirm) { setError("Passwords do not match"); return; }
    setError(""); setBusy(true);
    try {
      const { data } = await api.post(`/invites/${token}/accept`, { password });
      if (data.token) localStorage.setItem("access_token", data.token);
      setUser(data);
      navigate("/dashboard", { replace: true });
    } catch (err) {
      setError(formatApiErrorDetail(err.response?.data?.detail));
      setBusy(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50 p-6">
      <Card className="w-full max-w-md p-8 shadow-[0_8px_30px_rgb(0,0,0,0.04)] border-slate-200">
        <div className="flex items-center gap-2 mb-6">
          <Kanban size={22} weight="duotone" className="text-indigo-600" />
          <span className="font-display font-semibold">SprintHub</span>
        </div>
        {!info && !loadError && <p className="text-sm text-slate-500">Loading invite…</p>}
        {loadError && (
          <div>
            <h2 className="font-display text-2xl font-semibold tracking-tight">Invite unavailable</h2>
            <p data-testid={INVITE.error} className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-md px-3 py-2 mt-4">{loadError}</p>
            <p className="text-sm text-slate-500 mt-4">Ask your admin to send a new invite.</p>
          </div>
        )}
        {info && (
          <form onSubmit={submit} className="space-y-4">
            <div>
              <h2 className="font-display text-2xl font-semibold tracking-tight">Join {info.org_name}</h2>
              <p className="text-sm text-slate-500 mt-2">
                Set a password for <span className="font-medium text-slate-700">{info.email}</span> to finish joining.
              </p>
            </div>
            <div>
              <Label htmlFor="invite-password">Password</Label>
              <Input id="invite-password" type="password" value={password} onChange={(e) => setPassword(e.target.value)}
                required minLength={6} className="mt-1.5" data-testid={INVITE.passwordInput} />
            </div>
            <div>
              <Label htmlFor="invite-confirm">Confirm password</Label>
              <Input id="invite-confirm" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)}
                required minLength={6} className="mt-1.5" data-testid={INVITE.passwordConfirmInput} />
            </div>
            {error && <div data-testid={INVITE.error} className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-md px-3 py-2">{error}</div>}
            <Button type="submit" disabled={busy} className="w-full h-11 bg-indigo-600 hover:bg-indigo-700" data-testid={INVITE.submitButton}>
              {busy ? "Working…" : "Accept & continue"}
            </Button>
          </form>
        )}
      </Card>
    </div>
  );
}
