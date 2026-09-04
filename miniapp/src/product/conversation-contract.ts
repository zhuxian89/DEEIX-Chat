import type { SendMessageRequest } from "@deeix/api-contract";

export type MessageBranchRequest = {
  branchReason: "default" | "retry" | "edit";
  parentMessagePublicID?: string;
  sourceMessagePublicID?: string;
};

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
  branch: MessageBranchRequest = { branchReason: "default" },
): SendMessageRequest {
  return {
    branchReason: branch.branchReason,
    clientRunID,
    content,
    contentType: fileIDs.length > 0 ? "mixed" : "text",
    ...(fileIDs.length > 0 ? { fileIDs: [...fileIDs] } : {}),
    knowledgeBaseIDs: [],
    model,
    ...(options && Object.keys(options).length > 0 ? { options } : {}),
    ...(branch.parentMessagePublicID ? { parentMessagePublicID: branch.parentMessagePublicID } : {}),
    ...(selectedToolIDs.length > 0 ? { selectedToolIDs: [...selectedToolIDs] } : {}),
    ...(branch.sourceMessagePublicID ? { sourceMessagePublicID: branch.sourceMessagePublicID } : {}),
  };
}

export type ImageRunRequest = {
  branchReason: "default" | "retry" | "edit";
  clientRunID: string;
  fileIDs?: string[];
  model: string;
  options?: Record<string, unknown>;
  parentMessagePublicID?: string;
  prompt: string;
  sourceMessagePublicID?: string;
};

export function createImageRunRequest(
  prompt: string,
  model: string,
  clientRunID: string,
  fileIDs: readonly string[] = [],
  options?: Record<string, unknown>,
  branch: MessageBranchRequest = { branchReason: "default" },
): ImageRunRequest {
  return {
    branchReason: branch.branchReason,
    clientRunID,
    ...(fileIDs.length > 0 ? { fileIDs: [...fileIDs] } : {}),
    model,
    ...(options && Object.keys(options).length > 0 ? { options } : {}),
    ...(branch.parentMessagePublicID ? { parentMessagePublicID: branch.parentMessagePublicID } : {}),
    prompt,
    ...(branch.sourceMessagePublicID ? { sourceMessagePublicID: branch.sourceMessagePublicID } : {}),
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
