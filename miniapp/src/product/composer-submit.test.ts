import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import React from "react";
import ts from "typescript";
import { composerKeyboardHandlers } from "./keyboard-layout";

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
  const requests: { text: string; resolve: (result: object) => void; reject: (error: Error) => void }[] = [];
  const request = (...args: unknown[]) => new Promise<object>((resolve, reject) => {
    requests.push({ text: args[name === "sendChat" ? 2 : 3] as string, resolve, reject });
  });
  const scope: Record<string, unknown> = {
    sessionRef: { current: { sendChat: request, generateImage: request } },
    currentConversation: { publicID: "conversation" },
    selectedChatModel: { platformModelName: "chat-model" }, selectedImageModel: { platformModelName: "image-model" },
    prompt: "stale draft", pendingImage: null, running: false, uploading: false,
    composerSendingRef: { current: false }, messageCounter: { current: 0 },
    networkSearchAvailable: false, networkSearchEnabled: false,
    applyOptimisticConversationTitle: () => false, resolveImageSubmitDecision: () => ({ task: "image_generation" }),
    createPendingImageTurn: () => [], MiniAppRequestAbortedError: class extends Error {},
  };
  for (const callback of ["enableChatAutoFollow", "setWorkspaceError", "setMessages", "setPrompt", "setPendingImage",
    "setRunning", "setStopping", "updateConversation", "refreshConversations", "refreshBalance"]) scope[callback] = () => {};
  Object.assign(scope, overrides);
  return { send: evaluate(declaration.initializer!.getText(page), scope) as (text: string) => Promise<boolean>, requests };
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
