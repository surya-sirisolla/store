import axios from "axios";
import { clearToken, getToken } from "./auth";

const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080",
});

api.interceptors.request.use((config) => {
  const token = getToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401 && typeof window !== "undefined") {
      clearToken();
      if (window.location.pathname !== "/login") {
        window.location.href = "/login";
      }
    }
    return Promise.reject(error);
  }
);

export default api;

// ── Auth ──────────────────────────────────────────────────────────────────────
export const login = (password: string) => api.post("/api/auth/login", { password });
export const changePassword = (current_password: string, new_password: string) =>
  api.post("/api/auth/change-password", { current_password, new_password });

export const getStats = () => api.get("/api/stats");

// ── Business profile ──────────────────────────────────────────────────────────
export const getBusinessProfile = () => api.get("/api/business-profile");
export const updateBusinessProfile = (data: object) =>
  api.put("/api/business-profile", data);

// ── Business locations (godowns synced from Livekeeping) ───────────────────────
export interface BusinessLocation {
  id: number;
  name: string;
  address: string;
  area: string;
  city: string;
  state: string;
  pincode: string;
  phone: string;
  source_id?: string;
  active: boolean;
}
export const getBusinessLocations = () => api.get("/api/business-locations");
export const updateBusinessLocation = (
  id: number,
  data: Partial<Omit<BusinessLocation, "id" | "source_id">>,
) => api.put(`/api/business-locations/${id}`, data);

// ── Categories ────────────────────────────────────────────────────────────────
export const getCategories = () => api.get("/api/categories");
export const createCategory = (data: object) => api.post("/api/categories", data);
export const deleteCategory = (id: number) => api.delete(`/api/categories/${id}`);

// ── Listings ──────────────────────────────────────────────────────────────────
export const getListings = (params?: object) =>
  api.get("/api/listings", { params });
export const createListing = (data: object) => api.post("/api/listings", data);
export const updateListing = (id: number, data: object) =>
  api.put(`/api/listings/${id}`, data);
export const deleteListing = (id: number) => api.delete(`/api/listings/${id}`);

// ── Bulk upload ───────────────────────────────────────────────────────────────
export const inspectExcel = (file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return api.post("/api/bulk/inspect", fd);
};
export const confirmImport = (file: File, categoryId: number, mapping: object) => {
  const fd = new FormData();
  fd.append("file", file);
  fd.append("category_id", String(categoryId));
  fd.append("mapping", JSON.stringify(mapping));
  return api.post("/api/bulk/import", fd);
};
export const getBulkJobs = () => api.get("/api/bulk/jobs");
export const getBulkJob = (id: number) => api.get(`/api/bulk/jobs/${id}`);
export const aiImportEstimate = (file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return api.post("/api/bulk/ai-import/estimate", fd);
};
export const aiImportExcel = (file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return api.post("/api/bulk/ai-import", fd);
};

// ── Livekeeping stock sync ────────────────────────────────────────────────────
export interface LivekeepingCreds {
  token?: string;
  company_id?: string;
  user_id?: string;
  sync_stock?: boolean;
  sync_godowns?: boolean;
  sync_profile?: boolean;
}
export const getLivekeepingConfig = () => api.get("/api/integrations/livekeeping");
export const saveLivekeepingConfig = (data: LivekeepingCreds) =>
  api.put("/api/integrations/livekeeping", data);
export const checkLivekeepingToken = () =>
  api.post("/api/integrations/livekeeping/check");

// ── Scheduled jobs (Automation) ───────────────────────────────────────────────
export interface JobScheduleInput {
  enabled: boolean;
  schedule_kind: "manual" | "interval" | "daily";
  interval_hours: number;
  daily_time: string;
}
export const getJobs = () => api.get("/api/jobs");
export const saveJobSchedule = (key: string, data: JobScheduleInput) =>
  api.put(`/api/jobs/${key}/schedule`, data);
export const runJob = (key: string) => api.post(`/api/jobs/${key}/run`);

// Reminder/broadcast jobs (owner-created).
export interface BroadcastConfig {
  header?: string;
  recipient_days?: number;
  include_staff?: boolean;
  max_items?: number;
  throttle_seconds?: number;
  daily_cap?: number;
}
export interface BroadcastInput {
  name: string;
  schedule: JobScheduleInput;
  config: BroadcastConfig;
}
export const createJob = (data: BroadcastInput) => api.post("/api/jobs", data);
export const updateJob = (key: string, data: BroadcastInput) =>
  api.put(`/api/jobs/${key}`, data);
export const deleteJob = (key: string) => api.delete(`/api/jobs/${key}`);
export const previewBroadcast = (data: BroadcastInput) =>
  api.post("/api/jobs/preview", data);

// ── Staff (owner-only) ────────────────────────────────────────────────────────
export const getStaff = () => api.get("/api/staff");
export const createStaff = (data: object) => api.post("/api/staff", data);
export const deleteStaff = (id: number) => api.delete(`/api/staff/${id}`);

// ── Bot monitor ───────────────────────────────────────────────────────────────
export const getBotStats = () => api.get("/api/bot/stats");
export const getBotContacts = (range?: string) =>
  api.get("/api/bot/contacts", { params: range ? { range } : {} });
// ── Alerts (customer waitlist + restock-ready) ────────────────────────────────
export type AlertStatus = "logged" | "ready" | "notified" | "dismissed";
export const getAlerts = (status?: string) =>
  api.get("/api/alerts", { params: status ? { status } : {} });
export const getAlertCounts = () => api.get("/api/alerts/counts");
export const setAlertStatus = (id: number, status: AlertStatus) =>
  api.patch(`/api/alerts/${id}/status`, { status });
export const recheckAlerts = () => api.post("/api/alerts/recheck");

// ── Owner assistant (chat with the bot's brain about your data) ───────────────
export const assistantChat = (message: string, sessionId: string) =>
  api.post("/api/assistant/chat", { message, session_id: sessionId });
export const getAssistantStatus = () => api.get("/api/assistant/status");

// ── WhatsApp connection ───────────────────────────────────────────────────────
export const getWhatsAppStatus = () => api.get("/api/bot/whatsapp/status");
export const enableBot = () => api.post("/api/bot/whatsapp/enable");
export const disableBot = () => api.post("/api/bot/whatsapp/disable");

// ── LLM provider keys (owner-only) ────────────────────────────────────────────
export interface LLMKeyInput {
  provider: string;
  key?: string;
  base_url?: string;
  model?: string;
}
export const getLLMKeys = () => api.get("/api/settings/llm-keys");
export const setLLMKeys = (data: { primary: LLMKeyInput; fallback?: LLMKeyInput | null }) =>
  api.put("/api/settings/llm-keys", data);
export const deleteLLMKeys = () => api.delete("/api/settings/llm-keys");
// Promote the secondary provider to primary (swaps the two).
export const promoteLLMFallback = () => api.post("/api/settings/llm-keys/promote");
// Delete just the primary (secondary is promoted if present) or just the secondary.
export const deleteLLMPrimary = () => api.delete("/api/settings/llm-keys/primary");
export const deleteLLMFallback = () => api.delete("/api/settings/llm-keys/fallback");
export const detectLocalLLM = () => api.get("/api/settings/local-llm");
