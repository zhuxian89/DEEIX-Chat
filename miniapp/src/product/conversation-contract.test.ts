import assert from "node:assert/strict";
import test from "node:test";
import {
  cancelConversationRunPath,
  chatMessageStreamPath,
  createChatRunRequest,
  createImageRunRequest,
  deleteConversationPath,
  imageEditStreamPath,
  imageGenerationStreamPath,
  renameConversationPath,
  resumeConversationRunPath,
} from "./conversation-contract";

test("conversation management paths match the mature web API contract", () => {
  assert.equal(renameConversationPath("conversation/a"), "/api/v1/conversations/conversation%2Fa/title");
  assert.equal(deleteConversationPath("conversation/a", false), "/api/v1/conversations/conversation%2Fa");
  assert.equal(deleteConversationPath("conversation/a", true), "/api/v1/conversations/conversation%2Fa?delete_files=true");
});

test("chat and image starts carry the same run ID and Web model options", () => {
  assert.equal(chatMessageStreamPath("conversation/a"), "/api/v1/conversations/conversation%2Fa/messages/stream");
  assert.equal(imageGenerationStreamPath("conversation/a"), "/api/v1/conversations/conversation%2Fa/media/images/generations/stream");
  assert.equal(imageEditStreamPath("conversation/a"), "/api/v1/conversations/conversation%2Fa/media/images/edits/stream");
  assert.deepEqual(createChatRunRequest(
    "你好",
    "chat-model",
    "run-1",
    ["file-1"],
    {
      reasoning: { effort: "high" },
      tools: [{ type: "web_search_preview" }],
    },
    [11, 12],
  ), {
    branchReason: "default",
    clientRunID: "run-1",
    content: "你好",
    contentType: "mixed",
    fileIDs: ["file-1"],
    knowledgeBaseIDs: [],
    model: "chat-model",
    options: {
      reasoning: { effort: "high" },
      tools: [{ type: "web_search_preview" }],
    },
    selectedToolIDs: [11, 12],
  });
  assert.deepEqual(createImageRunRequest("一只猫", "image-model", "run-2", [], { aspect_ratio: "16:9" }), {
    branchReason: "default",
    clientRunID: "run-2",
    model: "image-model",
    options: { aspect_ratio: "16:9" },
    prompt: "一只猫",
  });
  assert.deepEqual(createImageRunRequest("换成夜景", "edit-model", "run-3", ["file-1"]), {
    branchReason: "default",
    clientRunID: "run-3",
    fileIDs: ["file-1"],
    model: "edit-model",
    prompt: "换成夜景",
  });
});

test("regeneration keeps the mature Web message branch contract", () => {
  const branch = {
    branchReason: "retry" as const,
    parentMessagePublicID: "user-parent-1",
    sourceMessagePublicID: "assistant-source-1",
  };
  assert.deepEqual(
    createChatRunRequest("原始问题", "chat-model", "run-chat-retry", [], undefined, [], branch),
    {
      branchReason: "retry",
      clientRunID: "run-chat-retry",
      content: "原始问题",
      contentType: "text",
      knowledgeBaseIDs: [],
      model: "chat-model",
      parentMessagePublicID: "user-parent-1",
      sourceMessagePublicID: "assistant-source-1",
    },
  );
  assert.deepEqual(
    createImageRunRequest("雨夜城市", "image-model", "run-image-retry", [], undefined, branch),
    {
      branchReason: "retry",
      clientRunID: "run-image-retry",
      model: "image-model",
      parentMessagePublicID: "user-parent-1",
      prompt: "雨夜城市",
      sourceMessagePublicID: "assistant-source-1",
    },
  );
});

test("generation resume and cancel paths match the mature web API contract", () => {
  assert.equal(
    resumeConversationRunPath("run/a", 0),
    "/api/v1/conversation-runs/run%2Fa/stream?snapshot=true",
  );
  assert.equal(
    resumeConversationRunPath("run/a", 17),
    "/api/v1/conversation-runs/run%2Fa/stream?snapshot=true&after=17",
  );
  assert.equal(cancelConversationRunPath("run/a"), "/api/v1/conversation-runs/run%2Fa/cancel");
});
