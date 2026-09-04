export function isPlaceholderConversationTitle(title: string): boolean {
  return ["new chat", "新对话"].includes(title.trim().toLowerCase());
}

export function conversationTitleFromFirstUserMessage(content: string): string {
  const value = content.trim().replace(/\s+/gu, " ").replace(/^[\s"'`“”‘’]+|[\s"'`“”‘’]+$/gu, "");
  return value ? Array.from(value).slice(0, 16).join("").trim() : "";
}

export function preserveOptimisticConversationTitle<T extends TitledConversation>(
  current: T | undefined,
  refreshed: T,
): T {
  if (
    current &&
    isPlaceholderConversationTitle(refreshed.title) &&
    !isPlaceholderConversationTitle(current.title)
  ) {
    return { ...refreshed, title: current.title };
  }
  return refreshed;
}
export type TitledConversation = { title: string };
