import React, { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Card } from "@/components/ui/card";
import { CheckCircle, Clock, ListChecks, TrendUp } from "@phosphor-icons/react";
import { ResponsiveContainer, LineChart, Line, XAxis, YAxis, Tooltip, BarChart, Bar, CartesianGrid, PieChart, Pie, Cell, Legend } from "recharts";

const STATUS_COLORS = { todo: "#94A3B8", in_progress: "#3B82F6", review: "#F59E0B", done: "#10B981" };
const STATUS_LABELS = { todo: "To Do", in_progress: "In Progress", review: "Review", done: "Done" };

export default function Dashboard() {
  const { currentOrg } = useOrg();
  const [data, setData] = useState(null);

  useEffect(() => {
    if (!currentOrg) return;
    api.get(`/orgs/${currentOrg.org_id}/analytics`).then((r) => setData(r.data));
  }, [currentOrg]);

  if (!currentOrg) return <div className="p-8 text-slate-500">Loading workspace…</div>;
  if (!data) return <div className="p-8 text-slate-500">Loading analytics…</div>;

  const pie = Object.entries(data.by_status).map(([k, v]) => ({ name: STATUS_LABELS[k], value: v, key: k }));
  const totalHours = (data.total_logged_minutes / 60).toFixed(1);

  const stats = [
    { icon: ListChecks, label: "Total Tasks", value: data.total_tasks, tone: "text-slate-700 bg-slate-100" },
    { icon: CheckCircle, label: "Completed", value: data.completed_tasks, tone: "text-emerald-700 bg-emerald-100" },
    { icon: TrendUp, label: "Completion Rate", value: `${data.completion_rate.toFixed(0)}%`, tone: "text-indigo-700 bg-indigo-100" },
    { icon: Clock, label: "Hours Logged", value: totalHours, tone: "text-orange-700 bg-orange-100" },
  ];

  return (
    <div className="p-8 space-y-8">
      <div>
        <div className="text-xs uppercase tracking-[0.2em] font-medium text-slate-500">Overview</div>
        <h1 className="font-display text-4xl font-semibold tracking-tight mt-1">Dashboard</h1>
        <p className="text-sm text-slate-500 mt-2">Achievements and productivity across {currentOrg.name}.</p>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {stats.map((s, i) => (
          <Card key={i} className="p-6 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
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
              <h3 className="font-display font-semibold text-lg">Weekly productivity</h3>
              <p className="text-xs text-slate-500 mt-1">Minutes logged & tasks completed (last 7 days)</p>
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
    </div>
  );
}
