export type PublicConversation = { publicID: string };

export function mergeConversationPage<T extends PublicConversation>(
  current: readonly T[],
  next: readonly T[],
): T[] {
  const known = new Set(current.map((item) => item.publicID));
  return [...current, ...next.filter((item) => !known.has(item.publicID))];
}

export function conversationRefreshPageSize(currentCount: number, pageSize = 50): number {
  const normalizedPageSize = Math.max(1, Math.floor(pageSize));
  const normalizedCount = Math.max(0, Math.floor(currentCount));
  return Math.max(normalizedPageSize, Math.ceil(normalizedCount / normalizedPageSize) * normalizedPageSize);
}

export type ConversationSwipe = "close" | "none" | "open";

export function resolveConversationSwipe(deltaX: number, deltaY: number, threshold = 36): ConversationSwipe {
  if (Math.abs(deltaX) < threshold || Math.abs(deltaX) <= Math.abs(deltaY)) {
    return "none";
  }
  return deltaX < 0 ? "open" : "close";
}

export function conversationSwipeOffset(deltaX: number, actionWidth: number, initiallyOpen: boolean): number {
  const width = Math.max(0, actionWidth);
  const initialOffset = initiallyOpen ? -width : 0;
  return Math.min(0, Math.max(-width, initialOffset + deltaX));
}

export function settleConversationSwipe(
  deltaX: number,
  deltaY: number,
  initiallyOpen: boolean,
): Exclude<ConversationSwipe, "none"> {
  const swipe = resolveConversationSwipe(deltaX, deltaY, 24);
  if (swipe === "none") {
    return initiallyOpen ? "open" : "close";
  }
  return swipe;
}
