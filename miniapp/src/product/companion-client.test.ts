import assert from "node:assert/strict";
import test from "node:test";
import { CompanionClient, canOfferCompanionGreeting, companionBeijingDate, companionImageIDs, companionStreamPath } from "./companion-client";
import { companionDisplayText } from "./companion-message";
import type { ApiRequest } from "@/platform/transport";
import type { ReminderCommandResult, ReminderState } from "@deeix/api-contract";

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

test("a late reminder read cannot restore a banner after a successful pause or deletion", async () => {
  const empty: ReminderState = { plans: [], turns: [], occurrences: [], serverTime: "2026-10-17T12:06:40Z" };
  let finishRead!: (state: ReminderState) => void;
  const result: ReminderCommandResult = { handled: true, snapshot: empty, turn: { id: "deleted", input: "", reminderIDs: [], reply: "已删除", createdAt: empty.serverTime } };
  const client = new CompanionClient(<T>(request: ApiRequest): Promise<T> => request.method === "POST"
    ? Promise.resolve(result as T) : new Promise<ReminderState>((resolve) => { finishRead = resolve; }) as Promise<T>);
  const received: ReminderState[] = [];
  const unsubscribe = client.subscribeReminders((state) => received.push(state));
  const oldRead = client.reminders();
  await client.reminderCommand({ requestID: "delete", action: "delete" });
  finishRead({ ...empty, serverTime: "2026-10-17T12:06:35Z" });
  await oldRead;
  assert.deepEqual(received, [empty]);
  unsubscribe();
  await client.reminderCommand({ requestID: "second", action: "delete" });
  assert.equal(received.length, 1);
});

test("ordinary chat does not clear reminder attention and subscription observers are isolated per session", async () => {
  const result: ReminderCommandResult = { handled: false, snapshot: { plans: [], turns: [], occurrences: [], serverTime: "" }, turn: { id: "", input: "", reminderIDs: [], reply: "", createdAt: "" } };
  const client = new CompanionClient(async <T>(): Promise<T> => result as T);
  let received = 0;
  client.subscribeReminders(() => { received++; });
  await client.reminderCommand({ requestID: "ordinary", input: "今天怎么样" });
  assert.equal(received, 0);
  const other = new CompanionClient(async <T>(): Promise<T> => ({ ...result, handled: true }) as T);
  await other.reminderCommand({ requestID: "other", action: "delete" });
  assert.equal(received, 0);
});

test("reads during a mutation are not broadcast before its confirmed result", async () => {
  const state: ReminderState = { plans: [], turns: [], occurrences: [], serverTime: "2026-10-17T12:06:40Z" };
  let finishWrite!: (result: ReminderCommandResult) => void;
  const client = new CompanionClient(<T>(request: ApiRequest): Promise<T> => request.method === "POST"
    ? new Promise<ReminderCommandResult>((resolve) => { finishWrite = resolve; }) as Promise<T> : Promise.resolve(state as T));
  const received: ReminderState[] = [];
  client.subscribeReminders((next) => received.push(next));
  const write = client.reminderCommand({ requestID: "pause", action: "pause" });
  await client.reminders();
  assert.equal(received.length, 0);
  finishWrite({ handled: true, snapshot: state, turn: { id: "paused", input: "", reminderIDs: [], reply: "已暂停", createdAt: state.serverTime } });
  await write;
  assert.deepEqual(received, [state]);
});
