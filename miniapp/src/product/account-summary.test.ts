import assert from "node:assert/strict";
import test from "node:test";
import { formatAccountDate, formatUSD, periodUsagePercent } from "./account-summary";

test("subscription usage percent is bounded and safe for missing credit", () => {
  assert.equal(periodUsagePercent(25, 100), 25);
  assert.equal(periodUsagePercent(150, 100), 100);
  assert.equal(periodUsagePercent(-10, 100), 0);
  assert.equal(periodUsagePercent(10, 0), 0);
});

test("account values use concise user-facing formatting", () => {
  assert.equal(formatUSD(12.6), "$12.60");
  assert.equal(formatUSD(Number.NaN), "$0.00");
  assert.equal(formatAccountDate("2026-09-30T08:00:00Z"), "2026/09/30");
  assert.equal(formatAccountDate(null), "暂无");
});
