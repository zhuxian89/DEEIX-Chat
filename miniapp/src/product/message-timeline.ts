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

export type ConversationImage = {
  fileID?: string;
  source?: string;
  status?: string;
};

export type ConversationMessage = {
  activityStatus?: string;
  branchReason?: string;
  contentType?: string;
  createdAt?: string;
  id: string;
  imageFileID?: string;
  imageSource?: string;
  imageStatus?: string;
  images?: ConversationImage[];
  modelName?: string;
  pending?: boolean;
  parentPublicID?: string;
  processTrace?: ConversationProcessTrace;
  role: "assistant" | "user";
  runID?: string;
  sourcePublicID?: string;
  text: string;
};

function imageAttachments(raw: string): ConversationImage[] {
  if (!raw.trim()) {
    return [];
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    const seen = new Set<string>();
    return parsed.flatMap((entry: unknown) => {
      if (!entry || typeof entry !== "object") return [];
      const item = entry as AttachmentSnapshot;
      const fileID = typeof item.file_id === "string" ? item.file_id.trim() : "";
      const kind = String(item.kind ?? "").toLowerCase();
      const category = String(item.file_category ?? "").toLowerCase();
      const mimeType = String(item.mime_type ?? "").toLowerCase();
      if (!fileID || seen.has(fileID) ||
        !(kind === "image" || category === "image" || mimeType.startsWith("image/"))) return [];
      seen.add(fileID);
      return [{ fileID, status: "正在加载图片" }];
    });
  } catch {
    return [];
  }
}

export function messageImages(message: ConversationMessage): ConversationImage[] {
  if (message.images?.length) return message.images;
  return message.imageFileID || message.imageSource
    ? [{ fileID: message.imageFileID, source: message.imageSource, status: message.imageStatus }]
    : [];
}

export function messageImageFileIDs(message: ConversationMessage): string[] {
  return messageImages(message).flatMap((image) => image.fileID ? [image.fileID] : []);
}

export function applyMessageImageDownload(
  message: ConversationMessage,
  fileID: string,
  source: string | null,
  status = source ? undefined : "图片加载失败",
): ConversationMessage {
  const images = messageImages(message).map((image) => image.fileID === fileID
    ? { ...image, source: source || undefined, status }
    : image);
  const first = images[0];
  return {
    ...message,
    images,
    // Keep the legacy generation view working while retaining failed file IDs for retries.
    imageFileID: first?.status === "图片加载失败" ? undefined : first?.fileID,
    imageSource: first?.source,
    imageStatus: first?.status,
  };
}

export function messageFromAPI(message: MessageResponse | PublicSharedMessageResponse): ConversationMessage | null {
  if (message.role !== "user" && message.role !== "assistant") {
    return null;
  }
  const text = message.content.trim();
  const images = imageAttachments(message.attachments);
  const imageFileID = images[0]?.fileID ?? "";
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
    createdAt: message.createdAt,
    imageFileID: imageFileID || undefined,
    images: images.length ? images : undefined,
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
  const createdAt = new Date().toISOString();
  return [
    { id: userID, createdAt, imageSource: inputImageSource, role: "user", text: prompt },
    {
      id: assistantID,
      createdAt,
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
  const existing = messageImages(message);
  const progressFileID = progress.imageFileID?.trim();
  const imageFileID = progressFileID || message.imageFileID || existing[0]?.fileID;
  const index = imageFileID
    ? existing.findIndex((image) => image.fileID === imageFileID)
    : existing.length === 1 && !existing[0].fileID ? 0 : -1;
  const targetIndex = index < 0 && existing.length === 1 && !existing[0].fileID ? 0 : index;
  // A stream can emit several images without identifying which persisted file a preview belongs to.
  const progressSource = progressFileID || existing.length <= 1 ? progress.imageSource?.trim() : undefined;
  const imageSource = progressSource || existing[targetIndex]?.source ||
    (imageFileID === message.imageFileID ? message.imageSource : undefined);
  const imageStatus = progress.status?.trim() || message.imageStatus;
  let images = message.images;
  if (imageFileID || imageSource) {
    const photo: ConversationImage = { fileID: imageFileID, source: imageSource, status: imageSource ? undefined : imageStatus };
    images = targetIndex >= 0
      ? existing.map((image, itemIndex) => itemIndex === targetIndex ? photo : image)
      : [...existing, photo];
  }
  return {
    ...message,
    images,
    imageSource,
    imageFileID,
    imageStatus,
    pending: progress.pending ?? message.pending,
  };
}
