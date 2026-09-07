import assert from "node:assert/strict";
import test from "node:test";
import type Taro from "@tarojs/taro";
import type { StreamEvent } from "./chunked-transport";

// Match the switches injected by Taro's webpack plugin in the mini program.
Object.assign(globalThis, {
  ENABLE_INNER_HTML: false,
  ENABLE_ADJACENT_HTML: false,
  ENABLE_CLONE_NODE: false,
  ENABLE_CONTAINS: false,
  ENABLE_SIZE_APIS: false,
  ENABLE_TEMPLATE_CONTENT: false,
});
const taro = require("@tarojs/taro") as typeof Taro;
const { startChunkedJSONRequest } = require("./chunked-transport") as typeof import("./chunked-transport");

async function runResponse(statusCode: number, data: unknown, chunks: Uint8Array[] = []) {
  const originalRequest = taro.request;
  const events: StreamEvent[] = [];
  let onChunk: ((event: { data: ArrayBuffer }) => void) | undefined;
  taro.request = ((options: Taro.request.Option) => {
    queueMicrotask(() => {
      for (const chunk of chunks) {
        onChunk?.({ data: Uint8Array.from(chunk).buffer });
      }
      options.success?.({ statusCode, data, header: {}, cookies: [], errMsg: "request:ok" });
    });
    return {
      abort() {},
      onChunkReceived(callback: typeof onChunk) { onChunk = callback; },
    };
  }) as typeof Taro.request;
  try {
    const handle = startChunkedJSONRequest({
      url: "https://chat.example.com/api/v1/conversations/test/messages/stream",
      accessToken: "test-token",
      onEvent: (event) => events.push(event),
    });
    return { result: await handle.promise, events };
  } finally {
    taro.request = originalRequest;
  }
}

const errorEnvelope = { errorMsg: "invalid request body: content is required", errorCode: "request.invalid_body", data: null };

test("HTTP rejection displays the backend message and code from response data", async () => {
  for (const data of [errorEnvelope, JSON.stringify(errorEnvelope), new TextEncoder().encode(JSON.stringify(errorEnvelope)).buffer]) {
    await assert.rejects(runResponse(400, data), /content is required.*request\.invalid_body/u);
  }
});

test("chunked error envelopes are not mistaken for stream events", async () => {
  const bytes = new TextEncoder().encode(JSON.stringify({ ...errorEnvelope, errorMsg: "图片不可用" }));
  await assert.rejects(runResponse(400, "", [bytes.slice(0, 16), bytes.slice(16)]), /图片不可用.*request\.invalid_body/u);
});

test("HTTP errors without an API envelope retain their status", async () => {
  await assert.rejects(runResponse(502, "Bad Gateway"), /HTTP 502/u);
});

test("successful image chat keeps file progress, deltas and completion", async () => {
  const completed = { assistantMessage: { content: "图片中有一只猫" } };
  const streamEvents = [
    { type: "file_proc", seq: 1, message: "正在处理附件…" },
    { type: "delta", seq: 2, delta: "图片中有一只猫" },
    { type: "completed", seq: 3, data: completed },
  ];
  const bytes = new TextEncoder().encode(streamEvents.map((event) => JSON.stringify(event)).join("\n"));
  const { result, events } = await runResponse(200, "", [bytes.slice(0, 70), bytes.slice(70)]);
  assert.deepEqual(result.completedData, completed);
  assert.deepEqual(events, streamEvents);
  assert.equal(result.lastSeq, 3);
  assert.equal(result.eventCount, 3);
});

test("stream errors with saved data keep the existing completion behavior", async () => {
  const completed = { assistantMessage: { content: "部分回复" } };
  const bytes = new TextEncoder().encode(JSON.stringify({ type: "error", message: "interrupted", data: completed }));
  const { result } = await runResponse(200, "", [bytes]);
  assert.deepEqual(result.completedData, completed);
});
