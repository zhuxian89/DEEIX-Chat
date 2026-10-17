import type { ReminderState } from "@deeix/api-contract";
import { Button, Text, View } from "@tarojs/components";
import { useEffect } from "react";
import type { CompanionClient } from "@/product/companion-client";
import { useCompanionSubscriptions } from "./use-subscriptions";
import "./reminder-notice.scss";

export function ReminderSubscription({ client, reminders }: { client: CompanionClient; reminders: ReminderState | null }) {
  const { controller, settings, remembered, busy, error } = useCompanionSubscriptions(client);
  // Refresh on changes to delivery/commands, not on every countdown tick or poll.
  const refreshKey = `${reminders?.turns.length ?? 0}:${reminders?.occurrences.map((event) => `${event.id}:${event.notificationStatus}`).join(",") ?? ""}`;
  useEffect(() => { void controller.refresh(); }, [controller, refreshKey]);

  const available = Boolean(settings?.ready && settings.enabled && settings.credits > 0);
  const message = !settings ? "正在确认微信通知授权…" : !settings.ready ? "微信通知暂不可用，小程序内提醒照常显示。" : available
    ? `可用微信提醒：${settings.credits} 次。打开或退出小程序都可接收。` : "没有可用的微信通知次数，到期仍会在小程序内提醒。";
  return <View className={`reminderSubscription ${available ? "reminderSubscriptionReady" : ""}`}>
    <Text>{error || message}</Text>
    {settings?.ready && <Text className="reminderSubscriptionHint">{remembered && settings.enabled
      ? "已记住选择，使用小禾时会免弹窗补充通知次数。"
      : "允许时勾选“总是保持以上选择”，后续使用小禾可免弹窗补充。"}</Text>}
    <Button disabled={busy} onClick={() => { if (settings?.ready) void controller.request(true); else void controller.refresh(); }}>
      {busy ? "正在处理…" : !settings?.ready ? "重新加载" : available ? "补充通知次数" : "允许微信提醒"}
    </Button>
  </View>;
}
