import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import React from "react";
import ts from "typescript";
import { nextChatBottomScrollTop, shouldReleaseChatAutoFollow } from "./chat-auto-scroll";

test("streaming scroll releases only when the user actively moves upward", () => {
  assert.equal(shouldReleaseChatAutoFollow(300, 250, true), true);
  assert.equal(shouldReleaseChatAutoFollow(300, 250, false), false);
  assert.equal(shouldReleaseChatAutoFollow(250, 300, true), false);
  assert.equal(shouldReleaseChatAutoFollow(300, 297, true), false);
});

test("each streaming update retriggers the proven high scrollTop target", () => {
  const first = nextChatBottomScrollTop(0);
  const second = nextChatBottomScrollTop(first);
  const third = nextChatBottomScrollTop(second);

  assert.equal(first, 999_999);
  assert.equal(second, 999_998);
  assert.equal(third, 999_999);
});

test("chat and image workspaces share numeric auto-follow and image load settlement", () => {
  const source = readFileSync(
    resolve(process.cwd(), "src/pages/index/index.tsx"),
    "utf8",
  );

  assert.equal(source.match(/scrollTop=\{chatScrollTop\}/gu)?.length, 2);
  assert.equal(source.match(/scrollAnchoring(?:\s|=)/gu)?.length, 2);
  assert.equal(source.match(/bounces=\{false\}/gu)?.length, 2);
  assert.match(source, /\(screen === "chat" \|\| screen === "image"\) && chatAutoFollowRef\.current/u);
  assert.ok((source.match(/onLoad=\{handleConversationImageLoad\}/gu)?.length ?? 0) >= 2);
  assert.doesNotMatch(source, /scrollIntoView=/u);
});

test("image viewport disables native anchoring and animated scroll without losing auto-follow", () => {
  const source = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
  const imageViewport = source.match(/<ScrollView\s+className="imageCanvas"[\s\S]*?>/u)?.[0];
  assert.ok(imageViewport, "image viewport must exist");
  assert.match(imageViewport, /scrollAnchoring=\{false\}/u);
  assert.match(imageViewport, /scrollWithAnimation=\{false\}/u);
  assert.match(imageViewport, /bounces=\{false\}/u);
  assert.match(imageViewport, /scrollTop=\{chatScrollTop\}/u);

  const styles = readFileSync(resolve(process.cwd(), "src/pages/index/index.scss"), "utf8");
  assert.match(styles, /\.imageCanvas\s*\{[^}]*overflow-anchor:\s*none/u);
});

test("image scroll viewport is bounded above the composer and measures one complete content block", () => {
  const source = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
  const tree = ts.createSourceFile("index.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let imageViewport: ts.JsxElement | undefined;
  const visit = (node: ts.Node) => {
    if (ts.isJsxElement(node) && node.openingElement.attributes.properties.some((attribute) =>
      ts.isJsxAttribute(attribute) && attribute.name.getText(tree) === "className" &&
      attribute.initializer && ts.isStringLiteral(attribute.initializer) && attribute.initializer.text === "imageCanvas",
    )) imageViewport = node;
    ts.forEachChild(node, visit);
  };
  visit(tree);
  assert.ok(imageViewport);
  const children = imageViewport.children.filter((child) => !ts.isJsxText(child) || child.text.trim());
  assert.equal(children.length, 1, "native scroll height must include images AND action buttons in one block");
  assert.ok(ts.isJsxElement(children[0]), "all image messages need a content wrapper, not separate native scroll items");
  assert.match(children[0].openingElement.getText(tree), /className="imageCanvasContent"/u);
  assert.match(imageViewport.parent.getText(tree), /className="messageListShell imageCanvasShell"/u);

  const styles = readFileSync(resolve(process.cwd(), "src/pages/index/index.scss"), "utf8");
  assert.match(styles, /\.imageCanvasShell\s*\{[^}]*height:\s*0;[^}]*overflow:\s*hidden/u);
  const viewportStyles = Array.from(styles.matchAll(/\.imageCanvas\s*\{([^}]+)\}/gu), (match) => match[1]).join("\n");
  assert.match(viewportStyles, /position:\s*absolute/u);
  assert.match(viewportStyles, /height:\s*100%/u);
  assert.doesNotMatch(viewportStyles, /padding:/u, "spacing belongs to scrollable content, not the native viewport");
  assert.match(styles, /\.imageCanvasContent\s*\{[^}]*padding:\s*0 2px 24px/u);
});

test("manual image scrolling only updates the bottom button, not the native image subtree", { timeout: 5000 }, async () => {
  // Taro normally receives these compile-time switches from its webpack plugin.
  Object.assign(globalThis, {
    ENABLE_INNER_HTML: false,
    ENABLE_ADJACENT_HTML: false,
    ENABLE_CLONE_NODE: false,
    ENABLE_CONTAINS: false,
    ENABLE_SIZE_APIS: false,
    ENABLE_TEMPLATE_CONTENT: false,
  });
  const { document } = require("@tarojs/runtime");
  const renderer = require("@tarojs/react");
  const source = readFileSync(resolve(process.cwd(), "src/pages/index/index.tsx"), "utf8");
  const tree = ts.createSourceFile("index.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let imageShell: ts.JsxElement | undefined;
  const visit = (node: ts.Node) => {
    if (ts.isJsxElement(node) && node.openingElement.attributes.properties.some((attribute) =>
      ts.isJsxAttribute(attribute) && attribute.name.getText(tree) === "className" &&
      attribute.initializer && ts.isStringLiteral(attribute.initializer) &&
      attribute.initializer.text === "messageListShell imageCanvasShell",
    )) imageShell = node;
    ts.forEachChild(node, visit);
  };
  visit(tree);
  assert.ok(imageShell);

  // Execute the actual workspace JSX with the installed Taro renderer. AST checks
  // alone cannot catch a sibling removal rewriting the native scroll-view's data.
  const scope: Record<string, unknown> = {
    React, View: "view", ScrollView: "scroll-view", Image: "image", Text: "text", Button: "button",
    messages: [{ id: "generated", role: "assistant", imageSource: "fixture.png" }],
    running: false,
    chatScrollTop: 999_999,
    chatTouchingRef: { current: true },
    chatAutoFollow: false,
  };
  for (const name of [
    "handleChatScroll", "handleChatScrollToLower", "handleConversationImageLoad",
    "enableChatAutoFollow", "previewImage", "regenerateImageAnswer", "saveImage",
  ]) scope[name] = () => {};
  const compiled = ts.transpileModule(
    `const { ${Object.keys(scope).join(",")} } = scope; return (${imageShell.getText(tree)});`,
    { compilerOptions: { jsx: ts.JsxEmit.React, target: ts.ScriptTarget.ES2020 } },
  ).outputText;
  const workspace = new Function("scope", compiled);
  const root = document.createElement("root");
  const patches: Record<string, unknown>[] = [];
  const render = () => new Promise<void>((done) => {
    root.ctx = {
      setData(data: Record<string, unknown>, callback?: () => void) {
        patches.push(data);
        callback?.();
        done();
      },
    };
    renderer.render(workspace(scope), root);
  });
  try {
    await render();
    const shell = root.childNodes[0];
    const viewport = shell.childNodes[0];
    for (const following of [true, false, true]) {
      patches.length = 0;
      scope.chatAutoFollow = following;
      await render();
      const payload = JSON.stringify(patches);
      assert.doesNotMatch(payload, /fixture\.png|999999/u,
        "manual scrolling must not resend images or the stale programmatic scroll target");
      const buttonPath = shell.childNodes[1]._path;
      assert.ok(patches.flatMap((patch) => Object.keys(patch)).every((path) => path.startsWith(`${buttonPath}.`)),
        "only bottom-button properties may change when auto-follow toggles");
      assert.equal(shell.childNodes[0], viewport);
    }
  } finally {
    renderer.unmountComponentAtNode(root);
  }
});
