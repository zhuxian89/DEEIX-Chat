import assert from "node:assert/strict";
import test from "node:test";
import { beijingDate, isOverdue } from "./dates";

test("shortcuts use Beijing calendar dates across local timezone and year boundaries", () => {
  const now = Date.parse("2026-12-31T17:30:00-08:00");
  assert.equal(beijingDate(0, now), "2027-01-01");
  assert.equal(beijingDate(1, now), "2027-01-02");
  assert.equal(beijingDate(-1, now), "2026-12-31");
});
test("overdue respects due time while all-day tasks last until the next date", () => {
  const now = Date.parse("2026-09-10T12:00:00+08:00");
  assert.equal(isOverdue({ dueDate: "2026-09-10", dueTime: "11:59" }, now), true);
  assert.equal(isOverdue({ dueDate: "2026-09-10", dueTime: "12:01" }, now), false);
  assert.equal(isOverdue({ dueDate: "2026-09-10", dueTime: "" }, now), false);
  assert.equal(isOverdue({ dueDate: "2026-09-09", dueTime: "", completedAt: "done" }, now), false);
});
