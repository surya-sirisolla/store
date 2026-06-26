import Sidebar from "@/components/Sidebar";
import AuthGuard from "@/components/AuthGuard";

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGuard>
      <div className="flex min-h-screen bg-bg text-ink">
        <Sidebar />
        <main className="flex-1 p-6 lg:p-8 overflow-auto">{children}</main>
      </div>
    </AuthGuard>
  );
}
