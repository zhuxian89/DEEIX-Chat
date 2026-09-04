import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { createRequire } from "node:module";
import test from "node:test";

import { prepareMarkdownRenderer } from "./prepare-markdown-renderer.mjs";

const require = createRequire(import.meta.url);
const miniappRoot = resolve(import.meta.dirname, "..");
const packageRoot = resolve(miniappRoot, "node_modules/mp-html");

test("prepares a WeChat native component with the official Markdown plugin", async () => {
  const temporaryRoot = await mkdtemp(join(tmpdir(), "deeix-mp-html-"));
  const outputRoot = join(temporaryRoot, "mp-html");

  try {
    await prepareMarkdownRenderer({ packageRoot, outputRoot });

    const [componentSource, componentStyle, componentTemplate, nodeSource, pluginSource] = await Promise.all([
      readFile(join(outputRoot, "index.js"), "utf8"),
      readFile(join(outputRoot, "node/node.wxss"), "utf8"),
      readFile(join(outputRoot, "node/node.wxml"), "utf8"),
      readFile(join(outputRoot, "node/node.js"), "utf8"),
      readFile(join(outputRoot, "markdown/index.js"), "utf8"),
    ]);

    assert.match(componentSource, /require\(["']\.\/markdown\/index\.js["']\)/u);
    assert.match(componentSource, /markdown\s*:\s*Boolean/u);
    assert.match(componentStyle, /\.md-blockquote/u);
    assert.match(componentStyle, /\.md-table/u);
    assert.match(componentStyle, /\._codeCopy/u);
    assert.match(componentTemplate, /catchtap="copyCode"/u);
    assert.match(componentTemplate, />复制<\/view>/u);
    assert.match(nodeSource, /copyCode:function/u);
    assert.match(nodeSource, /setClipboardData/u);
    assert.match(pluginSource, /marked/u);

    const transpiledParserSource = await readFile(join(outputRoot, "markdown/marked.min.js"), "utf8");
    assert.doesNotMatch(transpiledParserSource, /(?:\)|\}|\]|\b[A-Za-z_$][\w$]*)=>/u);
    assert.doesNotMatch(transpiledParserSource, /\{\.\.\./u);
    assert.doesNotMatch(transpiledParserSource, /\?\.|\?\?/u);
    assert.doesNotMatch(transpiledParserSource, /\\p\{/u);

    const { marked } = require(join(outputRoot, "markdown/marked.min.js"));
    assert.match(marked("- 上传产物也能解析列表"), /<ul>[\s\S]*<li>/u);
  } finally {
    await rm(temporaryRoot, { force: true, recursive: true });
  }
});

test("the bundled parser covers the Markdown used by Web chat answers", () => {
  const { marked } = require(resolve(packageRoot, "plugins/markdown/marked.min.js"));
  const markdown = [
    "# 标题",
    "",
    "- 无序列表",
    "  - 嵌套项",
    "",
    "1. 有序列表",
    "",
    "> 引用内容",
    "",
    "**粗体** 和 *斜体* 以及 [链接](https://example.com)",
    "",
    "| 名称 | 值 |",
    "| --- | --- |",
    "| 模型 | 可用 |",
    "",
    "```ts",
    "const answer = 42;",
    "```",
  ].join("\n");

  const html = marked(markdown);
  for (const tag of ["<h1", "<ul", "<ol", "<li", "<blockquote", "<strong", "<em", "<a ", "<table", "<pre", "<code"]) {
    assert.ok(html.includes(tag), `expected rendered HTML to contain ${tag}`);
  }
  assert.match(html, /href="https:\/\/example\.com"/u);
});

test("partial streaming Markdown remains renderable and completes without stale markup", () => {
  const { marked } = require(resolve(packageRoot, "plugins/markdown/marked.min.js"));
  const partial = marked("正在生成 **重要内容");
  const complete = marked("正在生成 **重要内容**");

  assert.match(partial, /正在生成/u);
  assert.doesNotMatch(partial, /<strong>/u);
  assert.match(complete, /<strong>重要内容<\/strong>/u);
});
