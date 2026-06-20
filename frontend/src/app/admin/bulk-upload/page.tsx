"use client";
import { useState, useEffect } from "react";
import { getBulkJobs, getBulkJob, aiImportEstimate, aiImportExcel } from "@/lib/api";
import { CheckCircle, AlertCircle, Loader, Sparkles, FileSpreadsheet } from "lucide-react";

interface Job { id: number; file_name: string; status: string; total_rows: number; inserted: number; errors: number; created_at: string }

function StatusPill({ status }: { status: string }) {
  const cls =
    status === "done" ? "bg-accent/15 text-accent"
    : status === "failed" ? "bg-danger/15 text-danger"
    : "bg-warn/15 text-warn";
  return <span className={`text-xs rounded-full px-2 py-0.5 font-medium ${cls}`}>{status}</span>;
}

export default function BulkUploadPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [aiFile, setAiFile] = useState<File | null>(null);
  const [aiEstimating, setAiEstimating] = useState(false);
  const [aiEstimate, setAiEstimate] = useState<{ total_rows: number; estimated_calls: number } | null>(null);
  const [aiLoading, setAiLoading] = useState(false);
  const [aiJob, setAiJob] = useState<Job | null>(null);
  const [aiError, setAiError] = useState("");

  useEffect(() => { getBulkJobs().then((r) => setJobs(r.data)); }, []);

  function pollAIJob(id: number) {
    const tick = async () => {
      const res = await getBulkJob(id);
      setAiJob(res.data);
      if (res.data.status === "processing" || res.data.status === "pending") {
        setTimeout(tick, 3000);
      } else {
        setAiLoading(false);
        getBulkJobs().then((r) => setJobs(r.data));
      }
    };
    tick();
  }

  async function handleAIFileSelect(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    if (!f) return;
    setAiError(""); setAiJob(null); setAiEstimate(null); setAiFile(f); setAiEstimating(true);
    try {
      const res = await aiImportEstimate(f);
      setAiEstimate(res.data);
    } catch (err: any) {
      setAiError(err?.response?.data?.error || "Could not read this Excel file.");
      setAiFile(null);
    } finally { setAiEstimating(false); }
  }

  async function confirmAIImport() {
    if (!aiFile) return;
    setAiError(""); setAiLoading(true);
    try {
      const res = await aiImportExcel(aiFile);
      setAiEstimate(null);
      pollAIJob(res.data.job_id);
    } catch {
      setAiError("Could not start AI extraction. Make sure an AI key is configured.");
      setAiLoading(false);
    }
  }

  function cancelAIImport() { setAiFile(null); setAiEstimate(null); setAiError(""); }

  return (
    <div>
      <h1 className="text-2xl font-bold tracking-tight mb-1">Bulk Upload</h1>
      <p className="text-sm text-subtle mb-6">Drop a spreadsheet and let AI structure it into your directory.</p>

      <div className="bg-panel rounded-xl border border-line p-8 text-center mb-6">
        <div className="grid place-items-center w-12 h-12 rounded-xl bg-accent/10 text-accent mx-auto mb-3">
          <Sparkles size={24} />
        </div>
        <p className="text-ink mb-1 font-medium">AI Auto-Extract</p>
        <p className="text-sm text-muted mb-4 max-w-md mx-auto">
          Drop any Excel sheet — AI reads it directly into category, sub-category, name,
          quantity, price and description. No column mapping or target category needed.
        </p>
        {aiError && <p className="text-danger text-sm mb-3 flex items-center justify-center gap-1"><AlertCircle size={14} /> {aiError}</p>}

        {!aiEstimate && !aiLoading && !aiJob && (
          <label className="cursor-pointer bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-5 py-2 rounded-lg inline-flex items-center gap-2 transition">
            <FileSpreadsheet size={15} />
            {aiEstimating ? "Reading sheet…" : "Choose .xlsx / .xls file"}
            <input type="file" accept=".xlsx,.xls" onChange={handleAIFileSelect} className="hidden" disabled={aiEstimating} />
          </label>
        )}

        {aiEstimate && !aiLoading && (
          <div className="bg-warn/10 border border-warn/25 rounded-lg p-4 inline-block text-left">
            <p className="text-sm text-ink">
              <span className="font-semibold">{aiFile?.name}</span> has{" "}
              <span className="font-semibold">{aiEstimate.total_rows}</span> rows →{" "}
              this will make <span className="font-semibold">~{aiEstimate.estimated_calls}</span> AI call
              {aiEstimate.estimated_calls === 1 ? "" : "s"}.
            </p>
            <div className="flex gap-2 mt-3">
              <button onClick={confirmAIImport} className="bg-accent text-accent-ink hover:bg-accent-strong text-sm font-medium px-4 py-1.5 rounded-lg flex items-center gap-1.5">
                <Sparkles size={14} /> Confirm & Extract
              </button>
              <button onClick={cancelAIImport} className="text-sm px-4 py-1.5 rounded-lg border border-line text-muted hover:text-ink">Cancel</button>
            </div>
          </div>
        )}

        {aiLoading && !aiJob && (
          <p className="text-sm text-muted flex items-center justify-center gap-2"><Loader size={15} className="animate-spin" /> Starting extraction…</p>
        )}

        {aiJob && (
          <div className="mt-5 text-sm text-left bg-panel-2 border border-line rounded-lg p-4 inline-block">
            <p className="flex items-center gap-2">
              {aiJob.status === "done" ? <CheckCircle size={15} className="text-accent" /> : aiJob.status === "failed" ? <AlertCircle size={15} className="text-danger" /> : <Loader size={15} className="animate-spin text-accent" />}
              <span className="font-medium text-ink">{aiJob.file_name}</span>
              <StatusPill status={aiJob.status} />
            </p>
            {aiJob.status === "done" && (
              <p className="text-muted mt-1">Inserted {aiJob.inserted} of {aiJob.total_rows} rows{aiJob.errors > 0 ? `, ${aiJob.errors} errors` : ""}.</p>
            )}
            {(aiJob.status === "done" || aiJob.status === "failed") && (
              <button onClick={() => { setAiJob(null); setAiFile(null); setAiEstimate(null); }} className="text-accent text-xs mt-2 hover:underline">Upload another file</button>
            )}
          </div>
        )}
      </div>

      {jobs.length > 0 && (
        <div className="mt-8">
          <h2 className="text-lg font-semibold text-ink mb-3">Recent Jobs</h2>
          <div className="bg-panel rounded-xl border border-line overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-panel-2 text-subtle text-xs uppercase tracking-wide">
                <tr>
                  <th className="text-left px-5 py-3 font-medium">File</th>
                  <th className="text-left px-5 py-3 font-medium">Status</th>
                  <th className="text-left px-5 py-3 font-medium">Rows</th>
                  <th className="text-left px-5 py-3 font-medium">Inserted</th>
                  <th className="text-left px-5 py-3 font-medium">Errors</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {jobs.map((j) => (
                  <tr key={j.id} className="hover:bg-panel-2/60 transition">
                    <td className="px-5 py-3 text-ink">{j.file_name}</td>
                    <td className="px-5 py-3"><StatusPill status={j.status} /></td>
                    <td className="px-5 py-3 text-muted">{j.total_rows}</td>
                    <td className="px-5 py-3 text-accent">{j.inserted}</td>
                    <td className="px-5 py-3 text-danger">{j.errors}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
