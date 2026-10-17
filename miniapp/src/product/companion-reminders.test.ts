import assert from "node:assert/strict";
import test from "node:test";
import type { ReminderEvent, ReminderState } from "@deeix/api-contract";
import { reminderCardOccurrence, reminderCountdown, reminderMessages } from "./companion-reminders";

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
