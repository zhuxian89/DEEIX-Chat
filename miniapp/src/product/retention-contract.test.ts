import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import {
  conversationSearchPath,
  conversationSharePath,
  conversationStarPath,
  miniAppSharedConversationPath,
  sharedConversationClonePath,
  sharedConversationPath,
  userMemoryCollectionPath,
  userMemoryPath,
} from "./retention-contract";

test("history search, favorites and sharing reuse the mature Web endpoints", () => {
  assert.equal(
    conversationSearchPath("雨夜 城市", 2, 30),
    "/api/v1/conversations/search?page=2&page_size=30&q=%E9%9B%A8%E5%A4%9C%20%E5%9F%8E%E5%B8%82",
  );
  assert.equal(conversationStarPath("conversation/a"), "/api/v1/conversations/conversation%2Fa/star");
  assert.equal(conversationSharePath("conversation/a"), "/api/v1/conversations/conversation%2Fa/share");
  assert.equal(sharedConversationPath("share/a"), "/api/v1/shared-conversations/share%2Fa");
  assert.equal(sharedConversationClonePath("share/a"), "/api/v1/shared-conversations/share%2Fa/clone");
  assert.equal(
    miniAppSharedConversationPath("share/a"),
    "/pages/index/index?share=share%2Fa",
  );
});

test("preference memory reuses the mature Web memory endpoints", () => {
  assert.equal(userMemoryCollectionPath, "/api/v1/memories/profile");
  assert.equal(userMemoryPath("回复 风格"), "/api/v1/memories/profile/%E5%9B%9E%E5%A4%8D%20%E9%A3%8E%E6%A0%BC");
});

test("miniapp exposes the retained product capabilities through real secondary screens", () => {
  const page = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
  assert.match(page, /复制<\/Text>|>复制<\/Text>/u);
  assert.match(page, /重新生成/u);
  assert.match(page, /screen === "history"/u);
  assert.match(page, /搜索标题和对话内容/u);
  assert.match(page, /我的收藏/u);
  assert.match(page, /openType="share"/u);
  assert.match(page, /screen === "memories"/u);
  assert.match(page, /AI 偏好记忆/u);
});
