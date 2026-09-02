"use client";
import { useCallback, useEffect, useState } from "react";
import {
  getJobs, runJob, saveJobSchedule,
  getLivekeepingConfig, saveLivekeepingConfig, checkLivekeepingToken,
  JobScheduleInput,
} from "@/lib/api";
import {
  Puzzle, Boxes, RefreshCw, Play, CheckCircle2, XCircle, Clock,
  ShieldCheck, ShieldAlert, ShieldQuestion, Building2, List, MapPin, Link2, ChevronDown,
} from "lucide-react";

const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";

const LIVEKEEPING_KEY = "livekeeping_sync";

interface ChangedItem { name: string; qty?: number; old_qty?: number; fields?: string[] }
interface JobResult {
  total?: number; fetched?: number; created?: number; updated?: number; deleted?: number; errors?: number;
  locations_created?: number; locations_updated?: number; locations_deleted?: number;
  created_items?: ChangedItem[];
  updated_items?: ChangedItem[];
  deleted_items?: string[];
}
interface Job {
  key: string;
  name: string;
  enabled: boolean;
  schedule_kind: "manual" | "interval" | "daily";
  interval_hours: number;
  daily_time: string;
  last_status: "idle" | "running" | "success" | "failed";
  last_message: string;
  last_result: JobResult | null;
  last_run_at?: string | null;
  next_run_at?: string | null;
}
interface LkConfig {
  configured: boolean;
  token_preview: string;
  company_id: string;
  user_id: string;
  last_sync_at?: string;
  token_valid?: boolean | null;
  token_checked_at?: string;
  token_error?: string;
  sync_stock: boolean;
  sync_godowns: boolean;
  sync_profile: boolean;
}

function StatusBadge({ status }: { status: Job["last_status"] }) {
  const map = {
    running: { cls: "bg-amber-500/10 text-amber-600 border-amber-500/30", label: "Running", icon: <RefreshCw size={13} className="animate-spin" /> },
    success: { cls: "bg-emerald-500/10 text-emerald-600 border-emerald-500/30", label: "Success", icon: <CheckCircle2 size={13} /> },
    failed: { cls: "bg-red-500/10 text-red-600 border-red-500/30", label: "Failed", icon: <XCircle size={13} /> },
    idle: { cls: "bg-panel-2 text-subtle border-line", label: "Idle", icon: <Clock size={13} /> },
  }[status];
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full border ${map.cls}`}>
      {map.icon} {map.label}
    </span>
  );
}

function TokenPill({ lk }: { lk: LkConfig }) {
  if (lk.token_valid === true) {
    return <span className="inline-flex items-center gap-1.5 text-xs text-emerald-600"><ShieldCheck size={14} /> Token valid</span>;
  }
  if (lk.token_valid === false) {
    return <span className="inline-flex items-center gap-1.5 text-xs text-red-600"><ShieldAlert size={14} /> Token expired — paste a fresh one</span>;
  }
  return <span className="inline-flex items-center gap-1.5 text-xs text-subtle"><ShieldQuestion size={14} /> Not checked yet</span>;
}

// PullToggle is one kind of data the extension can import, with an on/off switch
// so the owner can sync only what they want.
function PullToggle({
  icon, label, detail, checked, disabled, onChange,
}: {
  icon: React.ReactNode; label: string; detail: string;
  checked: boolean; disabled?: boolean; onChange: (v: boolean) => void;
}) {
  return (
    <label className={`flex items-start gap-2.5 py-2.5 ${disabled ? "opacity-60" : "cursor-pointer"}`}>
      <span className={`mt-0.5 shrink-0 ${checked ? "text-accent" : "text-subtle"}`}>{icon}</span>
      <div className="min-w-0 flex-1">
        <p className="text-sm text-ink">{label}</p>
        <p className="text-xs text-subtle">{detail}</p>
      </div>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="accent-accent mt-1 shrink-0"
      />
    </label>
  );
}

// fieldLabel maps an internal changed-field key to a friendlier label.
function fieldLabel(f: string): string {
  const map: Record<string, string> = {
    stock: "stock/values", hsn_code: "HSN", category: "category",
    name: "name", unit: "unit", quantity: "quantity", price: "price",
  };
  return map[f] ?? f;
}

// QtyTag shows an item's quantity — "qty N", or "qty A → B" when it changed.
function QtyTag({ it }: { it: ChangedItem }) {
  if (it.qty === undefined || it.qty === null) return null;
  const changed = it.old_qty !== undefined && it.old_qty !== null && it.old_qty !== it.qty;
  return (
    <span className="text-[11px] font-mono text-ink whitespace-nowrap shrink-0">
      {changed ? <>qty <span className="text-subtle">{it.old_qty} →</span> {it.qty}</> : <>qty {it.qty}</>}
    </span>
  );
}

// NameList renders a titled, scrollable list of item names (for Removed).
function NameList({ title, tone, names, total }: { title: string; tone: "emerald" | "red"; names: string[]; total: number }) {
  const dot = tone === "emerald" ? "bg-emerald-500" : "bg-red-500";
  return (
    <div className="bg-panel-2/50 border border-line rounded-lg p-3">
      <p className="text-xs font-medium text-ink mb-1.5 flex items-center gap-1.5">
        <span className={`w-1.5 h-1.5 rounded-full ${dot}`} /> {title} ({total})
      </p>
      {names.length === 0 ? (
        <p className="text-xs text-subtle">None</p>
      ) : (
        <div className="max-h-52 overflow-auto space-y-0.5 pr-1">
          {names.map((n, i) => <p key={i} className="text-xs text-muted truncate" title={n}>{n || "(unnamed)"}</p>)}
          {total > names.length && <p className="text-[10px] text-subtle pt-1">…showing first {names.length}</p>}
        </div>
      )}
    </div>
  );
}

// ItemQtyList renders items with their quantity (for New) — name + qty.
function ItemQtyList({ title, items, total }: { title: string; items: ChangedItem[]; total: number }) {
  return (
    <div className="bg-panel-2/50 border border-line rounded-lg p-3">
      <p className="text-xs font-medium text-ink mb-1.5 flex items-center gap-1.5">
        <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" /> {title} ({total})
      </p>
      {items.length === 0 ? (
        <p className="text-xs text-subtle">None</p>
      ) : (
        <div className="max-h-52 overflow-auto space-y-0.5 pr-1">
          {items.map((it, i) => (
            <div key={i} className="flex items-center justify-between gap-2 text-xs">
              <span className="text-muted truncate" title={it.name}>{it.name || "(unnamed)"}</span>
              <QtyTag it={it} />
            </div>
          ))}
          {total > items.length && <p className="text-[10px] text-subtle pt-1">…showing first {items.length}</p>}
        </div>
      )}
    </div>
  );
}

// UpdatedList renders updated items with their quantity and the fields changed.
function UpdatedList({ items, total }: { items: ChangedItem[]; total: number }) {
  return (
    <div className="bg-panel-2/50 border border-line rounded-lg p-3">
      <p className="text-xs font-medium text-ink mb-1.5 flex items-center gap-1.5">
        <span className="w-1.5 h-1.5 rounded-full bg-amber-500" /> Updated ({total})
      </p>
      {items.length === 0 ? (
        <p className="text-xs text-subtle">None</p>
      ) : (
        <div className="max-h-52 overflow-auto space-y-1.5 pr-1">
          {items.map((it, i) => (
            <div key={i} className="text-xs">
              <div className="flex items-center justify-between gap-2">
                <span className="text-muted truncate" title={it.name}>{it.name || "(unnamed)"}</span>
                <QtyTag it={it} />
              </div>
              {it.fields && it.fields.length > 0 && (
                <div className="flex flex-wrap gap-1 mt-0.5">
                  {it.fields.map((f) => (
                    <span key={f} className="text-[10px] bg-panel border border-line rounded px-1 py-0.5 text-subtle">{fieldLabel(f)}</span>
                  ))}
                </div>
              )}
            </div>
          ))}
          {total > items.length && <p className="text-[10px] text-subtle pt-1">…showing first {items.length}</p>}
        </div>
      )}
    </div>
  );
}

// SyncChanges is the expandable "what changed" panel for the last sync.
function SyncChanges({ res }: { res: JobResult }) {
  const [open, setOpen] = useState(false);
  const created = res.created_items ?? [];
  const updated = res.updated_items ?? [];
  const deleted = res.deleted_items ?? [];
  if (created.length + updated.length + deleted.length === 0) return null;
  return (
    <div className="mt-2">
      <button onClick={() => setOpen((o) => !o)} className="flex items-center gap-1 text-xs text-accent hover:underline">
        <ChevronDown size={13} className={`transition-transform ${open ? "rotate-180" : ""}`} />
        {open ? "Hide changed items" : "Show changed items"}
      </button>
      {open && (
        <div className="mt-2 grid grid-cols-1 sm:grid-cols-3 gap-3">
          <ItemQtyList title="New" items={created} total={res.created ?? created.length} />
          <UpdatedList items={updated} total={res.updated ?? updated.length} />
          <NameList title="Removed" tone="red" names={deleted} total={res.deleted ?? deleted.length} />
        </div>
      )}
    </div>
  );
}

// LivekeepingExtension is the full home for the Livekeeping integration: connect,
// status, what it imports, run now, last result, and the sync schedule.
function LivekeepingExtension() {
  const [job, setJob] = useState<Job | null>(null);
  const [lk, setLk] = useState<LkConfig | null>(null);

  const loadJob = useCallback(() => {
    getJobs().then((r) => {
      const j = (r.data as Job[]).find((x) => x.key === LIVEKEEPING_KEY);
      setJob(j ?? null);
    }).catch(() => {});
  }, []);
  const loadLk = useCallback(() => {
    getLivekeepingConfig().then((r) => setLk(r.data as LkConfig)).catch(() => {});
  }, []);
  const refresh = useCallback(() => { loadJob(); loadLk(); }, [loadJob, loadLk]);

  useEffect(() => { refresh(); }, [refresh]);

  const running = job?.last_status === "running";
  useEffect(() => {
    const t = setInterval(refresh, running ? 3000 : 15000);
    return () => clearInterval(t);
  }, [running, refresh]);

  // Credentials.
  const [token, setToken] = useState("");
  const [savingToken, setSavingToken] = useState(false);
  const [checking, setChecking] = useState(false);
  const [companyDraft, setCompanyDraft] = useState<string | null>(null);
  const [savingCompany, setSavingCompany] = useState(false);
  const companyId = companyDraft ?? (lk?.company_id || "");
  const [runMsg, setRunMsg] = useState("");

  async function saveToken() {
    if (!token.trim()) return;
    setSavingToken(true);
    try {
      await saveLivekeepingConfig({ token: token.trim() });
      setToken("");
      await checkLivekeepingToken().catch(() => {});
      refresh();
    } finally { setSavingToken(false); }
  }
  async function checkToken() {
    setChecking(true);
    try { await checkLivekeepingToken(); refresh(); } finally { setChecking(false); }
  }
  async function saveCompany() {
    setSavingCompany(true);
    try { await saveLivekeepingConfig({ company_id: companyId.trim() }); setCompanyDraft(null); refresh(); }
    finally { setSavingCompany(false); }
  }
  async function run() {
    setRunMsg("");
    try { await runJob(LIVEKEEPING_KEY); refresh(); }
    catch (e) { setRunMsg(apiError(e) || "Could not start the sync."); }
  }

  // Sync-scope toggles. Optimistically update, persist, then re-read the truth.
  const [savingScope, setSavingScope] = useState(false);
  async function toggleScope(part: "sync_stock" | "sync_godowns" | "sync_profile", value: boolean) {
    setSavingScope(true);
    setLk((prev) => (prev ? { ...prev, [part]: value } : prev));
    try { await saveLivekeepingConfig({ [part]: value }); }
    finally { setSavingScope(false); refresh(); }
  }

  const nothingSelected = !!lk && !lk.sync_stock && !lk.sync_godowns && !lk.sync_profile;
  const res = job?.last_result;

  return (
    <div className="bg-panel rounded-xl border border-line p-5 lg:p-6 space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-3 min-w-0">
          <div className="grid place-items-center w-11 h-11 rounded-xl bg-accent/15 text-accent shrink-0"><Boxes size={20} /></div>
          <div className="min-w-0">
            <div className="flex items-center gap-2.5 flex-wrap">
              <h2 className="font-semibold text-ink">Livekeeping</h2>
              {job && <StatusBadge status={job.last_status} />}
            </div>
            <p className="text-xs text-subtle mt-0.5">Pulls your live catalog from the Livekeeping cloud (goapi.livekeeping.com).</p>
          </div>
        </div>
        <button
          onClick={run}
          disabled={running || !lk?.configured || nothingSelected}
          title={!lk?.configured ? "Add your token first" : nothingSelected ? "Select at least one thing to sync" : "Pull the selected data now"}
          className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50 shrink-0"
        >
          {running ? <RefreshCw size={15} className="animate-spin" /> : <Play size={15} />}
          {running ? "Syncing…" : "Sync now"}
        </button>
      </div>

      {/* Last run summary */}
      <div>
        <p className="text-xs text-subtle">
          {job?.last_run_at ? <>Last synced {new Date(job.last_run_at).toLocaleString()}</> : "Never synced yet"}
          {job?.enabled && job.next_run_at && job.schedule_kind !== "manual" && (
            <> · Next {new Date(job.next_run_at).toLocaleString()}</>
          )}
        </p>
        {job?.last_message && (
          <p className={`text-sm mt-1 ${job.last_status === "failed" ? "text-danger" : "text-ink"}`}>{job.last_message}</p>
        )}
        {res && job?.last_status !== "running" && (
          <div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-subtle mt-1">
            {typeof res.created === "number" && <span><span className="text-emerald-600 font-medium">{res.created}</span> new</span>}
            {typeof res.updated === "number" && <span><span className="text-ink font-medium">{res.updated}</span> updated</span>}
            {typeof res.deleted === "number" && <span><span className="text-ink font-medium">{res.deleted}</span> removed</span>}
            {typeof res.errors === "number" && res.errors > 0 && <span className="text-danger">{res.errors} skipped</span>}
          </div>
        )}
        {res && job?.last_status !== "running" && <SyncChanges res={res} />}
        {runMsg && <p className="text-sm text-danger mt-1">{runMsg}</p>}
      </div>

      {/* What to sync — the owner picks which parts each run imports. */}
      {lk && (
        <div className="border-t border-line pt-4">
          <div className="flex items-center justify-between mb-1">
            <p className="text-xs uppercase tracking-wider text-subtle font-medium">What to sync</p>
            {savingScope && <span className="text-[11px] text-subtle">Saving…</span>}
          </div>
          <div className="divide-y divide-line">
            <PullToggle
              icon={<List size={15} />} label="Stock items → Listings"
              detail="Names, HSN codes, units, quantities and prices — kept in sync."
              checked={lk.sync_stock} disabled={savingScope}
              onChange={(v) => toggleScope("sync_stock", v)}
            />
            <PullToggle
              icon={<MapPin size={15} />} label="Godowns → Locations"
              detail="Each branch/warehouse becomes a location you can address."
              checked={lk.sync_godowns} disabled={savingScope}
              onChange={(v) => toggleScope("sync_godowns", v)}
            />
            <PullToggle
              icon={<Building2 size={15} />} label="Business profile"
              detail="Refreshes your business name, email and phone numbers."
              checked={lk.sync_profile} disabled={savingScope}
              onChange={(v) => toggleScope("sync_profile", v)}
            />
          </div>
          {nothingSelected && (
            <p className="text-xs text-amber-600 mt-1">Nothing selected — turn on at least one to sync (auto-sync is paused too).</p>
          )}
        </div>
      )}

      {/* Connect */}
      {lk && (
        <div className="border-t border-line pt-4 space-y-3">
          <div className="flex items-center justify-between">
            <label className="text-xs text-subtle">Livekeeping token</label>
            <TokenPill lk={lk} />
          </div>
          <div className="flex gap-2">
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={lk.configured ? `Saved (${lk.token_preview}) — paste a new one to replace` : "Paste your token here"}
              className={inputCls}
            />
            <button onClick={saveToken} disabled={savingToken || !token.trim()} className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50 shrink-0">
              {savingToken ? "Saving…" : "Save"}
            </button>
            <button onClick={checkToken} disabled={checking || !lk.configured} className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50 shrink-0">
              {checking ? "Checking…" : "Check token"}
            </button>
          </div>
          {lk.token_valid === false && lk.token_error && <p className="text-xs text-danger">{lk.token_error}</p>}

          <div>
            <label className="block text-xs text-subtle mb-1">Company ID</label>
            <div className="flex gap-2">
              <input type="text" value={companyId} onChange={(e) => setCompanyDraft(e.target.value)} placeholder="Livekeeping company id" className={inputCls} />
              <button onClick={saveCompany} disabled={savingCompany || companyId.trim() === (lk.company_id || "")} className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50 shrink-0">
                {savingCompany ? "Saving…" : "Save"}
              </button>
            </div>
            <p className="text-[11px] text-subtle mt-1">Your user ID is read from the token automatically.</p>
          </div>
        </div>
      )}

      {/* Schedule — mounted once the job loads, so its form seeds from real values. */}
      {job && <ScheduleEditor job={job} onSaved={refresh} />}
    </div>
  );
}

// ScheduleEditor is the "auto-sync" cadence form. It's a separate component that
// seeds its state from the job via useState initializers (mounted only after the
// job loads), so background polling never clobbers in-progress edits.
function ScheduleEditor({ job, onSaved }: { job: Job; onSaved: () => void }) {
  const [enabled, setEnabled] = useState(job.enabled);
  const [kind, setKind] = useState<Job["schedule_kind"]>(job.schedule_kind);
  const [intervalHours, setIntervalHours] = useState(String(job.interval_hours));
  const [dailyTime, setDailyTime] = useState(job.daily_time);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState("");

  async function save() {
    setSaving(true); setMsg("");
    try {
      const payload: JobScheduleInput = {
        enabled, schedule_kind: kind,
        interval_hours: Number(intervalHours) || 1, daily_time: dailyTime,
      };
      await saveJobSchedule(LIVEKEEPING_KEY, payload);
      setMsg("Schedule saved.");
      onSaved();
    } catch (e) { setMsg(apiError(e) || "Could not save the schedule."); }
    finally { setSaving(false); }
  }

  return (
    <div className="border-t border-line pt-4 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-ink">Auto-sync schedule</span>
        <label className="flex items-center gap-2 text-xs text-subtle cursor-pointer">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-accent" />
          Run automatically
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <select value={kind} onChange={(e) => setKind(e.target.value as Job["schedule_kind"])} className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40">
          <option value="manual">Manual only</option>
          <option value="interval">Every N hours</option>
          <option value="daily">Daily at a time</option>
        </select>
        {kind === "interval" && (
          <div className="flex items-center gap-2 text-sm text-subtle">
            Every
            <input type="number" min={1} max={168} value={intervalHours} onChange={(e) => setIntervalHours(e.target.value)} className="w-20 bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40" />
            hours
          </div>
        )}
        {kind === "daily" && (
          <div className="flex items-center gap-2 text-sm text-subtle">
            at
            <input type="time" value={dailyTime} onChange={(e) => setDailyTime(e.target.value)} className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40" />
            <span className="text-xs">(UTC)</span>
          </div>
        )}
      </div>
      <div className="flex items-center gap-3">
        <button onClick={save} disabled={saving} className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50">
          {saving ? "Saving…" : "Save schedule"}
        </button>
        {msg && <span className="text-xs text-subtle">{msg}</span>}
      </div>
    </div>
  );
}

// Registry of available extensions. Add future integrations here.
const EXTENSIONS = [
  { id: "livekeeping", render: () => <LivekeepingExtension /> },
];

export default function ExtensionsPage() {
  return (
    <div>
      <div className="flex items-center gap-2.5 mb-1">
        <Puzzle size={22} className="text-accent" />
        <h1 className="text-2xl font-bold tracking-tight">Extensions</h1>
      </div>
      <p className="text-sm text-subtle mb-6 flex items-center gap-1.5">
        <Link2 size={14} /> Connect external services that pull data into your store. More coming soon.
      </p>

      <div className="space-y-6">
        {EXTENSIONS.map((ext) => (
          <div key={ext.id}>{ext.render()}</div>
        ))}
      </div>
    </div>
  );
}

function apiError(e: unknown): string {
  return (e as { response?: { data?: { error?: string } } })?.response?.data?.error || "";
}
