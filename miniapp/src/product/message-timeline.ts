import type { MessageResponse } from "@deeix/api-contract";
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
  contentType?: string;
  id: string;
  imageFileID?: string;
  imageSource?: string;
  imageStatus?: string;
  modelName?: string;
  pending?: boolean;
  processTrace?: ConversationProcessTrace;
  role: "assistant" | "user";
  runID?: string;
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

export function messageFromAPI(message: MessageResponse): ConversationMessage | null {
  if (message.role !== "user" && message.role !== "assistant") {
    return null;
  }
  const text = message.content.trim();
  const attachment = imageAttachment(message.attachments);
  const imageFileID = typeof attachment?.file_id === "string" ? attachment.file_id.trim() : "";
  const pending = message.role === "assistant" && message.status.trim().toLowerCase() === "pending";
  const contentType = message.contentType.trim().toLowerCase();
  const imageMessage = contentType === "image" || Boolean(imageFileID);
  if (!text && !imageFileID && !pending) {
    return null;
  }
  return {
    activityStatus: pending && !imageMessage ? "正在继续生成回复" : undefined,
    id: message.publicID || String(message.id),
    contentType,
    imageFileID: imageFileID || undefined,
    imageStatus: imageFileID ? "图片生成完成" : pending && imageMessage ? "正在生成图片" : undefined,
    modelName: message.platformModelName.trim() || undefined,
    pending,
    processTrace: message.role === "assistant"
      ? normalizeConversationProcessTrace(message.processTrace)
      : undefined,
    role: message.role,
    runID: message.runID.trim() || undefined,
    text,
  };
}

export function createPendingImageTurn(
  prompt: string,
  userID: string,
  assistantID: string,
  inputImageSource?: string,
  pendingStatus = "正在生成图片",
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
