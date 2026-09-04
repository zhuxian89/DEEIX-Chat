import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";

const page = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
const stylesheet = readFileSync(resolve(process.cwd(), "src/pages/index/index.scss"), "utf8");

test("home keeps only the two task entries now that both workspaces select models", () => {
  assert.match(page, /<Text className="quickTitle">AI 对话<\/Text>/);
  assert.match(page, /<Text className="quickTitle">AI 生图<\/Text>/);
  assert.doesNotMatch(page, /自定义对话|advancedEntry|advancedTitle|advancedHint/);
  assert.doesNotMatch(stylesheet, /\.advanced(?:Entry|Icon|Body|Title|Hint)/);
});

test("workspace model selection uses a full-width custom sheet so long names remain readable", () => {
  assert.doesNotMatch(page, /<Picker\b/);
  assert.match(page, /function ModelPickerSheet/);
  assert.match(stylesheet, /\.modelPickerSheet\s*\{[^}]*width:\s*100%/);
  assert.match(stylesheet, /\.modelPickerName\s*\{[^}]*white-space:\s*normal[^}]*word-break:\s*break-all/);
});
