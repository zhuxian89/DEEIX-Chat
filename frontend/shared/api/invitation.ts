import { authedRequest } from "@/shared/api/authed-client";
import type { PagePayload } from "@/shared/api/common.types";
import type { InvitationPanel, InvitationRelationship, InvitedUser } from "@/shared/api/invitation.types";

export async function getInvitationPanel(accessToken: string): Promise<InvitationPanel> {
  return authedRequest<InvitationPanel>(`/api/v1/me/invitation`, { accessToken }, true);
}

export async function listInvitedUsers(
  accessToken: string,
  options: { page?: number; pageSize?: number } = {},
): Promise<PagePayload<InvitedUser>> {
  const page = options.page && options.page > 0 ? options.page : 1;
  const pageSize = options.pageSize && options.pageSize > 0 ? options.pageSize : 20;
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  return authedRequest<PagePayload<InvitedUser>>(`/api/v1/me/invitations?${params.toString()}`, { accessToken }, true);
}

export async function listInvitationRelationships(
  accessToken: string,
  options: { page?: number; pageSize?: number; inviterUserId?: number; invitedUserId?: number } = {},
): Promise<PagePayload<InvitationRelationship>> {
  const page = options.page && options.page > 0 ? options.page : 1;
  const pageSize = options.pageSize && options.pageSize > 0 ? options.pageSize : 20;
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  if (options.inviterUserId && options.inviterUserId > 0) params.set("inviter_user_id", String(options.inviterUserId));
  if (options.invitedUserId && options.invitedUserId > 0) params.set("invited_user_id", String(options.invitedUserId));
  return authedRequest<PagePayload<InvitationRelationship>>(`/api/v1/admin/invitations?${params.toString()}`, { accessToken }, true);
}
