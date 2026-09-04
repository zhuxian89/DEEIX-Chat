import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";

const stylesheet = readFileSync(resolve(process.cwd(), "src/pages/index/index.scss"), "utf8");

test("chat bubbles follow compact WeChat-style alignment and colors", () => {
  assert.match(stylesheet, /\.message\s*\{[^}]*width:\s*100%/);
  assert.match(stylesheet, /\.message-user\s*\{[^}]*align-items:\s*flex-end/);
  assert.match(stylesheet, /\.message-assistant\s*\{[^}]*align-items:\s*flex-start/);
  assert.match(stylesheet, /\.messageContent\s*\{[^}]*max-width:\s*82%/);
  assert.match(stylesheet, /\.message-user \.messageContent\s*\{[^}]*background:\s*#95ec69[^}]*color:\s*#111/);
  assert.match(stylesheet, /\.message-assistant \.messageContent\s*\{[^}]*background:\s*#fff/);
});
