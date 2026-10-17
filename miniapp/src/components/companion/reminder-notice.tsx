import type { ReminderEvent, ReminderState } from "@deeix/api-contract";
import { Button, Text, View } from "@tarojs/components";
import Taro, { useDidHide, useDidShow } from "@tarojs/taro";
import { useEffect, useMemo, useRef, useState } from "react";
import type { CompanionClient } from "@/product/companion-client";
import { followupTime } from "@/product/companion-assistant";
import { ReminderAttention } from "@/product/reminder-attention";
import "./reminder-notice.scss";

type NoticeState = { due: ReminderEvent[]; visible: ReminderEvent[] };
const emptyNotice: NoticeState = { due: [], visible: [] };

export function useReminderNotice(client: CompanionClient | null) {
  const tracker = useMemo(() => new ReminderAttention(), [client]);
  const [snapshot, setSnapshot] = useState<{ tracker: ReminderAttention; notice: NoticeState } | null>(null);
  const foreground = useRef(true);
  const refresh = useRef(() => {});
  useDidShow(() => { foreground.current = true; refresh.current(); });
  useDidHide(() => { foreground.current = false; });

  useEffect(() => {
    if (!client) return;
    let disposed = false;
    let working = false;
    const accept = (state: ReminderState) => {
      if (disposed || !foreground.current) return;
      const fresh = tracker.accept(state);
      setSnapshot({ tracker, notice: { due: tracker.due, visible: tracker.visible } });
      if (fresh) void Promise.resolve().then(() => Taro.vibrateShort({ type: "heavy" })).catch(() => {});
    };
    const unsubscribe = client.subscribeReminders(accept);
    const tick = () => {
      if (disposed || !foreground.current || working) return;
      working = true;
      void client.reminders().catch(() => {}).finally(() => { working = false; });
    };
    const show = () => { foreground.current = true; tick(); };
    const hide = () => { foreground.current = false; };
    refresh.current = tick;
    Taro.onAppShow(show);
    Taro.onAppHide(hide);
    const timer = setInterval(tick, 5000);
    tick();
    return () => {
      disposed = true;
      refresh.current = () => {};
      unsubscribe();
      clearInterval(timer);
      Taro.offAppShow(show);
      Taro.offAppHide(hide);
    };
  }, [client, tracker]);

  const notice = snapshot?.tracker === tracker ? snapshot.notice : emptyNotice;
  return { ...notice, dismiss: () => {
    tracker.dismiss();
    setSnapshot({ tracker, notice: { due: tracker.due, visible: tracker.visible } });
  } };
}

export function ReminderNotice({ events, onOpen, onDismiss }: {
  events: ReminderEvent[]; onOpen(): void; onDismiss(): void;
}) {
  if (!events.length) return null;
  return <View className="reminderNotice" role="alert">
    <View className="reminderNoticeHeading"><Text className="reminderNoticeIcon">!</Text><Text>小禾提醒 · 时间到了</Text>
      <Button className="reminderNoticeDismiss" onClick={onDismiss}>收起</Button></View>
    <Text className="reminderNoticeTitle">{events[0].title}</Text>
    <View className="reminderNoticeFooter">
      <Text>{events.length > 1 ? `共 ${events.length} 项提醒待处理` : followupTime(events[0].dueAt)}</Text>
      <Button className="reminderNoticeOpen" onClick={onOpen}>去处理 ›</Button>
    </View>
  </View>;
}

export function ReminderHomeCard({ events, onOpen }: { events: ReminderEvent[]; onOpen(): void }) {
  if (!events.length) return null;
  return <View className="reminderHomeCard" onClick={onOpen}>
    <View className="reminderHomeHeading"><Text className="reminderHomeBadge">到期提醒</Text><Text>{events.length} 项待处理</Text></View>
    {events.slice(0, 3).map((event) => <View className="reminderHomeItem" key={event.id}>
      <Text className="reminderHomeTitle">{event.title}</Text><Text className="reminderHomeTime">{followupTime(event.dueAt)}</Text>
    </View>)}
    <View className="reminderHomeAction"><Text>打开提醒，完成或推迟</Text><Text>立即处理 ›</Text></View>
  </View>;
}
