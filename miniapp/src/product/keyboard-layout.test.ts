import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import { composerKeyboardHandlers, composerKeyboardStyle } from "./keyboard-layout";

const pageSource = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
const taroConfigSource = readFileSync(resolve(process.cwd(), "config/index.ts"), "utf8");

test("textarea focus lifts the composer even without a global keyboard notification", () => {
  let height = 0;
  const handlers = composerKeyboardHandlers((next) => { height = next; });

  handlers.onFocus({ detail: { height: 312 } });
  assert.deepEqual(composerKeyboardStyle(height), { paddingBottom: "312px" });

  handlers.onKeyboardHeightChange({ detail: { height: 368 } });
  assert.deepEqual(composerKeyboardStyle(height), { paddingBottom: "368px" });

  handlers.onBlur();
  assert.equal(composerKeyboardStyle(height), undefined);
});

test("Android can report height after focus and hide the keyboard without blurring", () => {
  let height = 0;
  const handlers = composerKeyboardHandlers((next) => { height = next; });

  handlers.onFocus({ detail: {} });
  assert.equal(height, 0);
  handlers.onKeyboardHeightChange({ detail: { height: 280 } });
  assert.equal(height, 280);
  handlers.onKeyboardHeightChange({ detail: { height: 0 } });
  assert.equal(composerKeyboardStyle(height), undefined);

  handlers.onFocus({ detail: {} });
  handlers.onKeyboardHeightChange({ detail: { height: 300 } });
  assert.equal(height, 300);
});

test("missing or invalid height does not discard an already visible keyboard", () => {
  let height = 312;
  const handlers = composerKeyboardHandlers((next) => { height = next; });

  handlers.onFocus({ detail: {} });
  handlers.onKeyboardHeightChange({ detail: { height: NaN } });
  handlers.onKeyboardHeightChange({ detail: { height: Infinity } });
  assert.equal(height, 312);
});

test("visible keyboard reserves its height without adding a visual gap", () => {
  assert.deepEqual(composerKeyboardStyle(312), { paddingBottom: "312px" });
});

test("hidden keyboard leaves safe-area padding to stylesheet defaults", () => {
  assert.equal(composerKeyboardStyle(0), undefined);
});

test("composer hides the native iOS confirm bar above the keyboard", () => {
  assert.match(pageSource, /<Textarea[\s\S]*?showConfirmBar=\{false\}[\s\S]*?\/>/u);
});

test("composer replaces the native iOS toolbar with a minimal keyboard accessory", () => {
  assert.match(taroConfigSource, /enablekeyboardAccessory:\s*true/u);
  assert.match(pageSource, /<Textarea[\s\S]*?<KeyboardAccessory\s+style=\{\{\s*height:\s*"1px"\s*\}\}\s*\/>[\s\S]*?<\/Textarea>/u);
});

test("AI notice is hidden while the keyboard is visible", () => {
  assert.match(pageSource, /keyboardHeight\s*<=\s*0\s*\?\s*\(\s*<Text className="aiNotice">/u);
});
