"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { Store } from "lucide-react";
import { login } from "@/lib/api";
import { setToken } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await login(password);
      setToken(res.data.token);
      router.push("/admin/dashboard");
    } catch {
      setError("Incorrect password.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-bg text-ink">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-sm bg-panel border border-line rounded-xl p-6 space-y-4"
      >
        <div className="flex items-center gap-2.5">
          <div className="grid place-items-center w-8 h-8 rounded-lg bg-accent/15 text-accent">
            <Store size={18} />
          </div>
          <span className="font-semibold text-sm tracking-tight">Business Console</span>
        </div>

        <div className="space-y-1.5">
          <label htmlFor="password" className="text-sm text-muted">
            Password
          </label>
          <input
            id="password"
            type="password"
            autoFocus
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full rounded-lg border border-line bg-panel-2 px-3 py-2 text-sm outline-none focus:border-accent"
          />
        </div>

        {error && <p className="text-sm text-red-500">{error}</p>}

        <button
          type="submit"
          disabled={loading || !password}
          className="w-full rounded-lg bg-accent text-white text-sm font-medium py-2 disabled:opacity-50"
        >
          {loading ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </div>
  );
}
