import assert from "node:assert/strict";
import test from "node:test";
import { followupDateParts, followupDueAt, subscribeToFollowup } from "./companion-assistant";
import { CompanionClient } from "./companion-client";
import type { ApiRequest } from "@/platform/transport";
import type { CompanionAssistantState } from "@deeix/api-contract";
import { CompanionSubscriptions } from "./companion-subscriptions";

test("followup dates always round-trip with UTC+8, including day rollover", () => {
  assert.deepEqual(followupDateParts("2026-10-16T22:30:00Z"), { date: "2026-10-17", time: "06:30" });
  assert.equal(followupDueAt("2026-10-17", "06:30"), "2026-10-16T22:30:00.000Z");
  for (const [day, hour] of [["2026-02-30", "09:00"], ["2026-10-17", "25:00"], ["wrong", "09:00"]]) assert.equal(followupDueAt(day, hour), null);
});

test("notification grants require a configured template and an explicit accepted choice", async () => {
  const settings = { ready: false, enabled: false, credits: 0, templateID: "template" };
  let prompts = 0, grants = 0;
  const client = { setNotifications: async () => { grants++; } };
  await assert.rejects(subscribeToFollowup(settings, async () => { prompts++; return "accept"; }, client));
  assert.equal(prompts, 0);
  for (const choice of ["reject", "ban", "filter", ""]) assert.equal(await subscribeToFollowup({ ...settings, ready: true }, async () => choice, client), false);
  assert.equal(grants, 0);
  assert.equal(await subscribeToFollowup({ ...settings, ready: true }, async () => "accept", client), true);
  assert.equal(grants, 1);
  await assert.rejects(subscribeToFollowup({ ...settings, ready: true }, async () => "accept", { setNotifications: async () => { throw new Error("offline"); } }));
});

test("assistant edits preserve revision and use authenticated scoped endpoints", async () => {
  const requests: ApiRequest[] = [];
  const client = new CompanionClient(async <T>(request: ApiRequest) => { requests.push(request); return {} as T; });
  await client.contextConversations("旅行/出差", 2);
  await client.setContextPreferences({ enabled: true, excluded: ["private"] });
  const item = { id: "followup", title: "出发", dueAt: "2026-12-01T01:00:00Z", revision: 3, status: "canceled" as const };
  await client.saveFollowup(item);
  assert.equal(requests[0].path, `/api/v1/companion/assistant/conversations?q=${encodeURIComponent("旅行/出差")}&page=2`);
  assert.deepEqual(requests[1].body, { enabled: true, excluded: ["private"] });
  assert.deepEqual(requests[2].body, item);
  assert.equal(requests[2].method, "POST");
});

function subscriptionFixture() {
  const notifications = { ready: true, enabled: true, credits: 0, templateID: "template" };
  const choices = { mainSwitch: true, itemSettings: {} as Record<string, string> };
  const receipts: string[] = [];
  let requests = 0, failReport = false;
  let response: () => Promise<string> = async () => "accept";
  const controller = new CompanionSubscriptions({
    assistant: async () => ({ notifications: { ...notifications } }) as CompanionAssistantState,
    setNotifications: async (enabled, _accepted, _template, requestID) => {
      if (!enabled) { notifications.enabled = false; notifications.credits = 0; return; }
      assert.ok(requestID);
      if (!receipts.includes(requestID)) { receipts.push(requestID); notifications.enabled = true; notifications.credits++; }
      if (failReport) throw new Error("response lost after commit");
    },
  }, {
    choices: async () => choices,
    request: () => { requests++; return response(); },
  });
  return { controller, choices, notifications, receipts, requests: () => requests,
    failReport: (value: boolean) => { failReport = value; }, respond: (value: () => Promise<string>) => { response = value; } };
}

test("automatic subscription needs remembered acceptance and a click, never just a page read", async () => {
  const f = subscriptionFixture();
  await f.controller.refresh();
  assert.equal(await f.controller.request(), false);
  for (const choice of ["reject", "ban", "filter"]) {
    f.choices.itemSettings.template = choice;
    await f.controller.refresh();
    assert.equal(await f.controller.request(), false);
  }
  f.choices.itemSettings.template = "accept";
  await f.controller.refresh();
  assert.equal(f.requests(), 0);
  const first = f.controller.request();
  assert.equal(f.requests(), 1, "WeChat call must occur synchronously in the click stack");
  assert.equal(await f.controller.request(), false, "overlapping clicks share one in-flight request");
  assert.equal(await first, true);
  assert.equal(await f.controller.request(), true);
  assert.equal(f.notifications.credits, 2);
  f.controller.invalidate();
  assert.equal(await f.controller.request(), false, "foreground return must refresh saved preferences first");
  f.choices.mainSwitch = false;
  await f.controller.refresh();
  assert.equal(await f.controller.request(), false);
});

test("accepted subscription retries keep the receipt and do not call WeChat twice", async () => {
  const f = subscriptionFixture();
  await f.controller.refresh();
  f.failReport(true);
  assert.equal(await f.controller.request(true), false);
  assert.equal(f.notifications.credits, 1);
  assert.match(f.controller.state.error, /同步/u);
  f.failReport(false);
  assert.equal(await f.controller.request(true), true);
  assert.equal(f.requests(), 1);
  assert.equal(f.receipts.length, 1);
  assert.equal(f.notifications.credits, 1);
});

test("refreshes cannot overwrite a grant with stale notification settings", async () => {
  const notifications = { ready: true, enabled: true, credits: 0, templateID: "template" };
  let finishRead!: () => void, finishGrant!: () => void;
  let delayRead = false, reads = 0;
  const controller = new CompanionSubscriptions({
    assistant: async () => {
      reads++;
      const snapshot = { notifications: { ...notifications } } as CompanionAssistantState;
      if (delayRead) await new Promise<void>((resolve) => { finishRead = resolve; });
      return snapshot;
    },
    setNotifications: async () => {
      await new Promise<void>((resolve) => { finishGrant = resolve; });
      notifications.credits++;
    },
  }, { choices: async () => ({ mainSwitch: true, itemSettings: { template: "accept" } }), request: async () => "accept" });
  await controller.refresh();
  delayRead = true;
  const staleRead = controller.refresh();
  const grant = controller.request();
  await controller.refresh();
  assert.equal(reads, 2, "a read during a grant must be deferred to the final refresh");
  delayRead = false;
  finishGrant();
  assert.equal(await grant, true);
  assert.equal(controller.state.settings?.credits, 1);
  finishRead();
  await staleRead;
  assert.equal(controller.state.settings?.credits, 1);
  assert.equal(controller.state.error, "");
});

test("disabling wins over an in-flight acceptance and prevents silent re-enabling", async () => {
  const f = subscriptionFixture();
  f.choices.itemSettings.template = "accept";
  await f.controller.refresh();
  let accept!: (choice: string) => void;
  f.respond(() => new Promise<string>((resolve) => { accept = resolve; }));
  const grant = f.controller.request();
  const disable = f.controller.disable();
  assert.equal(await f.controller.request(), false);
  accept("accept");
  await Promise.all([grant, disable]);
  assert.equal(f.notifications.enabled, false);
  assert.equal(f.notifications.credits, 0);
  assert.equal(await f.controller.request(), false);
});

test("rejected or failed platform calls never report a grant", async () => {
  for (const choice of ["reject", "ban", "filter", ""]) {
    const f = subscriptionFixture();
    await f.controller.refresh();
    f.respond(async () => choice);
    assert.equal(await f.controller.request(true), false);
    assert.equal(f.receipts.length, 0);
  }
  const f = subscriptionFixture();
  await f.controller.refresh();
  f.respond(async () => { throw new Error("platform offline"); });
  assert.equal(await f.controller.request(true), false);
  assert.equal(f.receipts.length, 0);
});
