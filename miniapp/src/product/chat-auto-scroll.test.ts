import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";
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
  assert.equal(source.match(/scrollAnchoring/gu)?.length, 2);
  assert.equal(source.match(/bounces=\{false\}/gu)?.length, 2);
  assert.match(source, /\(screen === "chat" \|\| screen === "image"\) && chatAutoFollowRef\.current/u);
  assert.ok((source.match(/onLoad=\{handleConversationImageLoad\}/gu)?.length ?? 0) >= 2);
  assert.doesNotMatch(source, /scrollIntoView=/u);
});
