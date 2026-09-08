import assert from "node:assert/strict";
import test from "node:test";
import { canOfferCompanionContinuation, CompanionInitiativeVisit, withCompanionInitiative, type CompanionIdlePresence } from "./companion-initiative";
import { CompanionClient, type CompanionInitiative, type CompanionState } from "./companion-client";
import type { ApiRequest } from "@/platform/transport";

const presence: CompanionIdlePresence = {
  foreground: true, typing: false, replying: false, readingHistory: false, unavailable: false,
  mode: "normal", lastInteractionAt: 1000, replyVisibleAt: 1000, latestReplyID: 12, readThroughID: 12,
};

test("proactive continuation waits after reading and interaction, and respects less/off modes", () => {
  assert.equal(canOfferCompanionContinuation(presence, 45_999), false);
  assert.equal(canOfferCompanionContinuation(presence, 46_000), true);
  for (const change of [
    { foreground: false }, { typing: true }, { replying: true }, { readingHistory: true },
    { unavailable: true }, { readThroughID: 11 }, { latestReplyID: 0 }, { replyVisibleAt: 0 },
    { lastInteractionAt: 30_000 }, { mode: "off" as const }, { mode: "less" as const },
  ]) assert.equal(canOfferCompanionContinuation({ ...presence, ...change }, 46_000), false);
  assert.equal(canOfferCompanionContinuation({ ...presence, mode: "less" }, 91_000), true);
});

function message(): CompanionInitiative {
  const now = Date.now();
  return { id: "candidate-one", kind: "idle", text: "窗边侧光也很适合练习人像。", afterMessageID: 12,
    createdAt: new Date(now).toISOString(), expiresAt: new Date(now + 60_000).toISOString(), acceptedAt: new Date(now).toISOString() };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

test("one page visit can only publish one initiative even with concurrent timer ticks", async () => {
  const visit = new CompanionInitiativeVisit();
  let prepared = 0, accepted = 0, displayed = 0;
  const client = {
    prepareInitiative: async () => { prepared += 1; return message(); },
    acceptInitiative: async () => { accepted += 1; return message(); },
  };
  await Promise.all(Array.from({ length: 5 }, () => visit.consider(client, "idle", 12, () => true, () => { displayed += 1; })));
  assert.deepEqual([prepared, accepted, displayed], [1, 1, 1]);
});

test("typing, scrolling or leaving while generating discards the candidate without accepting", async () => {
  const visit = new CompanionInitiativeVisit();
  const candidate = deferred<CompanionInitiative>();
  let accepted = 0, displayed = 0;
  const work = visit.consider({
    prepareInitiative: () => candidate.promise,
    acceptInitiative: async () => { accepted += 1; return message(); },
  }, "idle", 12, () => true, () => { displayed += 1; });
  visit.invalidate();
  candidate.resolve(message());
  await work;
  assert.deepEqual([accepted, displayed], [0, 0]);
});

test("a candidate finishing acceptance after the user starts interacting is not inserted on screen", async () => {
  const visit = new CompanionInitiativeVisit();
  const acceptance = deferred<CompanionInitiative>();
  let displayed = 0;
  const work = visit.consider({
    prepareInitiative: async () => message(),
    acceptInitiative: () => { visit.invalidate(); return acceptance.promise; },
  }, "idle", 12, () => true, () => { displayed += 1; });
  acceptance.resolve(message());
  await work;
  assert.equal(displayed, 0);
});

test("silence, expired candidates, errors and changed eligibility never create a visible message", async () => {
  for (const result of [null, { ...message(), expiresAt: new Date(0).toISOString() }, { ...message(), expiresAt: "invalid" }]) {
    let accepted = 0;
    await new CompanionInitiativeVisit().consider({
      prepareInitiative: async () => result,
      acceptInitiative: async () => { accepted += 1; return message(); },
    }, "home", 0, () => true, () => assert.fail("unexpected message"));
    assert.equal(accepted, 0);
  }
  await new CompanionInitiativeVisit().consider({
    prepareInitiative: async () => { throw new Error("unavailable"); },
    acceptInitiative: async () => assert.fail("unexpected acceptance"),
  }, "idle", 12, () => true, () => assert.fail("unexpected message"));
  let eligible = true;
  await new CompanionInitiativeVisit().consider({
    prepareInitiative: async () => { eligible = false; return message(); },
    acceptInitiative: async () => assert.fail("stale eligibility"),
  }, "idle", 12, () => eligible, () => assert.fail("unexpected message"));
});

test("accepted home greetings update the entry and deduplicate persisted initiative history", () => {
  const state = { initiatives: [], greeting: "old" } as unknown as CompanionState;
  const item = { ...message(), kind: "home" };
  const updated = withCompanionInitiative(withCompanionInitiative(state, item), item);
  assert.equal(updated.initiatives.length, 1);
  assert.equal(updated.greeting, item.text);
  assert.equal(updated.greetingID, item.id);
  assert.equal(updated.greetingOffered, true);
});

test("initiative requests reuse authenticated transport and support a successful silent decision", async () => {
  const requests: ApiRequest[] = [];
  const client = new CompanionClient(async <T>(request: ApiRequest): Promise<T> => {
    requests.push(request); return { message: null } as T;
  });
  assert.equal(await client.prepareInitiative("idle", "visit-123", 12), null);
  assert.equal(await client.acceptInitiative("id/one", "visit-123"), null);
  await client.setProactivity("less");
  assert.deepEqual(requests, [
    { path: "/api/v1/companion/initiatives", method: "POST", body: { kind: "idle", visitID: "visit-123", afterMessageID: 12 } },
    { path: "/api/v1/companion/initiatives/id%2Fone/accept", method: "POST", body: { visitID: "visit-123" } },
    { path: "/api/v1/companion/preferences", method: "PATCH", body: { proactivity: "less" } },
  ]);
});
