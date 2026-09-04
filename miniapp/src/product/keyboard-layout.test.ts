import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import { composerKeyboardStyle } from "./keyboard-layout";

const pageSource = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
const taroConfigSource = readFileSync(resolve(process.cwd(), "config/index.ts"), "utf8");

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
