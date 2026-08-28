import React, { useMemo } from "react";

const STATUS_COLOR = { todo: "#94A3B8", in_progress: "#3B82F6", review: "#F59E0B", done: "#10B981" };

function daysBetween(a, b) {
  return Math.round((new Date(b) - new Date(a)) / 86400000);
}

export default function GanttView({ tasks, onEdit }) {
  const withDates = tasks.filter((t) => t.start_date && t.due_date);

  const { start, end, days } = useMemo(() => {
    if (withDates.length === 0) {
      const now = new Date();
      const s = new Date(now); s.setDate(s.getDate() - 3);
      const e = new Date(now); e.setDate(e.getDate() + 14);
      return { start: s, end: e, days: 17 };
    }
    const s = new Date(Math.min(...withDates.map((t) => new Date(t.start_date))));
    const e = new Date(Math.max(...withDates.map((t) => new Date(t.due_date))));
    s.setDate(s.getDate() - 1);
    e.setDate(e.getDate() + 1);
    return { start: s, end: e, days: daysBetween(s, e) + 1 };
  }, [tasks]);

  const dayList = Array.from({ length: days }, (_, i) => {
    const d = new Date(start); d.setDate(d.getDate() + i);
    return d;
  });

  return (
    <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-[0_8px_30px_rgb(0,0,0,0.04)]">
      <div className="overflow-x-auto">
        <div style={{ minWidth: `${240 + days * 40}px` }}>
          {/* Header */}
          <div className="flex bg-slate-50 border-b border-slate-200 sticky top-0">
            <div className="w-60 shrink-0 px-4 py-3 text-xs uppercase tracking-wider text-slate-500 font-medium">Task</div>
            <div className="flex">
              {dayList.map((d, i) => (
                <div key={i} className={`w-10 py-3 text-center text-[10px] border-l border-slate-100 ${d.getDay() === 0 || d.getDay() === 6 ? "bg-slate-50" : ""}`}>
                  <div className="text-slate-500 uppercase">{d.toLocaleDateString(undefined, { weekday: "short" }).slice(0, 2)}</div>
                  <div className="text-slate-800 font-medium">{d.getDate()}</div>
                </div>
              ))}
            </div>
          </div>
          {/* Rows */}
          {withDates.map((t) => {
            const offset = daysBetween(start, t.start_date);
            const len = Math.max(1, daysBetween(t.start_date, t.due_date) + 1);
            return (
              <div key={t.task_id} className="flex border-b border-slate-100 hover:bg-slate-50/50 group" data-testid={`gantt-row-${t.task_id}`}>
                <div className="w-60 shrink-0 px-4 py-2.5 text-sm font-medium text-slate-800 truncate">{t.title}</div>
                <div className="flex-1 relative py-2.5" style={{ minHeight: 40 }}>
                  <div className="absolute inset-0 flex pointer-events-none">
                    {dayList.map((d, i) => (
                      <div key={i} className={`w-10 border-l border-slate-100 ${d.getDay() === 0 || d.getDay() === 6 ? "bg-slate-50/50" : ""}`} />
                    ))}
                  </div>
                  <div
                    onClick={() => onEdit(t)}
                    className="absolute top-2 h-6 rounded-md cursor-pointer shadow-sm transition-transform hover:-translate-y-0.5 flex items-center px-2 text-[11px] font-medium text-white overflow-hidden"
                    style={{ left: offset * 40 + 4, width: len * 40 - 8, background: STATUS_COLOR[t.status] }}
                    data-testid={`gantt-bar-${t.task_id}`}
                  >
                    {t.title}
                  </div>
                </div>
              </div>
            );
          })}
          {withDates.length === 0 && (
            <div className="p-12 text-center text-slate-500 text-sm">No tasks with dates. Add start & due dates to see them here.</div>
          )}
        </div>
      </div>
    </div>
  );
}
