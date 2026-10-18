import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import React from "react";
import ts from "typescript";
import { composerKeyboardHandlers } from "./keyboard-layout";
import {
  applyImageProgress, applyMessageImageDownload, createPendingImageTurn, latestVisibleMessages,
  messageFromAPI, messageImageFileIDs, messageImages, type ConversationMessage,
} from "./message-timeline";

const page = source("src/pages/index/index.tsx");
const companion = source("src/components/companion/companion.tsx");

function source(path: string) {
  return ts.createSourceFile(path, readFileSync(resolve(process.cwd(), path), "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
}

function findNode(tree: ts.Node, matches: (node: ts.Node) => boolean): ts.Node {
  let found: ts.Node | undefined;
  const visit = (node: ts.Node) => {
    if (matches(node)) found = node;
    else ts.forEachChild(node, visit);
  };
  visit(tree);
  assert.ok(found, "expected production node to exist");
  return found;
}

function evaluate(expression: string, scope: Record<string, unknown>) {
  const compiled = ts.transpileModule(
    `const { ${Object.keys(scope).join(",")} } = scope; return (${expression});`,
    { compilerOptions: { jsx: ts.JsxEmit.React, target: ts.ScriptTarget.ES2020 } },
  ).outputText;
  return new Function("scope", compiled)(scope);
}

for (const mode of ["chat", "image", "companion"] as const) {
  test(`${mode} native confirm sends the latest value; typing, newlines and IME do not send`, async () => {
    const tree = mode === "companion" ? companion : page;
    const className = mode === "companion" ? "companionInput" : "composerInput";
    const textarea = findNode(tree, (node) => {
      const opening = ts.isJsxElement(node) ? node.openingElement : ts.isJsxSelfClosingElement(node) ? node : null;
      return Boolean(opening?.attributes.properties.some((prop) => ts.isJsxAttribute(prop)
        && prop.name.getText(tree) === "className" && prop.initializer?.getText(tree) === `"${className}"`));
    });
    const sent: string[] = [];
    const drafts: string[] = [];
    const submit = async (text: string) => { sent.push(text); };
    const unexpectedSubmit = () => assert.fail("confirm used the wrong conversation mode");
    const scope: Record<string, unknown> = {
      React, Textarea: "textarea", KeyboardAccessory: "keyboard-accessory", composerKeyboardHandlers,
      screen: mode, prompt: "stale draft", draft: "stale draft", pendingImage: null,
      running: false, uploading: false, speechActive: false, stopping: false,
      busy: false, loading: false, memoryOpen: false, composerComposingRef: { current: false },
      photos: [], choosing: false, photoComposer: { blocked: false },
      setKeyboardHeight: () => {}, setKeyboard: () => {}, measureHistory: () => {}, recordCompanionInteraction: () => {},
      setPrompt: (value: string) => drafts.push(value), setDraft: (value: string) => drafts.push(value),
      sendChat: mode === "chat" ? submit : unexpectedSubmit,
      generateImage: mode === "image" ? submit : unexpectedSubmit, send: submit,
    };
    const props = () => evaluate(textarea.getText(tree), scope).props;
    const input = props();
    assert.equal(input.confirmType, "send");
    assert.equal(input.confirmHold, true);
    input.onInput({ detail: { value: "first line\nsecond line", keyCode: 13 } });
    assert.deepEqual(drafts, ["first line\nsecond line"]);
    assert.deepEqual(sent, []);

    input.onKeyboardCompositionStart();
    await input.onConfirm({ detail: { value: "候选词" } });
    assert.deepEqual(sent, []);
    input.onKeyboardCompositionEnd();
    await input.onConfirm({ detail: { value: "最终文字" } });
    assert.deepEqual(sent, ["最终文字"]);

    input.onKeyboardCompositionStart();
    input.onBlur();
    await input.onConfirm({ detail: { value: "重新聚焦后发送" } });
    assert.deepEqual(sent, ["最终文字", "重新聚焦后发送"]);

    scope.speechActive = true;
    await props().onConfirm({ detail: { value: "must not send during speech input" } });
    scope.speechActive = false;
    scope[mode === "companion" ? "loading" : "stopping"] = true;
    await props().onConfirm({ detail: { value: "must not send while unavailable" } });
    assert.deepEqual(sent, ["最终文字", "重新聚焦后发送"]);
  });
}

function workspaceSendFixture(name: "sendChat" | "generateImage", overrides: Record<string, unknown> = {}) {
  const declaration = findNode(page, (node) => ts.isVariableDeclaration(node) && node.name.getText(page) === name) as ts.VariableDeclaration;
  const requests: { text: string; fileIDs: unknown; resolve: (result: object) => void; reject: (error: Error) => void }[] = [];
  const request = (...args: unknown[]) => new Promise<object>((resolve, reject) => {
    requests.push({ text: args[name === "sendChat" ? 2 : 3] as string, fileIDs: args[name === "sendChat" ? 4 : 5], resolve, reject });
  });
  const scope: Record<string, unknown> = {
    sessionRef: { current: { sendChat: request, generateImage: request, listMessages: async () => [] } },
    currentConversation: { publicID: "conversation" },
    selectedChatModel: { platformModelName: "chat-model" }, selectedImageModel: { platformModelName: "image-model" },
    prompt: "stale draft", pendingImage: null, running: false, uploading: false,
    composerSendingRef: { current: false }, messageCounter: { current: 0 },
    networkSearchAvailable: false, networkSearchEnabled: false,
    messages: [], historyLoadCounter: { current: 0 },
    chatPhotos: { snapshot: () => ({ photos: [], choosing: false, limit: 9 }), clear: () => {} },
    applyImageProgress, applyMessageImageDownload, latestVisibleMessages, messageFromAPI, messageImageFileIDs,
    downloadMessagePhotos: async () => {},
    applyOptimisticConversationTitle: () => false, resolveImageSubmitDecision: () => ({ task: "image_generation" }),
    createPendingImageTurn: () => [], MiniAppRequestAbortedError: class extends Error {},
  };
  for (const callback of ["enableChatAutoFollow", "setWorkspaceError", "setMessages", "setPrompt", "setPendingImage",
    "setRunning", "setStopping", "updateConversation", "refreshConversations", "refreshBalance"]) scope[callback] = () => {};
  Object.assign(scope, overrides);
  return { send: evaluate(declaration.initializer!.getText(page), scope) as (text: string) => Promise<boolean>, requests, scope };
}

function photoDraft() {
  const photos = ["third", "first", "second"].map((fileID) => ({ id: fileID, fileID, path: `temp://${fileID}`, status: "ready" }));
  let clears = 0;
  return {
    photos,
    snapshot: () => ({ photos, choosing: false, limit: 9 }),
    clear: () => { clears += 1; },
    get clears() { return clears; },
  };
}

test("chat sends every selected photo in order and clears only after successful acknowledgement", async () => {
  const chatPhotos = photoDraft();
  const { send, requests } = workspaceSendFixture("sendChat", {
    chatPhotos,
    pendingImage: { fileID: "unrelated-edit-image", localPath: "temp://edit" },
  });
  const sending = send("");
  assert.deepEqual(requests[0].fileIDs, ["third", "first", "second"]);
  assert.equal(chatPhotos.clears, 0);
  requests[0].resolve({ text: "three photos" });
  await sending;
  assert.equal(chatPhotos.clears, 1);
});

test("chat blocks partial photo uploads, failed photos, open pickers and reduced limits", async () => {
  for (const snapshot of [
    { photos: [{ fileID: "one", status: "ready" }, { status: "uploading" }], choosing: false, limit: 9 },
    { photos: [{ fileID: "one", status: "ready" }, { status: "failed" }], choosing: false, limit: 9 },
    { photos: [{ status: "ready" }], choosing: false, limit: 9 },
    { photos: [], choosing: true, limit: 9 },
    { photos: [{ fileID: "one", status: "ready" }, { fileID: "two", status: "ready" }], choosing: false, limit: 1 },
  ]) {
    const { send, requests } = workspaceSendFixture("sendChat", {
      chatPhotos: { snapshot: () => snapshot, clear: () => assert.fail("blocked draft cleared") },
    });
    assert.equal(await send("compare photos"), false);
    assert.equal(requests.length, 0);
  }
});

test("an unpersisted failed photo turn retains all photos and restores its text", async () => {
  const chatPhotos = photoDraft();
  const drafts: string[] = [];
  const { send, requests } = workspaceSendFixture("sendChat", { chatPhotos, setPrompt: (value: string) => drafts.push(value) });
  const sending = send("compare photos");
  requests[0].reject(new Error("disconnected"));
  await sending;
  assert.equal(chatPhotos.clears, 0);
  assert.equal(drafts.at(-1), "compare photos");
});

test("a disconnected photo stream clears the draft when the photo-only turn already persisted", async () => {
  const chatPhotos = photoDraft();
  const fixture = workspaceSendFixture("sendChat", { chatPhotos });
  const session = (fixture.scope.sessionRef as { current: Record<string, unknown> }).current;
  session.listMessages = async () => [{
    attachments: JSON.stringify(chatPhotos.photos.map((photo) => ({ file_id: photo.fileID, kind: "image" }))),
    branchReason: "default", content: "请参考附件", contentType: "mixed", createdAt: "2026-10-17T00:00:00Z",
    errorMessage: "", parentPublicID: "", platformModelName: "chat-model", publicID: "persisted-user",
    role: "user", runID: "", sourcePublicID: "", status: "success",
  }];
  const sending = fixture.send("");
  fixture.requests[0].reject(new Error("disconnected"));
  await sending;
  assert.equal(chatPhotos.clears, 1);
});

test("an old chat acknowledgement cannot clear photos after switching conversations", async () => {
  const chatPhotos = photoDraft();
  const historyLoadCounter = { current: 0 };
  const { send, requests } = workspaceSendFixture("sendChat", { chatPhotos, historyLoadCounter });
  const sending = send("compare photos");
  historyLoadCounter.current += 1;
  requests[0].resolve({ text: "reply from old conversation" });
  await sending;
  assert.equal(chatPhotos.clears, 0);
});

test("image editing retains the original photo when the persisted turn replaces the optimistic message", async () => {
  let messages: ConversationMessage[] = [];
  const { send, requests } = workspaceSendFixture("generateImage", {
    pendingImage: { fileID: "original-photo", localPath: "temp://original" },
    createPendingImageTurn,
    resolveImageSubmitDecision: () => ({ task: "image_edit" }),
    setMessages: (update: (current: ConversationMessage[]) => ConversationMessage[]) => { messages = update(messages); },
  });
  const sending = send("edit this photo");
  requests[0].resolve({
    imageSource: "temp://edited",
    userMessage: {
      attachments: JSON.stringify([{ file_id: "original-photo", kind: "image" }]),
      branchReason: "default", content: "edit this photo", contentType: "mixed", createdAt: "2026-10-18T00:00:00Z",
      errorMessage: "", parentPublicID: "", platformModelName: "image-model", publicID: "persisted-user",
      role: "user", runID: "", sourcePublicID: "", status: "success",
    },
  });
  await sending;
  const original = messages.find((message) => message.id === "persisted-user");
  assert.ok(original);
  assert.equal(messageImages(original)[0].source, "temp://original");
  assert.equal(messageImages(original)[0].status, undefined);
  assert.deepEqual(messageImageFileIDs(original), ["original-photo"]);
});

for (const photoIDs of [["first-photo"], ["first-photo", "second-photo"]]) {
test(`a chat reply with ${photoIDs.length} photos only associates unambiguous stream previews`, async () => {
  let messages: ConversationMessage[] = [];
  const queued: ConversationMessage[] = [];
  const { send, requests } = workspaceSendFixture("sendChat", {
    setMessages: (update: (current: ConversationMessage[]) => ConversationMessage[]) => { messages = update(messages); },
    downloadMessagePhotos: async (items: ConversationMessage[]) => { queued.push(...items); },
  });
  const sending = send("show photos");
  requests[0].resolve({
    text: "two photos", imageSource: "temp://first-photo",
    assistantMessage: {
      attachments: JSON.stringify(photoIDs.map((file_id) => ({ file_id, kind: "image" }))),
      branchReason: "default", content: "two photos", contentType: "mixed", createdAt: "2026-10-18T00:00:00Z",
      errorMessage: "", parentPublicID: "", platformModelName: "chat-model", publicID: "persisted-assistant",
      role: "assistant", runID: "", sourcePublicID: "", status: "success",
    },
  });
  await sending;
  const reply = messages.find((message) => message.id === "persisted-assistant");
  assert.ok(reply);
  assert.deepEqual(messageImages(reply).map((image) => [image.fileID, image.source]),
    photoIDs.map((fileID) => [fileID, photoIDs.length === 1 ? "temp://first-photo" : undefined]));
  assert.deepEqual(queued, [reply]);
});
}

for (const outcome of ["success", "failure"] as const) {
  test(`a stale photo regeneration ${outcome} cannot write into a newly opened conversation`, async () => {
    const declaration = findNode(page, (node) => ts.isVariableDeclaration(node) && node.name.getText(page) === "regenerateChatAnswer") as ts.VariableDeclaration;
    const original: ConversationMessage[] = [
      { id: "parent", role: "user", text: "compare", images: [{ fileID: "first" }, { fileID: "second" }] },
      { id: "answer", role: "assistant", text: "old answer", parentPublicID: "parent" },
    ];
    let current = original;
    let requestedFileIDs: readonly string[] = [];
    let onProgress: ((progress: object) => void) | undefined;
    let resolveRequest: (result: object) => void = () => {};
    let rejectRequest: (cause: Error) => void = () => {};
    const historyLoadCounter = { current: 0 };
    const composerSendingRef = { current: false };
    const scope: Record<string, unknown> = {
      sessionRef: { current: { sendChat: (...args: unknown[]) => {
        requestedFileIDs = args[4] as string[];
        onProgress = args[3] as (progress: object) => void;
        return new Promise<object>((resolve, reject) => { resolveRequest = resolve; rejectRequest = reject; });
      } } },
      currentConversation: { publicID: "original-conversation" }, selectedChatModel: { platformModelName: "model" },
      messages: original, running: false, composerSendingRef, historyLoadCounter, messageCounter: { current: 0 },
      messageImageFileIDs, messageFromAPI, applyImageProgress, applyMessageImageDownload,
      networkSearchAvailable: false, networkSearchEnabled: false,
      Taro: { showToast: () => {} }, MiniAppRequestAbortedError: class extends Error {},
      setMessages: (value: ConversationMessage[] | ((items: ConversationMessage[]) => ConversationMessage[])) => {
        current = typeof value === "function" ? value(current) : value;
      },
    };
    const stateUpdates: unknown[] = [];
    for (const name of ["enableChatAutoFollow", "setRunning", "setStopping", "setWorkspaceError", "refreshBalance", "refreshConversations", "downloadMessagePhotos"]) {
      scope[name] = (value: unknown) => { stateUpdates.push(value); };
    }
    const regenerate = evaluate(declaration.initializer!.getText(page), scope) as (message: ConversationMessage) => Promise<void>;
    const request = regenerate(original[1]);
    assert.deepEqual(requestedFileIDs, ["first", "second"]);
    historyLoadCounter.current += 1;
    composerSendingRef.current = true;
    current = [{ id: "new-conversation-photo", role: "user", text: "new turn", images: [{ fileID: "new" }] }];
    const expected = current;
    stateUpdates.length = 0;
    onProgress?.({ text: "late old answer" });
    if (outcome === "success") resolveRequest({ text: "late answer", imageSource: "temp://old-photo" });
    else rejectRequest(new Error("late failure"));
    await request;
    assert.equal(current, expected);
    assert.deepEqual(stateUpdates, []);
    assert.equal(composerSendingRef.current, true, "an old request cannot unlock a new request");
  });
}

for (const name of ["sendChat", "generateImage"] as const) {
  test(`${name} prevents duplicate requests before React rerenders and unlocks after failure`, async () => {
    const { send, requests } = workspaceSendFixture(name);
    const first = send("latest text");
    assert.equal(await send("latest text"), false);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].text, "latest text");
    requests[0].reject(new Error("request failed"));
    await first;
    const retry = send("retry text");
    assert.equal(requests.length, 2);
    requests[1].resolve({ text: "reply" });
    await retry;
    const next = send("next text");
    assert.equal(requests.length, 3);
    requests[2].resolve({ text: "reply" });
    await next;
  });

  test(`${name} ignores empty input and blocks submits while uploading or generating`, async () => {
    for (const overrides of [{}, { uploading: true }, { running: true }]) {
      const { send, requests } = workspaceSendFixture(name, overrides);
      assert.equal(await send(Object.keys(overrides).length ? "ready to send" : " \n "), false);
      assert.deepEqual(requests, []);
    }
  });
}
