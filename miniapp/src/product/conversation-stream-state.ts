import type { StreamEvent } from "@/platform/chunked-transport";
import {
  applyProcessUpdateEvent,
  applyUpstreamThinkEvent,
  type ConversationProcessTrace,
  normalizeConversationProcessTrace,
  resolveConversationActivity,
} from "./conversation-trace";
import type { ImageSubmitTask } from "./image-task";

export type ConversationStreamState = {
  imageSource: string | null;
  imageTask?: ImageSubmitTask;
  lastSeq: number;
  processTrace?: ConversationProcessTrace;
  status: string;
  text: string;
};

export function emptyConversationStreamState(
  initialText = "",
  initialImageSource: string | null = null,
  processTrace?: ConversationProcessTrace,
  imageTask?: ImageSubmitTask,
): ConversationStreamState {
  return { imageSource: initialImageSource, imageTask, lastSeq: 0, processTrace, status: "", text: initialText };
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
  if (event.type === "media_status") {
    return { ...state, lastSeq, status: resolveImageMediaStatus(event, state.imageTask) };
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

function resolveImageMediaStatus(event: StreamEvent, imageTask?: ImageSubmitTask): string {
  const status = typeof event.status === "string" ? event.status.trim().toLowerCase() : "";
  const message = typeof event.message === "string" ? event.message.trim() : "";
  const rawStage = `${status} ${message}`.toLowerCase();
  const editing = imageTask === "image_edit";

  if (/\b(queue|queued|pending)\b/.test(rawStage)) {
    return editing ? "图片编辑任务排队中" : "图片任务排队中";
  }
  if (/saving_artifact|\b(saving|save|persisting|uploading)\b/.test(rawStage)) {
    return "正在保存图片";
  }
  if (/\b(downloading|loading|fetching)\b/.test(rawStage)) {
    return "正在加载图片";
  }
  if (/\b(running|generating|generation|processing)\b/.test(rawStage)) {
    return editing ? "AI 正在编辑图片" : "AI 正在生成图片";
  }
  if (/[\u3400-\u9fff]/.test(message)) {
    return message;
  }
  return "正在处理图片";
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
