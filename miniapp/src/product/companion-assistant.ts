import type { CompanionNotifications } from "@deeix/api-contract";
import type { CompanionClient } from "./companion-client";

export function followupDateParts(value: string): { date: string; time: string } {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return { date: "", time: "09:00" };
  const local = new Date(parsed + 8 * 3600_000).toISOString();
  return { date: local.slice(0, 10), time: local.slice(11, 16) };
}

export function followupDueAt(date: string, time: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}$/u.test(date) || !/^\d{2}:\d{2}$/u.test(time)) return null;
  const value = `${date}T${time}:00+08:00`;
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return null;
  const parts = followupDateParts(value);
  return parts.date === date && parts.time === time ? new Date(parsed).toISOString() : null;
}

export function followupTime(value: string): string {
  const parts = followupDateParts(value);
  return parts.date ? `${parts.date} ${parts.time}` : "时间待确认";
}

export function companionDecisionText(reason: string): string {
  return ({
    off: "你选择了先开口，主动消息和跟进提醒已暂停",
    busy: "正在聊天，稍后再尝试",
    unread: "上一条回复还未读，暂不打扰",
    already_delivered: "这一轮已主动联系过你",
    conversation_closed: "你表示暂时结束聊天，先不打扰",
    awaiting_reply: "正在等你的回答，暂不追问",
    cooldown_or_context: "还在间隔期，或当前内容不适合主动接话",
    no_suitable_message: "暂时没有合适的新话题",
    generation_failed: "这次生成失败，会稍后重试",
    delivered: "主动消息已送达会话",
    refresh_pending: "正在核对最新计划，确认前暂缓自动提醒",
    source_unavailable: "暂时无法读取事项来源，会稍后重试",
    delivery_failed: "跟进消息暂未写入，会稍后重试",
    sent: "微信通知已发送",
    queued: "微信通知等待发送",
    canceled: "本次微信通知已取消",
    uncertain: "未能确认微信是否送达，请在这里查看跟进",
    not_configured: "微信通知暂未配置，已保留应用内提醒",
    not_authorized: "没有可用的微信订阅授权，已保留应用内提醒",
    dispatching: "微信通知发送中",
    expired: "已超过提醒时效，未再发送",
  } as Record<string, string>)[reason] ?? "等待下一次检查";
}

// Call directly from a user gesture. Merely opening settings never asks for consent.
export async function subscribeToFollowup(
  settings: CompanionNotifications,
  request: (templateID: string) => Promise<string>,
  client: Pick<CompanionClient, "setNotifications">,
): Promise<boolean> {
  if (!settings.ready || !settings.templateID) throw new Error("微信通知暂未配置");
  const choice = await request(settings.templateID);
  if (choice !== "accept") return false;
  await client.setNotifications(true, true, settings.templateID);
  return true;
}
