import type { CompanionNotifications, ReminderState } from "@deeix/api-contract";
import { Button, Text, View } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { useEffect, useRef, useState } from "react";
import type { CompanionClient } from "@/product/companion-client";
import { subscribeToFollowup } from "@/product/companion-assistant";
import "./reminder-notice.scss";

export function ReminderSubscription({ client, reminders }: { client: CompanionClient; reminders: ReminderState | null }) {
  const [settings, setSettings] = useState<CompanionNotifications | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const version = useRef(0);
  // Refresh on changes to delivery/commands, not on every countdown tick or poll.
  const refreshKey = `${reminders?.turns.length ?? 0}:${reminders?.occurrences.map((event) => `${event.id}:${event.notificationStatus}`).join(",") ?? ""}`;
  const reload = async () => {
    const read = ++version.current;
    const next = await client.assistant();
    if (mounted.current && read === version.current) { setSettings(next.notifications); setError(""); }
  };
  useEffect(() => {
    mounted.current = true;
    void reload().catch(() => { if (mounted.current) setError("微信授权状态暂未加载，点此重试。"); });
    return () => { mounted.current = false; version.current++; };
  }, [client, refreshKey]);

  const subscribe = async () => {
    if (busy) return;
    setBusy(true); setError("");
    try {
      if (settings?.ready) {
        const accepted = await subscribeToFollowup(settings, async (id) => {
          const result = await Taro.requestSubscribeMessage({ tmplIds: [id] } as Taro.requestSubscribeMessage.Option);
          const choice: unknown = Reflect.get(result, id);
          return typeof choice === "string" ? choice : "reject";
        }, client);
        if (!accepted) throw new Error("未获得微信授权，仍会显示小程序内提醒。");
      }
      await reload();
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : "授权未完成，请重试。");
    } finally { if (mounted.current) setBusy(false); }
  };

  const available = Boolean(settings?.ready && settings.enabled && settings.credits > 0);
  const message = !settings ? "正在确认微信通知授权…" : !settings.ready ? "微信通知暂不可用，小程序内提醒照常显示。" : available
    ? "微信通知已就绪，可发送下一次提醒。" : "微信通知未就绪：没有可用授权，到期仅在小程序内提醒。";
  return <View className={`reminderSubscription ${available ? "reminderSubscriptionReady" : ""}`}>
    <Text>{error || message}</Text>
    {available && <Text className="reminderSubscriptionHint">本次授权用于一条微信通知，不论你是否打开小程序。</Text>}
    {(!available || error) && <Button disabled={busy} onClick={() => void subscribe()}>
      {busy ? "正在处理…" : settings?.ready ? "允许下一次微信提醒" : "重新加载"}
    </Button>}
  </View>;
}
