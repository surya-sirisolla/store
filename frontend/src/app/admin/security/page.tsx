"use client";
import { useState } from "react";
import { changePassword } from "@/lib/api";
import { ShieldCheck, KeyRound, CheckCircle } from "lucide-react";

const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";

export default function SecurityPage() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  const tooShort = next.length > 0 && next.length < 8;
  const mismatch = confirm.length > 0 && next !== confirm;
  const canSubmit = current.length > 0 && next.length >= 8 && next === confirm;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(""); setDone(false);
    if (!canSubmit) return;
    setBusy(true);
    try {
      await changePassword(current, next);
      setDone(true);
      setCurrent(""); setNext(""); setConfirm("");
      setTimeout(() => setDone(false), 4000);
    } catch (err: unknown) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || "Could not change the password.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="flex items-center gap-2.5 mb-1">
        <ShieldCheck size={22} className="text-accent" />
        <h1 className="text-2xl font-bold tracking-tight">Security</h1>
      </div>
      <p className="text-sm text-subtle mb-6">Change the password used to log in to this console.</p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          <form onSubmit={submit} className="bg-panel rounded-xl border border-line p-5 lg:p-6 space-y-4 max-w-md">
            <div className="flex items-center gap-2 mb-1">
              <KeyRound size={17} className="text-accent" />
              <h2 className="font-semibold text-ink">Console password</h2>
            </div>

            <div>
              <label className="block text-sm font-medium text-muted mb-1">Current password</label>
              <input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} className={inputCls} />
            </div>
            <div>
              <label className="block text-sm font-medium text-muted mb-1">New password</label>
              <input type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} className={inputCls} />
              {tooShort && <p className="text-xs text-danger mt-1">At least 8 characters.</p>}
            </div>
            <div>
              <label className="block text-sm font-medium text-muted mb-1">Confirm new password</label>
              <input type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} className={inputCls} />
              {mismatch && <p className="text-xs text-danger mt-1">Passwords don&apos;t match.</p>}
            </div>

            {error && <p className="text-sm text-danger">{error}</p>}

            <div className="flex items-center gap-3 pt-1">
              <button type="submit" disabled={busy || !canSubmit} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-5 py-2 rounded-lg disabled:opacity-50">
                {busy ? "Updating…" : "Update password"}
              </button>
              {done && <span className="flex items-center gap-1 text-accent text-sm"><CheckCircle size={15} /> Password updated</span>}
            </div>
          </form>
        </div>

        <div className="space-y-5">
          <div className="bg-panel rounded-xl border border-line p-5">
            <h2 className="font-semibold text-ink text-sm mb-2">Good to know</h2>
            <ul className="text-sm text-muted space-y-1.5 list-disc pl-5 marker:text-subtle">
              <li>The password is stored hashed (bcrypt) in the database — never in plain text.</li>
              <li>Changing it doesn&apos;t sign out your current session.</li>
              <li>Login is rate-limited to slow down guessing.</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
