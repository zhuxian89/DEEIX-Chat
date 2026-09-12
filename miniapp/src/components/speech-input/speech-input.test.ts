import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import React from "react";
import ts from "typescript";
import { SpeechHoldGesture, speechTouches } from "../../platform/speech-gesture";
import { composerKeyboardStyle } from "../../product/keyboard-layout";
import { SpeechInput, type SpeechRecorder } from "../../product/speech-input";

// Exercise the production component with Taro's actual renderer and touch events.
// Only native microphone, keyboard and backend calls are replaced by fixtures.
Object.assign(globalThis, {
  ENABLE_INNER_HTML: false, ENABLE_ADJACENT_HTML: false, ENABLE_CLONE_NODE: false,
  ENABLE_CONTAINS: false, ENABLE_SIZE_APIS: false, ENABLE_TEMPLATE_CONTENT: false,
});
const { document, createEvent } = require("@tarojs/runtime");
const renderer = require("@tarojs/react");
const source = readFileSync(resolve(process.cwd(), "src/components/speech-input/speech-input.tsx"), "utf8");
const tree = ts.createSourceFile("speech-input.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const componentSource = tree.statements.filter((node) => !ts.isImportDeclaration(node)).map((node) => node.getText(tree)).join("\n");

type Element = {
  uid: string;
  className: string;
  props: Record<string, unknown>;
  childNodes: Element[];
  dispatchEvent(event: unknown): void;
};
const tick = () => new Promise<void>((done) => setTimeout(done, 20));
async function waitFor(ready: () => boolean) {
  for (let i = 0; i < 100; i++) {
    if (ready()) return;
    await tick();
  }
  assert.fail("Taro component did not reach the expected state");
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

async function mount(options: {
  transcribe?: () => Promise<{ text: string }>;
  onSend?: (text: string) => Promise<boolean>;
} = {}) {
  let events!: Parameters<SpeechRecorder["start"]>[0];
  let hide: (() => void) | undefined;
  const active: boolean[] = [];
  const sent: string[] = [];
  const file = { path: "preview.mp3", duration: 1400, size: 3000 };
  const recorder: SpeechRecorder = {
    authorize: async () => {},
    start(next) {
      events = next;
      return { stop: () => next.onStop(file), cancel: () => next.onStop(file) };
    },
    read: async () => "audio",
    remove: () => {},
  };
  const scope = {
    React, useEffect: React.useEffect, useRef: React.useRef, useState: React.useState,
    Button: "button", Text: "text", Textarea: "textarea", View: "view",
    SpeechHoldGesture, speechTouches, SpeechInput, speechRecorder: recorder, composerKeyboardStyle,
    Taro: {
      hideKeyboard: async () => {}, getWindowInfo: () => ({ windowWidth: 400 }),
      onAppHide: (callback: () => void) => { hide = callback; }, offAppHide: () => { hide = undefined; },
      onAppShow: () => {}, offAppShow: () => {},
      onKeyboardHeightChange: () => {}, offKeyboardHeightChange: () => {},
    },
  };
  const compiled = ts.transpileModule(
    `const { ${Object.keys(scope).join(",")} } = scope; ${componentSource}`,
    { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } },
  ).outputText;
  const Component = new Function("scope", "exports", `${compiled}\nreturn exports.SpeechComposer;`)(scope, {});
  const root = document.createElement("root");
  root.ctx = { setData(_data: unknown, callback?: () => void) { callback?.(); } };
  const props = {
    client: { capabilities: async () => ({ enabled: true }), transcribe: options.transcribe ?? (async () => ({ text: "刚刚说的话" })) },
    draft: "原来的草稿", disabled: false,
    onSend: async (text: string) => { sent.push(text); return options.onSend ? options.onSend(text) : true; },
    onActive: (value: boolean) => { active.push(value); }, onError: (error: string) => assert.fail(error),
  };
  const render = () => renderer.render(React.createElement(Component, props,
    React.createElement("textarea", { className: "originalDraft", value: props.draft })), root);
  function find(className: string, node: Element = root): Element | undefined {
    if (node.className?.split(" ").includes(className)) return node;
    for (const child of node.childNodes ?? []) {
      const found = find(className, child);
      if (found) return found;
    }
  }
  function get(className: string) {
    const node = find(className);
    assert.ok(node, `missing ${className}`);
    return node;
  }
  function dispatch(node: Element, type: string, data: Record<string, unknown> = {}) {
    node.dispatchEvent(createEvent({ type, target: { id: node.uid }, currentTarget: { id: node.uid }, ...data }, node));
  }
  const click = (name: string) => dispatch(get(name), "tap");
  const touch = [{ identifier: 1, clientX: 200, clientY: 700 }];
  async function record() {
    if (!find("speechHold")) click("speechButton");
    await waitFor(() => Boolean(find("speechHold")));
    const heldNode = get("speechHold");
    dispatch(heldNode, "touchstart", { touches: touch });
    await tick();
    events.onStart();
    await waitFor(() => Boolean(find("speechOverlay")));
    assert.equal(get("speechHold"), heldNode, "recording decoration must not replace the native touch owner");
    dispatch(heldNode, "touchend", { changedTouches: touch });
  }
  render();
  await waitFor(() => Boolean(find("speechButton")));
  return {
    props, active, sent, find, get, click, record,
    edit(text: string) { dispatch(get("speechPreviewInput"), "input", { detail: { value: text } }); },
    hide() { hide?.(); },
    disable() { props.disabled = true; render(); },
    unmount() { renderer.unmountComponentAtNode(root); },
  };
}

test("release opens an editable separate preview; only its Send submits, once", { timeout: 10000 }, async () => {
  const request = deferred<boolean>();
  const ui = await mount({ onSend: () => request.promise });
  try {
    await ui.record();
    await waitFor(() => Boolean(ui.find("speechPreviewInput")));
    assert.deepEqual(ui.sent, []);
    assert.equal(ui.props.draft, "原来的草稿");
    assert.equal(ui.get("speechPreviewInput").props.value, "原来的草稿 刚刚说的话");
    assert.equal(ui.active.at(-1), true, "preview should pause companion initiative");
    ui.edit("  修改后的文字  ");
    ui.click("speechPreviewSend");
    ui.click("speechPreviewSend");
    assert.deepEqual(ui.sent, ["修改后的文字"]);
    request.resolve(true);
    await waitFor(() => !ui.find("speechPreviewInput"));
    assert.equal(ui.active.at(-1), false);
  } finally { ui.unmount(); }
});

test("cancelling the independent preview preserves the original draft", { timeout: 10000 }, async () => {
  const ui = await mount();
  try {
    await ui.record();
    await waitFor(() => Boolean(ui.find("speechPreviewInput")));
    ui.edit("取消时丢弃这段识别文字");
    ui.click("speechPreviewCancel");
    await waitFor(() => !ui.find("speechPreviewInput"));
    ui.click("speechButton");
    await waitFor(() => Boolean(ui.find("originalDraft")));
    assert.equal(ui.get("originalDraft").props.value, "原来的草稿");
    assert.deepEqual(ui.sent, []);
  } finally { ui.unmount(); }
});

test("empty edits cannot send and a blocked send keeps editable text for retry", { timeout: 10000 }, async () => {
  const ui = await mount({ onSend: async () => false });
  try {
    await ui.record();
    await waitFor(() => Boolean(ui.find("speechPreviewInput")));
    ui.edit("   ");
    ui.click("speechPreviewSend");
    assert.deepEqual(ui.sent, []);
    ui.edit("想生成的图片");
    ui.click("speechPreviewSend");
    await waitFor(() => Boolean(ui.find("speechPreviewError")));
    assert.equal(ui.get("speechPreviewInput").props.value, "想生成的图片");
    ui.click("speechPreviewCancel");
    await waitFor(() => !ui.find("speechPreviewInput"));
  } finally { ui.unmount(); }
});

test("leaving during recognition discards the late preview without sending", { timeout: 10000 }, async () => {
  const response = deferred<{ text: string }>();
  const ui = await mount({ transcribe: () => response.promise });
  try {
    await ui.record();
    ui.hide();
    response.resolve({ text: "迟到的文字" });
    await waitFor(() => !ui.find("speechOverlay"));
    await tick();
    assert.equal(ui.find("speechPreviewInput"), undefined);
    assert.equal(ui.props.draft, "原来的草稿");
    assert.deepEqual(ui.sent, []);
  } finally { ui.unmount(); }
});

test("a running parent closes preview and ignores a late Send result", { timeout: 10000 }, async () => {
  const request = deferred<boolean>();
  const ui = await mount({ onSend: () => request.promise });
  try {
    await ui.record();
    await waitFor(() => Boolean(ui.find("speechPreviewInput")));
    ui.click("speechPreviewSend");
    ui.disable();
    await waitFor(() => !ui.find("speechPreviewInput"));
    request.resolve(false);
    await tick();
    assert.equal(ui.find("speechPreviewError"), undefined);
    assert.equal(ui.active.at(-1), false);
  } finally { ui.unmount(); }
});
