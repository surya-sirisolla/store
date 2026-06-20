"use client";
import { useEffect, useState } from "react";
import { getStats, getBotStats } from "@/lib/api";
import { List, FolderTree, Users, MessageSquare, Bell } from "lucide-react";

interface Stats {
  listings: number;
  categories: number;
  staff: number;
  bot_interactions: number;
}
interface BotStats {
  interactions_today: number;
  pending_alerts: number;
}

function Card({ label, value, icon }: { label: string; value: number; icon: React.ReactNode }) {
  return (
    <div className="bg-panel rounded-xl border border-line p-5 flex items-center gap-4 hover:border-line-2 transition">
      <div className="p-3 rounded-lg bg-accent/10 text-accent">{icon}</div>
      <div>
        <p className="text-2xl font-bold text-ink">{value}</p>
        <p className="text-sm text-muted">{label}</p>
      </div>
    </div>
  );
}

export default function Dashboard() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [bot, setBot] = useState<BotStats | null>(null);

  useEffect(() => {
    getStats().then((r) => setStats(r.data));
    getBotStats().then((r) => setBot(r.data));
  }, []);

  if (!stats) return <p className="text-subtle">Loading…</p>;

  return (
    <div>
      <h1 className="text-2xl font-bold tracking-tight mb-6">Dashboard</h1>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <Card label="Listings" value={stats.listings} icon={<List size={22} />} />
        <Card label="Categories" value={stats.categories} icon={<FolderTree size={22} />} />
        <Card label="Staff" value={stats.staff} icon={<Users size={22} />} />
        <Card label="Bot Interactions" value={stats.bot_interactions} icon={<MessageSquare size={22} />} />
      </div>

      {bot && (
        <div className="bg-panel rounded-xl border border-line p-5">
          <h2 className="font-semibold text-ink mb-3">WhatsApp Bot — today</h2>
          <div className="flex flex-wrap gap-3">
            <div className="flex items-center gap-2 bg-panel-2 border border-line rounded-lg px-4 py-3">
              <MessageSquare size={16} className="text-accent" />
              <span className="text-xl font-bold text-ink">{bot.interactions_today}</span>
              <span className="text-sm text-muted">interactions today</span>
            </div>
            <div className="flex items-center gap-2 bg-panel-2 border border-line rounded-lg px-4 py-3">
              <Bell size={16} className="text-warn" />
              <span className="text-xl font-bold text-ink">{bot.pending_alerts}</span>
              <span className="text-sm text-muted">pending alerts</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
