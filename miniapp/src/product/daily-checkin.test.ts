import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import { wheelGradient, wheelRotationForPrize } from "./daily-checkin";

const prizes = [
  { prizeKey: "calls_10", calls: 10, weightBps: 3500 },
  { prizeKey: "calls_20", calls: 20, weightBps: 3000 },
  { prizeKey: "calls_30", calls: 30, weightBps: 2000 },
  { prizeKey: "calls_50", calls: 50, weightBps: 1000 },
  { prizeKey: "calls_100", calls: 100, weightBps: 400 },
  { prizeKey: "calls_200", calls: 200, weightBps: 100 },
];

test("daily check-in gives every configured prize a visible wheel segment", () => {
  assert.match(wheelGradient(prizes), /16\.666666666666664%/u);
  assert.match(wheelGradient(prizes), /100%/u);
});

test("daily check-in advances several turns and lands the selected prize under the pointer", () => {
  const rotation = wheelRotationForPrize(prizes, "calls_50", 0);
  assert.ok(rotation >= 4 * 360);
  assert.ok(rotation < 6 * 360);
  const normalized = ((rotation % 360) + 360) % 360;
  assert.ok(Math.abs(normalized - 150) < 0.00001);
});

test("daily check-in keeps advancing on another business day", () => {
  const first = wheelRotationForPrize(prizes, "calls_10", 0);
  const second = wheelRotationForPrize(prizes, "calls_200", first);
  assert.ok(second > first + 3 * 360);
});

test("daily check-in blocks duplicate taps before starting the network request", () => {
  const source = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
  const guardIndex = source.indexOf("isDailyCheckinClaimingRef.current = true");
  const requestIndex = source.indexOf("await session.claimDailyCheckin()");
  assert.ok(guardIndex >= 0);
  assert.ok(requestIndex > guardIndex);
});
