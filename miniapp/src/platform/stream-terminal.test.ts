import assert from "node:assert/strict";
import test from "node:test";
import { classifyStreamTerminal } from "./stream-terminal";

test("completed and interrupted-with-data events both settle with backend data", () => {
  const completed = classifyStreamTerminal({ type: "completed", data: { assistantMessage: { content: "完成" } } });
  const interrupted = classifyStreamTerminal({ type: "error", message: "upstream interrupted", data: { assistantMessage: { content: "部分结果" } } });
  assert.equal(completed.kind, "completed");
  assert.equal(interrupted.kind, "completed");
});

test("moderation terminal becomes a clear user-facing error", () => {
  const terminal = classifyStreamTerminal({ type: "moderation_blocked", eventID: "event-1" });
  assert.equal(terminal.kind, "error");
  if (terminal.kind === "error") {
    assert.match(terminal.error.message, /内容未通过安全审核/u);
    assert.match(terminal.error.message, /event-1/u);
  }
});
