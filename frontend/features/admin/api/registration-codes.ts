import { authedRequest } from "@/shared/api/authed-client";

export type AdminRegistrationCodeDTO = {
  id: number;
  code: string;
  codeHint: string;
  status: string;
  usedByUserID: number;
  usedAt: string | null;
  createdByUserID: number;
  createdAt: string;
  updatedAt: string;
};

export type RegistrationCodePage = { results: AdminRegistrationCodeDTO[]; total: number };

export async function listAdminRegistrationCodes(accessToken: string, page = 1, pageSize = 50, status = ""): Promise<RegistrationCodePage> {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (status) params.set("status", status);
  return authedRequest<RegistrationCodePage>(`/api/v1/admin/registration-codes?${params.toString()}`, { accessToken }, true);
}

export async function createAdminRegistrationCodes(accessToken: string, quantity: number): Promise<{ results: AdminRegistrationCodeDTO[] }> {
  return authedRequest<{ results: AdminRegistrationCodeDTO[] }>("/api/v1/admin/registration-codes", { method: "POST", accessToken, body: { quantity } }, true);
}

export async function deleteAdminRegistrationCode(accessToken: string, id: number): Promise<{ deleted: boolean }> {
  return authedRequest<{ deleted: boolean }>(`/api/v1/admin/registration-codes/${id}`, { method: "DELETE", accessToken }, true);
}
