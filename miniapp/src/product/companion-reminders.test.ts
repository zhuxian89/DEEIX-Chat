import assert from "node:assert/strict";
import test from "node:test";
import type { ReminderEvent, ReminderState } from "@deeix/api-contract";
import { reminderCardOccurrence, reminderCountdown, reminderError, reminderMessages } from "./companion-reminders";
import { dueReminderEvents, ReminderAttention } from "./reminder-attention";

test("reminder errors translate standard error envelopes and preserve unexpected details", () => {
  assert.equal(reminderError(new Error("service unavailable（service.unavailable）")), "暂时未能处理这条消息，请稍后重试。提醒尚未创建或修改。");
  assert.equal(reminderError(new Error("resource conflict（resource.conflict）")), "提醒已更新，请刷新后重试。");
  assert.equal(reminderError(new Error("网络连接中断")), "网络连接中断");
});

test("countdown uses server time, crosses days and never goes negative", () => {
  const now = Date.parse("2026-10-16T15:30:00Z");
  assert.equal(reminderCountdown("2026-10-16T16:30:00Z", now), "还有 1小时 0秒");
  assert.equal(reminderCountdown("2026-10-17T16:31:02Z", now), "还有 1天 1小时 1分 2秒");
  assert.equal(reminderCountdown("2026-10-16T15:30:00Z", now), "已到时间");
  assert.equal(reminderCountdown("0001-01-01T00:00:00Z", now), "暂无下次提醒");
});

test("card actions target the displayed next time instead of a later snoozed occurrence", () => {
  const due: ReminderEvent = { id: "due", reminderID: "medicine", title: "喝药", dueAt: "2026-10-16T08:00:00Z", notifiedAt: "2026-10-16T08:00:02Z", notificationStatus: "not_configured", status: "due", revision: 1 };
  const snoozed: ReminderEvent = { ...due, id: "snoozed", status: "pending", dueAt: "2026-10-18T08:00:00Z" };
  const occurrences = [snoozed, due];
  assert.equal(reminderCardOccurrence({ id: "medicine", nextAt: due.dueAt }, occurrences)?.id, "due");
  // The next calendar time has no stored occurrence yet; let the server create it.
  const plan = { id: "medicine", nextAt: "2026-10-17T08:00:00Z" };
  assert.equal(reminderCardOccurrence(plan, [snoozed]), undefined);
  assert.equal(reminderCardOccurrence(plan, occurrences, "snoozed")?.id, "snoozed");
  assert.equal(reminderCardOccurrence(plan, occurrences, "missing"), undefined);
  assert.equal(reminderCardOccurrence({ ...plan, id: "another" }, occurrences, "due"), undefined);
});

test("persisted reminder turns and notifications keep stable chronological identities", () => {
  const state: ReminderState = { plans: [], serverTime: "2026-10-16T08:00:00Z", turns: [{ id: "create", input: "一个小时后提醒我喝药", reply: "已设置", reminderIDs: ["medicine"], createdAt: "2026-10-16T07:00:00Z" }], occurrences: [
    { id: "due", reminderID: "medicine", title: "喝药", dueAt: "2026-10-16T08:00:00Z", notifiedAt: "2026-10-16T08:00:02Z", notificationStatus: "not_configured", status: "due", revision: 1 },
    { id: "future", reminderID: "medicine", title: "喝药", dueAt: "2026-10-17T08:00:00Z", notifiedAt: "0001-01-01T00:00:00Z", notificationStatus: "", status: "pending", revision: 1 },
  ] };
  const messages = reminderMessages(state);
  assert.deepEqual(messages.map((m) => m.role), ["user", "assistant", "assistant"]);
  assert.equal(messages[2].occurrenceID, "due");
  assert.deepEqual(reminderMessages(state).map((m) => m.id), messages.map((m) => m.id));
  state.occurrences[0].status = "canceled";
  assert.equal(reminderMessages(state).length, 2);
});

function attentionState(): ReminderState {
  return { serverTime: "2026-10-17T12:06:40Z", turns: [], plans: [{
    id: "parcel", title: "拿快递", status: "active", revision: 1, description: "单次提醒", nextAt: "2026-10-17T12:06:35Z",
    schedule: { kind: "once", startAt: "2026-10-17T12:06:35Z", day: 0, intervalMinutes: 0, limit: 0, month: 0, times: [], until: "", weekdays: [] },
  }], occurrences: [{
    id: "parcel-today", reminderID: "parcel", title: "拿快递", status: "due", revision: 1,
    dueAt: "2026-10-17T12:06:35Z", notifiedAt: "2026-10-17T12:06:39Z", notificationStatus: "not_authorized",
  }] };
}

test("foreground reminders remain visible whether the WeChat send succeeds or lacks consent", () => {
  const state = attentionState();
  for (const status of ["not_authorized", "sent", "queued", "uncertain"]) {
    state.occurrences[0].notificationStatus = status;
    assert.equal(dueReminderEvents(state)[0]?.title, "拿快递");
  }
  for (const status of ["pending", "completed", "canceled", "suspended", "missed"]) {
    state.occurrences[0].status = status;
    assert.deepEqual(dueReminderEvents(state), []);
  }
  state.occurrences[0].status = "due";
  for (const status of ["paused", "deleted", "ended"]) {
    state.plans[0].status = status;
    assert.deepEqual(dueReminderEvents(state), []);
  }
  state.plans = [];
  assert.deepEqual(dueReminderEvents(state), []);
});

test("dismissal only hides the banner; pending reminders stay on the home card without repeated vibration", () => {
  const tracker = new ReminderAttention();
  const state = attentionState();
  assert.equal(tracker.accept(state), true);
  assert.equal(tracker.visible.length, 1);
  tracker.dismiss();
  assert.equal(tracker.visible.length, 0);
  assert.equal(tracker.due.length, 1);
  assert.equal(tracker.accept(structuredClone(state)), false);
  assert.equal(tracker.visible.length, 0);
  const next = { ...state.occurrences[0], id: "another-due-event" };
  state.occurrences.push(next);
  assert.equal(tracker.accept(state), true);
  assert.deepEqual(tracker.visible.map((event) => event.id), [next.id]);
  assert.equal(tracker.due.length, 2);
});

test("completion clears attention and a snoozed occurrence alerts again at its new due time", () => {
  const tracker = new ReminderAttention();
  const state = attentionState();
  tracker.accept(state); tracker.dismiss();
  state.occurrences[0] = { ...state.occurrences[0], status: "pending", revision: 2, dueAt: "2026-10-17T12:16:35Z", notifiedAt: "0001-01-01T00:00:00Z" };
  assert.equal(tracker.accept(state), false);
  assert.equal(tracker.due.length, 0);
  state.occurrences[0].status = "due";
  state.occurrences[0].notifiedAt = "2026-10-17T12:16:39Z";
  assert.equal(tracker.accept(state), false); // Server time still precedes the new due time.
  state.serverTime = "2026-10-17T12:16:40Z";
  assert.equal(tracker.accept(state), true);
  assert.equal(tracker.visible.length, 1);
  state.occurrences[0].status = "completed";
  tracker.accept(state);
  assert.equal(tracker.visible.length, 0);
  assert.equal(tracker.due.length, 0);
  assert.equal(new ReminderAttention().due.length, 0); // A new account starts with no previous reminders.
});
