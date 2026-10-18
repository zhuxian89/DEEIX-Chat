import assert from "node:assert/strict";
import test from "node:test";
import { formatMessageTime } from "./message-time";
import type { MessageResponse } from "@deeix/api-contract";
import {
  applyImageProgress,
  applyMessageImageDownload,
  createPendingImageTurn,
  latestVisibleMessages,
  messageFromAPI,
  messageImageFileIDs,
  messageImages,
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
  assert.equal(message.createdAt, "2026-09-02T00:00:00Z");
  assert.equal(message.imageStatus, "正在加载图片");
});

test("history preserves every distinct photo in selection order and ignores malformed attachments", () => {
  const message = messageFromAPI(serverMessage({
    role: "user",
    attachments: JSON.stringify([
      null,
      { file_id: "photo-3", mime_type: "image/jpeg" },
      { file_id: "document", kind: "document" },
      { file_id: "photo-1", file_category: "image" },
      { file_id: "photo-3", kind: "image" },
      { file_id: "photo-2", kind: "image" },
    ]),
  }));
  assert.ok(message);
  assert.deepEqual(messageImageFileIDs(message), ["photo-3", "photo-1", "photo-2"]);
  assert.equal(message.imageFileID, "photo-3");
  assert.equal(messageImages(message).length, 3);
});

test("out-of-order downloads keep photo order and failed IDs available for regeneration", () => {
  const original = messageFromAPI(serverMessage({
    role: "user",
    attachments: JSON.stringify([1, 2, 3].map((number) => ({ file_id: `photo-${number}`, kind: "image" }))),
  }));
  assert.ok(original);
  const last = applyMessageImageDownload(original, "photo-3", "temp://three");
  const failed = applyMessageImageDownload(last, "photo-1", null);
  const complete = applyMessageImageDownload(failed, "photo-2", "temp://two");
  assert.deepEqual(messageImages(complete).map((photo) => photo.source), [undefined, "temp://two", "temp://three"]);
  assert.deepEqual(messageImageFileIDs(complete), ["photo-1", "photo-2", "photo-3"]);
  assert.equal(complete.imageFileID, undefined);
  assert.equal(complete.imageStatus, "图片加载失败");
  const recovered = applyMessageImageDownload(complete, "photo-1", "temp://one");
  assert.equal(recovered.imageFileID, "photo-1");
  assert.equal(recovered.imageSource, "temp://one");
  assert.equal(recovered.images?.[0].status, undefined);
  assert.equal(original.images?.[0].source, undefined);
});

test("legacy single photos remain available in history, previews and regeneration", () => {
  const legacy = { id: "legacy", role: "user" as const, text: "", imageFileID: "photo", imageSource: "temp://photo" };
  assert.deepEqual(messageImageFileIDs(legacy), ["photo"]);
  assert.equal(messageImages(legacy)[0].source, "temp://photo");
});

test("failed persisted image messages remain visible so the Web-compatible retry branch is available", () => {
  const upstreamError = "非常抱歉，生成的图片可能违反了关于轻度性暗示或挑逗性主题的防护限制。";
  const message = messageFromAPI(serverMessage({
    contentType: "image",
    errorMessage: upstreamError,
    parentPublicID: "user-image-1",
    status: "error",
  }));

  assert.ok(message);
  assert.equal(message.id, "assistant-image-1");
  assert.equal(message.imageStatus, upstreamError);
  assert.equal(message.parentPublicID, "user-image-1");
  assert.equal(message.pending, false);
});

test("image generation creates a visible pending turn before the network completes", () => {
  const turn = createPendingImageTurn("雨后的未来城市", "local-user-1", "local-assistant-2");

  assert.equal(turn.length, 2);
  assert.deepEqual(turn.map((item) => item.role), ["user", "assistant"]);
  assert.equal(turn[0]?.text, "雨后的未来城市");
  assert.equal(turn[1]?.imageStatus, "AI 正在生成图片");
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
  assert.ok(pending.createdAt);
  assert.equal(updated.createdAt, pending.createdAt);
  assert.equal(updated.imageStatus, "正在处理图片");
});

test("resumed image output updates the matching photo without changing another attachment", () => {
  const pending = messageFromAPI(serverMessage({
    status: "pending",
    attachments: JSON.stringify(["first", "second"].map((file_id) => ({ file_id, kind: "image" }))),
  }));
  assert.ok(pending);
  const first = applyMessageImageDownload(pending, "first", "temp://first");
  const resumed = applyImageProgress(first, { imageFileID: "second", imageSource: "temp://second", pending: false });
  assert.deepEqual(messageImages(resumed).map((image) => [image.fileID, image.source]), [
    ["first", "temp://first"], ["second", "temp://second"],
  ]);
  assert.equal(resumed.imageFileID, "second");
  assert.equal(resumed.imageSource, "temp://second");
  assert.equal(resumed.pending, false);
  assert.equal(messageImages(first)[1].source, undefined);
});

test("a new image file ID never reuses the previous image source", () => {
  const previous = applyImageProgress(createPendingImageTurn("photo", "user", "assistant")[1], {
    imageFileID: "old-photo", imageSource: "temp://old",
  });
  const next = applyImageProgress(previous, { imageFileID: "new-photo" });
  assert.equal(next.imageFileID, "new-photo");
  assert.equal(next.imageSource, undefined);
  assert.deepEqual(messageImages(next).map((image) => [image.fileID, image.source]), [
    ["old-photo", "temp://old"], ["new-photo", undefined],
  ]);
});

test("a generated preview receives its final file ID without a duplicate thumbnail", () => {
  const preview = applyImageProgress(createPendingImageTurn("photo", "user", "assistant")[1], {
    imageSource: "temp://preview",
  });
  const completed = applyImageProgress(preview, { imageFileID: "persisted-photo", pending: false });
  assert.deepEqual(messageImages(completed).map((image) => [image.fileID, image.source]), [["persisted-photo", "temp://preview"]]);
});

test("message time uses Beijing time across dates and explicit timezone offsets", () => {
  assert.equal(formatMessageTime("2026-12-31T16:30:00Z"), "2027-01-01 00:30");
  assert.equal(formatMessageTime("2026-10-13T14:30:00+08:00"), "2026-10-13 14:30");
  assert.equal(formatMessageTime("2026-10-12T23:30:00-07:00"), "2026-10-13 14:30");
  assert.equal(formatMessageTime(), "");
  assert.equal(formatMessageTime(""), "");
  assert.equal(formatMessageTime("invalid"), "");
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
  assert.equal(message.imageStatus, "AI 正在生成图片");
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
