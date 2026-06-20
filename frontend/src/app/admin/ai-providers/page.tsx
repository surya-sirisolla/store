"use client";
import { useEffect, useState, useCallback } from "react";
import { getLLMKeys, setLLMKeys, deleteLLMKeys, detectLocalLLM } from "@/lib/api";
import { KeyRound, CheckCircle, Trash2, Pencil, Bot, Zap, Cpu, Server, Sparkles, Cloud, RefreshCw } from "lucide-react";

type ProviderId = "claude" | "gemini" | "openai" | "local";

const PROVIDERS: { id: ProviderId; label: string; placeholder: string; icon: React.ReactNode; cloud: boolean }[] = [
  { id: "claude", label: "Claude", placeholder: "sk-ant-...", icon: <Sparkles size={16} />, cloud: true },
  { id: "gemini", label: "Gemini", placeholder: "AIza...", icon: <Cloud size={16} />, cloud: true },
  { id: "openai", label: "OpenAI", placeholder: "sk-...", icon: <Bot size={16} />, cloud: true },
  { id: "local", label: "Local LLM", placeholder: "", icon: <Cpu size={16} />, cloud: false },
];
const meta = (id: string) => PROVIDERS.find((p) => p.id === id);
const providerLabel = (id?: string) => meta(id ?? "")?.label ?? id ?? "";

interface LocalEndpoint { name: string; base_url: string; models: string[] }
interface LocalInfo { available: boolean; endpoints: LocalEndpoint[] }

interface Sel { provider: ProviderId; key: string; baseURL: string; model: string }
interface KeyView { provider: ProviderId; masked: string; base_url?: string; model?: string }
interface KeyInfo {
  configured: boolean;
  source: "none" | "console" | "environment";
  primary: KeyView | null;
  fallback: KeyView | null;
}

const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";
const emptySel: Sel = { provider: "claude", key: "", baseURL: "", model: "" };

// One field: a neat single-select provider picker + the config for the choice.
function ProviderField({
  label, value, onChange, local, exclude,
}: {
  label: string; value: Sel; onChange: (s: Sel) => void; local: LocalInfo | null; exclude?: ProviderId;
}) {
  const m = meta(value.provider);
  const localEp = local?.endpoints[0];

  function pick(p: ProviderId) {
    if (p === "local") {
      onChange({ provider: "local", key: "", baseURL: localEp?.base_url ?? value.baseURL, model: localEp?.models?.[0] ?? value.model });
    } else {
      onChange({ ...value, provider: p });
    }
  }

  return (
    <div className="space-y-3">
      <p className="text-sm font-medium text-ink">{label}</p>

      {/* Single-select provider chips */}
      <div className="grid grid-cols-4 gap-2">
        {PROVIDERS.map((p) => {
          const disabled = (exclude && p.id === exclude) || (p.id === "local" && !local?.available);
          const selected = value.provider === p.id;
          return (
            <button
              key={p.id}
              type="button"
              disabled={disabled}
              onClick={() => pick(p.id)}
              className={`relative flex flex-col items-start gap-1.5 rounded-lg border px-3 py-2.5 text-left transition disabled:opacity-40 disabled:cursor-not-allowed ${
                selected ? "border-accent/60 bg-accent/10" : "border-line bg-panel-2 hover:border-line-2"
              }`}
            >
              <span className={`flex items-center gap-1.5 text-sm font-medium ${selected ? "text-accent" : "text-ink"}`}>
                {p.icon} {p.label}
              </span>
              <span className="text-[11px] text-subtle">
                {p.id === "local"
                  ? (local?.available ? `${localEp?.name} detected` : "not detected")
                  : "cloud API"}
              </span>
              {selected && <CheckCircle size={13} className="absolute top-2 right-2 text-accent" />}
            </button>
          );
        })}
      </div>

      {/* Config for the selected provider */}
      {m?.cloud ? (
        <input
          type="password" autoComplete="off" placeholder={`${m.label} API key — ${m.placeholder}`}
          value={value.key} onChange={(e) => onChange({ ...value, key: e.target.value })}
          className={`${inputCls} font-mono`}
        />
      ) : (
        <div className="space-y-2 bg-panel-2/50 border border-line rounded-lg p-3">
          <div className="flex items-center gap-2 text-xs text-muted">
            <Server size={13} /> {local?.available ? "Using your local server — no API key, no cost." : "No local server detected. Start Ollama/LM Studio, or enter details manually."}
          </div>
          <div className="grid grid-cols-2 gap-2">
            <input
              placeholder="Base URL — http://host.docker.internal:11434/v1"
              value={value.baseURL} onChange={(e) => onChange({ ...value, baseURL: e.target.value })}
              className={`${inputCls} font-mono text-xs`}
            />
            {localEp?.models?.length ? (
              <select value={value.model} onChange={(e) => onChange({ ...value, model: e.target.value })} className={inputCls}>
                <option value="">Select a model…</option>
                {localEp.models.map((mo) => <option key={mo} value={mo}>{mo}</option>)}
              </select>
            ) : (
              <input placeholder="Model id — e.g. llama3.1" value={value.model} onChange={(e) => onChange({ ...value, model: e.target.value })} className={`${inputCls} font-mono text-xs`} />
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function ProviderRow({ role, view }: { role: string; view: KeyView }) {
  const isLocal = view.provider === "local";
  return (
    <div className="flex items-center justify-between gap-3 py-2.5">
      <div className="flex items-center gap-2.5 min-w-0">
        <span className="text-accent shrink-0">{meta(view.provider)?.icon}</span>
        <div className="min-w-0">
          <p className="text-sm text-ink truncate">
            <span className="text-subtle">{role}</span> · {providerLabel(view.provider)}
          </p>
          <p className="text-xs text-subtle font-mono truncate">
            {isLocal ? `${view.model} · ${view.base_url}` : view.masked}
          </p>
        </div>
      </div>
      {isLocal ? (
        <span className="text-[11px] bg-accent/15 text-accent rounded-full px-2 py-0.5 shrink-0">Local · free</span>
      ) : (
        <span className="text-[11px] bg-panel-2 text-subtle border border-line rounded-full px-2 py-0.5 shrink-0">Cloud</span>
      )}
    </div>
  );
}

function ProvidersForm({ keyInfo, local, onSaved }: { keyInfo: KeyInfo | null; local: LocalInfo | null; onSaved: () => void }) {
  const [editing, setEditing] = useState(false);
  const [primary, setPrimary] = useState<Sel>(emptySel);
  const [useFallback, setUseFallback] = useState(false);
  const [fallback, setFallback] = useState<Sel>({ ...emptySel, provider: "gemini" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const configured = keyInfo?.configured;
  const showForm = !configured || editing;

  function reset() {
    setPrimary(emptySel); setUseFallback(false); setFallback({ ...emptySel, provider: "gemini" }); setError("");
  }

  function toPayload(s: Sel) {
    return s.provider === "local"
      ? { provider: "local", base_url: s.baseURL.trim(), model: s.model.trim() }
      : { provider: s.provider, key: s.key.trim() };
  }
  function valid(s: Sel) {
    return s.provider === "local" ? s.baseURL.trim().length > 0 && s.model.trim().length > 0 : s.key.trim().length >= 10;
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (useFallback && valid(fallback) && fallback.provider === primary.provider) {
      setError("The secondary must use a different provider than the primary.");
      return;
    }
    setBusy(true);
    try {
      await setLLMKeys({ primary: toPayload(primary), fallback: useFallback && valid(fallback) ? toPayload(fallback) : null });
      reset(); setEditing(false); onSaved();
    } catch (err: unknown) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || "Could not save");
    } finally { setBusy(false); }
  }

  async function remove() {
    if (!confirm("Remove the saved providers? The bot will stop until you add one.")) return;
    setBusy(true); await deleteLLMKeys(); setBusy(false); onSaved();
  }

  return (
    <div className="bg-panel rounded-xl border border-line p-5 lg:p-6">
      <div className="flex items-center gap-2 mb-1">
        <KeyRound size={17} className="text-accent" />
        <h2 className="font-semibold text-ink">Providers</h2>
      </div>
      <p className="text-sm text-muted mb-4">
        Pick a <span className="text-ink">primary</span> provider and an optional <span className="text-ink">secondary</span>
        {" "}fallback. The bot uses the primary and automatically switches to the secondary if it fails.
      </p>

      {configured && !editing && (
        <div className="bg-panel-2 rounded-lg border border-line px-4 divide-y divide-line">
          <ProviderRow role="Primary" view={keyInfo!.primary!} />
          {keyInfo?.fallback
            ? <ProviderRow role="Secondary" view={keyInfo.fallback} />
            : <p className="text-xs text-subtle py-2.5">No secondary configured</p>}
        </div>
      )}

      {configured && !editing && (
        <div className="flex items-center justify-between mt-4">
          <p className="text-xs text-subtle">Source: {keyInfo?.source === "console" ? "set here in the console" : "environment (.env)"}</p>
          <div className="flex gap-4">
            <button onClick={() => setEditing(true)} className="flex items-center gap-1 text-sm text-accent hover:underline"><Pencil size={13} /> Edit</button>
            {keyInfo?.source === "console" && (
              <button onClick={remove} disabled={busy} className="flex items-center gap-1 text-sm text-danger hover:underline"><Trash2 size={13} /> Delete</button>
            )}
          </div>
        </div>
      )}

      {showForm && (
        <form onSubmit={save} className="space-y-6">
          {error && <p className="text-danger text-sm">{error}</p>}

          <ProviderField label="Primary provider" value={primary} onChange={setPrimary} local={local} exclude={useFallback ? fallback.provider : undefined} />

          {useFallback ? (
            <div className="space-y-3 border-t border-line pt-5">
              <ProviderField label="Secondary provider (fallback)" value={fallback} onChange={setFallback} local={local} exclude={primary.provider} />
              <button type="button" onClick={() => setUseFallback(false)} className="text-xs text-subtle hover:text-ink">Remove secondary</button>
            </div>
          ) : (
            <button type="button" onClick={() => setUseFallback(true)} className="text-sm text-accent hover:underline">+ Add a secondary provider</button>
          )}

          <div className="flex gap-2 pt-1">
            <button type="submit" disabled={busy || !valid(primary)} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50">{busy ? "Saving…" : "Save & start bot"}</button>
            {configured && <button type="button" onClick={() => { setEditing(false); reset(); }} className="text-sm px-4 py-2 rounded-lg border border-line text-muted hover:text-ink">Cancel</button>}
          </div>
        </form>
      )}
    </div>
  );
}

function LocalStatusCard({ local, onRescan, scanning }: { local: LocalInfo | null; onRescan: () => void; scanning: boolean }) {
  const ep = local?.endpoints[0];
  return (
    <div className="bg-panel rounded-xl border border-line p-5">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <Cpu size={16} className="text-accent" />
          <h2 className="font-semibold text-ink text-sm">Local LLM</h2>
        </div>
        <button onClick={onRescan} disabled={scanning} className="text-subtle hover:text-ink disabled:opacity-50" aria-label="Re-scan">
          <RefreshCw size={14} className={scanning ? "animate-spin" : ""} />
        </button>
      </div>
      {local?.available ? (
        <div className="space-y-1.5">
          <p className="text-sm text-ink flex items-center gap-1.5"><span className="w-1.5 h-1.5 rounded-full bg-accent" /> {ep?.name} detected</p>
          <p className="text-xs text-subtle font-mono truncate">{ep?.base_url}</p>
          <p className="text-xs text-muted">{ep?.models.length} model{ep?.models.length === 1 ? "" : "s"} available</p>
        </div>
      ) : (
        <p className="text-sm text-muted">No local server running. Start Ollama or LM Studio, then re-scan to use it — free, no API key.</p>
      )}
    </div>
  );
}

export default function AIProvidersPage() {
  const [keyInfo, setKeyInfo] = useState<KeyInfo | null>(null);
  const [local, setLocal] = useState<LocalInfo | null>(null);
  const [scanning, setScanning] = useState(false);

  const loadKey = useCallback(async () => {
    try { const r = await getLLMKeys(); setKeyInfo(r.data); } catch { /* ignore */ }
  }, []);
  const loadLocal = useCallback(async () => {
    setScanning(true);
    try { const r = await detectLocalLLM(); setLocal(r.data); } catch { setLocal({ available: false, endpoints: [] }); }
    finally { setScanning(false); }
  }, []);

  useEffect(() => { loadKey(); loadLocal(); }, [loadKey, loadLocal]);

  return (
    <div>
      <div className="flex items-center gap-2.5 mb-1">
        <Bot size={22} className="text-accent" />
        <h1 className="text-2xl font-bold tracking-tight">AI Providers</h1>
      </div>
      <p className="text-sm text-subtle mb-6">The AI models that power your WhatsApp bot — cloud or your own local server.</p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          <ProvidersForm keyInfo={keyInfo} local={local} onSaved={loadKey} />
        </div>

        <div className="space-y-5">
          <LocalStatusCard local={local} onRescan={loadLocal} scanning={scanning} />

          <div className="bg-panel rounded-xl border border-line p-5">
            <div className="flex items-center gap-2 mb-2">
              <Zap size={16} className="text-accent" />
              <h2 className="font-semibold text-ink text-sm">How it works</h2>
            </div>
            <ul className="text-sm text-muted space-y-1.5 list-disc pl-5 marker:text-subtle">
              <li>The bot uses your <span className="text-ink">primary</span> provider for every reply.</li>
              <li>If the primary fails, it automatically retries on the <span className="text-ink">secondary</span>.</li>
              <li><span className="text-ink">Local LLM</span> runs on your machine — no API key, no per-message cost.</li>
              <li>Changing this restarts the bot within seconds — no QR re-scan needed.</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
