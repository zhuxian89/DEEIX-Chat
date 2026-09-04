import type { StreamEvent } from "@/platform/chunked-transport";
import {
  applyProcessUpdateEvent,
  applyUpstreamThinkEvent,
  type ConversationProcessTrace,
  normalizeConversationProcessTrace,
  resolveConversationActivity,
} from "./conversation-trace";

export type ConversationStreamState = {
  imageSource: string | null;
  lastSeq: number;
  processTrace?: ConversationProcessTrace;
  status: string;
  text: string;
};

export function emptyConversationStreamState(
  initialText = "",
  initialImageSource: string | null = null,
  processTrace?: ConversationProcessTrace,
): ConversationStreamState {
  return { imageSource: initialImageSource, lastSeq: 0, processTrace, status: "", text: initialText };
}

export function applyConversationStreamEvent(
  state: ConversationStreamState,
  event: StreamEvent,
): ConversationStreamState {
  const lastSeq = typeof event.seq === "number" && Number.isFinite(event.seq)
    ? Math.max(state.lastSeq, event.seq)
    : state.lastSeq;
  if (event.type === "delta" && typeof event.delta === "string") {
    const nextText = event.replace === true ? event.delta : `${state.text}${event.delta}`;
    return {
      ...state,
      lastSeq,
      status: resolveConversationActivity(state.processTrace, nextText),
      text: nextText,
    };
  }
  if (event.type === "process_update") {
    const processTrace = applyProcessUpdateEvent(state.processTrace, event);
    return {
      ...state,
      lastSeq,
      processTrace,
      status: resolveConversationActivity(processTrace, state.text),
    };
  }
  if (event.type === "upstream_think_delta") {
    const processTrace = applyUpstreamThinkEvent(state.processTrace, event);
    return {
      ...state,
      lastSeq,
      processTrace,
      status: resolveConversationActivity(processTrace, state.text),
    };
  }
  if (event.type === "completed") {
    const completed = event.data && typeof event.data === "object" && !Array.isArray(event.data)
      ? event.data as Record<string, unknown>
      : null;
    const assistantMessage = completed?.assistantMessage && typeof completed.assistantMessage === "object"
      ? completed.assistantMessage as Record<string, unknown>
      : null;
    const processTrace = normalizeConversationProcessTrace(assistantMessage?.processTrace) ?? state.processTrace;
    return { ...state, lastSeq, processTrace };
  }
  if (event.type === "media_status" && typeof event.message === "string") {
    return { ...state, lastSeq, status: event.message };
  }
  if ((event.type === "file_proc" || event.type === "rag_search") && typeof event.message === "string") {
    return { ...state, lastSeq, status: event.message.trim() };
  }
  if (event.type === "moderation_checking") {
    return { ...state, lastSeq, status: "正在进行安全检查…" };
  }
  const imageSource = imageSourceFromEvent(event);
  return imageSource ? { ...state, imageSource, lastSeq } : { ...state, lastSeq };
}

function imageSourceFromEvent(event: StreamEvent): string | null {
  if (event.type !== "media_image_delta" || typeof event.b64_json !== "string") {
    return null;
  }
  const base64 = event.b64_json.trim();
  if (!base64) {
    return null;
  }
  if (base64.startsWith("data:image/")) {
    return base64;
  }
  const mimeType = typeof event.mime_type === "string" && event.mime_type.startsWith("image/")
    ? event.mime_type
    : "image/png";
  return `data:${mimeType};base64,${base64}`;
}
