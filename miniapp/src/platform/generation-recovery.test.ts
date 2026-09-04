import assert from "node:assert/strict";
import test from "node:test";
import {
  createClientRunID,
  isWechatRequestInterrupted,
  runRecoverableStream,
  WechatRequestError,
  type RecoverableStreamHandle,
} from "./generation-recovery";

function resolvedHandle<T>(value: T, lastSeq = 0): RecoverableStreamHandle<T> {
  return { abort() {}, lastSeq: () => lastSeq, promise: Promise.resolve(value) };
}

function rejectedHandle(error: Error, lastSeq: number): RecoverableStreamHandle<never> {
  return { abort() {}, lastSeq: () => lastSeq, promise: Promise.reject(error) };
}

test("recognizes WeChat background request interruptions", () => {
  assert.equal(isWechatRequestInterrupted(new WechatRequestError("request:fail interrupted", 600003)), true);
  assert.equal(isWechatRequestInterrupted({ errMsg: "request:fail interrupted" }), true);
  assert.equal(isWechatRequestInterrupted(new Error("request:fail timeout")), false);
});

test("starts a generation once and resumes from the greatest observed event sequence", async () => {
  const interruption = new WechatRequestError("request:fail interrupted", 600003);
  let startCalls = 0;
  const resumeAfter: number[] = [];
  const handles = [
    rejectedHandle(interruption, 7),
    rejectedHandle(interruption, 11),
    resolvedHandle("completed", 12),
  ];

  const result = await runRecoverableStream({
    start() {
      startCalls += 1;
      return handles[0];
    },
    resume(afterSeq) {
      resumeAfter.push(afterSeq);
      return handles[resumeAfter.length];
    },
    shouldResume: isWechatRequestInterrupted,
    waitUntilResume: async () => {},
  });

  assert.equal(result, "completed");
  assert.equal(startCalls, 1);
  assert.deepEqual(resumeAfter, [7, 11]);
});

test("does not resume ordinary request failures", async () => {
  let resumeCalls = 0;
  await assert.rejects(
    runRecoverableStream({
      start: () => rejectedHandle(new Error("request:fail timeout"), 4),
      resume: () => {
        resumeCalls += 1;
        return resolvedHandle("unexpected");
      },
      shouldResume: isWechatRequestInterrupted,
      waitUntilResume: async () => {},
    }),
    /timeout/u,
  );
  assert.equal(resumeCalls, 0);
});

test("does not resume after the user cancels while waiting for foreground", async () => {
  let canceled = false;
  let resumeCalls = 0;
  await assert.rejects(
    runRecoverableStream({
      start: () => rejectedHandle(new WechatRequestError("request:fail interrupted", 600003), 3),
      resume: () => {
        resumeCalls += 1;
        return resolvedHandle("unexpected");
      },
      shouldResume: isWechatRequestInterrupted,
      waitUntilResume: async () => {
        canceled = true;
      },
      isCanceled: () => canceled,
    }),
    /interrupted/u,
  );
  assert.equal(resumeCalls, 0);
});

test("client run IDs are backend-compatible and unique", () => {
  const first = createClientRunID();
  const second = createClientRunID();
  assert.match(first, /^run_[a-zA-Z0-9_-]+$/u);
  assert.ok(first.length <= 64);
  assert.notEqual(first, second);
});
