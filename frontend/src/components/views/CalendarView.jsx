import React, { useState, useMemo } from "react";
import { Button } from "@/components/ui/button";
import { CaretLeft, CaretRight } from "@phosphor-icons/react";

const STATUS_COLOR = {
  todo: "bg-slate-200 text-slate-700 border-slate-300",
  in_progress: "bg-blue-100 text-blue-700 border-blue-200",
  review: "bg-amber-100 text-amber-700 border-amber-200",
  done: "bg-emerald-100 text-emerald-700 border-emerald-200",
};

export default function CalendarView({ tasks, onEdit }) {
  const [cursor, setCursor] = useState(new Date());
  const year = cursor.getFullYear();
  const month = cursor.getMonth();

  const cells = useMemo(() => {
    const first = new Date(year, month, 1);
    const start = new Date(first);
    start.setDate(start.getDate() - first.getDay());
    return Array.from({ length: 42 }, (_, i) => {
      const d = new Date(start);
      d.setDate(d.getDate() + i);
      return d;
    });
  }, [year, month]);

  const tasksByDate = useMemo(() => {
    const map = {};
    for (const t of tasks) {
      if (!t.due_date) continue;
      const key = t.due_date;
      (map[key] = map[key] || []).push(t);
    }
    return map;
  }, [tasks]);

  const monthLabel = cursor.toLocaleDateString(undefined, { month: "long", year: "numeric" });
  const today = new Date().toDateString();

  return (
    <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
      <div className="flex items-center justify-between px-6 py-4 border-b border-slate-200">
        <h3 className="font-display text-lg font-semibold">{monthLabel}</h3>
        <div className="flex gap-1">
          <Button variant="outline" size="sm" onClick={() => setCursor(new Date(year, month - 1, 1))} data-testid="cal-prev"><CaretLeft size={14} /></Button>
          <Button variant="outline" size="sm" onClick={() => setCursor(new Date())} data-testid="cal-today">Today</Button>
          <Button variant="outline" size="sm" onClick={() => setCursor(new Date(year, month + 1, 1))} data-testid="cal-next"><CaretRight size={14} /></Button>
        </div>
      </div>
      <div className="grid grid-cols-7 border-b border-slate-200 bg-slate-50">
        {["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((d) => (
          <div key={d} className="px-3 py-2 text-xs uppercase tracking-wider text-slate-500 font-medium text-center">{d}</div>
        ))}
      </div>
      <div className="grid grid-cols-7">
        {cells.map((d, i) => {
          const inMonth = d.getMonth() === month;
          const iso = d.toISOString().slice(0, 10);
          const items = tasksByDate[iso] || [];
          const isToday = d.toDateString() === today;
          return (
            <div key={i} className={`min-h-[110px] border-b border-r border-slate-100 p-2 ${inMonth ? "bg-white" : "bg-slate-50/50"}`}>
              <div className={`text-xs mb-1.5 inline-flex items-center justify-center rounded-full w-6 h-6 ${isToday ? "bg-indigo-600 text-white font-semibold" : inMonth ? "text-slate-700" : "text-slate-400"}`}>
                {d.getDate()}
              </div>
              <div className="space-y-1">
                {items.slice(0, 3).map((t) => (
                  <div
                    key={t.task_id}
                    onClick={() => onEdit(t)}
                    className={`text-[11px] px-1.5 py-0.5 rounded border cursor-pointer truncate ${STATUS_COLOR[t.status]}`}
                    data-testid={`cal-event-${t.task_id}`}
                  >
                    {t.title}
                  </div>
                ))}
                {items.length > 3 && <div className="text-[10px] text-slate-400 px-1">+{items.length - 3} more</div>}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
