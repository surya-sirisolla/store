"use client";
import { useEffect, useState } from "react";
import { getStaff, createStaff, deleteStaff } from "@/lib/api";
import { Plus, Trash2, X } from "lucide-react";

interface Staff {
  id: number;
  name: string;
  email: string;
  phone: string;
  active: boolean;
}

const DIAL_CODES = ["+91", "+1", "+44", "+61", "+971", "+65", "+49", "+33"];
const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";

export default function StaffPage() {
  const [staff, setStaff] = useState<Staff[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", email: "" });
  const [dialCode, setDialCode] = useState("+91");
  const [localNumber, setLocalNumber] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const load = () => getStaff().then((r) => setStaff(r.data || []));
  useEffect(() => { load(); }, []);

  function resetForm() {
    setForm({ name: "", email: "" });
    setDialCode("+91");
    setLocalNumber("");
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSaving(true);
    try {
      const phone = `${dialCode}${localNumber.replace(/[^0-9]/g, "")}`;
      await createStaff({ ...form, phone });
      resetForm();
      setShowForm(false);
      load();
    } catch (err: unknown) {
      const ax = err as { response?: { data?: { error?: string } } };
      setError(ax.response?.data?.error || "Error creating staff");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    if (!confirm("Remove this staff member?")) return;
    await deleteStaff(id);
    load();
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Staff</h1>
          <p className="text-sm text-subtle mt-0.5">{staff.length} member{staff.length === 1 ? "" : "s"}</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg transition">
          <Plus size={16} /> Add Staff
        </button>
      </div>
      <p className="text-sm text-muted mb-6">Staff don&apos;t log in here — adding someone just registers their WhatsApp number so the bot recognises them for staff-only answers (e.g. who messaged today, pending alerts).</p>

      {showForm && (
        <form onSubmit={submit} className="bg-panel rounded-xl border border-line p-5 mb-6 space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-ink">New Staff Member</h2>
            <button type="button" onClick={() => setShowForm(false)} className="text-subtle hover:text-ink"><X size={16} /></button>
          </div>
          {error && <p className="text-danger text-sm">{error}</p>}
          <input required placeholder="Full Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className={inputCls} />
          <input required type="email" placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} className={inputCls} />
          <div>
            <div className="flex gap-2">
              <select value={dialCode} onChange={(e) => setDialCode(e.target.value)} className="bg-panel-2 border border-line rounded-lg px-2 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40">
                {DIAL_CODES.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
              <input required inputMode="numeric" placeholder="WhatsApp number" value={localNumber} onChange={(e) => setLocalNumber(e.target.value)} className={`flex-1 ${inputCls}`} />
            </div>
            <p className="text-xs text-subtle mt-1">Will be saved as <span className="font-mono text-muted">{dialCode}{localNumber.replace(/[^0-9]/g, "") || "…"}</span></p>
          </div>
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50">{saving ? "Saving…" : "Create"}</button>
            <button type="button" onClick={() => { setShowForm(false); resetForm(); }} className="text-sm px-4 py-2 rounded-lg border border-line text-muted hover:text-ink">Cancel</button>
          </div>
        </form>
      )}

      <div className="bg-panel rounded-xl border border-line overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-panel-2 text-subtle text-xs uppercase tracking-wide">
            <tr>
              <th className="text-left px-5 py-3 font-medium">Name</th>
              <th className="text-left px-5 py-3 font-medium">Email</th>
              <th className="text-left px-5 py-3 font-medium">WhatsApp</th>
              <th className="px-5 py-3"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {staff.map((u) => (
              <tr key={u.id} className="hover:bg-panel-2/60 transition">
                <td className="px-5 py-3 font-medium text-ink">{u.name}</td>
                <td className="px-5 py-3 text-muted">{u.email}</td>
                <td className="px-5 py-3 text-muted font-mono">{u.phone || "—"}</td>
                <td className="px-5 py-3 text-right"><button onClick={() => handleDelete(u.id)} className="text-subtle hover:text-danger transition"><Trash2 size={15} /></button></td>
              </tr>
            ))}
            {staff.length === 0 && <tr><td colSpan={4} className="px-5 py-10 text-center text-subtle">No staff yet.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
