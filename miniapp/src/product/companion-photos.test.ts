import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import ts from "typescript";
import type { ConversationMessage } from "./message-timeline";

type DisplayMessage = ConversationMessage & { fileIDs?: string[] };
type SelectedPhoto = { id: string; path: string; fileID?: string; status: "ready" | "uploading" | "failed" };

function actualHandler(name: string, scope: Record<string, unknown>) {
  const source = readFileSync(resolve(process.cwd(), "src/components/companion/companion.tsx"), "utf8");
  const tree = ts.createSourceFile("companion.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let handler: ts.ArrowFunction | undefined;
  const visit = (node: ts.Node) => {
    if (ts.isVariableDeclaration(node) && node.name.getText(tree) === name && node.initializer && ts.isArrowFunction(node.initializer)) handler = node.initializer;
    ts.forEachChild(node, visit);
  };
  visit(tree);
  assert.ok(handler, `actual ${name} handler is present`);
  const compiled = ts.transpile(`with (scope) { return (${handler.getText(tree)}); }`, { target: ts.ScriptTarget.ES2022 });
  return new Function("scope", compiled)(scope);
}

function sendFixture() {
  const selected: SelectedPhoto[] = [
    { id: "one", path: "local-one.jpg", fileID: "file-one", status: "ready" },
    { id: "two", path: "local-two.jpg", fileID: "file-two", status: "ready" },
  ];
  const sent: unknown[][] = [];
  const scope = {
    selected,
    photoComposer: {
      snapshot: () => ({ photos: scope.selected, choosing: false, limit: 9 }),
      clear: () => { scope.selected = []; },
    },
    conversation: { publicID: "companion-conversation" },
    state: { model: { id: 1 } },
    busyRef: { current: false },
    loading: false,
    speechActive: false,
    draft: "一起看看这两张照片",
    messages: [] as DisplayMessage[],
    history: [] as DisplayMessage[],
    cause: null as Error | null,
    mounted: { current: true },
    autoFollow: { current: false },
    reminderRetry: { current: null },
    reminderVersion: { current: 0 },
    COMPANION_NAME: "小禾",
    setBusy: () => {},
    setError: () => {},
    setFollowing: () => {},
    setDraft: (text: string) => { scope.draft = text; },
    setMessages: (update: (current: DisplayMessage[]) => DisplayMessage[]) => { scope.messages = update(scope.messages); },
    recordCompanionInteraction: () => {},
    updateStreamMessage: () => {},
    toDisplay: (message: DisplayMessage) => message,
    loadImages: async () => {},
    loadHistory: async () => { scope.messages = scope.history; return scope.history; },
    errorMessage: (cause: Error) => cause.message,
    MiniAppRequestAbortedError: class extends Error {},
    session: { sendChat: async (...args: unknown[]) => {
      sent.push(args);
      if (scope.cause) throw scope.cause;
      return { text: "两张照片的区别是……" };
    } },
  };
  return { scope, sent, send: actualHandler("send", scope) as () => Promise<boolean> };
}

test("companion submits every selected photo together and preserves local display order", async () => {
  const { scope, sent, send } = sendFixture();
  assert.equal(await send(), true);
  assert.deepEqual(sent[0][4], ["file-one", "file-two"]);
  assert.deepEqual(scope.messages[0].images, [
    { fileID: "file-one", source: "local-one.jpg" },
    { fileID: "file-two", source: "local-two.jpg" },
  ]);
  assert.deepEqual(scope.selected, []);
});

test("companion never silently excludes an uploading or failed photo", async () => {
  for (const status of ["uploading", "failed"] as const) {
    const { scope, sent, send } = sendFixture();
    scope.selected[1] = { ...scope.selected[1], fileID: undefined, status };
    assert.equal(await send(), false);
    assert.equal(sent.length, 0);
    assert.equal(scope.selected.length, 2);
    assert.equal(scope.draft, "一起看看这两张照片");
  }
});

test("companion retains photos on rejected send but clears a confirmed persisted turn", async () => {
  for (const persisted of [false, true]) {
    const { scope, send } = sendFixture();
    scope.cause = new Error("connection interrupted");
    scope.history = persisted ? [{ id: "new-server-message", role: "user", text: scope.draft, fileIDs: ["file-one", "file-two"] }] : [];
    assert.equal(await send(), persisted);
    assert.equal(scope.selected.length, persisted ? 0 : 2);
    assert.equal(scope.draft, persisted ? "" : "一起看看这两张照片");
    assert.equal(scope.busyRef.current, false);
  }
});

test("companion does not mistake an older turn with the same photos for a failed new send", async () => {
  const { scope, send } = sendFixture();
  const previous: DisplayMessage = { id: "old-message", role: "user", text: scope.draft, fileIDs: ["file-one", "file-two"] };
  scope.messages = [previous];
  scope.history = [previous];
  scope.cause = new Error("send rejected");
  await send();
  assert.equal(scope.selected.length, 2);
});

test("companion recognizes the persisted fallback text when photos were sent without a caption", async () => {
  const { scope, send } = sendFixture();
  scope.draft = "";
  scope.history = [{ id: "new-photo-message", role: "user", text: "请参考附件", fileIDs: ["file-one", "file-two"] }];
  scope.cause = new Error("reply interrupted");
  await send();
  assert.equal(scope.selected.length, 0);
});

test("companion keeps each failed history photo in place and retries only that photo", async () => {
  const calls: string[] = [];
  const completions = new Map<string, { resolve: (source: string) => void; reject: (cause: Error) => void }>();
  const scope = {
    mounted: { current: true },
    imageLoads: { current: new Set<string>() },
    messages: [{ id: "message", role: "user", text: "", fileIDs: ["one", "two"], images: [{ fileID: "one" }, { fileID: "two" }] }] as DisplayMessage[],
    setMessages: (update: (current: DisplayMessage[]) => DisplayMessage[]) => { scope.messages = update(scope.messages); },
    session: { downloadMessageImage: (fileID: string) => {
      calls.push(fileID);
      return new Promise<string>((resolve, reject) => { completions.set(fileID, { resolve, reject }); });
    } },
  };
  const load = actualHandler("loadImages", scope) as (messages: DisplayMessage[], fileID?: string) => Promise<void>;
  const first = load(scope.messages);
  completions.get("two")!.resolve("download-two.jpg");
  completions.get("one")!.reject(new Error("temporary failure"));
  await first;
  assert.deepEqual(scope.messages[0].images, [
    { fileID: "one", source: undefined, status: "图片加载失败，点此重试" },
    { fileID: "two", source: "download-two.jpg", status: undefined },
  ]);
  const retry = load(scope.messages, "one");
  completions.get("one")!.resolve("download-one.jpg");
  await retry;
  assert.deepEqual(calls, ["one", "two", "one"]);
  assert.deepEqual(scope.messages[0].images?.map((photo) => photo.source), ["download-one.jpg", "download-two.jpg"]);
});

test("companion treats an empty image download as retryable and releases its download lock", async () => {
  let calls = 0;
  const scope = {
    mounted: { current: true },
    imageLoads: { current: new Set<string>() },
    messages: [{ id: "message", role: "user", text: "", fileIDs: ["one"], images: [{ fileID: "one" }] }] as DisplayMessage[],
    setMessages: (update: (current: DisplayMessage[]) => DisplayMessage[]) => { scope.messages = update(scope.messages); },
    session: { downloadMessageImage: async () => ++calls === 1 ? null : "download-one.jpg" },
  };
  const load = actualHandler("loadImages", scope) as (messages: DisplayMessage[], fileID?: string) => Promise<void>;
  await load(scope.messages);
  assert.equal(scope.messages[0].images?.[0].status, "图片加载失败，点此重试");
  assert.equal(scope.imageLoads.current.size, 0);
  await load(scope.messages, "one");
  assert.equal(scope.messages[0].images?.[0].source, "download-one.jpg");
  assert.equal(calls, 2);
});

test("companion preserves fetched history for send reconciliation when reminder refresh fails", async () => {
  const history: DisplayMessage[] = [{ id: "persisted-photo", role: "user", text: "", fileIDs: ["one", "two"] }];
  const scope = {
    mounted: { current: true },
    messages: [] as DisplayMessage[],
    error: "",
    session: { listMessages: async () => history },
    visibleMessages: (messages: DisplayMessage[]) => messages,
    setMessages: (messages: DisplayMessage[]) => { scope.messages = messages; },
    loadImages: async () => {},
    refreshReminders: async () => { throw new Error("reminders unavailable"); },
    setError: (error: string) => { scope.error = error; },
    errorMessage: (cause: Error) => cause.message,
  };
  const load = actualHandler("loadHistory", scope) as (id: string) => Promise<DisplayMessage[]>;
  assert.deepEqual(await load("conversation"), history);
  assert.deepEqual(scope.messages, history);
  assert.equal(scope.error, "reminders unavailable");
});
