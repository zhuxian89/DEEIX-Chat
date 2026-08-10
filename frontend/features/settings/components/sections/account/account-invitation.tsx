"use client";

import * as React from "react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { CopyActionButton } from "@/shared/components/copy-action";
import { SettingsSection } from "@/shared/components/settings-layout";
import { useInvitationPanel } from "@/features/settings/hooks/use-invitation-panel";

function nanousdToUSD(value: number): number {
  return value / 1_000_000_000;
}

function formatDate(value: string): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

export function AccountInvitationSection() {
  const t = useTranslations("settings.invitationPage");
  const { loading, panel, invitedUsers, invitedTotal, page, error, refresh } = useInvitationPanel();
  const pageSize = 20;
  const totalPages = Math.max(1, Math.ceil(invitedTotal / pageSize));

  return (
    <SettingsSection title={t("title")}>
      {loading && !panel ? (
        <p className="text-xs text-muted-foreground">{t("loading")}</p>
      ) : null}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}

      {panel ? (
        <>
          <div className="flex items-center justify-between gap-4">
            <p className="min-w-0 flex-1 text-xs font-medium">{t("invitationCode")}</p>
            <div className="flex min-w-0 max-w-[min(60vw,26rem)] shrink items-center gap-2 rounded-lg bg-muted/35 px-2 py-1 text-xs text-muted-foreground">
              <span className="max-w-[min(75vw,26rem)] truncate">{panel.invitationCode || "-"}</span>
              <CopyActionButton
                type="button"
                variant="ghost"
                size="icon"
                value={panel.invitationCode || ""}
                messages={{
                  copied: t("toasts.copied"),
                  failed: t("toasts.copyFailed"),
                  failedDescription: t("toasts.retryLater"),
                }}
                disabled={!panel.invitationCode}
                aria-label={t("copyInvitationCode")}
                className="size-4 p-3"
              />
            </div>
          </div>

          <div className="flex items-center justify-between gap-4">
            <p className="min-w-0 flex-1 text-xs font-medium">{t("inviteLink")}</p>
            <div className="flex min-w-0 max-w-[min(60vw,26rem)] shrink items-center gap-2 rounded-lg bg-muted/35 px-2 py-1 text-xs text-muted-foreground">
              <span className="max-w-[min(75vw,26rem)] truncate">{panel.inviteLink || "-"}</span>
              <CopyActionButton
                type="button"
                variant="ghost"
                size="icon"
                value={panel.inviteLink || ""}
                messages={{
                  copied: t("toasts.copied"),
                  failed: t("toasts.copyFailed"),
                  failedDescription: t("toasts.retryLater"),
                }}
                disabled={!panel.inviteLink}
                aria-label={t("copyInviteLink")}
                className="size-4 p-3"
              />
            </div>
          </div>

          <p className="text-xs text-muted-foreground">{t("inviteCount", { count: panel.inviteCount })}</p>

          <div className="space-y-2 pt-2">
            <p className="text-xs font-medium">{t("invitedList")}</p>
            {invitedUsers.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t("noInvitedUsers")}</p>
            ) : (
              <div className="space-y-1">
                {invitedUsers.map((user) => (
                  <div key={user.relationshipId} className="flex items-center justify-between gap-2 rounded-lg bg-muted/20 px-2 py-1">
                    <div className="min-w-0">
                      <p className="truncate text-xs font-medium">{user.invitedDisplayName || user.invitedUsername || "-"}</p>
                      <p className="truncate text-[10px] text-muted-foreground">{formatDate(user.invitedAt)}</p>
                    </div>
                    <span className="shrink-0 text-[10px] text-muted-foreground">
                      +${nanousdToUSD(user.inviterRewardNanousd).toFixed(4)}
                    </span>
                  </div>
                ))}
              </div>
            )}
            {totalPages > 1 ? (
              <div className="flex items-center justify-between pt-1">
                <Button type="button" variant="outline" size="sm" disabled={page <= 1 || loading} onClick={() => void refresh(page - 1)}>
                  {t("actions.prev")}
                </Button>
                <span className="text-[10px] text-muted-foreground">
                  {t("pagination", { page, total: totalPages })}
                </span>
                <Button type="button" variant="outline" size="sm" disabled={page >= totalPages || loading} onClick={() => void refresh(page + 1)}>
                  {t("actions.next")}
                </Button>
              </div>
            ) : null}
          </div>
        </>
      ) : null}
    </SettingsSection>
  );
}
