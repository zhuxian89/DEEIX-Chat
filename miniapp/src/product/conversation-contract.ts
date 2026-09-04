import type { SendMessageRequest } from "@deeix/api-contract";

function pathID(value: string): string {
  return encodeURIComponent(value.trim());
}

export function chatMessageStreamPath(conversationID: string): string {
  return `/api/v1/conversations/${pathID(conversationID)}/messages/stream`;
}

export function imageGenerationStreamPath(conversationID: string): string {
  return `/api/v1/conversations/${pathID(conversationID)}/media/images/generations/stream`;
}

export function imageEditStreamPath(conversationID: string): string {
  return `/api/v1/conversations/${pathID(conversationID)}/media/images/edits/stream`;
}

export function createChatRunRequest(
  content: string,
  model: string,
  clientRunID: string,
  fileIDs: readonly string[] = [],
  options?: Record<string, unknown>,
  selectedToolIDs: readonly number[] = [],
): SendMessageRequest {
  return {
    branchReason: "default",
    clientRunID,
    content,
    contentType: fileIDs.length > 0 ? "mixed" : "text",
    fileIDs: fileIDs.length > 0 ? [...fileIDs] : undefined,
    knowledgeBaseIDs: [],
    model,
    options: options && Object.keys(options).length > 0 ? options : undefined,
    selectedToolIDs: selectedToolIDs.length > 0 ? [...selectedToolIDs] : undefined,
  };
}

export type ImageRunRequest = {
  branchReason: "default";
  clientRunID: string;
  fileIDs?: string[];
  model: string;
  options?: Record<string, unknown>;
  prompt: string;
};

export function createImageRunRequest(
  prompt: string,
  model: string,
  clientRunID: string,
  fileIDs: readonly string[] = [],
  options?: Record<string, unknown>,
): ImageRunRequest {
  return {
    branchReason: "default",
    clientRunID,
    ...(fileIDs.length > 0 ? { fileIDs: [...fileIDs] } : {}),
    model,
    ...(options && Object.keys(options).length > 0 ? { options } : {}),
    prompt,
  };
}

export function renameConversationPath(conversationID: string): string {
  return `/api/v1/conversations/${pathID(conversationID)}/title`;
}

export function deleteConversationPath(conversationID: string, deleteFiles: boolean): string {
  const path = `/api/v1/conversations/${pathID(conversationID)}`;
  return deleteFiles ? `${path}?delete_files=true` : path;
}

export function resumeConversationRunPath(runID: string, afterSeq: number): string {
  const path = `/api/v1/conversation-runs/${pathID(runID)}/stream?snapshot=true`;
  const normalized = Math.max(0, Math.floor(afterSeq));
  return normalized > 0 ? `${path}&after=${normalized}` : path;
}

export function cancelConversationRunPath(runID: string): string {
  return `/api/v1/conversation-runs/${pathID(runID)}/cancel`;
}
