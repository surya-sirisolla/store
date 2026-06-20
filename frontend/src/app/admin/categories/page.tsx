"use client";
import { useEffect, useState, useMemo } from "react";
import { getCategories, createCategory, deleteCategory } from "@/lib/api";
import { Plus, Trash2, ChevronRight, ChevronDown, FolderTree, Folder, X, Layers, Maximize2, Minimize2 } from "lucide-react";

interface Category {
  id: number;
  name: string;
  slug: string;
  level: number;
  parent_id?: number;
  children?: Category[];
}

function CategoryNode({
  cat, onDelete, expanded, onToggle,
}: {
  cat: Category; onDelete: (id: number) => void; expanded: Set<number>; onToggle: (id: number) => void;
}) {
  const childCount = cat.children?.length ?? 0;
  const hasChildren = childCount > 0;
  const isOpen = expanded.has(cat.id);

  return (
    <div>
      <div
        onClick={() => hasChildren && onToggle(cat.id)}
        className={`flex items-center justify-between py-2 pr-2 rounded-lg hover:bg-panel-2 group transition ${hasChildren ? "cursor-pointer" : ""}`}
        style={{ paddingLeft: `${cat.level * 20 + 8}px` }}
      >
        <div className="flex items-center gap-2 min-w-0">
          {hasChildren ? (
            isOpen ? <ChevronDown size={14} className="text-subtle shrink-0" /> : <ChevronRight size={14} className="text-subtle shrink-0" />
          ) : (
            <span className="w-3.5 shrink-0" />
          )}
          {cat.level === 0
            ? <FolderTree size={15} className="text-accent shrink-0" />
            : <Folder size={13} className="text-subtle shrink-0" />}
          <span className="text-sm text-ink font-medium truncate">{cat.name}</span>
          <span className="text-xs text-subtle font-mono hidden sm:inline">/{cat.slug}</span>
          {hasChildren && (
            <span className="text-[11px] bg-panel-2 text-muted border border-line rounded-full px-2 py-0.5 shrink-0">
              {childCount} sub
            </span>
          )}
        </div>
        <button
          onClick={(e) => { e.stopPropagation(); onDelete(cat.id); }}
          className="opacity-0 group-hover:opacity-100 text-subtle hover:text-danger transition shrink-0"
          aria-label="Delete category"
        >
          <Trash2 size={14} />
        </button>
      </div>
      {hasChildren && isOpen && cat.children!.map((child) => (
        <CategoryNode key={child.id} cat={child} onDelete={onDelete} expanded={expanded} onToggle={onToggle} />
      ))}
    </div>
  );
}

export default function CategoriesPage() {
  const [cats, setCats] = useState<Category[]>([]);
  const [flat, setFlat] = useState<Category[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [parentId, setParentId] = useState<number | "">("");
  const [saving, setSaving] = useState(false);
  const [expanded, setExpanded] = useState<Set<number>>(new Set());

  function flattenCats(cats: Category[], result: Category[] = []): Category[] {
    for (const c of cats) {
      result.push(c);
      if (c.children) flattenCats(c.children, result);
    }
    return result;
  }

  const load = () => getCategories().then((r) => {
    setCats(r.data);
    setFlat(flattenCats(r.data));
  });

  useEffect(() => { load(); }, []);

  function toggle(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  const expandableIds = useMemo(() => {
    const set = new Set<number>();
    function walk(list: Category[]) {
      for (const c of list) {
        if (c.children && c.children.length > 0) { set.add(c.id); walk(c.children); }
      }
    }
    walk(cats);
    return set;
  }, [cats]);

  function expandAll() { setExpanded(new Set(expandableIds)); }
  function collapseAll() { setExpanded(new Set()); }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    await createCategory({ name, parent_id: parentId || undefined });
    setName(""); setParentId("");
    setShowForm(false);
    setSaving(false);
    load();
  }

  async function handleDelete(id: number) {
    if (!confirm("Delete this category?")) return;
    await deleteCategory(id);
    load();
  }

  const inputCls = "w-full bg-panel-2 border border-line rounded-lg px-3 py-2 text-sm text-ink focus:outline-none focus:ring-2 focus:ring-accent/40 focus:border-accent/40";
  const topLevel = cats.length;
  const subLevel = flat.length - cats.length;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Categories</h1>
          <p className="text-sm text-subtle mt-0.5">{flat.length} total · organise your listings into a tree</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} className="flex items-center gap-2 bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg transition">
          <Plus size={16} /> Add Category
        </button>
      </div>

      {showForm && (
        <form onSubmit={submit} className="bg-panel rounded-xl border border-line p-5 mb-6 space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="font-semibold text-ink">New Category</h2>
            <button type="button" onClick={() => setShowForm(false)} className="text-subtle hover:text-ink"><X size={16} /></button>
          </div>
          <input placeholder="Category name" required value={name} onChange={(e) => setName(e.target.value)} className={inputCls} />
          <div>
            <label className="text-xs text-subtle mb-1 block">Parent (leave empty for a top-level category)</label>
            <select value={parentId} onChange={(e) => setParentId(e.target.value ? Number(e.target.value) : "")} className={inputCls}>
              <option value="">Top level (no parent)</option>
              {flat.map((c) => <option key={c.id} value={c.id}>{"— ".repeat(c.level)}{c.name}</option>)}
            </select>
          </div>
          <div className="flex gap-2">
            <button type="submit" disabled={saving} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-2 rounded-lg disabled:opacity-50">{saving ? "Saving…" : "Create"}</button>
            <button type="button" onClick={() => setShowForm(false)} className="text-sm px-4 py-2 rounded-lg border border-line text-muted hover:text-ink">Cancel</button>
          </div>
        </form>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          <div className="bg-panel rounded-xl border border-line">
            <div className="flex items-center justify-between px-4 py-2.5 border-b border-line">
              <p className="text-xs text-subtle">{topLevel} top-level · {subLevel} sub-categories</p>
              <div className="flex items-center gap-3">
                <button onClick={expandAll} className="flex items-center gap-1 text-xs text-muted hover:text-ink"><Maximize2 size={12} /> Expand all</button>
                <button onClick={collapseAll} className="flex items-center gap-1 text-xs text-muted hover:text-ink"><Minimize2 size={12} /> Collapse all</button>
              </div>
            </div>
            <div className="p-3">
              {cats.length === 0 ? (
                <div className="flex flex-col items-center text-center py-12">
                  <FolderTree size={28} className="text-subtle mb-3" />
                  <p className="text-muted">No categories yet.</p>
                  <p className="text-sm text-subtle mt-1">Add one to start organising your listings.</p>
                </div>
              ) : (
                <div className="space-y-0.5">
                  {cats.map((cat) => <CategoryNode key={cat.id} cat={cat} onDelete={handleDelete} expanded={expanded} onToggle={toggle} />)}
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="space-y-5">
          <div className="bg-panel rounded-xl border border-line p-5">
            <div className="flex items-center gap-2 mb-3">
              <Layers size={16} className="text-accent" />
              <h2 className="font-semibold text-ink text-sm">Overview</h2>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted">Top-level categories</span>
                <span className="text-ink font-medium">{topLevel}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-muted">Sub-categories</span>
                <span className="text-ink font-medium">{subLevel}</span>
              </div>
              <div className="flex items-center justify-between text-sm pt-2 border-t border-line">
                <span className="text-muted">Total</span>
                <span className="text-ink font-medium">{flat.length}</span>
              </div>
            </div>
          </div>

          <div className="bg-panel rounded-xl border border-line p-5">
            <h2 className="font-semibold text-ink text-sm mb-2">Tips</h2>
            <ul className="text-sm text-muted space-y-1.5 list-disc pl-5 marker:text-subtle">
              <li>Click a category to expand or collapse its sub-categories.</li>
              <li>The count badge shows how many sub-categories it has.</li>
              <li>Deleting a category does not delete its listings.</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
