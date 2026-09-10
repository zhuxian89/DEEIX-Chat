import type { Operation, OperationResult, Snapshot, SyncResult, TodoTask } from "./types";

export interface CacheStorage { read(key: string): string | undefined; write(key: string, value: string): void }
type Pending = { operation: Operation; problem?: OperationResult };
type Cache = { format: 1; ownerKey: string; snapshot: Snapshot; pending: Pending[] };
const empty = (): Snapshot => ({ lists: [], tasks: [] });

export class TodoStore {
  private cache: Cache;
  private readonly key: string;
  constructor(readonly ownerKey: string, private readonly storage: CacheStorage) {
    if (!ownerKey) throw new Error("需要先确认微信身份");
    this.key = `miniapp-todo:v1:${ownerKey}`;
    this.cache = { format: 1, ownerKey, snapshot: empty(), pending: [] };
    const saved = storage.read(this.key);
    if (saved) {
      let parsed: Cache;
      try { parsed = JSON.parse(saved) as Cache; } catch { throw new Error("本地待办数据无法读取，请保留缓存并重试"); }
      if (parsed.format !== 1 || parsed.ownerKey !== ownerKey || !Array.isArray(parsed.pending)
        || !Array.isArray(parsed.snapshot?.tasks) || !Array.isArray(parsed.snapshot?.lists)) {
        throw new Error("本地待办数据版本不兼容，请保留缓存");
      }
      this.cache = parsed;
    }
  }
  get pending(): readonly Pending[] { return this.cache.pending; }
  get serverSnapshot(): Snapshot { return this.cache.snapshot; }
  get view(): Snapshot { return overlay(this.cache.snapshot, this.cache.pending.map((item) => item.operation)); }
  enqueue(operation: Operation): void {
    this.commit({ ...this.cache, pending: [...this.cache.pending, { operation }] });
  }
  replaceSnapshot(snapshot: Snapshot): void { this.commit({ ...this.cache, snapshot }); }
  accept(result: SyncResult): void {
    const receipts = new Map(result.results.map((item) => [item.id, item]));
    const pending = this.cache.pending.flatMap((item): Pending[] => {
      const receipt = receipts.get(item.operation.id);
      if (!receipt) return [item];
      return receipt.status === "applied" || receipt.status === "duplicate" ? [] : [{ ...item, problem: receipt }];
    });
    this.commit({ ...this.cache, snapshot: result.snapshot, pending });
  }
  discardEntity(entityID: string): void {
    // Dependent edits on the same entity were based on the rejected branch.
    this.commit({ ...this.cache, pending: this.cache.pending.filter((item) => item.operation.entityID !== entityID) });
  }
  private commit(next: Cache): void {
    const encoded = JSON.stringify(next);
    if (encoded.length > 12 * 1024 * 1024) throw new Error("本机待办较多，请先联网同步并整理后继续");
    this.storage.write(this.key, encoded);
    this.cache = next;
  }
}

export function overlay(snapshot: Snapshot, operations: readonly Operation[]): Snapshot {
  const tasks = new Map(snapshot.tasks.map((task) => [task.id, { ...task }]));
  const lists = new Map(snapshot.lists.map((list) => [list.id, { ...list }]));
  for (const op of operations) {
    const task = tasks.get(op.entityID);
    const stamp = `pending:${op.id}`;
    let changed = false;
    if (op.kind === "task.save" && op.task) {
      tasks.set(op.entityID, { createdAt: stamp, seriesID: "", occurrence: 0,
        ...task, ...op.task, updatedAt: stamp, version: op.baseVersion + 1 } as TodoTask);
      changed = true;
      if (task && task.listID !== op.task.listID) for (const child of tasks.values()) {
        if (child.parentID === task.id) { child.listID = op.task.listID; child.version += 1; child.updatedAt = stamp; }
      }
    } else if (task && op.kind === "task.complete") {
      if (!!task.completedAt === !!op.completed) continue;
      const completedAt = task.completedAt;
      task.completedAt = op.completed ? stamp : undefined;
      task.version = op.baseVersion + 1; task.updatedAt = stamp; changed = true;
      for (const child of tasks.values()) {
        if (child.parentID !== task.id || child.deletedAt) continue;
        if ((op.completed && !child.completedAt) || (!op.completed && child.completedAt === completedAt && child.updatedAt === completedAt)) {
          child.completedAt = task.completedAt; child.version += 1; child.updatedAt = stamp;
        }
      }
    } else if (task && (op.kind === "task.delete" || op.kind === "task.restore")) {
      if (!!task.deletedAt === (op.kind === "task.delete")) continue;
      const deletedAt = task.deletedAt;
      task.deletedAt = op.kind === "task.delete" ? stamp : undefined;
      task.version = op.baseVersion + 1; task.updatedAt = stamp; changed = true;
      for (const child of tasks.values()) if (child.parentID === task.id) {
        if ((op.kind === "task.delete" && !child.deletedAt) || (op.kind === "task.restore" && child.deletedAt === deletedAt)) {
          child.deletedAt = task.deletedAt; child.version += 1; child.updatedAt = stamp;
        }
      }
    } else if (op.kind === "list.save" && op.list) {
      lists.set(op.entityID, { ...op.list, version: op.baseVersion + 1 });
    } else if (op.kind === "list.delete") {
      lists.delete(op.entityID);
      for (const item of tasks.values()) if (item.listID === op.entityID) { item.listID = "inbox"; item.version += 1; item.updatedAt = stamp; }
    }
    const changedTask = tasks.get(op.entityID);
    if (changed && changedTask?.parentID) {
      const parent = tasks.get(changedTask.parentID);
      if (parent) { parent.version += 1; parent.updatedAt = stamp; }
    }
  }
  return { lists: [...lists.values()], tasks: [...tasks.values()] };
}
