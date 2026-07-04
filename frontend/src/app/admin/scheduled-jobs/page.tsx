"use client";
import { useCallback, useEffect, useState } from "react";
import {
  getJobs, saveJobSchedule, runJob,
  createJob, updateJob, deleteJob, previewBroadcast,
  getLivekeepingConfig, saveLivekeepingConfig, checkLivekeepingToken,
  JobScheduleInput, BroadcastConfig, BroadcastInput,
} from "@/lib/api";
import { RefreshCw, Play, CheckCircle2, XCircle, Clock, ShieldCheck, ShieldAlert, ShieldQuestion, Plus, Pencil, Trash2, Megaphone, Eye, Users } from "lucide-react";

const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";

interface JobResult {
  total?: number; fetched?: number; created?: number; updated?: number; deleted?: number; errors?: number;
  sent?: number; failed?: number; recipients?: number; kept?: number;
}
interface Job {
  key: string;
  name: string;
  type: "system" | "broadcast";
  deletable: boolean;
  config?: BroadcastConfig | null;
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
}

const LIVEKEEPING_KEY = "livekeeping_sync";

export default function ScheduledJobsPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [lk, setLk] = useState<LkConfig | null>(null);

  const loadJobs = useCallback(() => {
    getJobs().then((r) => setJobs(r.data as Job[])).catch(() => {});
  }, []);
  const loadLk = useCallback(() => {
    getLivekeepingConfig().then((r) => setLk(r.data as LkConfig)).catch(() => {});
  }, []);

  useEffect(() => { loadJobs(); loadLk(); }, [loadJobs, loadLk]);

  // Poll while anything is running so status + last result stay live.
  const anyRunning = jobs.some((j) => j.last_status === "running");
  useEffect(() => {
    const interval = anyRunning ? 3000 : 15000;
    const t = setInterval(() => { loadJobs(); loadLk(); }, interval);
    return () => clearInterval(t);
  }, [anyRunning, loadJobs, loadLk]);

  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<Job | null>(null);

  const broadcasts = jobs.filter((j) => j.type === "broadcast");
  const systemJobs = jobs.filter((j) => j.type !== "broadcast");
  const refresh = () => { loadJobs(); loadLk(); };

  function openCreate() { setEditing(null); setShowForm(true); }
  function openEdit(job: Job) { setEditing(job); setShowForm(true); }
  function closeForm() { setShowForm(false); setEditing(null); }

  return (
    <div>
      <div className="flex items-start justify-between gap-3 mb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Scheduled Jobs</h1>
          <p className="text-sm text-subtle mt-0.5">Send reminders to your customers and run automated tasks on a schedule.</p>
        </div>
        {!showForm && (
          <button onClick={openCreate} className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg transition shrink-0">
            <Plus size={16} /> Create reminder
          </button>
        )}
      </div>

      {showForm && (
        <BroadcastForm job={editing} onSaved={() => { closeForm(); refresh(); }} onCancel={closeForm} />
      )}

      {/* Owner reminders */}
      <section className="mb-8">
        <h2 className="text-xs uppercase tracking-wider text-subtle font-medium mb-3">Your reminders</h2>
        <div className="space-y-4">
          {broadcasts.map((job) => (
            <BroadcastCard key={job.key} job={job} onEdit={() => openEdit(job)} onChanged={refresh} />
          ))}
          {broadcasts.length === 0 && !showForm && (
            <div className="bg-panel rounded-xl border border-dashed border-line p-8 text-center">
              <Megaphone size={26} className="mx-auto text-subtle mb-3" />
              <p className="text-sm text-subtle">No reminders yet. Create one to message your customers about featured items &amp; offers on a schedule.</p>
            </div>
          )}
        </div>
      </section>

      {/* Built-in system jobs */}
      <section>
        <h2 className="text-xs uppercase tracking-wider text-subtle font-medium mb-3">System jobs</h2>
        <div className="space-y-4">
          {systemJobs.map((job) => (
            <JobCard
              key={job.key}
              job={job}
              lk={job.key === LIVEKEEPING_KEY ? lk : null}
              onChanged={refresh}
            />
          ))}
        </div>
      </section>
    </div>
  );
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

function JobCard({ job, lk, onChanged }: { job: Job; lk: LkConfig | null; onChanged: () => void }) {
  // Schedule form, seeded from the job. Not reset by background polling so the
  // owner's in-progress edits survive a refresh.
  const [enabled, setEnabled] = useState(job.enabled);
  const [kind, setKind] = useState<Job["schedule_kind"]>(job.schedule_kind);
  const [intervalHours, setIntervalHours] = useState(String(job.interval_hours));
  const [dailyTime, setDailyTime] = useState(job.daily_time);
  const [savingSchedule, setSavingSchedule] = useState(false);
  const [scheduleMsg, setScheduleMsg] = useState("");

  // Livekeeping credentials.
  const [token, setToken] = useState("");
  const [savingToken, setSavingToken] = useState(false);
  const [checking, setChecking] = useState(false);
  const [runMsg, setRunMsg] = useState("");
  // Company id is editable; shows the saved value until the owner edits it.
  const [companyDraft, setCompanyDraft] = useState<string | null>(null);
  const [savingCompany, setSavingCompany] = useState(false);
  const companyId = companyDraft ?? (lk?.company_id || "");

  const running = job.last_status === "running";

  async function saveSchedule() {
    setSavingSchedule(true);
    setScheduleMsg("");
    try {
      const payload: JobScheduleInput = {
        enabled,
        schedule_kind: kind,
        interval_hours: Number(intervalHours) || 1,
        daily_time: dailyTime,
      };
      await saveJobSchedule(job.key, payload);
      setScheduleMsg("Schedule saved.");
      onChanged();
    } catch (e) {
      setScheduleMsg(apiError(e) || "Could not save the schedule.");
    } finally {
      setSavingSchedule(false);
    }
  }

  async function saveToken() {
    if (!token.trim()) return;
    setSavingToken(true);
    try {
      await saveLivekeepingConfig({ token: token.trim() });
      setToken("");
      // A new token's validity is unknown — check it right away.
      await checkLivekeepingToken().catch(() => {});
      onChanged();
    } finally {
      setSavingToken(false);
    }
  }

  async function checkToken() {
    setChecking(true);
    try {
      await checkLivekeepingToken();
      onChanged();
    } finally {
      setChecking(false);
    }
  }

  async function saveCompany() {
    setSavingCompany(true);
    try {
      await saveLivekeepingConfig({ company_id: companyId.trim() });
      setCompanyDraft(null);
      onChanged();
    } finally {
      setSavingCompany(false);
    }
  }

  async function run() {
    setRunMsg("");
    try {
      await runJob(job.key);
      onChanged();
    } catch (e) {
      setRunMsg(apiError(e) || "Could not start the job.");
    }
  }

  const res = job.last_result;

  return (
    <div className="bg-panel rounded-xl border border-line p-5 space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2.5">
            <h2 className="font-semibold text-ink">{job.name}</h2>
            <StatusBadge status={job.last_status} />
          </div>
          <p className="text-xs text-subtle mt-1">
            {job.last_run_at ? <>Last run {new Date(job.last_run_at).toLocaleString()}</> : "Never run yet"}
            {job.enabled && job.next_run_at && job.schedule_kind !== "manual" && (
              <> · Next run {new Date(job.next_run_at).toLocaleString()}</>
            )}
          </p>
        </div>
        <button
          onClick={run}
          disabled={running}
          className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50 shrink-0"
        >
          {running ? <RefreshCw size={15} className="animate-spin" /> : <Play size={15} />}
          {running ? "Running…" : "Run now"}
        </button>
      </div>

      {job.last_message && (
        <p className={`text-sm ${job.last_status === "failed" ? "text-danger" : "text-ink"}`}>{job.last_message}</p>
      )}
      {res && job.last_status !== "running" && (
        <div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-subtle">
          {typeof res.created === "number" && <span><span className="text-emerald-600 font-medium">{res.created}</span> new</span>}
          {typeof res.updated === "number" && <span><span className="text-ink font-medium">{res.updated}</span> updated</span>}
          {typeof res.deleted === "number" && <span><span className="text-ink font-medium">{res.deleted}</span> removed</span>}
          {typeof res.errors === "number" && res.errors > 0 && <span className="text-danger">{res.errors} skipped</span>}
        </div>
      )}
      {runMsg && <p className="text-sm text-danger">{runMsg}</p>}

      {/* Livekeeping credentials — only for that job. */}
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
            <button
              onClick={saveToken}
              disabled={savingToken || !token.trim()}
              className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50 shrink-0"
            >
              {savingToken ? "Saving…" : "Save"}
            </button>
            <button
              onClick={checkToken}
              disabled={checking || !lk.configured}
              className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50 shrink-0"
            >
              {checking ? "Checking…" : "Check token"}
            </button>
          </div>
          {lk.token_valid === false && lk.token_error && (
            <p className="text-xs text-danger">{lk.token_error}</p>
          )}

          <div>
            <label className="block text-xs text-subtle mb-1">Company ID</label>
            <div className="flex gap-2">
              <input
                type="text"
                value={companyId}
                onChange={(e) => setCompanyDraft(e.target.value)}
                placeholder="Livekeeping company id"
                className={inputCls}
              />
              <button
                onClick={saveCompany}
                disabled={savingCompany || companyId.trim() === (lk.company_id || "")}
                className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50 shrink-0"
              >
                {savingCompany ? "Saving…" : "Save"}
              </button>
            </div>
            <p className="text-[11px] text-subtle mt-1">Your user ID is read from the token automatically. Each sync also refreshes your business profile from Livekeeping.</p>
          </div>
        </div>
      )}

      {/* Schedule */}
      <div className="border-t border-line pt-4 space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-ink">Schedule</span>
          <label className="flex items-center gap-2 text-xs text-subtle cursor-pointer">
            <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-accent" />
            Run automatically
          </label>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <select
            value={kind}
            onChange={(e) => setKind(e.target.value as Job["schedule_kind"])}
            className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40"
          >
            <option value="manual">Manual only</option>
            <option value="interval">Every N hours</option>
            <option value="daily">Daily at a time</option>
          </select>

          {kind === "interval" && (
            <div className="flex items-center gap-2 text-sm text-subtle">
              Every
              <input
                type="number" min={1} max={168}
                value={intervalHours}
                onChange={(e) => setIntervalHours(e.target.value)}
                className="w-20 bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40"
              />
              hours
            </div>
          )}
          {kind === "daily" && (
            <div className="flex items-center gap-2 text-sm text-subtle">
              at
              <input
                type="time"
                value={dailyTime}
                onChange={(e) => setDailyTime(e.target.value)}
                className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40"
              />
              <span className="text-xs">(UTC)</span>
            </div>
          )}
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={saveSchedule}
            disabled={savingSchedule}
            className="text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50"
          >
            {savingSchedule ? "Saving…" : "Save schedule"}
          </button>
          {scheduleMsg && <span className="text-xs text-subtle">{scheduleMsg}</span>}
        </div>
      </div>
    </div>
  );
}

function scheduleSummary(job: Pick<Job, "enabled" | "schedule_kind" | "interval_hours" | "daily_time">): string {
  if (job.schedule_kind === "manual") return "Manual only";
  const base = job.schedule_kind === "interval" ? `Every ${job.interval_hours}h` : `Daily at ${job.daily_time} UTC`;
  return job.enabled ? base : `${base} (paused)`;
}

// ScheduleFields is the shared "when to run" editor (manual / every N hours /
// daily at a time) used by the reminder form.
function ScheduleFields({
  enabled, setEnabled, kind, setKind, intervalHours, setIntervalHours, dailyTime, setDailyTime,
}: {
  enabled: boolean; setEnabled: (b: boolean) => void;
  kind: Job["schedule_kind"]; setKind: (k: Job["schedule_kind"]) => void;
  intervalHours: string; setIntervalHours: (s: string) => void;
  dailyTime: string; setDailyTime: (s: string) => void;
}) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-ink">When to run</span>
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
    </div>
  );
}

function BroadcastForm({ job, onSaved, onCancel }: { job: Job | null; onSaved: () => void; onCancel: () => void }) {
  const cfg = job?.config || {};
  const [name, setName] = useState(job?.name || "");
  const [header, setHeader] = useState(cfg.header || "");
  const [recipientDays, setRecipientDays] = useState(String(cfg.recipient_days ?? 0));
  const [includeStaff, setIncludeStaff] = useState(cfg.include_staff ?? false);
  const [maxItems, setMaxItems] = useState(String(cfg.max_items ?? 5));
  const [throttle, setThrottle] = useState(String(cfg.throttle_seconds ?? 8));
  const [dailyCap, setDailyCap] = useState(String(cfg.daily_cap ?? 200));

  const [enabled, setEnabled] = useState(job?.enabled ?? true);
  const [kind, setKind] = useState<Job["schedule_kind"]>(job?.schedule_kind || "daily");
  const [intervalHours, setIntervalHours] = useState(String(job?.interval_hours ?? 24));
  const [dailyTime, setDailyTime] = useState(job?.daily_time || "10:00");

  const [preview, setPreview] = useState<{ preview: string; recipients: number } | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  function buildInput(): BroadcastInput {
    return {
      name: name.trim(),
      schedule: { enabled, schedule_kind: kind, interval_hours: Number(intervalHours) || 1, daily_time: dailyTime },
      config: {
        header: header.trim(),
        recipient_days: Number(recipientDays) || 0,
        include_staff: includeStaff,
        max_items: Number(maxItems) || 5,
        throttle_seconds: Number(throttle) || 8,
        daily_cap: Number(dailyCap) || 200,
      },
    };
  }

  async function doPreview() {
    setPreviewing(true); setError("");
    try {
      const r = await previewBroadcast(buildInput());
      setPreview(r.data as { preview: string; recipients: number });
    } catch (e) { setError(apiError(e) || "Could not build a preview."); }
    finally { setPreviewing(false); }
  }

  async function save() {
    if (!name.trim()) { setError("Give the reminder a name."); return; }
    setSaving(true); setError("");
    try {
      if (job) await updateJob(job.key, buildInput());
      else await createJob(buildInput());
      onSaved();
    } catch (e) { setError(apiError(e) || "Could not save the reminder."); setSaving(false); }
  }

  const numCls = "w-24 bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40";

  return (
    <div className="bg-panel rounded-xl border border-line p-5 mb-8 space-y-5">
      <div className="flex items-center justify-between">
        <h2 className="font-semibold text-ink">{job ? "Edit reminder" : "New reminder"}</h2>
        <button onClick={onCancel} className="text-subtle hover:text-ink"><XCircle size={18} /></button>
      </div>

      <div>
        <label className="block text-xs text-subtle mb-1">Name</label>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Weekly offers" className={inputCls} />
      </div>

      <div className="border-t border-line pt-4 space-y-3">
        <span className="text-sm font-medium text-ink">Message</span>
        <p className="text-xs text-subtle">The message is built automatically from the items you&apos;ve starred as <span className="text-amber-500">featured/offers</span> on the Listings page. Add an optional opening line below.</p>
        <input value={header} onChange={(e) => setHeader(e.target.value)} placeholder="Opening line (optional) — e.g. This week's specials just for you!" className={inputCls} />
        <div className="flex items-center gap-2 text-sm text-subtle">
          Include up to
          <input type="number" min={1} max={20} value={maxItems} onChange={(e) => setMaxItems(e.target.value)} className={numCls} />
          featured items
        </div>
      </div>

      <div className="border-t border-line pt-4 space-y-3">
        <span className="text-sm font-medium text-ink flex items-center gap-1.5"><Users size={14} /> Who to send to</span>
        <div className="flex flex-wrap items-center gap-3 text-sm text-subtle">
          <select value={recipientDays} onChange={(e) => setRecipientDays(e.target.value)} className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40">
            <option value="0">All customers who&apos;ve messaged the bot</option>
            <option value="7">Active in the last 7 days</option>
            <option value="30">Active in the last 30 days</option>
            <option value="90">Active in the last 90 days</option>
          </select>
          <label className="flex items-center gap-2 text-xs cursor-pointer">
            <input type="checkbox" checked={includeStaff} onChange={(e) => setIncludeStaff(e.target.checked)} className="accent-accent" />
            Include staff numbers
          </label>
        </div>
        <p className="text-xs text-subtle">Opted-out customers (who replied “STOP”) are always excluded.</p>
      </div>

      <div className="border-t border-line pt-4 space-y-3">
        <span className="text-sm font-medium text-ink">Sending pace <span className="font-normal text-subtle">(protects your number from being flagged)</span></span>
        <div className="flex flex-wrap items-center gap-4 text-sm text-subtle">
          <div className="flex items-center gap-2">
            Wait
            <input type="number" min={3} max={120} value={throttle} onChange={(e) => setThrottle(e.target.value)} className={numCls} />
            sec between messages
          </div>
          <div className="flex items-center gap-2">
            Max
            <input type="number" min={1} max={1000} value={dailyCap} onChange={(e) => setDailyCap(e.target.value)} className={numCls} />
            per run
          </div>
        </div>
      </div>

      <div className="border-t border-line pt-4">
        <ScheduleFields
          enabled={enabled} setEnabled={setEnabled}
          kind={kind} setKind={setKind}
          intervalHours={intervalHours} setIntervalHours={setIntervalHours}
          dailyTime={dailyTime} setDailyTime={setDailyTime}
        />
      </div>

      <div className="border-t border-line pt-4 space-y-3">
        <button onClick={doPreview} disabled={previewing} className="flex items-center gap-2 text-sm font-medium px-4 py-2 rounded-lg border border-line text-ink hover:bg-panel-2 disabled:opacity-50">
          <Eye size={15} /> {previewing ? "Building…" : "Preview message"}
        </button>
        {preview && (
          <div className="bg-panel-2 border border-line rounded-lg p-4">
            <p className="text-xs text-subtle mb-2">Goes to <span className="font-semibold text-ink">{preview.recipients}</span> customer{preview.recipients === 1 ? "" : "s"}:</p>
            <pre className="text-sm text-ink whitespace-pre-wrap font-sans">{preview.preview}</pre>
          </div>
        )}
      </div>

      {error && <p className="text-sm text-danger">{error}</p>}
      <div className="flex gap-2">
        <button onClick={save} disabled={saving} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50">
          {saving ? "Saving…" : job ? "Save changes" : "Create reminder"}
        </button>
        <button onClick={onCancel} className="text-sm px-4 py-2 rounded-lg border border-line text-muted hover:text-ink">Cancel</button>
      </div>
    </div>
  );
}

function BroadcastCard({ job, onEdit, onChanged }: { job: Job; onEdit: () => void; onChanged: () => void }) {
  const [msg, setMsg] = useState("");
  const running = job.last_status === "running";
  const res = job.last_result;

  async function run() {
    if (!confirm("Send this reminder now? Real WhatsApp messages will go to your opted-in customers.")) return;
    setMsg("");
    try { await runJob(job.key); onChanged(); }
    catch (e) { setMsg(apiError(e) || "Could not start the send."); }
  }
  async function remove() {
    if (!confirm(`Delete the reminder “${job.name}”?`)) return;
    setMsg("");
    try { await deleteJob(job.key); onChanged(); }
    catch (e) { setMsg(apiError(e) || "Could not delete."); }
  }

  return (
    <div className="bg-panel rounded-xl border border-line p-5 space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5 flex-wrap">
            <h3 className="font-semibold text-ink">{job.name}</h3>
            <StatusBadge status={job.last_status} />
          </div>
          <p className="text-xs text-subtle mt-1">
            {scheduleSummary(job)}
            {job.last_run_at ? <> · Last sent {new Date(job.last_run_at).toLocaleString()}</> : <> · Never sent</>}
            {job.enabled && job.next_run_at && job.schedule_kind !== "manual" && (
              <> · Next {new Date(job.next_run_at).toLocaleString()}</>
            )}
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button onClick={run} disabled={running} className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-3 py-1.5 rounded-lg disabled:opacity-50">
            {running ? <RefreshCw size={14} className="animate-spin" /> : <Play size={14} />}
            {running ? "Sending…" : "Send now"}
          </button>
          <button onClick={onEdit} title="Edit" className="text-subtle hover:text-ink p-1.5 rounded-lg hover:bg-panel-2"><Pencil size={15} /></button>
          <button onClick={remove} title="Delete" className="text-subtle hover:text-danger p-1.5 rounded-lg hover:bg-panel-2"><Trash2 size={15} /></button>
        </div>
      </div>
      {job.last_message && <p className={`text-sm ${job.last_status === "failed" ? "text-danger" : "text-ink"}`}>{job.last_message}</p>}
      {res && job.last_status !== "running" && (typeof res.sent === "number") && (
        <div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-subtle">
          <span><span className="text-emerald-600 font-medium">{res.sent}</span> sent</span>
          {typeof res.failed === "number" && res.failed > 0 && <span className="text-danger">{res.failed} failed</span>}
        </div>
      )}
      {msg && <p className="text-sm text-danger">{msg}</p>}
    </div>
  );
}

function apiError(e: unknown): string {
  return (e as { response?: { data?: { error?: string } } })?.response?.data?.error || "";
}
