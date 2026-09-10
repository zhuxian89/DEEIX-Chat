import assert from "node:assert/strict";
import test from "node:test";
import { TodoStore, overlay, type CacheStorage } from "./store";
import { newDraft, type Operation, type Snapshot, type TodoTask } from "./types";

function storage(): CacheStorage {
  const values = new Map<string, string>();
  return { read: (key) => values.get(key), write: (key, value) => { values.set(key, value); } };
}
const empty: Snapshot = { lists: [], tasks: [] };
function save(title = "记得喝水"): Operation {
  const task = { ...newDraft(), title };
  return { id: task.id, entityID: task.id, kind: "task.save", baseVersion: 0, task };
}
test("offline operations survive restart and remain isolated by verified owner", () => {
  const disk = storage();
  const store = new TodoStore("owner-a", disk);
  store.enqueue(save());
  assert.equal(new TodoStore("owner-a", disk).view.tasks[0].title, "记得喝水");
  assert.equal(new TodoStore("owner-b", disk).view.tasks.length, 0);
});
test("storage failure never reports a successful local edit", () => {
  const store = new TodoStore("owner", { read: () => undefined, write: () => { throw new Error("full"); } });
  assert.throws(() => store.enqueue(save()), /full/);
  assert.equal(store.pending.length, 0);
  assert.equal(store.view.tasks.length, 0);
});
test("a conflict keeps the local branch across snapshots and restarts", () => {
  const disk = storage();
  const store = new TodoStore("owner", disk);
  const operation = save("我的修改");
  store.enqueue(operation);
  store.accept({ results: [{ id: operation.id, status: "conflict", message: "changed" }], snapshot: empty });
  const restored = new TodoStore("owner", disk);
  assert.equal(restored.pending[0].operation.task?.title, "我的修改");
  assert.equal(restored.pending[0].problem?.status, "conflict");
});
test("applied retry receipt settles exactly one operation and preserves subsequent edits", () => {
  const store = new TodoStore("owner", storage());
  const first = save();
  const second = save("第二件事");
  store.enqueue(first); store.enqueue(second);
  store.accept({ results: [{ id: first.id, status: "duplicate" }], snapshot: empty });
  assert.deepEqual(store.pending.map((item) => item.operation.id), [second.id]);
  assert.equal(store.view.tasks[0].title, "第二件事");
});
test("1000 tasks and pending edits restore without loss", () => {
  const disk = storage();
  const store = new TodoStore("owner", disk);
  for (let index = 0; index < 1000; index += 1) store.enqueue(save(`任务 ${index}`));
  assert.equal(new TodoStore("owner", disk).view.tasks.length, 1000);
});

function parentAndChild(): Snapshot {
  const parent = { ...newDraft(), title: "parent", version: 1, seriesID: "", occurrence: 0, createdAt: "created", updatedAt: "created" } as TodoTask;
  const child = { ...parent, id: "child", parentID: parent.id, title: "step" };
  return { lists: [], tasks: [parent, child] };
}
test("offline parent completion and undo restore child status and exact version", () => {
  const snapshot = parentAndChild(); const parent = snapshot.tasks[0];
  const operations: Operation[] = [
    { id: "complete", entityID: parent.id, kind: "task.complete", baseVersion: 1, completed: true },
    { id: "undo", entityID: parent.id, kind: "task.complete", baseVersion: 2, completed: false },
  ];
  const child = overlay(snapshot, operations).tasks.find((task) => task.id === "child")!;
  assert.equal(child.completedAt, undefined); assert.equal(child.version, 3);
});
test("parent delete and restore keep previously deleted children deleted", () => {
  const snapshot = parentAndChild(); const parent = snapshot.tasks[0];
  snapshot.tasks[1].deletedAt = "earlier-deletion";
  const result = overlay(snapshot, [
    { id: "delete", entityID: parent.id, kind: "task.delete", baseVersion: 1 },
    { id: "restore", entityID: parent.id, kind: "task.restore", baseVersion: 2 },
  ]).tasks.find((task) => task.id === "child")!;
  assert.equal(result.deletedAt, "earlier-deletion"); assert.equal(result.version, 1);
});
test("undo leaves a child's later independent change alone and ignores no-op completion", () => {
  const snapshot = parentAndChild(); const parent = snapshot.tasks[0];
  const result = overlay(snapshot, [
    { id: "complete", entityID: parent.id, kind: "task.complete", baseVersion: 1, completed: true },
    { id: "child-undo", entityID: "child", kind: "task.complete", baseVersion: 2, completed: false },
    { id: "child-complete", entityID: "child", kind: "task.complete", baseVersion: 3, completed: true },
    { id: "parent-undo", entityID: parent.id, kind: "task.complete", baseVersion: 4, completed: false },
    { id: "noop", entityID: parent.id, kind: "task.complete", baseVersion: 5, completed: false },
  ]);
  const child = result.tasks.find((task) => task.id === "child")!;
  assert.ok(child.completedAt); assert.equal(child.version, 4);
  assert.equal(result.tasks[0].version, 5);
});
