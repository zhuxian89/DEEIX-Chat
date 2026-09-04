import assert from "node:assert/strict";
import test from "node:test";
import type { MessageResponse } from "@deeix/api-contract";
import {
  applyImageProgress,
  createPendingImageTurn,
  latestVisibleMessages,
  messageFromAPI,
} from "./message-timeline";

function serverMessage(overrides: Partial<MessageResponse>): MessageResponse {
  return {
    attachments: "[]",
    branchReason: "default",
    cacheReadTokens: 0,
    cacheWriteTokens: 0,
    content: "",
    contentType: "text",
    conversationID: 1,
    createdAt: "2026-09-02T00:00:00Z",
    editedAt: null,
    errorCode: "",
    errorMessage: "",
    id: 1,
    inputTokens: 0,
    latencyMS: 0,
    modelIcon: "",
    modelVendor: "",
    myFeedback: "",
    outputTokens: 0,
    parentMessageID: null,
    parentPublicID: "",
    platformModelName: "image-model",
    publicID: "assistant-image-1",
    reasoningTokens: 0,
    role: "assistant",
    runID: "",
    sourceMessageID: null,
    sourcePublicID: "",
    status: "success",
    thumbsDownCount: 0,
    thumbsUpCount: 0,
    tokenUsage: 0,
    updatedAt: "2026-09-02T00:00:00Z",
    upstreamModelName: "image-model",
    userID: 1,
    ...overrides,
  };
}

test("historical image-only assistant messages remain visible", () => {
  const message = messageFromAPI(serverMessage({
    attachments: JSON.stringify([{
      file_id: "file-image-1",
      file_name: "generated.png",
      kind: "image",
      mime_type: "image/png",
    }]),
  }));

  assert.ok(message);
  assert.equal(message.imageFileID, "file-image-1");
  assert.equal(message.imageStatus, "图片生成完成");
});

test("image generation creates a visible pending turn before the network completes", () => {
  const turn = createPendingImageTurn("雨后的未来城市", "local-user-1", "local-assistant-2");

  assert.equal(turn.length, 2);
  assert.deepEqual(turn.map((item) => item.role), ["user", "assistant"]);
  assert.equal(turn[0]?.text, "雨后的未来城市");
  assert.equal(turn[1]?.imageStatus, "正在生成图片");
  assert.equal(turn[1]?.pending, true);
});

test("image editing keeps the source image visible in the optimistic turn", () => {
  const turn = createPendingImageTurn("改成夜景", "local-user-1", "local-assistant-2", "temp://source", "正在编辑图片");
  assert.equal(turn[0]?.imageSource, "temp://source");
  assert.equal(turn[1]?.imageStatus, "正在编辑图片");
});

test("image stream progress updates the same pending assistant message", () => {
  const pending = createPendingImageTurn("测试", "local-user-1", "local-assistant-2")[1];
  assert.ok(pending);

  const updated = applyImageProgress(pending, {
    imageSource: "data:image/png;base64,AAAA",
    status: "正在处理图片",
  });

  assert.equal(updated.id, pending.id);
  assert.equal(updated.imageSource, "data:image/png;base64,AAAA");
  assert.equal(updated.imageStatus, "正在处理图片");
});

test("pending history preserves the backend run ID for web-compatible resume", () => {
  const message = messageFromAPI(serverMessage({
    contentType: "image",
    runID: "run_resume_1",
    status: "pending",
  }));

  assert.ok(message);
  assert.equal(message.runID, "run_resume_1");
  assert.equal(message.pending, true);
  assert.equal(message.imageStatus, "正在生成图片");
});

test("pending chat history is not mislabeled as image generation", () => {
  const message = messageFromAPI(serverMessage({
    content: "已生成的部分正文",
    contentType: "markdown",
    runID: "run_chat_1",
    status: "pending",
  }));

  assert.ok(message);
  assert.equal(message.runID, "run_chat_1");
  assert.equal(message.imageStatus, undefined);
  assert.equal(message.activityStatus, "正在继续生成回复");
});

test("chat history preserves the backend thinking and tool trace", () => {
  const message = messageFromAPI(serverMessage({
    content: "根据搜索结果，这是最终回答。",
    processTrace: {
      enabled: true,
      status: "completed",
      tools: {
        contentMarkdown: "已完成联网搜索",
        payloadJSON: JSON.stringify({ tool_calls: [{ name: "web_search_exa", status: "success" }] }),
        status: "completed",
        summary: "1 次工具调用已完成",
        title: "工具调用",
        updatedAt: "2026-09-03T00:00:01Z",
      },
      upstreamThink: {
        contentMarkdown: "需要查询最新资料。",
        status: "completed",
        summary: "思考完成",
        title: "模型思考",
        updatedAt: "2026-09-03T00:00:01Z",
      },
    },
  }));

  assert.equal(message?.processTrace?.upstreamThink?.contentMarkdown, "需要查询最新资料。");
  assert.equal(message?.processTrace?.tools?.summary, "1 次工具调用已完成");
});

test("message history preserves the Web branch metadata", () => {
  const message = messageFromAPI(serverMessage({
    branchReason: "retry",
    content: "新回答",
    parentPublicID: "user-parent-1",
    publicID: "assistant-retry-1",
    sourcePublicID: "assistant-source-1",
  }));

  assert.equal(message?.branchReason, "retry");
  assert.equal(message?.parentPublicID, "user-parent-1");
  assert.equal(message?.sourcePublicID, "assistant-source-1");
});

test("message history shows the latest visible branch like the Web client", () => {
  const root = messageFromAPI(serverMessage({ id: 1, publicID: "user-1", role: "user", content: "问题" }));
  const oldReply = messageFromAPI(serverMessage({ id: 2, publicID: "assistant-old", parentPublicID: "user-1", content: "旧回答" }));
  const oldFollowup = messageFromAPI(serverMessage({ id: 3, publicID: "user-old-child", parentPublicID: "assistant-old", role: "user", content: "旧分支追问" }));
  const retryReply = messageFromAPI(serverMessage({ id: 4, publicID: "assistant-retry", parentPublicID: "user-1", sourcePublicID: "assistant-old", branchReason: "retry", content: "新回答" }));

  const visible = latestVisibleMessages([root, oldReply, oldFollowup, retryReply].filter((item): item is NonNullable<typeof item> => Boolean(item)));
  assert.deepEqual(visible.map((item) => item.id), ["user-1", "assistant-retry"]);
});

test("message history keeps the latest visible suffix when the tail page omits old ancestors", () => {
  const user = messageFromAPI(serverMessage({ id: 80, publicID: "user-80", parentPublicID: "assistant-79", role: "user", content: "继续" }));
  const assistant = messageFromAPI(serverMessage({ id: 81, publicID: "assistant-81", parentPublicID: "user-80", content: "回答" }));
  const visible = latestVisibleMessages([user, assistant].filter((item): item is NonNullable<typeof item> => Boolean(item)));
  assert.deepEqual(visible.map((item) => item.id), ["user-80", "assistant-81"]);
});
