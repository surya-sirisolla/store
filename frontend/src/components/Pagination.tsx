"use client";
import { ChevronLeft, ChevronRight } from "lucide-react";

// pageRange builds a compact list of page numbers with ellipses, e.g.
// 1 … 4 5 [6] 7 8 … 20 — always showing first, last, and a window around current.
function pageRange(current: number, total: number): (number | "…")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);
  const out: (number | "…")[] = [1];
  const start = Math.max(2, current - 1);
  const end = Math.min(total - 1, current + 1);
  if (start > 2) out.push("…");
  for (let i = start; i <= end; i++) out.push(i);
  if (end < total - 1) out.push("…");
  out.push(total);
  return out;
}

export default function Pagination({
  page,
  total,
  pageSize,
  onChange,
}: {
  page: number;
  total: number;
  pageSize: number;
  onChange: (p: number) => void;
}) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  if (total === 0) return null;

  const from = (page - 1) * pageSize + 1;
  const to = Math.min(page * pageSize, total);

  const btn =
    "min-w-8 h-8 px-2 grid place-items-center rounded-lg text-sm border transition disabled:opacity-40 disabled:cursor-not-allowed";

  return (
    <div className="flex items-center justify-between gap-3 mt-4 flex-wrap">
      <p className="text-xs text-subtle">
        Showing <span className="text-muted">{from}–{to}</span> of{" "}
        <span className="text-muted">{total}</span> · page{" "}
        <span className="text-muted">{page}</span> of{" "}
        <span className="text-muted">{totalPages}</span>
      </p>

      <div className="flex items-center gap-1">
        <button
          onClick={() => onChange(page - 1)}
          disabled={page <= 1}
          aria-label="Previous page"
          className={`${btn} border-line text-muted hover:bg-panel-2 hover:text-ink`}
        >
          <ChevronLeft size={15} />
        </button>

        {pageRange(page, totalPages).map((p, i) =>
          p === "…" ? (
            <span key={`e${i}`} className="px-1.5 text-subtle text-sm">…</span>
          ) : (
            <button
              key={p}
              onClick={() => onChange(p)}
              className={`${btn} ${
                p === page
                  ? "bg-accent/15 border-accent/40 text-accent font-medium"
                  : "border-line text-muted hover:bg-panel-2 hover:text-ink"
              }`}
            >
              {p}
            </button>
          )
        )}

        <button
          onClick={() => onChange(page + 1)}
          disabled={page >= totalPages}
          aria-label="Next page"
          className={`${btn} border-line text-muted hover:bg-panel-2 hover:text-ink`}
        >
          <ChevronRight size={15} />
        </button>
      </div>
    </div>
  );
}
