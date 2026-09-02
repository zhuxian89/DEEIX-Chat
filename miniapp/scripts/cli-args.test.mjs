import assert from "node:assert/strict";
import test from "node:test";

import { readSingleArgument } from "./cli-args.mjs";

test("reads a direct pnpm script argument", () => {
  assert.equal(readSingleArgument(["https://chat.20260310.best"], "usage"), "https://chat.20260310.best");
});

test("ignores a pnpm-forwarded argument separator", () => {
  assert.equal(readSingleArgument(["--", "https://chat.20260310.best"], "usage"), "https://chat.20260310.best");
});

test("rejects missing or ambiguous arguments", () => {
  assert.throws(() => readSingleArgument(["--"], "usage text"), /usage text/u);
  assert.throws(() => readSingleArgument(["one", "two"], "usage text"), /exactly one argument/u);
});
