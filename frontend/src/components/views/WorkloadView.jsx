import React, { useMemo } from "react";
import { Card } from "@/components/ui/card";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";

export default function WorkloadView({ tasks, members }) {
  const workload = useMemo(() => {
    const rows = members.map((m) => {
      const mine = tasks.filter((t) => t.assignee_id === m.user_id && t.status !== "done");
      const hours = mine.reduce((s, t) => s + (t.estimate_hours || 0), 0);
      const logged = mine.reduce((s, t) => s + (t.logged_minutes || 0), 0) / 60;
      return {
        user: m,
        active: mine.length,
        hours,
        logged,
        byStatus: {
          todo: mine.filter((t) => t.status === "todo").length,
          in_progress: mine.filter((t) => t.status === "in_progress").length,
          review: mine.filter((t) => t.status === "review").length,
        },
      };
    });
    const unassigned = tasks.filter((t) => !t.assignee_id && t.status !== "done");
    if (unassigned.length) {
      rows.push({
        user: { user_id: "unassigned", name: "Unassigned", email: "" },
        active: unassigned.length,
        hours: unassigned.reduce((s, t) => s + (t.estimate_hours || 0), 0),
        logged: 0,
        byStatus: {
          todo: unassigned.filter((t) => t.status === "todo").length,
          in_progress: unassigned.filter((t) => t.status === "in_progress").length,
          review: unassigned.filter((t) => t.status === "review").length,
        },
      });
    }
    return rows;
  }, [tasks, members]);

  const maxHours = Math.max(40, ...workload.map((w) => w.hours));

  return (
    <div className="space-y-3">
      {workload.map((w) => {
        const pct = Math.min(100, (w.hours / maxHours) * 100);
        const overloaded = w.hours > 40;
        return (
          <Card key={w.user.user_id} className="p-5 border-slate-200 shadow-[0_8px_30px_rgb(0,0,0,0.04)]" data-testid={`workload-row-${w.user.user_id}`}>
            <div className="flex items-center gap-4">
              <Avatar className="h-11 w-11">
                <AvatarImage src={w.user.picture} />
                <AvatarFallback className="bg-indigo-100 text-indigo-700 text-sm">{(w.user.name || w.user.email || "?").slice(0, 2).toUpperCase()}</AvatarFallback>
              </Avatar>
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between mb-2">
                  <div>
                    <div className="font-medium text-slate-900">{w.user.name || w.user.email}</div>
                    <div className="text-xs text-slate-500">{w.active} active · {w.hours.toFixed(1)}h estimated · {w.logged.toFixed(1)}h logged</div>
                  </div>
                  <div className={`text-sm font-semibold ${overloaded ? "text-orange-600" : "text-slate-700"}`}>
                    {w.hours.toFixed(1)}h / 40h
                  </div>
                </div>
                <div className="h-2.5 bg-slate-100 rounded-full overflow-hidden flex">
                  <div className={`h-full ${overloaded ? "bg-orange-500" : "bg-indigo-600"} transition-all`} style={{ width: `${pct}%` }} />
                </div>
                <div className="flex gap-4 mt-3 text-xs text-slate-600">
                  <span><span className="inline-block w-2 h-2 rounded-full bg-slate-400 mr-1.5" />Todo: {w.byStatus.todo}</span>
                  <span><span className="inline-block w-2 h-2 rounded-full bg-blue-500 mr-1.5" />In Progress: {w.byStatus.in_progress}</span>
                  <span><span className="inline-block w-2 h-2 rounded-full bg-amber-500 mr-1.5" />Review: {w.byStatus.review}</span>
                </div>
              </div>
            </div>
          </Card>
        );
      })}
      {workload.length === 0 && <div className="text-center py-12 text-slate-500">No members yet.</div>}
    </div>
  );
}
