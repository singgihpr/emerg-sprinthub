import React, { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { Play, Stop, Clock } from "@phosphor-icons/react";
import { Button } from "@/components/ui/button";

export default function TaskTimer() {
  const { currentOrg } = useOrg();
  const [timer, setTimer] = useState(null);
  const [task, setTask] = useState(null);
  const [elapsed, setElapsed] = useState(0);

  const refresh = async () => {
    if (!currentOrg) return;
    try {
      const { data } = await api.get(`/orgs/${currentOrg.org_id}/timer`);
      if (data && data.task_id) {
        setTimer(data);
        const tasks = await api.get(`/orgs/${currentOrg.org_id}/tasks`);
        setTask(tasks.data.find((t) => t.task_id === data.task_id) || null);
      } else {
        setTimer(null); setTask(null);
      }
    } catch { /* ignore */ }
  };

  useEffect(() => { refresh(); }, [currentOrg]);

  useEffect(() => {
    if (!timer) return;
    const t = setInterval(() => {
      setElapsed(Math.floor((Date.now() - new Date(timer.started_at).getTime()) / 1000));
    }, 1000);
    return () => clearInterval(t);
  }, [timer]);

  useEffect(() => {
    const handler = () => refresh();
    window.addEventListener("timer-changed", handler);
    return () => window.removeEventListener("timer-changed", handler);
  }, [currentOrg]);

  const stop = async () => {
    await api.post(`/orgs/${currentOrg.org_id}/timer/stop`);
    setTimer(null); setTask(null); setElapsed(0);
    window.dispatchEvent(new Event("data-changed"));
  };

  if (!timer) {
    return (
      <div className="flex items-center gap-2 text-xs text-slate-500">
        <Clock size={16} />
        <span>No active timer</span>
      </div>
    );
  }

  const mm = String(Math.floor(elapsed / 60)).padStart(2, "0");
  const ss = String(elapsed % 60).padStart(2, "0");

  return (
    <div className="flex items-center gap-3 px-3 py-1.5 rounded-full bg-orange-50 border border-orange-200 timer-active" data-testid="timer-widget">
      <div className="w-2 h-2 rounded-full bg-orange-500" />
      <div className="text-sm">
        <span className="font-medium text-orange-800 truncate max-w-[180px] inline-block align-middle">{task?.title || "Task"}</span>
        <span className="font-mono text-orange-600 ml-3">{mm}:{ss}</span>
      </div>
      <Button size="sm" variant="ghost" className="h-7 px-2 hover:bg-orange-100 text-orange-700" onClick={stop} data-testid="timer-stop-btn">
        <Stop size={14} weight="fill" />
      </Button>
    </div>
  );
}
