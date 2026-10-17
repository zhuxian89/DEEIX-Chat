import assert from "node:assert/strict";
import test from "node:test";
import { followupDateParts, followupDueAt, subscribeToFollowup } from "./companion-assistant";
import { CompanionClient } from "./companion-client";
import type { ApiRequest } from "@/platform/transport";

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
