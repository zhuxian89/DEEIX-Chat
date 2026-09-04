import assert from "node:assert/strict";
import test from "node:test";
import {
  conversationSwipeOffset,
  conversationRefreshPageSize,
  mergeConversationPage,
  resolveConversationSwipe,
  settleConversationSwipe,
} from "./conversation-list";

test("conversation pages merge without duplicate conversations", () => {
  const merged = mergeConversationPage(
    [{ publicID: "conversation-1" }, { publicID: "conversation-2" }],
    [{ publicID: "conversation-2" }, { publicID: "conversation-3" }],
  );
  assert.deepEqual(merged.map((item) => item.publicID), ["conversation-1", "conversation-2", "conversation-3"]);
});

test("refresh page size stays aligned to subsequent 50-item pages", () => {
  assert.equal(conversationRefreshPageSize(0), 50);
  assert.equal(conversationRefreshPageSize(50), 50);
  assert.equal(conversationRefreshPageSize(75), 100);
  assert.equal(conversationRefreshPageSize(101), 150);
});

test("horizontal swipe opens and closes actions without hijacking vertical scrolling", () => {
  assert.equal(resolveConversationSwipe(-50, 4), "open");
  assert.equal(resolveConversationSwipe(50, 4), "close");
  assert.equal(resolveConversationSwipe(-20, 2), "none");
  assert.equal(resolveConversationSwipe(-50, 60), "none");
});

test("conversation row follows the finger within its action boundary", () => {
  assert.equal(conversationSwipeOffset(-48, 152, false), -48);
  assert.equal(conversationSwipeOffset(-200, 152, false), -152);
  assert.equal(conversationSwipeOffset(40, 152, false), 0);
  assert.equal(conversationSwipeOffset(42, 152, true), -110);
});

test("conversation swipe settles from its prior open state", () => {
  assert.equal(settleConversationSwipe(-28, 3, false), "open");
  assert.equal(settleConversationSwipe(28, 3, true), "close");
  assert.equal(settleConversationSwipe(5, 40, true), "open");
  assert.equal(settleConversationSwipe(5, 40, false), "close");
});
