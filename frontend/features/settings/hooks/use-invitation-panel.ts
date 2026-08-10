"use client";

import * as React from "react";
import { useTranslations } from "next-intl";

import { getInvitationPanel, listInvitedUsers } from "@/shared/api/invitation";
import type { InvitationPanel, InvitedUser } from "@/shared/api/invitation.types";
import { useAuthSession } from "@/shared/auth/auth-session-context";

export interface InvitationPanelState {
  loading: boolean;
  panel: InvitationPanel | null;
  invitedUsers: InvitedUser[];
  invitedTotal: number;
  page: number;
  error: string | null;
  refresh: (nextPage?: number) => Promise<void>;
}

export function useInvitationPanel(): InvitationPanelState {
  const { accessToken } = useAuthSession();
  const t = useTranslations("settings.invitationPage");
  const [loading, setLoading] = React.useState(true);
  const [panel, setPanel] = React.useState<InvitationPanel | null>(null);
  const [invitedUsers, setInvitedUsers] = React.useState<InvitedUser[]>([]);
  const [invitedTotal, setInvitedTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [error, setError] = React.useState<string | null>(null);

  const refresh = React.useCallback(
    async (nextPage?: number) => {
      const targetPage = nextPage ?? 1;
      if (!accessToken) {
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      try {
        const [panelData, usersData] = await Promise.all([
          getInvitationPanel(accessToken),
          listInvitedUsers(accessToken, { page: targetPage, pageSize: 20 }),
        ]);
        setPanel(panelData);
        setInvitedUsers(usersData.results ?? []);
        setInvitedTotal(usersData.total ?? 0);
        setPage(targetPage);
      } catch {
        setError(t("errors.loadFailed"));
      } finally {
        setLoading(false);
      }
    },
    [accessToken, t],
  );

  React.useEffect(() => {
    void refresh(1);
  }, [refresh]);

  return { loading, panel, invitedUsers, invitedTotal, page, error, refresh };
}
