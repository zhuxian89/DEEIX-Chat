import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { ChunkedJSONParser } from "./stream-parser";

function bytes(value: string): Uint8Array {
  return new TextEncoder().encode(value);
}

describe("ChunkedJSONParser", () => {
  it("decodes Chinese and emoji split inside UTF-8 code points", () => {
    const source = bytes('{"type":"delta","delta":"你好🙂"}\n');
    const parser = new ChunkedJSONParser();
    const events: unknown[] = [];

    for (const byte of source) {
      events.push(...parser.push(Uint8Array.of(byte)));
    }
    events.push(...parser.finish());

    assert.deepEqual(events, [{ type: "delta", delta: "你好🙂" }]);
  });

  it("parses multiple JSON documents in one chunk and preserves braces inside strings", () => {
    const parser = new ChunkedJSONParser();
    const events = parser.push(
      bytes('{"type":"delta","delta":"{测试}"}\n{"type":"usage","output_tokens":2}\n'),
    );

    assert.deepEqual(events, [
      { type: "delta", delta: "{测试}" },
      { type: "usage", output_tokens: 2 },
    ]);
    assert.deepEqual(parser.finish(), []);
  });

  it("retains an arbitrary JSON boundary until the next chunk", () => {
    const parser = new ChunkedJSONParser();
    assert.deepEqual(parser.push(bytes('{"type":"delta","del')), []);
    assert.deepEqual(parser.push(bytes('ta":"完成"}')), [{ type: "delta", delta: "完成" }]);
    assert.deepEqual(parser.finish(), []);
  });

  it("rejects an incomplete tail", () => {
    const parser = new ChunkedJSONParser();
    parser.push(bytes('{"type":"delta"'));

    assert.throws(() => parser.finish(), /incomplete JSON/u);
  });
});
