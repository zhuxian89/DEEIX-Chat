import type { StreamEvent } from "./chunked-transport";

export type StreamTerminalResult =
  | { kind: "completed"; data: unknown }
  | { kind: "error"; error: Error }
  | { kind: "none" };

export function classifyStreamTerminal(event: StreamEvent): StreamTerminalResult {
  if (event.type === "completed") {
    return { kind: "completed", data: event.data };
  }
  if (event.type === "error") {
    if (typeof event.data !== "undefined" && event.data !== null) {
      return { kind: "completed", data: event.data };
    }
    const errorCode = typeof event.errorCode === "string" ? ` / ${event.errorCode}` : "";
    return {
      kind: "error",
      error: new Error(`${typeof event.message === "string" ? event.message : "stream failed"}${errorCode}`),
    };
  }
  if (event.type === "moderation_blocked") {
    const eventID = typeof event.eventID === "string" && event.eventID.trim() ? `（事件 ID：${event.eventID.trim()}）` : "";
    return { kind: "error", error: new Error(`内容未通过安全审核，请调整后重试${eventID}`) };
  }
  return { kind: "none" };
}
