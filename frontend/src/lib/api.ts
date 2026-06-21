import axios from "axios";

// No login: this is a single-operator console, open like PicoClaw itself.
const api = axios.create({
  baseURL: process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080",
});

export default api;

export const getStats = () => api.get("/api/stats");

// ── Business profile ──────────────────────────────────────────────────────────
export const getBusinessProfile = () => api.get("/api/business-profile");
export const updateBusinessProfile = (data: object) =>
  api.put("/api/business-profile", data);

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

// ── Staff (owner-only) ────────────────────────────────────────────────────────
export const getStaff = () => api.get("/api/staff");
export const createStaff = (data: object) => api.post("/api/staff", data);
export const deleteStaff = (id: number) => api.delete(`/api/staff/${id}`);

// ── Bot monitor ───────────────────────────────────────────────────────────────
export const getBotStats = () => api.get("/api/bot/stats");
export const getBotContacts = (range?: string) =>
  api.get("/api/bot/contacts", { params: range ? { range } : {} });
export const getAlerts = (status?: string) =>
  api.get("/api/bot/alerts", { params: status ? { status } : {} });
export const markAlertNotified = (id: number) =>
  api.patch(`/api/bot/alerts/${id}/notified`);

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
export const detectLocalLLM = () => api.get("/api/settings/local-llm");
