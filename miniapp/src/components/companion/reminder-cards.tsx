import type { ReminderCommandRequest, ReminderPlan, ReminderState } from "@deeix/api-contract";
import { Button, Text, View } from "@tarojs/components";
import { companionDecisionText, followupTime } from "@/product/companion-assistant";
import { reminderCardOccurrence, reminderCountdown } from "@/product/companion-reminders";
import "./reminder-cards.scss";

export type ReminderAction = Pick<ReminderCommandRequest, "action" | "id" | "revision"> & Partial<ReminderCommandRequest>;

export function ReminderCards({ state, ids, occurrenceID, now, busy, onAction, onEdit }: {
  state: ReminderState; ids?: string[]; occurrenceID?: string; now: number; busy: boolean;
  onAction(action: ReminderAction): void; onEdit(plan: ReminderPlan): void;
}) {
  const plans = ids ? ids.map((id) => state.plans.find((p) => p.id === id)).filter((p): p is ReminderPlan => Boolean(p)) : state.plans;
  return <View className="reminderCards">
    {ids?.length && !plans.length ? <Text className="reminderHint">提醒已删除</Text> : null}
    {!ids && !plans.length && <Text className="reminderHint">直接说“一个小时后提醒我吃药”或“每天晚上八点提醒我陪孩子写作业”。</Text>}
    {plans.map((p) => {
      const event = reminderCardOccurrence(p, state.occurrences, occurrenceID);
      const actionable = !occurrenceID || event?.status === "pending" || event?.status === "due";
      const perform = (action: ReminderCommandRequest["action"], extra: Partial<ReminderCommandRequest> = {}) => onAction({ action, id: p.id, revision: p.revision, occurrenceID: event?.id ?? "", ...extra });
      const eventLabel = event && ({ completed: "本次已完成", skipped: "本次已跳过", missed: "本次已错过" } as Record<string, string>)[event.status];
      return <View className="reminderCard" key={p.id}>
        <Text className="reminderTitle">{p.title}</Text>
        <Text className="reminderRule">{p.description}</Text>
        <Text className="reminderCountdown">{p.status === "paused" ? "已暂停" : p.status === "ended" ? "已结束" : reminderCountdown(p.nextAt, now)}</Text>
        {p.status === "active" && Date.parse(p.nextAt) > 0 && <Text className="reminderHint">{followupTime(p.nextAt)}</Text>}
        {eventLabel && <Text className="reminderHint">{eventLabel}</Text>}
        {event?.notificationStatus && <Text className="reminderHint">{companionDecisionText(event.notificationStatus)}</Text>}
        <View className="reminderActions">
          {p.status === "active" && actionable && <>
            <Button disabled={busy} onClick={() => perform("complete")}>完成本次</Button>
            <Button disabled={busy} onClick={() => perform("snooze", { delayMinutes: 10 })}>10 分钟后</Button>
            {p.schedule.kind !== "once" && <Button disabled={busy} onClick={() => perform("skip")}>跳过本次</Button>}
          </>}
          {p.status !== "ended" && <Button disabled={busy} onClick={() => perform(p.status === "paused" ? "resume" : "pause")}>{p.status === "paused" ? "恢复" : "暂停"}</Button>}
          <Button disabled={busy} onClick={() => onEdit(p)}>修改</Button>
          <Button disabled={busy} onClick={() => perform("delete")}>删除</Button>
        </View>
      </View>;
    })}
  </View>;
}
