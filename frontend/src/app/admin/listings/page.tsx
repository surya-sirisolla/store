"use client";
import { useEffect, useState } from "react";
import {
  getListings, getCategories, createListing, updateListing, deleteListing,
} from "@/lib/api";
import Pagination from "@/components/Pagination";
import { Plus, Trash2, Search, X, Minus, Star } from "lucide-react";

const PAGE_SIZE = 15;

interface Listing {
  id: number;
  name: string;
  description: string;
  quantity?: number | null;
  price?: number | null;
  hsn_code?: string | null;
  unit?: string | null;
  featured?: boolean;
  offer_text?: string | null;
  category: { id: number; name: string };
  active: boolean;
}

interface Category { id: number; name: string; level: number; children?: Category[] }

function flatCats(cats: Category[], result: Category[] = []): Category[] {
  for (const c of cats) { result.push(c); if (c.children) flatCats(c.children, result); }
  return result;
}

export default function ListingsPage() {
  const [listings, setListings] = useState<Listing[]>([]);
  const [total, setTotal] = useState(0);
  const [cats, setCats] = useState<Category[]>([]);
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const [catFilter, setCatFilter] = useState("");
  const [stockFilter, setStockFilter] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ name: "", quantity: "", price: "", category_id: 0 });
  const [saving, setSaving] = useState(false);
  const [priceDrafts, setPriceDrafts] = useState<Record<number, string>>({});

  const searching = q.trim() !== "";

  const load = () => {
    getListings({ page, q, category_id: catFilter || undefined, stock: stockFilter || undefined }).then((r) => {
      setListings(r.data.data);
      setTotal(r.data.total);
    });
  };

  useEffect(() => { load(); }, [page, q, catFilter, stockFilter]);
  useEffect(() => { getCategories().then((r) => setCats(flatCats(r.data))); }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!form.category_id) return;
    setSaving(true);
    await createListing({
      name: form.name,
      category_id: form.category_id,
      quantity: form.quantity.trim() === "" ? undefined : Number(form.quantity),
      price: form.price.trim() === "" ? undefined : Number(form.price),
    });
    setForm({ name: "", quantity: "", price: "", category_id: 0 });
    setShowForm(false);
    setSaving(false);
    load();
  }

  async function handleDelete(id: number) {
    if (!confirm("Delete this listing?")) return;
    await deleteListing(id);
    load();
  }

  async function changeQuantity(l: Listing, delta: number) {
    const next = Math.max(0, (l.quantity ?? 0) + delta);
    setListings((prev) => prev.map((x) => (x.id === l.id ? { ...x, quantity: next } : x)));
    try {
      await updateListing(l.id, { quantity: next });
    } catch {
      load();
    }
  }

  function priceValue(l: Listing) {
    return priceDrafts[l.id] ?? (l.price != null ? String(l.price) : "");
  }

  async function commitPrice(l: Listing) {
    const raw = priceValue(l).trim();
    setPriceDrafts((d) => { const next = { ...d }; delete next[l.id]; return next; });
    if (raw === "") {
      if (l.price == null) return;
    }
    const next = raw === "" ? null : Number(raw);
    if (next !== null && Number.isNaN(next)) return;
    setListings((prev) => prev.map((x) => (x.id === l.id ? { ...x, price: next } : x)));
    try {
      await updateListing(l.id, { price: next });
    } catch {
      load();
    }
  }

  async function toggleFeatured(l: Listing) {
    const next = !l.featured;
    setListings((prev) => prev.map((x) => (x.id === l.id ? { ...x, featured: next } : x)));
    try {
      await updateListing(l.id, { featured: next });
    } catch {
      load();
    }
  }

  const [offerDrafts, setOfferDrafts] = useState<Record<number, string>>({});
  function offerValue(l: Listing) {
    return offerDrafts[l.id] ?? (l.offer_text ?? "");
  }
  async function commitOffer(l: Listing) {
    const raw = offerValue(l);
    setOfferDrafts((d) => { const next = { ...d }; delete next[l.id]; return next; });
    if (raw === (l.offer_text ?? "")) return;
    setListings((prev) => prev.map((x) => (x.id === l.id ? { ...x, offer_text: raw } : x)));
    try {
      await updateListing(l.id, { offer_text: raw });
    } catch {
      load();
    }
  }

  const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Listings</h1>
          <p className="text-sm text-subtle mt-0.5">{total} item{total === 1 ? "" : "s"} in your directory</p>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => setShowForm(!showForm)} className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg transition">
            <Plus size={16} /> Add Listing
          </button>
        </div>
      </div>

      <div className="flex gap-3 mb-4">
        <div className="relative flex-1">
          <Search size={16} className="absolute left-3 top-2.5 text-subtle" />
          <input value={q} onChange={(e) => { setQ(e.target.value); setPage(1); }} placeholder="Search listings…" className={`${inputCls} pl-9`} />
        </div>
        <select value={catFilter} onChange={(e) => { setCatFilter(e.target.value); setPage(1); }} className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40">
          <option value="">All Categories</option>
          {cats.map((c) => <option key={c.id} value={c.id}>{"—".repeat(c.level)} {c.name}</option>)}
        </select>
        <select value={stockFilter} onChange={(e) => { setStockFilter(e.target.value); setPage(1); }} className="bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40">
          <option value="">All stock</option>
          <option value="available">Available (&gt; 0)</option>
          <option value="out">Out of stock (0)</option>
          <option value="negative">Negative (&lt; 0)</option>
          <option value="unknown">No quantity</option>
        </select>
      </div>

      {showForm && (
        <form onSubmit={submit} className="bg-panel rounded-xl border border-line p-5 mb-6 space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-ink">New Listing</h2>
            <button type="button" onClick={() => setShowForm(false)} className="text-subtle hover:text-ink"><X size={16} /></button>
          </div>
          <input required placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className={inputCls} />
          <div className="grid grid-cols-2 gap-3">
            <input type="number" min={0} placeholder="Quantity" value={form.quantity} onChange={(e) => setForm({ ...form, quantity: e.target.value })} className={inputCls} />
            <input type="number" min={0} step="0.01" placeholder="Price (₹)" value={form.price} onChange={(e) => setForm({ ...form, price: e.target.value })} className={inputCls} />
          </div>
          <select required value={form.category_id} onChange={(e) => setForm({ ...form, category_id: Number(e.target.value) })} className={inputCls}>
            <option value={0}>Select Category</option>
            {cats.map((c) => <option key={c.id} value={c.id}>{"—".repeat(c.level)} {c.name}</option>)}
          </select>
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50">{saving ? "Saving…" : "Create"}</button>
            <button type="button" onClick={() => setShowForm(false)} className="text-sm px-4 py-2 rounded-lg border border-line text-muted hover:text-ink">Cancel</button>
          </div>
        </form>
      )}

      <div className="bg-panel rounded-xl border border-line overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-panel-2 text-subtle text-xs uppercase tracking-wide">
            <tr>
              <th className="text-left px-5 py-3 font-medium">Name</th>
              <th className="text-left px-5 py-3 font-medium">HSN Code</th>
              <th className="text-left px-5 py-3 font-medium">Category</th>
              <th className="text-left px-5 py-3 font-medium">Qty</th>
              <th className="text-left px-5 py-3 font-medium">Price</th>
              <th className="text-left px-5 py-3 font-medium">Offer</th>
              <th className="px-5 py-3"></th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {listings.map((l) => (
              <tr key={l.id} className="hover:bg-panel-2/60 transition">
                <td className="px-5 py-3 font-medium text-ink">{l.name}</td>
                <td className="px-5 py-3 text-muted tabular-nums">{l.hsn_code || "—"}</td>
                <td className="px-5 py-3"><span className="text-xs bg-panel-2 text-muted border border-line rounded px-2 py-0.5">{l.category?.name}</span></td>
                <td className="px-5 py-3">
                  <div className="inline-flex items-center gap-1.5 bg-panel-2 border border-line rounded-lg px-1 py-0.5">
                    <button onClick={() => changeQuantity(l, -1)} className="p-1 text-subtle hover:text-ink disabled:opacity-30" disabled={(l.quantity ?? 0) <= 0} aria-label="Decrease quantity">
                      <Minus size={12} />
                    </button>
                    <span className="text-ink text-sm w-7 text-center tabular-nums">{l.quantity ?? 0}</span>
                    <button onClick={() => changeQuantity(l, 1)} className="p-1 text-subtle hover:text-ink" aria-label="Increase quantity">
                      <Plus size={12} />
                    </button>
                  </div>
                </td>
                <td className="px-5 py-3">
                  <div className="flex items-center gap-1 text-muted">
                    <span className="text-xs">₹</span>
                    <input
                      type="number" min={0} step="0.01" placeholder="—"
                      value={priceValue(l)}
                      onChange={(e) => setPriceDrafts((d) => ({ ...d, [l.id]: e.target.value }))}
                      onBlur={() => commitPrice(l)}
                      onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }}
                      className="w-20 bg-transparent border-b border-transparent hover:border-line focus:border-accent text-sm text-ink focus:outline-none px-1 py-0.5"
                    />
                  </div>
                </td>
                <td className="px-5 py-3">
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => toggleFeatured(l)}
                      title={l.featured ? "Featured in reminders — click to remove" : "Mark as a featured offer for reminders"}
                      className={`shrink-0 transition ${l.featured ? "text-amber-500" : "text-subtle hover:text-amber-500"}`}
                      aria-label="Toggle featured"
                    >
                      <Star size={15} fill={l.featured ? "currentColor" : "none"} />
                    </button>
                    {l.featured && (
                      <input
                        type="text" placeholder="offer note"
                        value={offerValue(l)}
                        onChange={(e) => setOfferDrafts((d) => ({ ...d, [l.id]: e.target.value }))}
                        onBlur={() => commitOffer(l)}
                        onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }}
                        className="w-28 bg-transparent border-b border-line focus:border-accent text-xs text-ink focus:outline-none px-1 py-0.5"
                      />
                    )}
                  </div>
                </td>
                <td className="px-5 py-3 text-right">
                  <button onClick={() => handleDelete(l.id)} className="text-subtle hover:text-danger transition"><Trash2 size={15} /></button>
                </td>
              </tr>
            ))}
            {listings.length === 0 && <tr><td colSpan={7} className="px-5 py-10 text-center text-subtle">No listings found.</td></tr>}
          </tbody>
        </table>
      </div>

      {!searching && <Pagination page={page} total={total} pageSize={PAGE_SIZE} onChange={setPage} />}
      {searching && <p className="text-xs text-subtle mt-3">Showing {listings.length} search result{listings.length === 1 ? "" : "s"}. Clear the search to browse all with pages.</p>}
    </div>
  );
}
