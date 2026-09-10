import type {
  EntryStatusResponse, ExportResponse, MiniapptodoListResponse, OperationRequest,
  OperationResultResponse, SnapshotResponse, SyncResponse, TaskDraftRequest,
  TaskPageResponse, TaskResponse,
} from "@deeix/api-contract";

export type EntryStatus = EntryStatusResponse;
export type TodoList = MiniapptodoListResponse;
export type TaskDraft = TaskDraftRequest & {
  repeatKind: "none" | "daily" | "weekdays" | "weekly" | "monthly";
};
export type TodoTask = TaskResponse & { repeatKind: TaskDraft["repeatKind"] };
export type Snapshot = Omit<SnapshotResponse, "tasks"> & { tasks: TodoTask[] };
export type Operation = Omit<OperationRequest, "kind" | "task"> & {
  kind: "task.save" | "task.complete" | "task.delete" | "task.restore" | "list.save" | "list.delete";
  task?: TaskDraft;
};
export type OperationResult = Omit<OperationResultResponse, "status"> & { status: "applied" | "duplicate" | "conflict" | "invalid" };
export type SyncResult = Omit<SyncResponse, "results" | "snapshot"> & { results: OperationResult[]; snapshot: Snapshot };
export type TaskPage = Omit<TaskPageResponse, "results"> & { results: TodoTask[] };
export type TodoExport = ExportResponse;

// IDs are used for idempotency, never for authentication.
export function newID(): string {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (letter) => {
    const random = Math.floor(Math.random() * 16);
    return (letter === "x" ? random : (random & 3) | 8).toString(16);
  });
}

export function newDraft(listID = "inbox", parentID = ""): TaskDraft {
  return { id: newID(), listID, parentID, title: "", notes: "", dueDate: "", dueTime: "",
    timezone: "Asia/Shanghai", important: false, position: Date.now(), repeatKind: "none", repeatDay: 0 };
}

export function taskDraft(task: TodoTask): TaskDraft {
  const { id, listID, parentID, title, notes, dueDate, dueTime, timezone, important, position, repeatKind, repeatDay } = task;
  return { id, listID, parentID, title, notes, dueDate, dueTime, timezone, important, position, repeatKind, repeatDay };
}
