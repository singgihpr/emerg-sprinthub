import React, { useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { useAuth } from "@/context/AuthContext";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { ArrowsClockwise, Timer, ListChecks, Clock, Warning, WarningCircle } from "@phosphor-icons/react";

function fmtMinutes(m) {
  if (!m) return "0m";
  const h = Math.floor(m / 60);
  const mm = m % 60;
  return h ? `${h}h ${mm}m` : `${mm}m`;
}

function fmtElapsed(sec) {
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const mm = String(m).padStart(2, "0");
  const ss = String(s).padStart(2, "0");
  return h ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

const PRIO_STYLES = {
  urgent: "bg-red-100 text-red-700",
  high: "bg-orange-100 text-orange-700",
  medium: "bg-slate-100 text-slate-600",
  low: "bg-slate-50 text-slate-500",
};

export default function TeamActivity() {
  const { currentOrg } = useOrg();
  const { user } = useAuth();
  const [data, setData] = useState(null);
  const [tick, setTick] = useState(0);
  const [err, setErr] = useState("");

  const load = useCallback(async () => {
    if (!currentOrg) return;
    try {
      const { data } = await api.get(`/orgs/${currentOrg.org_id}/team-activity`);
      setData(data); setErr("");
    } catch (e) {
      setErr(e.response?.status === 403 ? "You need admin or manager role to view team activity." : "Failed to load");
    }
  }, [currentOrg]);

  useEffect(() => { load(); }, [load]);
  // Live refresh every 15s + tick every 1s for elapsed. Stop polling if we get an error (e.g. 403).
  useEffect(() => {
    if (err) return;
    const r = setInterval(load, 15000);
    const t = setInterval(() => setTick((v) => v + 1), 1000);
    return () => { clearInterval(r); clearInterval(t); };
  }, [load, err]);

  if (err) {
    return (
      <div className="p-8">
        <Card className="p-8 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)] flex items-center gap-4">
          <Warning size={28} className="text-amber-500" />
          <div>
            <div className="font-medium text-slate-900">Access restricted</div>
            <div className="text-sm text-slate-500 mt-1">{err}</div>
          </div>
        </Card>
      </div>
    );
  }

  if (!data) return <div className="p-8 text-slate-500">Loading team activity…</div>;

  const activeTimers = data.members.filter((m) => m.active_timer).length;
  const idleAlerts = data.members.filter((m) => m.active_timer && (m.active_timer.elapsed_seconds + tick) > 7200).length;
  const totalInProgress = data.members.reduce((s, m) => s + m.in_progress_tasks.length, 0);
  const totalLoggedToday = data.members.reduce((s, m) => s + m.logged_today_minutes, 0);

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Live</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Team Activity</h1>
          <p className="text-sm text-slate-500 mt-2">What everyone is working on, right now. Auto-refreshes every 15s.</p>
        </div>
        <Button variant="outline" onClick={load} className="gap-2" data-testid="refresh-activity-btn">
          <ArrowsClockwise size={14} /> Refresh
        </Button>
      </div>

      {/* KPI row */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className="p-5 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-orange-100 text-orange-700 flex items-center justify-center">
              <Timer size={20} weight="duotone" />
            </div>
            <div>
              <div className="text-2xl font-display font-semibold">{activeTimers}</div>
              <div className="text-xs text-slate-500">Active timers now</div>
            </div>
          </div>
        </Card>
        <Card className={`p-5 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)] ${idleAlerts > 0 ? "ring-2 ring-red-200" : ""}`} data-testid="idle-alerts-kpi">
          <div className="flex items-center gap-3">
            <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${idleAlerts > 0 ? "bg-red-100 text-red-700" : "bg-slate-100 text-slate-500"}`}>
              <WarningCircle size={20} weight="duotone" />
            </div>
            <div>
              <div className="text-2xl font-display font-semibold">{idleAlerts}</div>
              <div className="text-xs text-slate-500">Idle {'>'} 2h</div>
            </div>
          </div>
        </Card>
        <Card className="p-5 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-blue-100 text-blue-700 flex items-center justify-center">
              <ListChecks size={20} weight="duotone" />
            </div>
            <div>
              <div className="text-2xl font-display font-semibold">{totalInProgress}</div>
              <div className="text-xs text-slate-500">Tasks in progress</div>
            </div>
          </div>
        </Card>
        <Card className="p-5 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-indigo-100 text-indigo-700 flex items-center justify-center">
              <Clock size={20} weight="duotone" />
            </div>
            <div>
              <div className="text-2xl font-display font-semibold">{fmtMinutes(totalLoggedToday)}</div>
              <div className="text-xs text-slate-500">Logged today (team)</div>
            </div>
          </div>
        </Card>
      </div>

      {/* Member rows */}
      <div className="space-y-3">
        {data.members.map((m) => {
          const initials = (m.name || m.email || "?").slice(0, 2).toUpperCase();
          const elapsed = m.active_timer ? m.active_timer.elapsed_seconds + tick : 0;
          const overloaded = m.estimate_hours > 40;
          const capacityPct = Math.min(100, (m.estimate_hours / 40) * 100);
          return (
            <Card key={m.user_id} className="p-5 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]" data-testid={`activity-row-${m.user_id}`}>
              <div className="flex flex-col lg:flex-row lg:items-center gap-5">
                {/* Identity */}
                <div className="flex items-center gap-3 lg:w-64">
                  <Avatar className="h-11 w-11">
                    <AvatarImage src={m.picture} />
                    <AvatarFallback className="bg-indigo-100 text-indigo-700 text-sm">{initials}</AvatarFallback>
                  </Avatar>
                  <div className="min-w-0">
                    <div className="font-medium text-slate-900 truncate">{m.name || m.email}</div>
                    <div className="flex items-center gap-2 mt-0.5">
                      <Badge variant="secondary" className="capitalize text-[10px]">{m.role}</Badge>
                      <span className="text-xs text-slate-500 truncate">{m.email}</span>
                    </div>
                  </div>
                </div>

                {/* Active timer */}
                <div className="flex-1 min-w-0">
                  {m.active_timer ? (
                    <div className={`rounded-lg border px-3 py-2.5 flex items-center gap-3 ${elapsed > 7200 ? "border-red-300 bg-red-50/60" : "border-orange-200 bg-orange-50/60"}`} data-testid={`active-timer-${m.user_id}`}>
                      <span className={`w-2 h-2 rounded-full timer-active ${elapsed > 7200 ? "bg-red-500" : "bg-orange-500"}`} />
                      <div className="min-w-0 flex-1">
                        <div className={`text-xs uppercase tracking-wider font-medium ${elapsed > 7200 ? "text-red-600" : "text-orange-600"}`}>
                          {elapsed > 7200 ? "Idle > 2h" : "Working on"}
                        </div>
                        <div className="text-sm font-medium text-slate-900 truncate">{m.active_timer.task_title}</div>
                      </div>
                      {m.active_timer.project_key && (
                        <span className={`text-[10px] font-mono px-1.5 py-0.5 rounded bg-white border ${elapsed > 7200 ? "border-red-200 text-red-700" : "border-orange-200 text-orange-700"}`}>{m.active_timer.project_key}</span>
                      )}
                      <div className={`font-mono text-lg tabular-nums ${elapsed > 7200 ? "text-red-700" : "text-orange-700"}`}>{fmtElapsed(elapsed)}</div>
                    </div>
                  ) : (
                    <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 flex items-center gap-3 text-slate-500 text-sm">
                      <Timer size={16} />
                      <span>No active timer</span>
                    </div>
                  )}
                </div>

                {/* Stats */}
                <div className="flex items-center gap-6 text-sm">
                  <div>
                    <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium">Active</div>
                    <div className="font-display font-semibold">{m.active_count}</div>
                  </div>
                  <div>
                    <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium">Today</div>
                    <div className="font-display font-semibold">{fmtMinutes(m.logged_today_minutes)}</div>
                  </div>
                  <div>
                    <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium">Week</div>
                    <div className="font-display font-semibold">{fmtMinutes(m.logged_week_minutes)}</div>
                  </div>
                </div>
              </div>

              {/* Details grid */}
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-5 pt-5 border-t border-slate-100">
                {/* In progress list */}
                <div className="md:col-span-1">
                  <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium mb-2">In progress</div>
                  {m.in_progress_tasks.length === 0 ? (
                    <div className="text-xs text-slate-400">Nothing in progress</div>
                  ) : (
                    <ul className="space-y-1.5">
                      {m.in_progress_tasks.map((t) => (
                        <li key={t.task_id} className="flex items-center gap-2 text-sm">
                          <Badge className={`${PRIO_STYLES[t.priority]} border-0 text-[10px] capitalize`}>{t.priority}</Badge>
                          <span className="text-slate-800 truncate flex-1">{t.title}</span>
                          {t.project_key && <span className="text-[10px] font-mono text-slate-400">{t.project_key}</span>}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>

                {/* Capacity */}
                <div>
                  <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium mb-2">Workload capacity</div>
                  <div className="flex items-center justify-between text-xs mb-1.5">
                    <span className="text-slate-500">{m.estimate_hours.toFixed(1)}h estimated / 40h</span>
                    <span className={`font-semibold ${overloaded ? "text-orange-600" : "text-slate-700"}`}>{capacityPct.toFixed(0)}%</span>
                  </div>
                  <div className="h-2 bg-slate-100 rounded-full overflow-hidden">
                    <div className={`h-full ${overloaded ? "bg-orange-500" : "bg-indigo-600"} transition-all`} style={{ width: `${capacityPct}%` }} />
                  </div>
                </div>

                {/* Recent activity */}
                <div>
                  <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium mb-2">Recent time</div>
                  {m.recent_activity.length === 0 ? (
                    <div className="text-xs text-slate-400">No recent entries</div>
                  ) : (
                    <ul className="space-y-1 text-xs">
                      {m.recent_activity.slice(0, 3).map((e, i) => (
                        <li key={i} className="flex items-center justify-between gap-2">
                          <span className="text-slate-700 truncate flex-1">{e.task_title}</span>
                          <span className="font-mono text-slate-500 shrink-0">{fmtMinutes(e.minutes)}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>
            </Card>
          );
        })}
        {data.members.length === 0 && (
          <div className="text-center py-12 text-slate-500">No members yet.</div>
        )}
      </div>
    </div>
  );
}
