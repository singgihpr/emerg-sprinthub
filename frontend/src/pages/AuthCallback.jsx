import React, { useEffect, useRef } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { api } from "@/lib/api";
import { useAuth } from "@/context/AuthContext";

export default function AuthCallback() {
  const nav = useNavigate();
  const loc = useLocation();
  const { setUser, checkAuth } = useAuth();
  const done = useRef(false);

  useEffect(() => {
    if (done.current) return;
    done.current = true;
    const params = new URLSearchParams(loc.hash.replace(/^#/, ""));
    const session_id = params.get("session_id");
    if (!session_id) {
      nav("/login", { replace: true });
      return;
    }
    (async () => {
      try {
        const { data } = await api.post("/auth/google/session", { session_id });
        if (data.token) localStorage.setItem("access_token", data.token);
        setUser(data);
        window.history.replaceState({}, "", "/dashboard");
        await checkAuth();
        nav("/dashboard", { replace: true });
      } catch {
        nav("/login", { replace: true });
      }
    })();
  }, [loc.hash, nav, setUser, checkAuth]);

  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-50">
      <div className="text-slate-500">Signing you in…</div>
    </div>
  );
}
