"use client";
import { useCallback, useEffect, useState } from "react";
import {
  getAlerts, getAlertCounts, setAlertStatus, recheckAlerts, getBusinessProfile,
  AlertStatus,
} from "@/lib/api";
import { Bell, BellRing, MessageCircle, Check, X, RefreshCw, Package, Phone } from "lucide-react";

interface MatchedListing {
  id: number; name: string; quantity?: number | null; price?: number | null;
  category?: { name: string };
}
interface Alert {
  id: number;
  customer_name: string;
  customer_phone: string;
  item_query: string;
  category: string;
  availability: string;
  source: string;
  status: AlertStatus;
  listing_id?: number | null;
  ready_at?: string | null;
  notified_at?: string | null;
  created_at: string;
  matched_listing?: MatchedListing | null;
}
interface Counts { logged: number; ready: number; notified: number; dismissed: number; open: number }

const TABS: { id: string; label: string; countKey?: keyof Counts }[] = [
  { id: "", label: "All" },
  { id: "ready", label: "Ready to notify", countKey: "ready" },
  { id: "logged", label: "Waiting", countKey: "logged" },
  { id: "notified", label: "Notified", countKey: "notified" },
  { id: "dismissed", label: "Dismissed", countKey: "dismissed" },
];

export default function AlertsPage() {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [counts, setCounts] = useState<Counts | null>(null);
  const [tab, setTab] = useState("");
  const [business, setBusiness] = useState("");
  const [rechecking, setRechecking] = useState(false);
  const [recheckMsg, setRecheckMsg] = useState("");

  const load = useCallback(() => {
    getAlerts(tab || undefined).then((r) => setAlerts(r.data || [])).catch(() => {});
    getAlertCounts().then((r) => setCounts(r.data)).catch(() => {});
  }, [tab]);

  useEffect(() => { load(); }, [load]);
  useEffect(() => {
    getBusinessProfile().then((r) => setBusiness(r.data?.name || "")).catch(() => {});
  }, []);
  // Keep it fresh — a background sync can raise alerts while the owner watches.
  useEffect(() => {
    const t = setInterval(load, 15000);
    return () => clearInterval(t);
  }, [load]);

  async function act(id: number, status: AlertStatus) {
    await setAlertStatus(id, status);
    load();
  }

  async function recheck() {
    setRechecking(true);
    setRecheckMsg("");
    try {
      const r = await recheckAlerts();
      const n = r.data?.raised ?? 0;
      setRecheckMsg(n > 0 ? `${n} alert${n === 1 ? "" : "s"} now ready to notify.` : "No new matches — nothing back in stock yet.");
      load();
    } finally {
      setRechecking(false);
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Alerts</h1>
          <p className="text-sm text-subtle mt-0.5">Customers waiting to be notified when something is back in stock.</p>
        </div>
        <button
          onClick={recheck}
          disabled={rechecking}
          className="flex items-center gap-2 border border-line text-ink hover:bg-panel-2 text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50 shrink-0"
        >
          <RefreshCw size={15} className={rechecking ? "animate-spin" : ""} />
          {rechecking ? "Checking…" : "Re-check stock"}
        </button>
      </div>

      {recheckMsg && <p className="text-sm text-subtle mb-4">{recheckMsg}</p>}

      <div className="flex items-center gap-1 bg-panel-2 border border-line rounded-lg p-0.5 mb-5 w-fit flex-wrap">
        {TABS.map((t) => {
          const count = t.countKey && counts ? counts[t.countKey] : undefined;
          return (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`text-sm px-3 py-1.5 rounded-md transition flex items-center gap-1.5 ${tab === t.id ? "bg-accent/15 text-accent font-medium" : "text-muted hover:text-ink"}`}
            >
              {t.label}
              {count !== undefined && count > 0 && (
                <span className={`text-[11px] rounded-full px-1.5 ${t.id === "ready" ? "bg-emerald-500/20 text-emerald-600" : "bg-panel text-subtle"}`}>{count}</span>
              )}
            </button>
          );
        })}
      </div>

      <div className="space-y-3">
        {alerts.length === 0 && (
          <div className="bg-panel rounded-xl border border-line p-10 text-center">
            <Bell size={28} className="mx-auto text-subtle mb-3" />
            <p className="text-sm text-subtle">No alerts here. When the bot can&apos;t find something a customer wants, it offers to notify them — those requests show up here, and jump to &quot;Ready to notify&quot; the moment the item is back in stock.</p>
          </div>
        )}
        {alerts.map((al) => (
          <AlertCard key={al.id} alert={al} business={business} onAct={act} />
        ))}
      </div>
    </div>
  );
}

function waLink(phone: string, text: string): string {
  const digits = phone.replace(/[^\d]/g, "");
  return `https://wa.me/${digits}?text=${encodeURIComponent(text)}`;
}

function AlertCard({ alert, business, onAct }: { alert: Alert; business: string; onAct: (id: number, s: AlertStatus) => void }) {
  const isReady = alert.status === "ready";
  const isOpen = alert.status === "ready" || alert.status === "logged";
  const name = alert.customer_name?.trim();
  const biz = business || "our shop";

  const readyMsg = `Hi${name ? " " + name : ""}! Good news — the "${alert.item_query}" you asked about is now available at ${biz}. Would you like to reserve it? 😊`;
  const genericMsg = `Hi${name ? " " + name : ""}! This is ${biz}, about the "${alert.item_query}" you asked about.`;
  const message = isReady ? readyMsg : genericMsg;

  return (
    <div className={`rounded-xl border p-4 ${isReady ? "border-emerald-500/40 bg-emerald-500/5" : "border-line bg-panel"}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            {isReady ? <BellRing size={16} className="text-emerald-600 shrink-0" /> : <Bell size={16} className="text-subtle shrink-0" />}
            <span className="font-medium text-ink">{alert.item_query}</span>
            {alert.source === "bot_offered" && <span className="text-[11px] bg-panel-2 text-subtle border border-line rounded px-1.5 py-0.5">bot offered</span>}
          </div>
          <p className="text-xs text-muted flex items-center gap-1 mt-1">
            {name && <span>{name} · </span>}
            <Phone size={11} /> {alert.customer_phone}
          </p>

          {isReady && alert.matched_listing && (
            <div className="mt-2 inline-flex items-center gap-2 text-xs text-emerald-700 bg-emerald-500/10 border border-emerald-500/20 rounded-lg px-2.5 py-1">
              <Package size={13} />
              In stock now: {alert.matched_listing.name}
              {typeof alert.matched_listing.quantity === "number" && <span>· qty {alert.matched_listing.quantity}</span>}
              {typeof alert.matched_listing.price === "number" && <span>· ₹{alert.matched_listing.price}</span>}
            </div>
          )}
          {!isReady && isOpen && alert.availability && (
            <span className="mt-2 inline-block text-xs text-subtle">{alert.availability.replace(/_/g, " ")}</span>
          )}
          {alert.status === "notified" && <span className="mt-2 inline-flex items-center gap-1 text-xs text-accent"><Check size={12} /> Notified</span>}
          {alert.status === "dismissed" && <span className="mt-2 inline-block text-xs text-subtle">Dismissed</span>}
        </div>

        {isOpen && (
          <div className="flex items-center gap-2 shrink-0">
            <a
              href={waLink(alert.customer_phone, message)}
              target="_blank"
              rel="noopener noreferrer"
              className={`flex items-center gap-1.5 text-sm font-medium px-3 py-1.5 rounded-lg ${isReady ? "bg-emerald-600 text-white hover:bg-emerald-700" : "border border-line text-ink hover:bg-panel-2"}`}
            >
              <MessageCircle size={14} /> Message on WhatsApp
            </a>
            <button
              onClick={() => onAct(alert.id, "notified")}
              title="Mark as notified"
              className="flex items-center gap-1 text-sm text-subtle hover:text-accent px-2 py-1.5 rounded-lg hover:bg-panel-2"
            >
              <Check size={15} />
            </button>
            <button
              onClick={() => onAct(alert.id, "dismissed")}
              title="Dismiss"
              className="flex items-center gap-1 text-sm text-subtle hover:text-danger px-2 py-1.5 rounded-lg hover:bg-panel-2"
            >
              <X size={15} />
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
