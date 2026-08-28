import React, { useEffect, useState, useRef, useCallback } from "react";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { useAuth } from "@/context/AuthContext";
import { Textarea } from "@/components/ui/textarea";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { At, PaperPlaneTilt } from "@phosphor-icons/react";

function renderBody(body) {
  // Highlight @email mentions
  const parts = body.split(/(@[\w.+-]+@[\w-]+\.[\w.-]+)/g);
  return parts.map((p, i) =>
    p.startsWith("@") && p.includes("@", 1) ? (
      <span key={i} className="bg-indigo-100 text-indigo-700 rounded px-1 font-medium">{p}</span>
    ) : (
      <span key={i}>{p}</span>
    )
  );
}

function timeAgo(iso) {
  if (!iso) return "";
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

export default function TaskComments({ taskId, members }) {
  const { currentOrg } = useOrg();
  const { user } = useAuth();
  const [comments, setComments] = useState([]);
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [suggestOpen, setSuggestOpen] = useState(false);
  const textareaRef = useRef(null);

  const load = useCallback(async () => {
    if (!taskId || !currentOrg) return;
    try {
      const { data } = await api.get(`/orgs/${currentOrg.org_id}/tasks/${taskId}/comments`);
      setComments(data); setError("");
    } catch {
      setError("Failed to load comments");
    }
  }, [taskId, currentOrg]);

  useEffect(() => { load(); }, [load]);

  const submit = async (e) => {
    e.preventDefault();
    if (!body.trim() || busy) return;
    setBusy(true); setError("");
    try {
      await api.post(`/orgs/${currentOrg.org_id}/tasks/${taskId}/comments`, { body: body.trim() });
      setBody(""); await load();
    } catch {
      setError("Failed to post comment");
    } finally { setBusy(false); }
  };

  // Simple mention suggestion: when user types "@" show inline chip suggestions row
  const lastWord = body.split(/\s+/).pop() || "";
  const mentionQuery = lastWord.startsWith("@") ? lastWord.slice(1).toLowerCase() : null;
  const suggestions = mentionQuery !== null
    ? members.filter((m) => (m.email || "").toLowerCase().includes(mentionQuery) || (m.name || "").toLowerCase().includes(mentionQuery)).slice(0, 5)
    : [];

  const pickMention = (m) => {
    const words = body.split(/\s+/);
    words[words.length - 1] = `@${m.email}`;
    const next = words.join(" ") + " ";
    setBody(next);
    setSuggestOpen(false);
    // restore focus and place caret at end after React updates the DOM
    setTimeout(() => {
      const el = textareaRef.current;
      if (el) {
        el.focus();
        el.setSelectionRange(next.length, next.length);
      }
    }, 0);
  };

  return (
    <div className="border-t border-slate-100 pt-4 mt-4 space-y-3">
      <div className="flex items-center gap-2 text-[11px] uppercase tracking-wider text-slate-500 font-medium">
        <span>Comments</span>
        <span className="text-slate-400 normal-case tracking-normal">{comments.length}</span>
      </div>

      <div className="space-y-3 max-h-56 overflow-y-auto pr-1">
        {comments.length === 0 && (
          <div className="text-xs text-slate-400">No comments yet. Start the conversation.</div>
        )}
        {comments.map((c) => (
          <div key={c.comment_id} className="flex gap-3" data-testid={`comment-${c.comment_id}`}>
            <Avatar className="h-7 w-7 shrink-0">
              <AvatarImage src={c.author_picture} />
              <AvatarFallback className="text-[10px] bg-indigo-100 text-indigo-700">{(c.author_name || "?").slice(0, 2).toUpperCase()}</AvatarFallback>
            </Avatar>
            <div className="flex-1 min-w-0">
              <div className="flex items-baseline gap-2">
                <span className="text-sm font-medium text-slate-900">{c.author_name}</span>
                <span className="text-xs text-slate-400">{timeAgo(c.created_at)}</span>
              </div>
              <div className="text-sm text-slate-700 mt-0.5 whitespace-pre-wrap break-words">{renderBody(c.body)}</div>
            </div>
          </div>
        ))}
      </div>

      <form onSubmit={submit} className="relative">
        <div className="flex items-start gap-2">
          <Avatar className="h-7 w-7 shrink-0 mt-2">
            <AvatarImage src={user?.picture} />
            <AvatarFallback className="text-[10px] bg-slate-200 text-slate-700">{(user?.name || user?.email || "?").slice(0, 2).toUpperCase()}</AvatarFallback>
          </Avatar>
          <div className="flex-1">
            <Textarea
              ref={textareaRef}
              value={body}
              onChange={(e) => { setBody(e.target.value); setSuggestOpen(true); }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) submit(e);
              }}
              placeholder="Add a comment. Use @email to mention someone…"
              rows={2}
              data-testid="comment-input"
              className="text-sm"
            />
            {suggestOpen && suggestions.length > 0 && (
              <div className="mt-1 border border-slate-200 rounded-md bg-white shadow-sm max-h-40 overflow-auto">
                {suggestions.map((m) => (
                  <button
                    key={m.user_id}
                    type="button"
                    onClick={() => pickMention(m)}
                    className="w-full text-left px-3 py-1.5 text-sm hover:bg-indigo-50 flex items-center gap-2"
                    data-testid={`mention-${m.user_id}`}
                  >
                    <At size={14} className="text-slate-400" />
                    <span className="font-medium text-slate-900">{m.name || m.email}</span>
                    <span className="text-xs text-slate-500">{m.email}</span>
                  </button>
                ))}
              </div>
            )}
            <div className="flex items-center justify-between mt-2">
              <span className="text-[11px] text-slate-400">{error ? <span className="text-red-600">{error}</span> : "Cmd/Ctrl + Enter to send"}</span>
              <Button type="submit" size="sm" disabled={busy || !body.trim()} className="bg-indigo-600 hover:bg-indigo-700 gap-2" data-testid="comment-submit-btn">
                <PaperPlaneTilt size={14} weight="fill" /> Comment
              </Button>
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}
