import assert from "node:assert/strict";
import test from "node:test";
import {
  conversationTitleFromFirstUserMessage,
  isPlaceholderConversationTitle,
  preserveOptimisticConversationTitle,
} from "./conversation-metadata";

test("placeholder titles match the mature web metadata contract", () => {
  assert.equal(isPlaceholderConversationTitle("新对话"), true);
  assert.equal(isPlaceholderConversationTitle(" New Chat "), true);
  assert.equal(isPlaceholderConversationTitle("真实标题"), false);
});

test("an early list refresh does not replace the optimistic title with the server placeholder", () => {
  assert.deepEqual(
    preserveOptimisticConversationTitle(
      { title: "用户问题摘要", updatedAt: "old" },
      { title: "新对话", updatedAt: "new" },
    ),
    { title: "用户问题摘要", updatedAt: "new" },
  );
  assert.deepEqual(
    preserveOptimisticConversationTitle(
      { title: "用户问题摘要", updatedAt: "old" },
      { title: "后端生成标题", updatedAt: "new" },
    ),
    { title: "后端生成标题", updatedAt: "new" },
  );
});

test("first user message provides the same 16-character optimistic title as web", () => {
  assert.equal(conversationTitleFromFirstUserMessage("  “帮我  写一份   详细的产品发布计划和检查表”  "), "帮我 写一份 详细的产品发布计划");
});
