import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";

const page = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
const stylesheet = readFileSync(resolve(process.cwd(), "src/pages/index/index.scss"), "utf8");
const appConfig = readFileSync(resolve(process.cwd(), "src/app.config.ts"), "utf8");
const pageConfig = readFileSync(resolve(process.cwd(), "src/pages/index/index.config.ts"), "utf8");

test("home keeps only the two task entries now that both workspaces select models", () => {
  assert.match(page, /<Text className="quickTitle">AI 对话<\/Text>/);
  assert.match(page, /<Text className="quickTitle">AI 生图<\/Text>/);
  assert.doesNotMatch(page, /自定义对话|advancedEntry|advancedTitle|advancedHint/);
  assert.doesNotMatch(stylesheet, /\.advanced(?:Entry|Icon|Body|Title|Hint)/);
});

test("home keeps account details out of the primary task hierarchy", () => {
  assert.doesNotMatch(page, /className="welcome"|className="accountSummary"|className="avatarBadge"/);
  assert.match(page, /<Text className="homeTitle">今天想做什么？<\/Text>/);
  assert.match(page, /<Text className="versionNote">版本 \{MINIAPP_BUILD_VERSION\}<\/Text>/);
  assert.doesNotMatch(stylesheet, /\.welcome\s*\{|\.accountSummary\s*\{|\.avatarBadge\s*\{/);
});

test("the native navigation title stays stable while the build version remains on the home page", () => {
  assert.match(appConfig, /navigationBarTitleText:\s*"AI省着用"/);
  assert.match(pageConfig, /navigationBarTitleText:\s*"AI省着用"/);
  assert.doesNotMatch(`${appConfig}\n${pageConfig}`, /navigationBarTitleText:[^\n]*\d+\.\d+/);
});

test("a failed persisted image offers the mature Web manual retry branch", () => {
  assert.match(page, /message\.parentPublicID && !message\.id\.startsWith\("local-"\)/);
  assert.match(page, /onClick=\{\(\) => void regenerateImageAnswer\(message\)\}/);
  assert.match(page, />\s*重试\s*<\/Button>/);
});

test("workspace model selection uses a full-width custom sheet so long names remain readable", () => {
  assert.doesNotMatch(page, /<Picker\b/);
  assert.match(page, /function ModelPickerSheet/);
  assert.match(stylesheet, /\.modelPickerSheet\s*\{[^}]*width:\s*100%/);
  assert.match(stylesheet, /\.modelPickerName\s*\{[^}]*white-space:\s*normal[^}]*word-break:\s*break-all/);
});
