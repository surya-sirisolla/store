"use client";
import { useEffect, useState } from "react";
import {
  getBusinessProfile, updateBusinessProfile,
  getBusinessLocations, updateBusinessLocation, type BusinessLocation,
} from "@/lib/api";
import {
  Save, CheckCircle, Pencil, X, Building2, Mail, Phone,
  Clock, Info, Bot, Warehouse, MapPin,
} from "lucide-react";

interface Profile {
  name: string;
  email: string;
  phones: string[];
  hours: string;
}

const empty: Profile = { name: "", email: "", phones: [], hours: "" };

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
  return (
    <div className="bg-panel rounded-xl border border-line p-6">
      <div className="flex items-start justify-between gap-3 mb-1">
        <div className="flex items-center gap-3 min-w-0">
          <div className="grid place-items-center w-11 h-11 rounded-xl bg-accent/15 text-accent shrink-0">
            <Building2 size={20} />
          </div>
          <div className="min-w-0">
            <h2 className="text-lg font-semibold text-ink truncate">{p.name || "Unnamed business"}</h2>
          </div>
        </div>
        <button onClick={onEdit} className="flex items-center gap-1.5 text-sm text-accent hover:underline shrink-0">
          <Pencil size={13} /> Edit
        </button>
      </div>

      <div className="divide-y divide-line mt-3">
        <InfoRow icon={<Mail size={15} />} label="Email">
          {p.email || <span className="text-subtle">Not set</span>}
        </InfoRow>
        <InfoRow icon={<Phone size={15} />} label="Mobile numbers">
          {p.phones.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {p.phones.map((ph) => (
                <span key={ph} className="font-mono text-xs bg-panel-2 border border-line rounded px-2 py-0.5">{ph}</span>
              ))}
            </div>
          ) : <span className="text-subtle">Not set</span>}
        </InfoRow>
        <InfoRow icon={<Clock size={15} />} label="Opening hours">
          {p.hours || <span className="text-subtle">Not set</span>}
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
      <Field label="Email" value={p.email} onChange={(v) => set("email", v)} placeholder="hello@business.com" />
      <Field label="Mobile numbers (comma separated)" value={p.phones.join(", ")} onChange={(v) => set("phones", v.split(",").map((s) => s.trim()).filter(Boolean))} placeholder="+91 98765 43210" />
      <Field label="Opening Hours" value={p.hours} onChange={(v) => set("hours", v)} placeholder="Mon–Sat 9am–8pm" />

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

// composeAddress renders a location's structured parts into one readable line.
function composeAddress(l: BusinessLocation): string {
  return [l.address, l.area, l.city, l.state, l.pincode].map((s) => (s || "").trim()).filter(Boolean).join(", ");
}

// LocationRow shows one branch and lets the owner edit its full address inline.
function LocationRow({
  loc, onSaved,
}: {
  loc: BusinessLocation; onSaved: (l: BusinessLocation) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(loc);
  const [saving, setSaving] = useState(false);

  function start() {
    setDraft(loc);
    setEditing(true);
  }
  function setF<K extends keyof BusinessLocation>(k: K, v: BusinessLocation[K]) {
    setDraft((prev) => ({ ...prev, [k]: v }));
  }
  async function save() {
    setSaving(true);
    try {
      const r = await updateBusinessLocation(loc.id, {
        name: draft.name, address: draft.address, area: draft.area,
        city: draft.city, state: draft.state, pincode: draft.pincode, phone: draft.phone,
      });
      onSaved(r.data);
      setEditing(false);
    } finally {
      setSaving(false);
    }
  }

  const full = composeAddress(loc);

  return (
    <div className="flex items-start gap-3 py-3">
      <div className="grid place-items-center w-8 h-8 rounded-lg bg-panel-2 text-accent shrink-0">
        <MapPin size={15} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <p className="text-sm font-medium text-ink">{loc.name}</p>
          {!editing && (
            <button onClick={start} className="flex items-center gap-1 text-xs text-accent hover:underline shrink-0">
              <Pencil size={12} /> {full ? "Edit" : "Add address"}
            </button>
          )}
        </div>

        {editing ? (
          <div className="mt-2 space-y-2">
            <Field label="Address" value={draft.address} onChange={(v) => setF("address", v)} placeholder="Shop / building, street" />
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
              <Field label="Area" value={draft.area} onChange={(v) => setF("area", v)} />
              <Field label="City" value={draft.city} onChange={(v) => setF("city", v)} />
              <Field label="State" value={draft.state} onChange={(v) => setF("state", v)} />
              <Field label="Pincode" value={draft.pincode} onChange={(v) => setF("pincode", v)} />
            </div>
            <Field label="Branch phone (optional)" value={draft.phone} onChange={(v) => setF("phone", v)} placeholder="+91 98765 43210" />
            <div className="flex items-center gap-2 pt-1">
              <button onClick={save} disabled={saving} className="flex items-center gap-1 bg-accent text-accent-ink hover:bg-accent-strong text-xs font-medium px-3 py-2 rounded-lg disabled:opacity-50">
                <Save size={13} /> {saving ? "Saving…" : "Save"}
              </button>
              <button onClick={() => setEditing(false)} className="flex items-center gap-1 text-xs px-3 py-2 rounded-lg border border-line text-muted hover:text-ink">
                <X size={13} /> Cancel
              </button>
            </div>
          </div>
        ) : (
          <div className="mt-0.5 text-sm text-ink">
            {full || <span className="text-subtle">No address set</span>}
            {loc.phone && <span className="block text-xs text-muted mt-0.5 font-mono">{loc.phone}</span>}
          </div>
        )}
      </div>
    </div>
  );
}

// LocationsCard lists the business's physical locations (godowns synced from
// Livekeeping) and lets the owner attach/edit a full address for each one.
function LocationsCard() {
  const [locs, setLocs] = useState<BusinessLocation[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    getBusinessLocations()
      .then((r) => setLocs(Array.isArray(r.data) ? r.data : []))
      .finally(() => setLoaded(true));
  }, []);

  function onSaved(l: BusinessLocation) {
    setLocs((prev) => prev.map((x) => (x.id === l.id ? l : x)));
  }

  return (
    <div className="bg-panel rounded-xl border border-line p-6 mt-6">
      <div className="flex items-center gap-2 mb-1">
        <Warehouse size={16} className="text-accent" />
        <h2 className="font-semibold text-ink text-sm">Locations</h2>
      </div>
      <p className="text-xs text-subtle mb-3">
        Branches synced from Livekeeping. Add the full address for each — the bot uses these to tell customers which shop stocks an item and where it is.
      </p>

      {!loaded ? (
        <p className="text-sm text-subtle">Loading…</p>
      ) : locs.length === 0 ? (
        <p className="text-sm text-subtle">
          No locations yet. Run a Livekeeping sync (Scheduled Jobs → Livekeeping stock sync) and your branches will appear here to address.
        </p>
      ) : (
        <div className="divide-y divide-line">
          {locs.map((l) => (
            <LocationRow key={l.id} loc={l} onSaved={onSaved} />
          ))}
        </div>
      )}
    </div>
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
        ? { ...empty, ...r.data, phones: r.data.phones || [] }
        : empty;
      setP({ name: data.name, email: data.email || "", phones: data.phones, hours: data.hours || "" });
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
        Your business&apos;s contact details and branch locations — this is what the WhatsApp bot tells customers.
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
          <LocationsCard />
        </div>

        <div className="space-y-5">
          <div className="bg-panel rounded-xl border border-line p-5">
            <div className="flex items-center gap-2 mb-2">
              <Bot size={16} className="text-accent" />
              <h2 className="font-semibold text-ink text-sm">Why this matters</h2>
            </div>
            <ul className="text-sm text-muted space-y-1.5 list-disc pl-5 marker:text-subtle">
              <li>The main profile is your business&apos;s contact card — name, email, mobile and hours.</li>
              <li>Each location&apos;s address lets the bot say exactly which shop has an item.</li>
              <li>Keep addresses current — it&apos;s the first thing customers ask.</li>
            </ul>
          </div>
          <div className="bg-panel rounded-xl border border-line p-5">
            <div className="flex items-center gap-2 mb-2">
              <Info size={16} className="text-accent" />
              <h2 className="font-semibold text-ink text-sm">Tip</h2>
            </div>
            <p className="text-sm text-muted">Locations come from your Livekeeping godowns. Run a sync, then fill in each branch&apos;s full address here.</p>
          </div>
        </div>
      </div>
    </div>
  );
}
