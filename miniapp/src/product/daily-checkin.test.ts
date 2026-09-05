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

test("daily check-in wheel gives every configured prize a readable equal segment", () => {
  const segments = wheelSegments(prizes);
  for (const [index, segment] of segments.entries()) {
    assert.ok(Math.abs(segment.startPercent - index / prizes.length * 100) < 0.00001);
    assert.ok(Math.abs(segment.endPercent - (index + 1) / prizes.length * 100) < 0.00001);
  }
  assert.match(wheelGradient(prizes), /#ff9f43 0% 16\.666666666666664%/u);
  assert.match(wheelGradient(prizes), /#ffd15c 83\.33333333333334% 100%/u);
  assert.doesNotMatch(wheelGradient(prizes), /from -90deg/u);
});

test("daily check-in labels use the same segment midpoint as the wheel", () => {
  const [firstSegment] = wheelSegments(prizes);
  const position = wheelLabelPosition(firstSegment);
  assert.ok(Math.abs(Number.parseFloat(position.left) - 65) < 0.00001);
  assert.ok(Math.abs(Number.parseFloat(position.top) - 24.0192378865) < 0.00001);
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

test("home check-in entry animates only while today's reward is unclaimed", () => {
  const componentSource = readFileSync(
    resolve(process.cwd(), "src/components/daily-checkin-wheel.tsx"),
    "utf8",
  );
  const styleSource = readFileSync(resolve(process.cwd(), "src/pages/index/index.scss"), "utf8");

  assert.match(componentSource, /status\.claimed \? "checkinEntryClaimed" : "checkinEntryPending"/u);
  assert.match(componentSource, /status\.claimed \? null : \([\s\S]*checkinEntryGlow[\s\S]*checkinEntryShimmer/u);
  assert.match(styleSource, /\.checkinEntryPending[\s\S]*animation:\s*checkinEntryGradient/u);
  assert.match(styleSource, /@keyframes checkinEntryShimmer/u);
  assert.match(styleSource, /@media \(prefers-reduced-motion:\s*reduce\)/u);
});

test("daily check-in wheel center triggers the same guarded claim action as the button", () => {
  const source = readFileSync(resolve(process.cwd(), "src/components/daily-checkin-wheel.tsx"), "utf8");
  assert.match(source, /wheelCenterClaimed/u);
  assert.match(source, /className=\{`wheelCenter[\s\S]*onClick=\{onClaim\}/u);
  assert.match(source, /className="checkinButton"[\s\S]*onClick=\{onClaim\}/u);
});
