import type { ReminderEvent, ReminderPlan, ReminderState } from "@deeix/api-contract";
import type { ConversationMessage } from "./message-timeline";

export type ReminderMessage = ConversationMessage & { reminderIDs?: string[]; occurrenceID?: string };

export function reminderCardOccurrence(plan: Pick<ReminderPlan, "id" | "nextAt">, occurrences: ReminderEvent[], occurrenceID?: string): ReminderEvent | undefined {
  return occurrences.find((event) => event.reminderID === plan.id && (occurrenceID
    ? event.id === occurrenceID
    : (event.status === "due" || event.status === "pending") && Date.parse(event.dueAt) === Date.parse(plan.nextAt)));
}

export function reminderCountdown(value: string, now: number): string {
  const at = Date.parse(value);
  if (!Number.isFinite(at) || at <= 0) return "暂无下次提醒";
  const seconds = Math.ceil((at - now) / 1000);
  if (seconds <= 0) return "已到时间";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor(seconds % 86400 / 3600);
  const minutes = Math.floor(seconds % 3600 / 60);
  const rest = seconds % 60;
  return `还有 ${days ? `${days}天 ` : ""}${hours ? `${hours}小时 ` : ""}${minutes ? `${minutes}分 ` : ""}${rest}秒`;
}

export function reminderMessages(state: ReminderState): ReminderMessage[] {
  const result: ReminderMessage[] = [];
  for (const turn of state.turns) {
    if (turn.input) result.push({ id: `reminder-user-${turn.id}`, role: "user", text: turn.input, createdAt: turn.createdAt });
    result.push({ id: `reminder-reply-${turn.id}`, role: "assistant", text: turn.reply, createdAt: turn.createdAt, reminderIDs: turn.reminderIDs });
  }
  for (const event of state.occurrences) {
    if (Date.parse(event.notifiedAt) <= 0 || event.status === "canceled") continue;
    result.push({ id: `reminder-event-${event.id}`, role: "assistant", text: event.status === "missed" ? `错过了「${event.title}」的提醒时间。` : `到时间了，记得${event.title}。`, createdAt: event.notifiedAt, reminderIDs: [event.reminderID], occurrenceID: event.id });
  }
  return result.sort((a, b) => Date.parse(a.createdAt!) - Date.parse(b.createdAt!));
}

export function reminderRequestID(): string {
  return `reminder-${Date.now()}-${Math.random().toString(36).slice(2, 12)}`;
}

export function reminderError(cause: unknown): string {
  const message = cause instanceof Error ? cause.message : "操作未完成，请重试。";
  return ({ "resource conflict": "提醒已更新，请刷新后重试。", "resource not found": "提醒已删除或暂不可用，请刷新。", "invalid request": "请检查提醒时间，或刷新后重新操作。" } as Record<string, string>)[message] ?? message;
}
