import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
import React from "react";
import ts from "typescript";
import { nextChatBottomScrollTop, shouldReleaseChatAutoFollow } from "./chat-auto-scroll";

function scrollFixture() {
  const source = readFileSync(resolve(process.cwd(), "src/components/companion/companion.tsx"), "utf8");
  const tree = ts.createSourceFile("companion.tsx", source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  let shell: ts.JsxElement | undefined;
  const visit = (node: ts.Node) => {
    if (ts.isJsxElement(node) && node.openingElement.attributes.properties.some((attribute) =>
      ts.isJsxAttribute(attribute) && attribute.name.getText(tree) === "className" &&
      attribute.initializer && ts.isStringLiteral(attribute.initializer) &&
      attribute.initializer.text === "companionHistoryShell",
    )) shell = node;
    ts.forEachChild(node, visit);
  };
  visit(tree);
  assert.ok(shell);
  const threshold = source.match(/const COMPANION_BOTTOM_THRESHOLD_PX = \d+;/u)?.[0];
  assert.ok(threshold);
  // Execute the actual JSX and event handlers, including the native Taro tree.
  const scope = {
    React, View: "view", ScrollView: "scroll-view", Image: "image", Text: "text", Button: "button",
    Markdown: ({ children }: { children: React.ReactNode }) => React.createElement("text", null, children),
    CompanionAvatar: () => React.createElement("image", { src: "avatar.png" }),
    COMPANION_NAME: "fixture",
    timeline: [{ id: "answer", role: "assistant", text: "fixture-reply", images: ["fixture.png"] }],
    state: null,
    loading: false,
    following: false,
    scrollTop: 999_999,
    touching: { current: false },
    scrollingAt: { current: 0 },
    previousScrollTop: { current: 1200 },
    historyHeight: { current: 600 },
    autoFollow: { current: false },
    measureHistory: () => {},
    recordCompanionInteraction: () => {},
    setFollowing: (value: boolean) => { scope.following = value; },
    setScrollTop: (update: (current: number) => number) => { scope.scrollTop = update(scope.scrollTop); },
    nextChatBottomScrollTop,
    shouldReleaseChatAutoFollow,
  };
  const compiled = ts.transpile(`${threshold}\nwith (scope) { return (${shell.getText(tree)}); }`, {
    jsx: ts.JsxEmit.React, target: ts.ScriptTarget.ES2022,
  });
  const workspace = new Function("scope", compiled);
  return { scope, render: () => workspace(scope) };
}

test("companion recognizes the bottom while touching, with tolerance for small upward movements", () => {
  const { scope, render } = scrollFixture();
  const viewport = render().props.children[0].props;
  const scroll = (top: number) => viewport.onScroll({ detail: { scrollTop: top, scrollHeight: 2000 } });

  viewport.onTouchStart();
  scroll(1300); // 100 px remain: keep the return button.
  assert.equal(scope.following, false);
  scroll(1320); // Within 80 px: no forceful fling or finger release needed.
  assert.equal(scope.touching.current, true);
  assert.equal(scope.following, true);
  assert.equal(scope.autoFollow.current, true);
  scroll(1340);
  scroll(1328); // A small upward adjustment still leaves the latest message visible.
  assert.equal(scope.following, true);
  scroll(1310); // Reading older content outside the tolerance releases follow.
  assert.equal(scope.following, false);
  assert.equal(scope.autoFollow.current, false);
  viewport.onTouchEnd();
  scroll(1390); // Inertial scrolling also settles correctly.
  assert.equal(scope.following, true);
  assert.equal(scope.scrollTop, 999_999, "scroll events must not force a new programmatic scroll");

  scope.historyHeight.current = 0;
  scope.following = false;
  scope.autoFollow.current = false;
  viewport.onTouchStart();
  viewport.onScrollToLower(); // Native fallback works before the first measurement too.
  assert.equal(scope.following, true);
  assert.equal(scope.autoFollow.current, true);
});

test("companion bottom-button visibility does not resend messages or the old scroll target", { timeout: 5000 }, async () => {
  Object.assign(globalThis, {
    ENABLE_INNER_HTML: false, ENABLE_ADJACENT_HTML: false, ENABLE_CLONE_NODE: false,
    ENABLE_CONTAINS: false, ENABLE_SIZE_APIS: false, ENABLE_TEMPLATE_CONTENT: false,
  });
  const { document } = require("@tarojs/runtime");
  const renderer = require("@tarojs/react");
  const fixture = scrollFixture();
  const root = document.createElement("root");
  const patches: Record<string, unknown>[] = [];
  const render = () => new Promise<void>((done) => {
    root.ctx = { setData(data: Record<string, unknown>, callback?: () => void) {
      patches.push(data);
      callback?.();
      done();
    } };
    renderer.render(fixture.render(), root);
  });
  try {
    await render();
    const shell = root.childNodes[0];
    const viewport = shell.childNodes[0];
    for (const following of [true, false, true]) {
      patches.length = 0;
      fixture.scope.following = following;
      await render();
      assert.doesNotMatch(JSON.stringify(patches), /fixture\.png|fixture-reply|999999/u);
      const buttonPath = shell.childNodes[1]._path;
      assert.ok(patches.flatMap((patch) => Object.keys(patch)).every((path) => path.startsWith(`${buttonPath}.`)));
      assert.equal(shell.childNodes[0], viewport);
    }
  } finally {
    renderer.unmountComponentAtNode(root);
  }
});
