const USER_SCROLL_TAKEOVER_PX = 4;
const CHAT_BOTTOM_SCROLL_TOP = 999_999;

export function nextChatBottomScrollTop(currentScrollTop: number): number {
  return currentScrollTop === CHAT_BOTTOM_SCROLL_TOP
    ? CHAT_BOTTOM_SCROLL_TOP - 1
    : CHAT_BOTTOM_SCROLL_TOP;
}

export function shouldReleaseChatAutoFollow(
  previousScrollTop: number,
  nextScrollTop: number,
  userTouching: boolean,
): boolean {
  return userTouching && nextScrollTop < previousScrollTop - USER_SCROLL_TAKEOVER_PX;
}
