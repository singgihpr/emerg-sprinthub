import React, { useEffect, useState, useMemo } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { CheckCircle, Clock, ListChecks, TrendUp, CalendarBlank } from "@phosphor-icons/react";
import { ResponsiveContainer, LineChart, Line, XAxis, YAxis, Tooltip, BarChart, Bar, CartesianGrid, PieChart, Pie, Cell } from "recharts";

const STATUS_COLORS = { todo: "#94A3B8", in_progress: "#3B82F6", review: "#F59E0B", done: "#10B981" };
const STATUS_LABELS = { todo: "To Do", in_progress: "In Progress", review: "Review", done: "Done" };

const PERIODS = [
  { v: "7d", l: "Last 7 days" },
  { v: "30d", l: "Last 30 days" },
  { v: "month", l: "This month" },
  { v: "custom", l: "Custom range" },
];

const isoLocal = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

export default function Dashboard({ withPeriodFilter = false }) {
  const { currentOrg } = useOrg();
  const [data, setData] = useState(null);
  const [period, setPeriod] = useState("30d");
  const [customStart, setCustomStart] = useState("");
  const [customEnd, setCustomEnd] = useState("");

  const range = useMemo(() => {
    if (!withPeriodFilter) return null;
    const today = new Date();
    if (period === "7d") { const s = new Date(today); s.setDate(today.getDate() - 6); return { start: isoLocal(s), end: isoLocal(today) }; }
    if (period === "30d") { const s = new Date(today); s.setDate(today.getDate() - 29); return { start: isoLocal(s), end: isoLocal(today) }; }
    if (period === "month") { const s = new Date(today.getFullYear(), today.getMonth(), 1); return { start: isoLocal(s), end: isoLocal(today) }; }
    if (period === "custom" && customStart && customEnd) return { start: customStart, end: customEnd };
    return null;
  }, [withPeriodFilter, period, customStart, customEnd]);

  useEffect(() => {
    if (!currentOrg) return;
    let url = `/orgs/${currentOrg.org_id}/analytics`;
    if (withPeriodFilter && range) url += `?start=${range.start}&end=${range.end}`;
    setData(null);
    api.get(url).then((r) => setData(r.data));
  }, [currentOrg, withPeriodFilter, range]);

  if (!currentOrg) return <div className="p-8 text-slate-500">Loading workspace…</div>;

  const trendLabel = withPeriodFilter && range
    ? `${range.start} → ${range.end}`
    : "last 7 days";

  return (
    <div className="p-8 space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">{withPeriodFilter ? "Insights" : "Overview"}</div>
          <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">{withPeriodFilter ? "Analytics" : "Dashboard"}</h1>
          <p className="text-sm text-slate-500 mt-2">Achievements and productivity across {currentOrg.name}.</p>
        </div>
        {withPeriodFilter && (
          <div className="flex flex-wrap items-center gap-2" data-testid="analytics-period-controls">
            <CalendarBlank size={16} className="text-slate-400" />
            <Select value={period} onValueChange={setPeriod}>
              <SelectTrigger className="h-10 w-44 bg-white" data-testid="analytics-period-select">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PERIODS.map((p) => (
                  <SelectItem key={p.v} value={p.v} data-testid={`analytics-period-${p.v}`}>{p.l}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {period === "custom" && (
              <>
                <Input type="date" value={customStart} onChange={(e) => setCustomStart(e.target.value)} className="h-10 w-40 bg-white" data-testid="analytics-custom-start" />
                <span className="text-slate-400 text-sm">→</span>
                <Input type="date" value={customEnd} onChange={(e) => setCustomEnd(e.target.value)} className="h-10 w-40 bg-white" data-testid="analytics-custom-end" />
              </>
            )}
          </div>
        )}
      </div>

      {!data ? (
        <div className="text-slate-500">Loading analytics…</div>
      ) : (
        <DashboardBody data={data} trendLabel={trendLabel} />
      )}
    </div>
  );
}

function DashboardBody({ data, trendLabel }) {
  const pie = Object.entries(data.by_status).map(([k, v]) => ({ name: STATUS_LABELS[k], value: v, key: k }));
  const totalHours = (data.total_logged_minutes / 60).toFixed(1);

  const stats = [
    { icon: ListChecks, label: "Total Tasks", value: data.total_tasks, tone: "text-slate-700 bg-slate-100" },
    { icon: CheckCircle, label: "Completed", value: data.completed_tasks, tone: "text-emerald-700 bg-emerald-100" },
    { icon: TrendUp, label: "Completion Rate", value: `${data.completion_rate.toFixed(0)}%`, tone: "text-indigo-700 bg-indigo-100" },
    { icon: Clock, label: "Hours Logged", value: totalHours, tone: "text-orange-700 bg-orange-100" },
  ];

  return (
    <>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {stats.map((s, i) => (
          <Card key={i} className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]" data-testid={`stat-card-${i}`}>
            <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${s.tone}`}>
              <s.icon size={20} weight="duotone" />
            </div>
            <div className="mt-4 text-3xl font-display font-semibold tracking-tight">{s.value}</div>
            <div className="text-sm text-slate-500 mt-1">{s.label}</div>
          </Card>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="p-6 lg:col-span-2 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h3 className="font-display font-semibold text-lg">Productivity trend</h3>
              <p className="text-xs text-slate-500 mt-1">Minutes logged &amp; tasks completed ({trendLabel})</p>
            </div>
          </div>
          <ResponsiveContainer width="100%" height={260}>
            <LineChart data={data.time_series.map((t, i) => ({ ...t, completed: data.completed_series[i]?.completed || 0 }))}>
              <CartesianGrid stroke="#E2E8F0" strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="date" tick={{ fontSize: 11, fill: "#64748B" }} tickFormatter={(d) => d.slice(5)} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 11, fill: "#64748B" }} axisLine={false} tickLine={false} />
              <Tooltip contentStyle={{ borderRadius: 8, border: "1px solid #E2E8F0", fontSize: 12 }} />
              <Line type="monotone" dataKey="minutes" stroke="#4F46E5" strokeWidth={2.5} dot={{ r: 3 }} name="Minutes" />
              <Line type="monotone" dataKey="completed" stroke="#F97316" strokeWidth={2.5} dot={{ r: 3 }} name="Completed" />
            </LineChart>
          </ResponsiveContainer>
        </Card>

        <Card className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
          <h3 className="font-display font-semibold text-lg mb-6">Status breakdown</h3>
          <ResponsiveContainer width="100%" height={220}>
            <PieChart>
              <Pie data={pie} dataKey="value" nameKey="name" innerRadius={45} outerRadius={80} paddingAngle={2}>
                {pie.map((p) => <Cell key={p.key} fill={STATUS_COLORS[p.key]} />)}
              </Pie>
              <Tooltip contentStyle={{ borderRadius: 8, fontSize: 12 }} />
            </PieChart>
          </ResponsiveContainer>
          <div className="mt-4 space-y-2">
            {pie.map((p) => (
              <div key={p.key} className="flex items-center justify-between text-sm">
                <div className="flex items-center gap-2">
                  <div className="w-2.5 h-2.5 rounded-full" style={{ background: STATUS_COLORS[p.key] }} />
                  <span className="text-slate-600">{p.name}</span>
                </div>
                <span className="font-medium text-slate-900">{p.value}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>

      <Card className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
        <h3 className="font-display font-semibold text-lg mb-6">Estimated vs logged time</h3>
        <ResponsiveContainer width="100%" height={220}>
          <BarChart data={[
            { name: "Estimated", value: (data.total_estimate_minutes / 60).toFixed(1) },
            { name: "Logged", value: (data.total_logged_minutes / 60).toFixed(1) },
          ]}>
            <CartesianGrid stroke="#E2E8F0" strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="name" tick={{ fontSize: 12, fill: "#64748B" }} axisLine={false} tickLine={false} />
            <YAxis tick={{ fontSize: 11, fill: "#64748B" }} axisLine={false} tickLine={false} />
            <Tooltip contentStyle={{ borderRadius: 8, fontSize: 12 }} />
            <Bar dataKey="value" fill="#4F46E5" radius={[6, 6, 0, 0]} />
          </BarChart>
        </ResponsiveContainer>
      </Card>
    </>
  );
}
