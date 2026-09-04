import assert from "node:assert/strict";
import test from "node:test";
import { applyConversationStreamEvent, emptyConversationStreamState } from "./conversation-stream-state";

test("chat stream appends deltas and applies authoritative replacement snapshots", () => {
  let state = applyConversationStreamEvent(emptyConversationStreamState(), { type: "delta", delta: "旧", seq: 2 });
  state = applyConversationStreamEvent(state, { type: "delta", delta: "权威正文", replace: true, seq: 5 });
  state = applyConversationStreamEvent(state, { type: "delta", delta: "继续", seq: 6 });

  assert.equal(state.text, "权威正文继续");
  assert.equal(state.lastSeq, 6);
});

test("image stream preserves web media status and preview events", () => {
  let state = applyConversationStreamEvent(emptyConversationStreamState(), {
    type: "media_status",
    message: "正在保存图片",
    seq: 3,
  });
  state = applyConversationStreamEvent(state, {
    type: "media_image_delta",
    b64_json: "AAAA",
    mime_type: "image/webp",
    seq: 4,
  });

  assert.equal(state.status, "正在保存图片");
  assert.equal(state.imageSource, "data:image/webp;base64,AAAA");
  assert.equal(state.lastSeq, 4);
});

test("chat stream exposes a real Exa search only after the backend tool trace starts", () => {
  let state = applyConversationStreamEvent(emptyConversationStreamState(), {
    type: "process_update",
    seq: 3,
    trace: {
      enabled: true,
      status: "streaming",
      tools: {
        contentMarkdown: "**web_search_exa**",
        payloadJSON: JSON.stringify({
          tool_calls: [{ name: "web_search_exa", status: "requested", input_preview: "最新 AI 新闻" }],
        }),
        status: "streaming",
        summary: "1 次工具调用进行中",
        title: "工具调用",
        updatedAt: "2026-09-03T00:00:00Z",
      },
    },
  });

  assert.equal(state.status, "正在搜索互联网…");
  assert.equal(state.processTrace?.tools?.summary, "1 次工具调用进行中");

  state = applyConversationStreamEvent(state, {
    type: "process_update",
    seq: 4,
    trace: {
      enabled: true,
      status: "streaming",
      tools: {
        contentMarkdown: "**web_search_exa**",
        payloadJSON: JSON.stringify({
          tool_calls: [{ name: "web_search_exa", status: "success", output_preview: "找到 5 条结果" }],
        }),
        status: "completed",
        summary: "1 次工具调用已完成",
        title: "工具调用",
        updatedAt: "2026-09-03T00:00:01Z",
      },
    },
  });

  assert.equal(state.status, "正在整理搜索结果…");
});

test("chat stream keeps provider thinking separate from tool activity", () => {
  let state = applyConversationStreamEvent(emptyConversationStreamState(), {
    type: "upstream_think_delta",
    delta: "先分析问题",
    roundID: "round-1",
    status: "streaming",
    title: "模型思考",
  });
  state = applyConversationStreamEvent(state, {
    type: "upstream_think_delta",
    delta: "，再组织答案",
    roundID: "round-1",
    status: "streaming",
  });

  assert.equal(state.status, "正在思考…");
  assert.equal(state.processTrace?.upstreamThink?.contentMarkdown, "先分析问题，再组织答案");

  state = applyConversationStreamEvent(state, {
    type: "upstream_think_delta",
    contentMarkdown: "新的思考轮次",
    roundID: "round-2",
    status: "streaming",
  });
  assert.equal(state.processTrace?.upstreamThink?.contentMarkdown, "新的思考轮次");

  state = applyConversationStreamEvent(state, { type: "delta", delta: "最终回答" });
  assert.equal(state.status, "正在生成回复…");
  assert.equal(state.text, "最终回答");
});

test("chat stream does not claim network search when the backend reports no Exa call", () => {
  const state = applyConversationStreamEvent(emptyConversationStreamState(), {
    type: "process_update",
    trace: {
      enabled: true,
      status: "streaming",
      tools: {
        contentMarkdown: "**calculator**",
        payloadJSON: JSON.stringify({ tool_calls: [{ name: "calculator", status: "requested" }] }),
        status: "streaming",
        summary: "1 次工具调用进行中",
        title: "工具调用",
        updatedAt: "2026-09-03T00:00:00Z",
      },
    },
  });

  assert.equal(state.status, "正在调用工具…");
});

test("chat stream preserves backend attachment and retrieval activity messages", () => {
  let state = applyConversationStreamEvent(emptyConversationStreamState(), {
    type: "file_proc",
    message: "正在读取附件…",
  });
  assert.equal(state.status, "正在读取附件…");

  state = applyConversationStreamEvent(state, {
    type: "rag_search",
    message: "正在检索相关内容…",
  });
  assert.equal(state.status, "正在检索相关内容…");
});
