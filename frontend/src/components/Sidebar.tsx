"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  LayoutDashboard,
  Store,
  FolderTree,
  List,
  Upload,
  Users,
  Building2,
  Smartphone,
  Bot,
  Sparkles,
  CalendarClock,
  Bell,
  Puzzle,
  ShieldCheck,
  LogOut,
} from "lucide-react";
import { clearToken } from "@/lib/auth";

interface NavItem {
  href: string;
  label: string;
  icon: React.ReactNode;
}
interface NavSection {
  header?: string;
  items: NavItem[];
}

const sections: NavSection[] = [
  {
    items: [
      { href: "/admin/dashboard", label: "Dashboard", icon: <LayoutDashboard size={18} /> },
      { href: "/admin/assistant", label: "Assistant", icon: <Sparkles size={18} /> },
    ],
  },
  {
    header: "Catalog",
    items: [
      { href: "/admin/business-profile", label: "Business Profile", icon: <Building2 size={18} /> },
      { href: "/admin/categories", label: "Categories", icon: <FolderTree size={18} /> },
      { href: "/admin/listings", label: "Listings", icon: <List size={18} /> },
      // { href: "/admin/bulk-upload", label: "Bulk Upload", icon: <Upload size={18} /> },
    ],
  },
  {
    header: "Channels",
    items: [
      { href: "/admin/whatsapp", label: "WhatsApp", icon: <Smartphone size={18} /> },
      { href: "/admin/alerts", label: "Alerts", icon: <Bell size={18} /> },
    ],
  },
  {
    header: "Automation",
    items: [
      { href: "/admin/extensions", label: "Extensions", icon: <Puzzle size={18} /> },
      { href: "/admin/scheduled-jobs", label: "Scheduled Jobs", icon: <CalendarClock size={18} /> },
    ],
  },
  {
    header: "Settings",
    items: [
      { href: "/admin/ai-providers", label: "AI Providers", icon: <Bot size={18} /> },
      { href: "/admin/staff", label: "Staff", icon: <Users size={18} /> },
      { href: "/admin/security", label: "Security", icon: <ShieldCheck size={18} /> },
    ],
  },
];

export default function Sidebar() {
  const pathname = usePathname();
  const router = useRouter();

  function handleLogout() {
    clearToken();
    router.push("/login");
  }

  return (
    <aside className="w-60 min-h-screen bg-panel border-r border-line flex flex-col sticky top-0 h-screen">
      <div className="flex items-center gap-2.5 px-5 py-4 border-b border-line">
        <div className="grid place-items-center w-8 h-8 rounded-lg bg-accent/15 text-accent">
          <Store size={18} />
        </div>
        <span className="font-semibold text-sm tracking-tight">Business Console</span>
      </div>

      <nav className="flex-1 px-3 py-4 space-y-4 overflow-y-auto">
        {sections.map((section, i) => (
          <div key={i} className="space-y-0.5">
            {section.header && (
              <p className="px-3 pb-1 text-[10px] uppercase tracking-wider text-subtle font-medium">
                {section.header}
              </p>
            )}
            {section.items.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`relative flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition ${
                    active
                      ? "bg-accent/10 text-accent font-medium"
                      : "text-muted hover:bg-panel-2 hover:text-ink"
                  }`}
                >
                  {active && <span className="absolute left-0 top-1.5 bottom-1.5 w-0.5 rounded-full bg-accent" />}
                  {item.icon}
                  {item.label}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      <div className="px-3 py-4 border-t border-line">
        <button
          onClick={handleLogout}
          className="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm text-muted hover:bg-panel-2 hover:text-ink transition"
        >
          <LogOut size={18} />
          Log out
        </button>
      </div>
    </aside>
  );
}
