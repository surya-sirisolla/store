"use client";
import { useEffect, useRef, useState, useCallback } from "react";
import { assistantChat, getAssistantStatus } from "@/lib/api";
import { Bot, User, Send, Loader2, RefreshCw, AlertTriangle, Sparkles } from "lucide-react";

interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  error?: boolean;
}

const SESSION_KEY = "assistant_session_id";

const SUGGESTIONS = [
  "Who messaged the bot today?",
  "What are customers asking for that we don't carry?",
  "How many listings do we have, and in which categories?",
  "Any pending alert requests I should follow up on?",
];

function newSessionID(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) return crypto.randomUUID();
  return `s-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

export default function AssistantPage() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [online, setOnline] = useState<boolean | null>(null);
  const sessionRef = useRef<string>("");
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let sid = localStorage.getItem(SESSION_KEY);
    if (!sid) {
      sid = newSessionID();
      localStorage.setItem(SESSION_KEY, sid);
    }
    sessionRef.current = sid;
  }, []);

  const checkStatus = useCallback(async () => {
    try {
      const r = await getAssistantStatus();
      setOnline(!!r.data?.online);
    } catch {
      setOnline(false);
    }
  }, []);

  useEffect(() => {
    checkStatus();
    const t = setInterval(checkStatus, 15000);
    return () => clearInterval(t);
  }, [checkStatus]);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, sending]);

  const send = useCallback(
    async (text: string) => {
      const message = text.trim();
      if (!message || sending) return;
      setInput("");
      setMessages((m) => [...m, { role: "user", content: message }]);
      setSending(true);
      try {
        const r = await assistantChat(message, sessionRef.current);
        const reply = (r.data?.reply || "").trim() || "(no response)";
        setMessages((m) => [...m, { role: "assistant", content: reply }]);
      } catch (e) {
        const err =
          (e as { response?: { data?: { error?: string } } })?.response?.data?.error ||
          "Couldn't reach the assistant. Please try again.";
        setMessages((m) => [...m, { role: "assistant", content: err, error: true }]);
      } finally {
        setSending(false);
      }
    },
    [sending]
  );

  function newChat() {
    const sid = newSessionID();
    localStorage.setItem(SESSION_KEY, sid);
    sessionRef.current = sid;
    setMessages([]);
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send(input);
    }
  }

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)]">
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2.5">
            <Sparkles size={22} className="text-accent" /> Assistant
          </h1>
          {online !== null && (
            <span className="flex items-center gap-1.5 text-xs font-medium text-muted bg-panel border border-line rounded-full px-2.5 py-1">
              <span className={`w-2 h-2 rounded-full ${online ? "bg-accent" : "bg-danger"}`} />
              {online ? "Online" : "Offline"}
            </span>
          )}
        </div>
        <button
          onClick={newChat}
          className="flex items-center gap-2 text-sm text-muted hover:text-ink"
        >
          <RefreshCw size={15} /> New chat
        </button>
      </div>
      <p className="text-sm text-subtle mb-4">
        Ask about your business, listings, and the customers reaching your bot — it uses the same
        engine and data as your WhatsApp bot.
      </p>

      {online === false && (
        <div className="flex items-start gap-2 text-sm bg-danger/10 text-danger border border-danger/30 rounded-lg p-3 mb-4">
          <AlertTriangle size={16} className="mt-0.5 shrink-0" />
          <span>
            The assistant is offline. It shares the WhatsApp bot&apos;s engine — make sure the bot is
            enabled and connected.
          </span>
        </div>
      )}

      {/* ── Conversation ─────────────────────────────────────────────── */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto rounded-xl border border-line bg-panel p-4 space-y-4"
      >
        {messages.length === 0 && (
          <div className="h-full flex flex-col items-center justify-center text-center">
            <div className="p-3 rounded-full bg-accent/10 mb-3">
              <Bot size={30} className="text-accent" />
            </div>
            <p className="font-semibold text-ink">Ask me about your business</p>
            <p className="text-sm text-muted mt-1 max-w-md">
              I can look up listings, categories, who messaged the bot, and pending alerts.
            </p>
            <div className="grid sm:grid-cols-2 gap-2 mt-5 w-full max-w-xl">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s}
                  onClick={() => send(s)}
                  className="text-left text-sm text-muted hover:text-ink bg-panel-2 hover:bg-panel-2/70 border border-line rounded-lg px-3 py-2 transition"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((m, i) => (
          <div key={i} className={`flex gap-3 ${m.role === "user" ? "flex-row-reverse" : ""}`}>
            <div
              className={`grid place-items-center w-8 h-8 rounded-full shrink-0 ${
                m.role === "user" ? "bg-panel-2 text-muted" : "bg-accent/15 text-accent"
              }`}
            >
              {m.role === "user" ? <User size={15} /> : <Bot size={15} />}
            </div>
            <div
              className={`rounded-xl px-3.5 py-2.5 text-sm max-w-[80%] whitespace-pre-wrap break-words ${
                m.role === "user"
                  ? "bg-accent text-accent-ink"
                  : m.error
                  ? "bg-danger/10 text-danger border border-danger/30"
                  : "bg-panel-2 text-ink"
              }`}
            >
              {m.content}
            </div>
          </div>
        ))}

        {sending && (
          <div className="flex gap-3">
            <div className="grid place-items-center w-8 h-8 rounded-full shrink-0 bg-accent/15 text-accent">
              <Bot size={15} />
            </div>
            <div className="rounded-xl px-3.5 py-2.5 text-sm bg-panel-2 text-muted flex items-center gap-2">
              <Loader2 size={14} className="animate-spin" /> Thinking…
            </div>
          </div>
        )}
      </div>

      {/* ── Composer ─────────────────────────────────────────────────── */}
      <div className="mt-3 flex items-end gap-2">
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={onKeyDown}
          rows={1}
          placeholder="Ask about your listings, customers, or alerts…"
          className="flex-1 resize-none rounded-xl border border-line bg-panel px-3.5 py-3 text-sm text-ink placeholder:text-subtle focus:outline-none focus:border-line-2 max-h-40"
        />
        <button
          onClick={() => send(input)}
          disabled={sending || !input.trim()}
          className="grid place-items-center h-11 w-11 rounded-xl bg-accent text-accent-ink hover:bg-accent-strong disabled:opacity-40 shrink-0"
          aria-label="Send"
        >
          {sending ? <Loader2 size={18} className="animate-spin" /> : <Send size={18} />}
        </button>
      </div>
    </div>
  );
}
