import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import {
  wheelGradient,
  wheelLabelPosition,
  wheelRotationForPrize,
  wheelSegments,
} from "./daily-checkin";

const prizes = [
  { prizeKey: "calls_10", calls: 10, weightBps: 3500 },
  { prizeKey: "calls_20", calls: 20, weightBps: 3000 },
  { prizeKey: "calls_30", calls: 30, weightBps: 2000 },
  { prizeKey: "calls_50", calls: 50, weightBps: 1000 },
  { prizeKey: "calls_100", calls: 100, weightBps: 400 },
  { prizeKey: "calls_200", calls: 200, weightBps: 100 },
];

test("daily check-in wheel sizes match configured probabilities", () => {
  const segments = wheelSegments(prizes);
  assert.deepEqual(
    segments.map(({ startPercent, endPercent }) => [startPercent, endPercent]),
    [[0, 35], [35, 65], [65, 85], [85, 95], [95, 99], [99, 100]],
  );
  assert.match(wheelGradient(prizes), /#ff9f43 0% 35%/u);
  assert.match(wheelGradient(prizes), /#ffd15c 99% 100%/u);
  assert.doesNotMatch(wheelGradient(prizes), /from -90deg/u);
});

test("daily check-in labels use the same segment midpoint as the wheel", () => {
  const [firstSegment] = wheelSegments(prizes);
  const position = wheelLabelPosition(firstSegment);
  assert.ok(Math.abs(Number.parseFloat(position.left) - 76.7301957257) < 0.00001);
  assert.ok(Math.abs(Number.parseFloat(position.top) - 36.3802850078) < 0.00001);
});

test("daily check-in advances several turns and lands the selected prize under the pointer", () => {
  for (const segment of wheelSegments(prizes)) {
    const rotation = wheelRotationForPrize(prizes, segment.prize.prizeKey, 0);
    assert.ok(rotation >= 4 * 360);
    assert.ok(rotation < 6 * 360);
    const normalized = ((rotation % 360) + 360) % 360;
    const expected = (360 - segment.midpointDegrees) % 360;
    assert.ok(Math.abs(normalized - expected) < 0.00001);
  }
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

test("home only shows a compact check-in entry and opens the full wheel as a secondary screen", () => {
  const source = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
  const checkinScreenIndex = source.indexOf('if (screen === "checkin")');
  const wheelIndex = source.indexOf("<DailyCheckinWheel");
  const homeIndex = source.lastIndexOf('<View className="page homePage">');
  const homeSource = source.slice(homeIndex);

  assert.ok(checkinScreenIndex >= 0);
  assert.ok(wheelIndex > checkinScreenIndex && wheelIndex < homeIndex);
  assert.match(homeSource, /<DailyCheckinEntry/u);
  assert.doesNotMatch(homeSource, /<DailyCheckinWheel/u);
});
