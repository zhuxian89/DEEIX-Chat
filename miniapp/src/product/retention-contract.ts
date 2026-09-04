function pathID(value: string): string {
  return encodeURIComponent(value.trim());
}

export function conversationSearchPath(query: string, page = 1, pageSize = 30): string {
  const normalizedPage = Math.max(1, Math.floor(page));
  const normalizedPageSize = Math.max(1, Math.floor(pageSize));
  const normalizedQuery = query.trim();
  const queryPart = normalizedQuery ? `&q=${encodeURIComponent(normalizedQuery)}` : "";
  return `/api/v1/conversations/search?page=${normalizedPage}&page_size=${normalizedPageSize}${queryPart}`;
}

export function conversationStarPath(conversationID: string): string {
  return `/api/v1/conversations/${pathID(conversationID)}/star`;
}

export function conversationSharePath(conversationID: string): string {
  return `/api/v1/conversations/${pathID(conversationID)}/share`;
}

export function sharedConversationPath(shareID: string): string {
  return `/api/v1/shared-conversations/${pathID(shareID)}`;
}

export function sharedConversationClonePath(shareID: string): string {
  return `${sharedConversationPath(shareID)}/clone`;
}

export function sharedConversationFilePath(shareID: string, fileID: string): string {
  return `${sharedConversationPath(shareID)}/files/${pathID(fileID)}/content`;
}

export function miniAppSharedConversationPath(shareID: string): string {
  return `/pages/index/index?share=${pathID(shareID)}`;
}

export const userMemoryCollectionPath = "/api/v1/memories/profile";

export function userMemoryPath(memoryKey: string): string {
  return `${userMemoryCollectionPath}/${pathID(memoryKey)}`;
}
