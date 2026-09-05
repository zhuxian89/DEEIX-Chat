import type { MessageResponse, PublicSharedMessageResponse } from "@deeix/api-contract";
import {
  type ConversationProcessTrace,
  normalizeConversationProcessTrace,
} from "./conversation-trace";

type AttachmentSnapshot = {
  file_id?: unknown;
  file_name?: unknown;
  file_category?: unknown;
  kind?: unknown;
  mime_type?: unknown;
};

export type ImageProgress = {
  imageFileID?: string;
  imageSource?: string | null;
  pending?: boolean;
  status?: string;
};

export type ConversationMessage = {
  activityStatus?: string;
  branchReason?: string;
  contentType?: string;
  id: string;
  imageFileID?: string;
  imageSource?: string;
  imageStatus?: string;
  modelName?: string;
  pending?: boolean;
  parentPublicID?: string;
  processTrace?: ConversationProcessTrace;
  role: "assistant" | "user";
  runID?: string;
  sourcePublicID?: string;
  text: string;
};

function imageAttachment(raw: string): AttachmentSnapshot | null {
  if (!raw.trim()) {
    return null;
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return null;
    }
    return (parsed as AttachmentSnapshot[]).find((item) => {
      const kind = String(item.kind ?? "").toLowerCase();
      const category = String(item.file_category ?? "").toLowerCase();
      const mimeType = String(item.mime_type ?? "").toLowerCase();
      return typeof item.file_id === "string" && item.file_id.trim() !== "" &&
        (kind === "image" || category === "image" || mimeType.startsWith("image/"));
    }) ?? null;
  } catch {
    return null;
  }
}

export function messageFromAPI(message: MessageResponse | PublicSharedMessageResponse): ConversationMessage | null {
  if (message.role !== "user" && message.role !== "assistant") {
    return null;
  }
  const text = message.content.trim();
  const attachment = imageAttachment(message.attachments);
  const imageFileID = typeof attachment?.file_id === "string" ? attachment.file_id.trim() : "";
  const status = message.status.trim().toLowerCase();
  const pending = message.role === "assistant" && status === "pending";
  const contentType = message.contentType.trim().toLowerCase();
  const imageMessage = contentType === "image" || Boolean(imageFileID);
  const failedImageMessage = message.role === "assistant" && imageMessage && ["canceled", "error", "interrupted"].includes(status);
  if (!text && !imageFileID && !pending && !failedImageMessage) {
    return null;
  }
  return {
    activityStatus: pending && !imageMessage ? "正在继续生成回复" : undefined,
    branchReason: message.branchReason.trim() || undefined,
    id: message.publicID || ("id" in message ? String(message.id) : ""),
    contentType,
    imageFileID: imageFileID || undefined,
    imageStatus: imageFileID
      ? "正在加载图片"
      : pending && imageMessage
        ? "AI 正在生成图片"
        : failedImageMessage
          ? status === "canceled"
            ? "本次图片生成已停止"
            : message.errorMessage.trim() || "图片生成失败，请重试"
          : undefined,
    modelName: message.platformModelName.trim() || undefined,
    pending,
    parentPublicID: message.parentPublicID.trim() || undefined,
    processTrace: message.role === "assistant"
      ? normalizeConversationProcessTrace(message.processTrace)
      : undefined,
    role: message.role,
    runID: message.runID.trim() || undefined,
    sourcePublicID: message.sourcePublicID.trim() || undefined,
    text,
  };
}

export function latestVisibleMessages(messages: readonly ConversationMessage[]): ConversationMessage[] {
  const children = new Map<string, ConversationMessage[]>();
  for (const message of messages) {
    const parentKey = message.parentPublicID?.trim() ?? "";
    const siblings = children.get(parentKey) ?? [];
    siblings.push(message);
    children.set(parentKey, siblings);
  }

  const visible: ConversationMessage[] = [];
  const visited = new Set<string>();
  let parentKey = "";
  while (true) {
    const siblings = children.get(parentKey);
    const selected = siblings?.at(-1);
    if (!selected || visited.has(selected.id)) {
      break;
    }
    visited.add(selected.id);
    visible.push(selected);
    parentKey = selected.id;
  }
  if (visible.length > 0 || messages.length === 0) {
    return visible;
  }

  const byID = new Map(messages.map((message) => [message.id, message]));
  const suffix: ConversationMessage[] = [];
  let current = messages.at(-1);
  while (current && !visited.has(current.id)) {
    visited.add(current.id);
    suffix.push(current);
    current = current.parentPublicID ? byID.get(current.parentPublicID) : undefined;
  }
  return suffix.reverse();
}

export function createPendingImageTurn(
  prompt: string,
  userID: string,
  assistantID: string,
  inputImageSource?: string,
  pendingStatus = "AI 正在生成图片",
): ConversationMessage[] {
  return [
    { id: userID, imageSource: inputImageSource, role: "user", text: prompt },
    {
      id: assistantID,
      imageStatus: pendingStatus,
      pending: true,
      role: "assistant",
      text: "",
    },
  ];
}

export function applyImageProgress(
  message: ConversationMessage,
  progress: ImageProgress,
): ConversationMessage {
  return {
    ...message,
    imageSource: progress.imageSource?.trim() || message.imageSource,
    imageFileID: progress.imageFileID?.trim() || message.imageFileID,
    imageStatus: progress.status?.trim() || message.imageStatus,
    pending: progress.pending ?? message.pending,
  };
}
