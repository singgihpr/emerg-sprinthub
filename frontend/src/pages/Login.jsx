import React, { useState } from "react";
import { useAuth } from "@/context/AuthContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { formatApiErrorDetail } from "@/lib/api";
import { GoogleLogo, Kanban, ChartLine, Timer } from "@phosphor-icons/react";
import { Navigate } from "react-router-dom";

export default function Login() {
  const { user, login, register } = useAuth();
  const [mode, setMode] = useState("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (user && user.user_id) return <Navigate to="/dashboard" replace />;

  const submit = async (e) => {
    e.preventDefault();
    setError(""); setBusy(true);
    try {
      if (mode === "login") await login(email, password);
      else await register(email, password, name);
    } catch (err) {
      setError(formatApiErrorDetail(err.response?.data?.detail) || err.message);
    } finally {
      setBusy(false);
    }
  };

  const googleLogin = () => {
    // REMINDER: DO NOT HARDCODE THE URL, OR ADD ANY FALLBACKS OR REDIRECT URLS, THIS BREAKS THE AUTH
    const redirectUrl = window.location.origin + "/dashboard";
    window.location.href = `https://auth.emergentagent.com/?redirect=${encodeURIComponent(redirectUrl)}`;
  };

  return (
    <div className="min-h-screen grid lg:grid-cols-2 bg-slate-50">
      {/* Left brand panel */}
      <div className="hidden lg:flex flex-col justify-between p-12 bg-gradient-to-br from-indigo-600 via-indigo-700 to-slate-900 text-white relative overflow-hidden">
        <div className="absolute inset-0 opacity-10" style={{ backgroundImage: "radial-gradient(circle at 20% 20%, white 1px, transparent 1px), radial-gradient(circle at 80% 60%, white 1px, transparent 1px)", backgroundSize: "48px 48px" }} />
        <div className="relative">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-white/10 backdrop-blur border border-white/20 flex items-center justify-center">
              <Kanban size={22} weight="duotone" />
            </div>
            <span className="font-display text-xl font-semibold tracking-tight">SprintHub</span>
          </div>
        </div>
        <div className="relative space-y-8">
          <div>
            <h1 className="font-display text-5xl font-semibold tracking-tight leading-tight">
              Ship sprints.<br />Track time.<br />See progress.
            </h1>
            <p className="mt-6 text-lg text-indigo-100/90 max-w-md leading-relaxed">
              A calm, opinionated workspace for sprint backlogs, routine tasks, and time-honest productivity.
            </p>
          </div>
          <div className="grid grid-cols-3 gap-6">
            {[
              { icon: Kanban, label: "5 Views" },
              { icon: Timer, label: "Time Tracking" },
              { icon: ChartLine, label: "Real Insights" },
            ].map((f, i) => (
              <div key={i} className="p-4 rounded-lg bg-white/5 border border-white/10 backdrop-blur-sm">
                <f.icon size={22} weight="duotone" className="text-indigo-200 mb-3" />
                <div className="text-sm font-medium">{f.label}</div>
              </div>
            ))}
          </div>
        </div>
        <div className="relative text-xs text-indigo-200/70">
          © 2026 SprintHub — Focused work, delivered.
        </div>
      </div>

      {/* Right auth panel */}
      <div className="flex items-center justify-center p-6 sm:p-12">
        <Card className="w-full max-w-md p-8 shadow-[0_8px_30px_rgb(0,0,0,0.04)] border-slate-200">
          <div className="mb-8">
            <div className="lg:hidden flex items-center gap-2 mb-6">
              <Kanban size={22} weight="duotone" className="text-indigo-600" />
              <span className="font-display font-semibold">SprintHub</span>
            </div>
            <h2 className="font-display text-3xl font-semibold tracking-tight">Welcome back</h2>
            <p className="text-sm text-slate-500 mt-2">Sign in to your workspace or create a new one.</p>
          </div>

          <Button
            type="button"
            variant="outline"
            className="w-full h-11 gap-3 border-slate-300 hover:bg-slate-50"
            onClick={googleLogin}
            data-testid="google-login-btn"
          >
            <GoogleLogo size={20} weight="bold" />
            <span>Continue with Google</span>
          </Button>

          <div className="flex items-center gap-3 my-6">
            <div className="flex-1 h-px bg-slate-200" />
            <span className="text-xs uppercase tracking-widest text-slate-400">or</span>
            <div className="flex-1 h-px bg-slate-200" />
          </div>

          <Tabs value={mode} onValueChange={setMode}>
            <TabsList className="grid grid-cols-2 mb-6">
              <TabsTrigger value="login" data-testid="tab-login">Sign in</TabsTrigger>
              <TabsTrigger value="register" data-testid="tab-register">Create account</TabsTrigger>
            </TabsList>

            <form onSubmit={submit} className="space-y-4">
              {mode === "register" && (
                <div>
                  <Label htmlFor="name">Full name</Label>
                  <Input id="name" data-testid="input-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Jane Doe" required className="mt-1.5" />
                </div>
              )}
              <div>
                <Label htmlFor="email">Email</Label>
                <Input id="email" data-testid="input-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@company.com" required className="mt-1.5" />
              </div>
              <div>
                <Label htmlFor="password">Password</Label>
                <Input id="password" data-testid="input-password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" required minLength={6} className="mt-1.5" />
              </div>
              {error && <div data-testid="auth-error" className="text-sm text-red-600 bg-red-50 border border-red-200 rounded-md px-3 py-2">{error}</div>}
              <Button type="submit" disabled={busy} className="w-full h-11 bg-indigo-600 hover:bg-indigo-700" data-testid="submit-auth-btn">
                {busy ? "Working…" : mode === "login" ? "Sign in" : "Create account"}
              </Button>
            </form>
          </Tabs>
        </Card>
      </div>
    </div>
  );
}
