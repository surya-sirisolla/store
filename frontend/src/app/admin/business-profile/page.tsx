"use client";
import { useEffect, useState } from "react";
import { getBusinessProfile, updateBusinessProfile } from "@/lib/api";
import {
  Save, CheckCircle, Pencil, X, Building2, MapPin, Phone, MessageCircle,
  Clock, Tag, Info, Bot,
} from "lucide-react";

interface Profile {
  name: string;
  about: string;
  address: string;
  area: string;
  city: string;
  state: string;
  pincode: string;
  phones: string[];
  whatsapp: string;
  hours: string;
  services: string[];
}

const empty: Profile = {
  name: "", about: "", address: "", area: "", city: "", state: "",
  pincode: "", phones: [], whatsapp: "", hours: "", services: [],
};

const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";

function Field({ label, value, onChange, placeholder }: { label: string; value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <div>
      <label className="block text-sm font-medium text-muted mb-1">{label}</label>
      <input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className={inputCls} />
    </div>
  );
}

function InfoRow({ icon, label, children }: { icon: React.ReactNode; label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3 py-3">
      <div className="grid place-items-center w-8 h-8 rounded-lg bg-panel-2 text-accent shrink-0">{icon}</div>
      <div className="min-w-0">
        <p className="text-xs text-subtle">{label}</p>
        <div className="text-sm text-ink mt-0.5">{children}</div>
      </div>
    </div>
  );
}

function ProfileView({ p, onEdit }: { p: Profile; onEdit: () => void }) {
  const addressLine = [p.address, p.area, p.city, p.state, p.pincode].filter(Boolean).join(", ");
  return (
    <div className="bg-panel rounded-xl border border-line p-6">
      <div className="flex items-start justify-between gap-3 mb-1">
        <div className="flex items-center gap-3 min-w-0">
          <div className="grid place-items-center w-11 h-11 rounded-xl bg-accent/15 text-accent shrink-0">
            <Building2 size={20} />
          </div>
          <div className="min-w-0">
            <h2 className="text-lg font-semibold text-ink truncate">{p.name || "Unnamed business"}</h2>
            {p.about && <p className="text-sm text-muted mt-0.5">{p.about}</p>}
          </div>
        </div>
        <button onClick={onEdit} className="flex items-center gap-1.5 text-sm text-accent hover:underline shrink-0">
          <Pencil size={13} /> Edit
        </button>
      </div>

      <div className="divide-y divide-line mt-3">
        <InfoRow icon={<MapPin size={15} />} label="Address">
          {addressLine || <span className="text-subtle">Not set</span>}
        </InfoRow>
        <InfoRow icon={<Phone size={15} />} label="Phone numbers">
          {p.phones.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {p.phones.map((ph) => (
                <span key={ph} className="font-mono text-xs bg-panel-2 border border-line rounded px-2 py-0.5">{ph}</span>
              ))}
            </div>
          ) : <span className="text-subtle">Not set</span>}
        </InfoRow>
        <InfoRow icon={<MessageCircle size={15} />} label="WhatsApp number">
          {p.whatsapp || <span className="text-subtle">Not set</span>}
        </InfoRow>
        <InfoRow icon={<Clock size={15} />} label="Opening hours">
          {p.hours || <span className="text-subtle">Not set</span>}
        </InfoRow>
        <InfoRow icon={<Tag size={15} />} label="Services">
          {p.services.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {p.services.map((s) => (
                <span key={s} className="text-xs bg-panel-2 text-muted border border-line rounded-full px-2.5 py-0.5">{s}</span>
              ))}
            </div>
          ) : <span className="text-subtle">Not set</span>}
        </InfoRow>
      </div>
    </div>
  );
}

function ProfileForm({
  initial, onSave, onCancel, saving,
}: {
  initial: Profile; onSave: (p: Profile) => void; onCancel?: () => void; saving: boolean;
}) {
  const [p, setP] = useState<Profile>(initial);

  function set<K extends keyof Profile>(k: K, v: Profile[K]) {
    setP((prev) => ({ ...prev, [k]: v }));
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    onSave(p);
  }

  return (
    <form onSubmit={submit} className="bg-panel rounded-xl border border-line p-6 space-y-4">
      <Field label="Business Name *" value={p.name} onChange={(v) => set("name", v)} placeholder="Adonai Electronics" />
      <div>
        <label className="block text-sm font-medium text-muted mb-1">About</label>
        <textarea value={p.about} onChange={(e) => set("about", e.target.value)} rows={2} className={inputCls} placeholder="Short description of your business" />
      </div>
      <Field label="Address" value={p.address} onChange={(v) => set("address", v)} />
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
        <Field label="Area" value={p.area} onChange={(v) => set("area", v)} />
        <Field label="City" value={p.city} onChange={(v) => set("city", v)} />
        <Field label="State" value={p.state} onChange={(v) => set("state", v)} />
        <Field label="Pincode" value={p.pincode} onChange={(v) => set("pincode", v)} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="WhatsApp Number" value={p.whatsapp} onChange={(v) => set("whatsapp", v)} placeholder="+91 98765 43210" />
        <Field label="Opening Hours" value={p.hours} onChange={(v) => set("hours", v)} placeholder="Mon–Sat 9am–8pm" />
      </div>
      <Field label="Phones (comma separated)" value={p.phones.join(", ")} onChange={(v) => set("phones", v.split(",").map((s) => s.trim()).filter(Boolean))} />
      <Field label="Services (comma separated)" value={p.services.join(", ")} onChange={(v) => set("services", v.split(",").map((s) => s.trim()).filter(Boolean))} placeholder="Repairs, Installation, Delivery" />

      <div className="flex items-center gap-3 pt-2">
        <button type="submit" disabled={saving} className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-5 py-2 rounded-lg disabled:opacity-50">
          <Save size={16} /> {saving ? "Saving…" : "Save Profile"}
        </button>
        {onCancel && (
          <button type="button" onClick={onCancel} className="flex items-center gap-1.5 text-sm px-4 py-2 rounded-lg border border-line text-muted hover:text-ink">
            <X size={14} /> Cancel
          </button>
        )}
      </div>
    </form>
  );
}

export default function BusinessProfilePage() {
  const [p, setP] = useState<Profile>(empty);
  const [loaded, setLoaded] = useState(false);
  const [editing, setEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [justSaved, setJustSaved] = useState(false);

  useEffect(() => {
    getBusinessProfile().then((r) => {
      const data = r.data && r.data.name !== undefined
        ? { ...empty, ...r.data, phones: r.data.phones || [], services: r.data.services || [] }
        : empty;
      setP(data);
      setEditing(!data.name); // no profile yet -> go straight to the form
      setLoaded(true);
    });
  }, []);

  async function save(next: Profile) {
    setSaving(true);
    await updateBusinessProfile(next);
    setP(next);
    setSaving(false);
    setEditing(false);
    setJustSaved(true);
    setTimeout(() => setJustSaved(false), 3000);
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-1">
        <h1 className="text-2xl font-bold tracking-tight">Business Profile</h1>
        {justSaved && <span className="flex items-center gap-1 text-accent text-sm"><CheckCircle size={15} /> Saved</span>}
      </div>
      <p className="text-sm text-subtle mb-6">
        This is what the WhatsApp bot tells customers about your business (address, timings, contact).
      </p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          {!loaded ? (
            <div className="bg-panel rounded-xl border border-line p-6 text-sm text-subtle">Loading…</div>
          ) : editing ? (
            <ProfileForm initial={p} onSave={save} onCancel={p.name ? () => setEditing(false) : undefined} saving={saving} />
          ) : (
            <ProfileView p={p} onEdit={() => setEditing(true)} />
          )}
        </div>

        <div className="space-y-5">
          <div className="bg-panel rounded-xl border border-line p-5">
            <div className="flex items-center gap-2 mb-2">
              <Bot size={16} className="text-accent" />
              <h2 className="font-semibold text-ink text-sm">Why this matters</h2>
            </div>
            <ul className="text-sm text-muted space-y-1.5 list-disc pl-5 marker:text-subtle">
              <li>Your WhatsApp bot answers customers using exactly this info.</li>
              <li>Keep address &amp; hours current — it&apos;s the first thing customers ask.</li>
              <li>Services help the bot understand what you offer beyond your listings.</li>
            </ul>
          </div>
          <div className="bg-panel rounded-xl border border-line p-5">
            <div className="flex items-center gap-2 mb-2">
              <Info size={16} className="text-accent" />
              <h2 className="font-semibold text-ink text-sm">Tip</h2>
            </div>
            <p className="text-sm text-muted">List every phone number customers might call — the bot mentions all of them when asked.</p>
          </div>
        </div>
      </div>
    </div>
  );
}
