import React, { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ResponsiveContainer, LineChart, Line, XAxis, YAxis, Tooltip, CartesianGrid, Legend } from "recharts";

export default function BurndownDialog({ open, onOpenChange, sprint }) {
  const { currentOrg } = useOrg();
  const [data, setData] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open || !sprint) return;
    setData(null); setError("");
    api
      .get(`/orgs/${currentOrg.org_id}/sprints/${sprint.sprint_id}/burndown`)
      .then((r) => setData(r.data))
      .catch(() => setError("Failed to load burndown"));
  }, [open, sprint, currentOrg]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-baseline gap-3">
            <span>{sprint?.name || "Burndown"}</span>
            <span className="text-xs font-normal text-slate-500">Ideal vs actual remaining hours</span>
          </DialogTitle>
        </DialogHeader>
        {error ? (
          <div className="py-12 text-center text-red-600 text-sm">{error}</div>
        ) : !data ? (
          <div className="py-12 text-center text-slate-500 text-sm">Loading…</div>
        ) : data.series.length === 0 ? (
          <div className="py-12 text-center text-slate-500 text-sm">Set start & end dates on this sprint to see burndown.</div>
        ) : (
          <div className="space-y-4">
            <div className="grid grid-cols-3 gap-3">
              <div className="p-3 rounded-lg bg-slate-50 border border-slate-200">
                <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium">Total tasks</div>
                <div className="font-display font-semibold text-xl">{data.completed_tasks} / {data.total_tasks}</div>
              </div>
              <div className="p-3 rounded-lg bg-slate-50 border border-slate-200">
                <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium">Total est.</div>
                <div className="font-display font-semibold text-xl">{data.total_estimate_hours.toFixed(1)}h</div>
              </div>
              <div className="p-3 rounded-lg bg-slate-50 border border-slate-200">
                <div className="text-[11px] uppercase tracking-wider text-slate-500 font-medium">Duration</div>
                <div className="font-display font-semibold text-xl">{data.series.length}d</div>
              </div>
            </div>
            <div className="h-80 bg-white p-2">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={data.series} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                  <CartesianGrid stroke="#E2E8F0" strokeDasharray="3 3" vertical={false} />
                  <XAxis dataKey="date" tick={{ fontSize: 11, fill: "#64748B" }} tickFormatter={(d) => d.slice(5)} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 11, fill: "#64748B" }} axisLine={false} tickLine={false} label={{ value: "Hours", angle: -90, fontSize: 11, fill: "#64748B", position: "insideLeft" }} />
                  <Tooltip contentStyle={{ borderRadius: 8, border: "1px solid #E2E8F0", fontSize: 12 }} />
                  <Legend wrapperStyle={{ fontSize: 12 }} />
                  <Line type="monotone" dataKey="ideal" stroke="#94A3B8" strokeWidth={2} strokeDasharray="5 5" dot={false} name="Ideal" />
                  <Line type="monotone" dataKey="actual" stroke="#4F46E5" strokeWidth={2.5} dot={{ r: 3 }} connectNulls={false} name="Actual" />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
