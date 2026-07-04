"use client";
import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { QRCodeSVG } from "qrcode.react";
import {
  getWhatsAppStatus, enableBot, disableBot,
  getBotStats, getBotContacts,
} from "@/lib/api";
import {
  Smartphone, CheckCircle, Loader2, AlertTriangle, RefreshCw, KeyRound,
  Power, PowerOff, Unlink, Users, Bell, Activity, MessageSquare, Phone, Shield,
  Copy, Check, ExternalLink,
} from "lucide-react";

type Status =
  | "need_key" | "disabled" | "starting" | "waiting_for_scan"
  | "connected" | "logged_out" | "error";
interface StatusResp { status: Status; qr?: string; detail?: string; number?: string; chat_link?: string }

interface BotStats {
  total_interactions: number; pending_alerts: number;
  unique_contacts: number; contacts_today: number;
}
interface Contact {
  id: number; phone: string; display_name: string; is_staff: boolean;
  interactions: number; last_query: string; last_seen: string;
}
const STATUS_META: Record<Status, { label: string; dot: string }> = {
  need_key: { label: "Needs API key", dot: "bg-subtle" },
  disabled: { label: "Off", dot: "bg-subtle" },
  starting: { label: "Starting…", dot: "bg-warn animate-pulse" },
  waiting_for_scan: { label: "Waiting for scan", dot: "bg-warn animate-pulse" },
  connected: { label: "Connected", dot: "bg-accent" },
  logged_out: { label: "Disconnected", dot: "bg-danger" },
  error: { label: "Error", dot: "bg-danger" },
};
const RANGES = [
  { id: "all", label: "All time" }, { id: "today", label: "Today" },
  { id: "week", label: "Last week" }, { id: "month", label: "Last month" },
];
const RECONNECT_SETTLE_MS = 5000;

function timeAgo(iso: string): string {
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return new Date(iso).toLocaleDateString();
}

function StatCard({ label, value, icon }: { label: string; value: number; icon: React.ReactNode }) {
  return (
    <div className="bg-panel rounded-xl border border-line p-4 flex items-center gap-3 hover:border-line-2 transition">
      <div className="p-2.5 rounded-lg bg-accent/10 text-accent">{icon}</div>
      <div>
        <p className="text-xl font-bold text-ink">{value}</p>
        <p className="text-xs text-muted">{label}</p>
      </div>
    </div>
  );
}

export default function WhatsAppPage() {
  const [data, setData] = useState<StatusResp | null>(null);
  const [loading, setLoading] = useState(true);
  const [toggling, setToggling] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [reconnectPhase, setReconnectPhase] = useState("");
  const [copied, setCopied] = useState(false);

  const [stats, setStats] = useState<BotStats | null>(null);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [range, setRange] = useState("all");

  const loadStatus = useCallback(async () => {
    try { const r = await getWhatsAppStatus(); setData(r.data); }
    catch { setData({ status: "error", detail: "Could not reach the server." }); }
    finally { setLoading(false); }
  }, []);

  const loadMonitor = useCallback(async () => {
    getBotStats().then((r) => setStats(r.data));
    getBotContacts(range === "all" ? undefined : range).then((r) => setContacts(r.data?.contacts || []));
  }, [range]);

  const toggleBot = useCallback(async () => {
    setToggling(true);
    try {
      if (data?.status === "disabled") await enableBot(); else await disableBot();
      setLoading(true); await loadStatus();
    } finally { setToggling(false); }
  }, [data?.status, loadStatus]);

  const reconnect = useCallback(async () => {
    setReconnecting(true);
    try {
      setReconnectPhase("Disconnecting old session…"); await disableBot();
      setReconnectPhase("Restarting WhatsApp gateway…"); await new Promise((r) => setTimeout(r, RECONNECT_SETTLE_MS));
      setReconnectPhase("Requesting a new QR code…"); await enableBot();
      setLoading(true); await loadStatus();
    } finally { setReconnecting(false); setReconnectPhase(""); }
  }, [loadStatus]);

  useEffect(() => {
    loadStatus();
    const t = setInterval(loadStatus, 3000);
    return () => clearInterval(t);
  }, [loadStatus]);

  useEffect(() => {
    loadMonitor();
    const t = setInterval(loadMonitor, 10000);
    return () => clearInterval(t);
  }, [loadMonitor]);

  const status = data?.status;
  const isDisabled = status === "disabled";

  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2.5"><Smartphone size={22} className="text-accent" /> WhatsApp</h1>
          {status && (
            <span className="flex items-center gap-1.5 text-xs font-medium text-muted bg-panel border border-line rounded-full px-2.5 py-1">
              <span className={`w-2 h-2 rounded-full ${STATUS_META[status].dot}`} />
              {STATUS_META[status].label}
            </span>
          )}
        </div>
        <button onClick={() => { loadStatus(); loadMonitor(); }} className="flex items-center gap-2 text-sm text-muted hover:text-ink"><RefreshCw size={15} /> Refresh</button>
      </div>
      <p className="text-sm text-subtle mb-6">Connect the bot and see who&apos;s reaching your business — all in one place.</p>

      {/* ── Connection ─────────────────────────────────────────────── */}
      <div className="bg-panel rounded-xl border border-line p-6 mb-6">
          {loading && <div className="flex items-center gap-2 text-muted text-sm"><Loader2 size={18} className="animate-spin" /> Checking status…</div>}

          {!loading && status === "need_key" && (
            <div className="flex flex-col items-center text-center py-6">
              <div className="p-3 rounded-full bg-warn/10 mb-3"><KeyRound size={28} className="text-warn" /></div>
              <p className="font-semibold text-ink">No AI key configured</p>
              <p className="text-sm text-muted mt-1">Add a provider key under <Link href="/admin/ai-providers" className="text-accent hover:underline">AI Providers</Link> to start the bot.</p>
            </div>
          )}

          {!loading && status === "disabled" && (
            <div className="flex flex-col items-center text-center py-6">
              <div className="p-3 rounded-full bg-panel-2 mb-3"><PowerOff size={30} className="text-muted" /></div>
              <p className="font-semibold text-ink">Bot is turned off</p>
              <p className="text-sm text-muted mt-1 max-w-sm">Your WhatsApp number stays linked. Turn it back on below to start replying again.</p>
            </div>
          )}

          {!loading && status === "connected" && (
            <div className="flex flex-col items-center text-center py-6">
              <div className="p-3 rounded-full bg-accent/10 mb-3"><CheckCircle size={34} className="text-accent" /></div>
              <p className="font-semibold text-ink">WhatsApp is connected{data?.number ? <> as <span className="font-mono">{data.number}</span></> : ""}</p>
              <p className="text-sm text-muted mt-1 max-w-md">Your bot is live. Customers can message your number and it replies from your listings.</p>

              {data?.chat_link && (
                <div className="mt-5 w-full max-w-md text-left">
                  <p className="text-xs font-medium text-subtle uppercase tracking-wider mb-2">Share this link — anyone can message your bot</p>
                  <div className="flex items-center gap-2">
                    <input
                      readOnly
                      value={data.chat_link}
                      onFocus={(e) => e.currentTarget.select()}
                      className="flex-1 rounded-lg border border-line bg-panel-2 px-3 py-2 text-sm font-mono text-ink outline-none"
                    />
                    <button
                      onClick={async () => { await navigator.clipboard.writeText(data.chat_link!); setCopied(true); setTimeout(() => setCopied(false), 1500); }}
                      className="flex items-center gap-1.5 text-sm font-medium px-3 py-2 rounded-lg bg-panel-2 border border-line text-muted hover:text-ink whitespace-nowrap"
                    >
                      {copied ? <Check size={15} className="text-accent" /> : <Copy size={15} />}
                      {copied ? "Copied" : "Copy"}
                    </button>
                    <a
                      href={data.chat_link} target="_blank" rel="noopener noreferrer"
                      className="flex items-center gap-1.5 text-sm font-medium px-3 py-2 rounded-lg bg-accent text-accent-ink hover:bg-accent-strong whitespace-nowrap"
                    >
                      <ExternalLink size={15} /> Open
                    </a>
                  </div>
                  <div className="flex flex-col items-center mt-4">
                    <div className="p-3 bg-white rounded-xl"><QRCodeSVG value={data.chat_link} size={140} level="M" /></div>
                    <p className="text-xs text-subtle mt-2">Or let customers scan this to start a chat</p>
                  </div>
                </div>
              )}
            </div>
          )}

          {!loading && status === "logged_out" && (
            <div className="flex flex-col items-center text-center py-6">
              <div className="p-3 rounded-full bg-danger/10 mb-3"><Unlink size={30} className="text-danger" /></div>
              <p className="font-semibold text-ink">WhatsApp got disconnected</p>
              <p className="text-sm text-muted mt-1 max-w-sm">This number was unlinked from WhatsApp. Reconnect to get a fresh QR code.</p>
              <button onClick={reconnect} disabled={reconnecting} className="mt-4 flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-5 py-2 rounded-lg disabled:opacity-50">
                {reconnecting ? <Loader2 size={15} className="animate-spin" /> : <RefreshCw size={15} />}
                {reconnecting ? reconnectPhase || "Reconnecting…" : "Reconnect WhatsApp"}
              </button>
            </div>
          )}

          {!loading && status === "waiting_for_scan" && data?.qr && (
            <div className="flex flex-col items-center text-center">
              <p className="font-semibold text-ink mb-1">Scan to connect</p>
              <p className="text-sm text-muted mb-4 max-w-md">On your phone: <span className="text-ink font-medium">WhatsApp → Settings → Linked Devices → Link a Device</span>, then scan this code.</p>
              <div className="p-4 bg-white rounded-xl"><QRCodeSVG value={data.qr} size={224} level="L" /></div>
              <p className="text-xs text-subtle mt-3 flex items-center gap-1"><Loader2 size={12} className="animate-spin" /> Waiting for you to scan… (refreshes automatically)</p>
            </div>
          )}

          {!loading && status === "starting" && (
            <div className="flex flex-col items-center text-center py-6">
              <Loader2 size={28} className="animate-spin text-accent mb-3" />
              <p className="font-semibold text-ink">Bot is starting…</p>
              <p className="text-sm text-muted mt-1">The QR code will appear here in a few seconds.</p>
            </div>
          )}

          {!loading && status === "error" && (
            <div className="flex flex-col items-center text-center py-6">
              <div className="p-3 rounded-full bg-danger/10 mb-3"><AlertTriangle size={30} className="text-danger" /></div>
              <p className="font-semibold text-ink">Can&apos;t start the bot</p>
              <p className="text-sm text-muted mt-1">{data?.detail}</p>
            </div>
          )}

          {keyInfoReady(status) && (
            <div className="mt-5 pt-4 border-t border-line flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-ink">Bot is {isDisabled ? "off" : "on"}</p>
                <p className="text-xs text-subtle">{isDisabled ? "Customers won't get replies until you turn it on." : "Replying to customers on WhatsApp."}</p>
              </div>
              <button onClick={toggleBot} disabled={toggling}
                className={`flex items-center gap-2 text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50 ${isDisabled ? "bg-accent text-accent-ink hover:bg-accent-strong" : "bg-danger/15 text-danger hover:bg-danger/25"}`}>
                {toggling ? <Loader2 size={15} className="animate-spin" /> : isDisabled ? <Power size={15} /> : <PowerOff size={15} />}
                {isDisabled ? "Enable bot" : "Disable bot"}
              </button>
            </div>
          )}
      </div>

      {/* ── Monitor ──────────────────────────────────────────────────── */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
        <StatCard label="People who messaged" value={stats?.unique_contacts ?? 0} icon={<Users size={18} />} />
        <StatCard label="New today" value={stats?.contacts_today ?? 0} icon={<Activity size={18} />} />
        <StatCard label="Total interactions" value={stats?.total_interactions ?? 0} icon={<MessageSquare size={18} />} />
        <Link href="/admin/alerts" className="block">
          <StatCard label="Pending alerts" value={stats?.pending_alerts ?? 0} icon={<Bell size={18} />} />
        </Link>
      </div>

      <div className="grid grid-cols-1 gap-6">
        <div className="bg-panel rounded-xl border border-line p-5">
          <div className="flex items-center justify-between gap-2 mb-3 flex-wrap">
            <h2 className="font-semibold text-ink">Who messaged the bot</h2>
            <div className="flex items-center gap-1 bg-panel-2 border border-line rounded-lg p-0.5">
              {RANGES.map((r) => (
                <button key={r.id} onClick={() => setRange(r.id)}
                  className={`text-xs px-2.5 py-1 rounded-md transition ${range === r.id ? "bg-accent/15 text-accent font-medium" : "text-muted hover:text-ink"}`}>
                  {r.label}
                </button>
              ))}
            </div>
          </div>
          <div className="space-y-1 max-h-112 overflow-auto -mx-2 px-2">
            {contacts.length === 0 && (
              <p className="text-sm text-subtle py-8 text-center">
                {range === "all" ? "No one has messaged yet. Once people message your WhatsApp, each number shows up here with their history." : "No one messaged in this period."}
              </p>
            )}
            {contacts.map((ct) => (
              <div key={ct.id} className="flex items-start gap-3 py-2.5 border-b border-line last:border-0">
                <div className="grid place-items-center w-8 h-8 rounded-full bg-panel-2 text-muted shrink-0 mt-0.5"><Phone size={13} /></div>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-ink flex items-center gap-1.5 flex-wrap">
                    <span className="font-mono">{ct.phone}</span>
                    {ct.display_name && <span className="text-muted font-normal">· {ct.display_name}</span>}
                    {ct.is_staff && <span className="flex items-center gap-0.5 text-[11px] bg-accent/15 text-accent rounded px-1.5 py-0.5"><Shield size={10} /> Staff</span>}
                  </p>
                  {ct.last_query && <p className="text-xs text-subtle truncate mt-0.5">last: “{ct.last_query}”</p>}
                </div>
                <div className="text-right whitespace-nowrap">
                  <p className="text-xs text-muted">{ct.interactions}×</p>
                  <p className="text-xs text-subtle">{timeAgo(ct.last_seen)}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

// keyInfoReady decides whether to show the on/off toggle — only once the bot is
// past the "no key" / loading states (i.e. a real running/paired state).
function keyInfoReady(status?: Status): boolean {
  return status === "connected" || status === "disabled" || status === "waiting_for_scan" || status === "starting";
}
