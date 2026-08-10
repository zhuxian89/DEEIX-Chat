import { authedRequest } from "@/shared/api/authed-client";
import { apiRequest } from "@/shared/api/http-client";

export type WeChatActionOption = { key: string; label: string };

export type WeChatKeywordRule = {
  id: number;
  keyword: string;
  action: string;
  templateId: number;
  templateName: string;
  templateType: string;
  templateContent: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type WeChatReplyTemplate = {
  id: number;
  name: string;
  responseType: string;
  content: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type WeChatIssuanceRecord = {
  id: number;
  openID: string;
  registrationCodeId: number;
  code: string;
  status: string;
  usedByUserId: number;
  usedAt: string | null;
  createdAt: string;
};

export type WeChatInvocationLog = {
  id: number;
  openID: string;
  keyword: string;
  action: string;
  templateId: number;
  registrationCodeId: number;
  result: string;
  errorCode: string;
  errorMessage: string;
  createdAt: string;
};

export type WeChatSummary = {
  issuanceCount: number;
  successCount: number;
  failureCount: number;
};

export type WeChatBuildInfo = {
  product: string;
  version: string;
  commit: string;
  buildTime: string;
  buildID: string;
};

export type WeChatPage<T> = { results: T[]; total: number };

function query(params: Record<string, string | number | undefined>) {
  const values = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") values.set(key, String(value));
  }
  const encoded = values.toString();
  return encoded ? `?${encoded}` : "";
}

export function listAdminWeChatActions(accessToken: string) {
  return authedRequest<WeChatActionOption[]>("/api/v1/admin/wechat/actions", { accessToken }, true);
}

export function getAdminWeChatSummary(accessToken: string) {
  return authedRequest<WeChatSummary>("/api/v1/admin/wechat/summary", { accessToken }, true);
}

export function listAdminWeChatRules(accessToken: string, page = 1, pageSize = 100) {
  return authedRequest<WeChatPage<WeChatKeywordRule>>(`/api/v1/admin/wechat/rules${query({ page, page_size: pageSize })}`, { accessToken }, true);
}

export function createAdminWeChatRule(accessToken: string, body: { keyword: string; action: string; templateId: number; enabled?: boolean }) {
  return authedRequest<{ saved: boolean }>("/api/v1/admin/wechat/rules", { method: "POST", accessToken, body }, true);
}

export function updateAdminWeChatRule(accessToken: string, id: number, body: { keyword: string; action: string; templateId: number; enabled?: boolean }) {
  return authedRequest<{ saved: boolean }>(`/api/v1/admin/wechat/rules/${id}`, { method: "PATCH", accessToken, body }, true);
}

export function setAdminWeChatRuleEnabled(accessToken: string, id: number, enabled: boolean) {
  return authedRequest<{ saved: boolean }>(`/api/v1/admin/wechat/rules/${id}/enabled`, { method: "PATCH", accessToken, body: { enabled } }, true);
}

export function listAdminWeChatTemplates(accessToken: string, page = 1, pageSize = 100) {
  return authedRequest<WeChatPage<WeChatReplyTemplate>>(`/api/v1/admin/wechat/templates${query({ page, page_size: pageSize })}`, { accessToken }, true);
}

export function createAdminWeChatTemplate(accessToken: string, body: { name: string; responseType: string; content: string; enabled?: boolean }) {
  return authedRequest<{ saved: boolean }>("/api/v1/admin/wechat/templates", { method: "POST", accessToken, body }, true);
}

export function updateAdminWeChatTemplate(accessToken: string, id: number, body: { name: string; responseType: string; content: string; enabled?: boolean }) {
  return authedRequest<{ saved: boolean }>(`/api/v1/admin/wechat/templates/${id}`, { method: "PATCH", accessToken, body }, true);
}

export function setAdminWeChatTemplateEnabled(accessToken: string, id: number, enabled: boolean) {
  return authedRequest<{ saved: boolean }>(`/api/v1/admin/wechat/templates/${id}/enabled`, { method: "PATCH", accessToken, body: { enabled } }, true);
}

export function listAdminWeChatIssuances(accessToken: string, page = 1, pageSize = 25, q = "") {
  return authedRequest<WeChatPage<WeChatIssuanceRecord>>(`/api/v1/admin/wechat/issuances${query({ page, page_size: pageSize, q })}`, { accessToken }, true);
}

export function listAdminWeChatLogs(accessToken: string, page = 1, pageSize = 25, filters: { q?: string; action?: string; result?: string } = {}) {
  return authedRequest<WeChatPage<WeChatInvocationLog>>(`/api/v1/admin/wechat/logs${query({ page, page_size: pageSize, q: filters.q, action: filters.action, result: filters.result })}`, { accessToken }, true);
}

export function getPublicBuildInfo() {
  return apiRequest<WeChatBuildInfo>("/api/v1/version");
}
