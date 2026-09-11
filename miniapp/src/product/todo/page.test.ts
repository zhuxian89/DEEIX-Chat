import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import React from "react";
import ts from "typescript";
import { beijingDate, isOverdue } from "./dates";
import { TodoStore } from "./store";
import { newDraft, newID, taskDraft, type EntryStatus, type Operation, type Snapshot, type TodoTask } from "./types";

Object.assign(globalThis, { ENABLE_INNER_HTML: false, ENABLE_ADJACENT_HTML: false, ENABLE_CLONE_NODE: false, ENABLE_CONTAINS: false, ENABLE_SIZE_APIS: false, ENABLE_TEMPLATE_CONTENT: false });
const { document, createEvent } = require("@tarojs/runtime");
const renderer = require("@tarojs/react");
type Element = { uid: string; className: string; props: Record<string, unknown>; childNodes: Element[]; textContent: string; dispatchEvent(event: unknown): void };
const tick = () => new Promise<void>((done) => setTimeout(done, 20));
async function waitFor(ready: () => boolean) { for (let index = 0; index < 100; index++) { if (ready()) return; await tick(); } assert.fail("TODO page did not reach the expected state"); }
function compile(path: string, scope: Record<string, unknown>, exported: string) {
  const source = readFileSync(resolve(process.cwd(), path), "utf8");
  const tree = ts.createSourceFile(path, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const body = tree.statements.filter((node) => !ts.isImportDeclaration(node)).map((node) => node.getText(tree)).join("\n");
  const compiled = ts.transpileModule(`const { ${Object.keys(scope).join(",")} } = scope; ${body}`, { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText;
  return new Function("scope", "exports", `${compiled}\nreturn exports.${exported};`)(scope, {});
}
function makeTodo(owner: string) {
  const disk = new Map<string, string>();
  const store = new TodoStore(owner, { read: (key) => disk.get(key), write: (key, value) => { disk.set(key, value); } });
  const task: TodoTask = { ...newDraft(), title: `${owner} task`, dueDate: beijingDate(), version: 1, seriesID: "", occurrence: 0, createdAt: "created", updatedAt: "created" };
  const snapshot: Snapshot = { lists: [], tasks: [task] }; store.replaceSnapshot(snapshot);
  const operations: Omit<Operation, "id">[] = [];
  return { loading: false, syncing: false, error: "", online: true, snapshot, store: { current: store },
    client: { current: { query: async () => ({ results: [] as TodoTask[], total: 0 }), feedback: async (_content: string): Promise<EntryStatus> => ({ ownerKey: owner, unlocked: false }) } },
    enqueue: (operation: Omit<Operation, "id">) => { operations.push(operation); }, redraw() {}, async sync() {}, async connect() {}, operations };
}
async function mount() {
  const host = { todo: makeTodo("owner-a"), toasts: [] as string[], routes: [] as string[] };
  const scope = { React, useState: React.useState, useEffect: React.useEffect, useMemo: React.useMemo, useRef: React.useRef,
    Button: "button", Input: "input", Picker: "picker", ScrollView: "scroll-view", Switch: "switch", Text: "text", Textarea: "textarea", View: "view",
    beijingDate, isOverdue, newDraft, newID, taskDraft,
    Taro: { showToast: async ({ title }: { title: string }) => { host.toasts.push(title); }, reLaunch: async ({ url }: { url: string }) => { host.routes.push(url); }, showModal: async () => ({ confirm: true }) },
    aiPage: "/pages/index/index", messageOf: (error: unknown) => String(error), useTodo: () => host.todo };
  const TaskEditor = compile("src/components/todo/task-editor.tsx", scope, "TaskEditor");
  const Component = compile("src/pages/entry/index.tsx", { ...scope, TaskEditor }, "default");
  const root = document.createElement("root"); root.ctx = { setData(_data: unknown, callback?: () => void) { callback?.(); } };
  const render = () => renderer.render(React.createElement(Component), root);
  function findAll(className: string, node: Element = root): Element[] {
    const found = node.className?.split(" ").includes(className) ? [node] : [];
    return [...found, ...(node.childNodes ?? []).flatMap((child) => findAll(className, child))];
  }
  function get(className: string, index = 0): Element { const node = findAll(className)[index]; assert.ok(node, `missing ${className}`); return node; }
  function dispatch(node: Element, type: string, data: Record<string, unknown> = {}) { node.dispatchEvent(createEvent({ type, target: { id: node.uid }, currentTarget: { id: node.uid }, ...data }, node)); }
  const click = (className: string, index = 0) => dispatch(get(className, index), "tap");
  const input = (className: string, value: string) => dispatch(get(className), "input", { detail: { value } });
  render(); await waitFor(() => findAll("todo-task-content").length > 0);
  return { host, render, findAll, get, click, input, unmount: () => renderer.unmountComponentAtNode(root) };
}

test("saving an open editor keeps its original version after a remote update", async () => {
  const page = await mount();
  try {
    page.click("todo-task-content"); await waitFor(() => page.findAll("todo-editor").length > 0);
    page.input("todo-title-input", "my unsaved edit"); await tick();
    const current = page.host.todo.snapshot.tasks[0];
    page.host.todo.snapshot = { lists: [], tasks: [{ ...current, version: 2, title: "remote edit" }] };
    page.host.todo.store.current.replaceSnapshot(page.host.todo.snapshot); page.render(); await tick();
    page.click("todo-link", 1); await waitFor(() => page.host.todo.operations.length === 1);
    assert.equal(page.host.todo.operations[0].baseVersion, 1);
    assert.equal(page.host.todo.operations[0].task?.title, "my unsaved edit");
  } finally { page.unmount(); }
});

async function openFeedback(page: Awaited<ReturnType<typeof mount>>) {
  page.click("todo-nav-item", 2);
  await waitFor(() => page.findAll("todo-field").some((item) => item.textContent.includes("反馈与建议")));
  const index = page.findAll("todo-field").findIndex((item) => item.textContent.includes("反馈与建议"));
  page.click("todo-field", index);
  await waitFor(() => page.findAll("todo-notes").length === 1);
}

test("feedback has one form and follows the server result with loading feedback", async () => {
  const page = await mount();
  try {
    await openFeedback(page);
    assert.equal(page.findAll("todo-input").length, 0);
    assert.equal(page.findAll("todo-primary").length, 1);
    let finish!: (status: EntryStatus) => void;
    let submitted = "";
    page.host.todo.client.current.feedback = (content) => { submitted = content; return new Promise((resolve) => { finish = resolve; }); };
    page.input("todo-notes", " ordinary feedback "); await tick();
    page.click("todo-primary"); await waitFor(() => !!submitted);
    await tick();
    assert.equal(submitted, "ordinary feedback");
    assert.equal(page.get("todo-primary").props.disabled, true);
    finish({ ownerKey: "owner-a", unlocked: false });
    await waitFor(() => page.host.toasts.length === 1);
    assert.equal(page.host.toasts[0], "反馈成功");
    assert.equal(page.get("todo-notes").props.value, "");
    assert.deepEqual(page.host.routes, []);
    page.host.todo.client.current.feedback = async () => ({ ownerKey: "owner-a", unlocked: true });
    page.input("todo-notes", "configured value"); await tick();
    page.click("todo-primary"); await waitFor(() => page.host.routes.length === 1);
    assert.equal(page.host.routes[0], "/pages/index/index");
  } finally { page.unmount(); }
});

test("a late feedback response cannot navigate another account", async () => {
  const page = await mount();
  try {
    await openFeedback(page);
    let finish!: (status: EntryStatus) => void;
    page.host.todo.client.current.feedback = () => new Promise((resolve) => { finish = resolve; });
    page.input("todo-notes", "private input"); await tick();
    page.click("todo-primary"); await waitFor(() => !!finish);
    page.host.todo = makeTodo("owner-b"); page.render(); await tick();
    finish({ ownerKey: "owner-a", unlocked: true }); await tick();
    assert.deepEqual(page.host.routes, []);
    assert.deepEqual(page.host.toasts, []);
  } finally { page.unmount(); }
});

test("a verified account change discards the previous account's open draft", async () => {
  const page = await mount();
  try {
    page.click("todo-task-content"); await waitFor(() => page.findAll("todo-editor").length > 0);
    page.input("todo-title-input", "owner-a private draft"); await tick();
    page.host.todo = makeTodo("owner-b"); page.render();
    await waitFor(() => page.findAll("todo-editor").length === 0 && page.findAll("todo-task-title").length > 0);
    assert.equal(page.get("todo-task-title").textContent, "owner-b task");
    assert.equal(page.host.todo.operations.length, 0);
  } finally { page.unmount(); }
});

test("a late search response cannot appear in another account's workspace", async () => {
  const page = await mount();
  try {
    let finish!: (value: { results: TodoTask[]; total: number }) => void;
    let called = false;
    page.host.todo.client.current.query = () => { called = true; return new Promise((resolve) => { finish = resolve; }); };
    page.input("todo-search-input", "private"); await waitFor(() => called);
    const oldTask = { ...page.host.todo.snapshot.tasks[0], title: "private result" };
    page.host.todo = makeTodo("owner-b"); page.render(); await tick();
    finish({ results: [oldTask], total: 1 }); await tick();
    assert.equal(page.get("todo-task-title").textContent, "owner-b task");
    assert.equal(page.get("todo-search-input").props.value, "");
  } finally { page.unmount(); }
});
