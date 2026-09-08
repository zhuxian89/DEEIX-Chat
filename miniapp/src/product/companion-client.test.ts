import assert from "node:assert/strict";
import test from "node:test";
import { CompanionClient, canOfferCompanionGreeting, companionBeijingDate, companionImageIDs, companionStreamPath } from "./companion-client";
import { companionDisplayText } from "./companion-message";
import type { ApiRequest } from "@/platform/transport";

test("companion hides leaked time metadata while preserving the actual reply and user quotations", () => {
  const marker = "[历史消息时间：北京时间 2026-09-08 07:17:21]";
  const title = "“不认真”这个标签背后";
  for (const separator of ["", " ", "\n", "\r\n"]) {
    assert.equal(companionDisplayText({ role: "assistant", text: marker + separator + title }), title);
  }
  assert.equal(companionDisplayText({ role: "assistant", text: `## ${marker} ${title}\n\n${marker}\n接着聊。` }), `## ${title}\n\n接着聊。`);
  const user = { role: "user" as const, text: marker + "怎么还有这个？", pending: true };
  assert.equal(companionDisplayText(user), user.text);
  for (const text of ["北京时间 2026-09-08 07:17:21", "[2026-09-08] 明天见", "[标题](https://example.com)", "数组 a[0]", "参考[", "[历史消息时间：这里是用户讨论的例子]"]) {
    assert.equal(companionDisplayText({ role: "assistant", text }), text);
  }
});

test("companion never flashes a time marker when its stream is split at any character", () => {
  const marker = "[历史消息时间：北京时间 2026-09-08 07:17:21]";
  const reply = "“不认真”这个标签背后";
  const streamed = marker + reply;
  for (let end = 0; end <= streamed.length; end += 1) {
    const text = streamed.slice(0, end);
    assert.equal(companionDisplayText({ role: "assistant", text, pending: true }), end <= marker.length ? "" : text.slice(marker.length));
  }
  assert.equal(companionDisplayText({ role: "assistant", text: "前文。\n[历史消息时间：北京时间 2026-09", pending: false }), "前文。\n");
});

test("companion only greets when foreground and idle", () => {
  const presence = { foreground: true, typing: false, replying: false, lastInteractionAt: 0 };
  assert.equal(canOfferCompanionGreeting(presence, 2000), true);
  assert.equal(canOfferCompanionGreeting(presence, 1999), false);
  for (const change of [{ foreground: false }, { typing: true }, { replying: true }]) {
    assert.equal(canOfferCompanionGreeting({ ...presence, ...change }, 3000), false);
  }
});

test("companion keeps authenticated image IDs from mixed replies", () => {
  assert.deepEqual(companionImageIDs(JSON.stringify([
    { kind: "image", file_id: "one" }, { mime_type: "image/png", file_id: "two" },
    { kind: "image", file_id: "one" }, { kind: "file", file_id: "document" }, null,
  ])), ["one", "two"]);
  assert.deepEqual(companionImageIDs("invalid"), []);
  assert.deepEqual(companionImageIDs('{"file_id":"untrusted"}'), []);
  assert.equal(companionStreamPath(" a/b "), "/api/v1/companion/conversations/a%2Fb/messages/stream");
});

test("memory management uses tenant-authenticated requests and preserves values", async () => {
  const requests: ApiRequest[] = [];
  const client = new CompanionClient(async <T>(request: ApiRequest): Promise<T> => { requests.push(request); return {} as T; });
  await client.open(false);
  await client.setQuiet(true);
  await client.editMemory("a/b", "正确名字");
  await client.forget("a/b");
  await client.forget();
  await client.markRead(42);
  assert.deepEqual(requests, [
    { path: "/api/v1/companion/open", method: "POST", body: { allowGreeting: false } },
    { path: "/api/v1/companion/preferences", method: "PATCH", body: { quiet: true } },
    { path: "/api/v1/companion/memories/a%2Fb", method: "PATCH", body: { value: "正确名字" } },
    { path: "/api/v1/companion/memories/a%2Fb", method: "DELETE" },
    { path: "/api/v1/companion/memories", method: "DELETE" },
    { path: "/api/v1/companion/read", method: "POST", body: { messageID: 42 } },
  ]);
});

test("topic dates use Beijing's calendar across midnight regardless of device timezone", () => {
  assert.equal(companionBeijingDate("2026-09-07T16:00:00Z"), "2026-09-08");
  assert.equal(companionBeijingDate("2026-09-07T15:59:59Z"), "2026-09-07");
  assert.equal(companionBeijingDate("2026-09-08T00:00:00+08:00"), "2026-09-08");
  assert.equal(companionBeijingDate("invalid"), "日期未知");
});

test("topic preferences refer to the displayed source and preserve the rejection signal", async () => {
  const requests: ApiRequest[] = [];
  const client = new CompanionClient(async <T>(request: ApiRequest): Promise<T> => { requests.push(request); return {} as T; });
  await client.topicFeedback("https://www.bbc.com/news/topic", "avoid");
  assert.deepEqual(requests, [{ path: "/api/v1/companion/topics/feedback", method: "POST", body: { topicURL: "https://www.bbc.com/news/topic", preference: "avoid" } }]);
});
