import assert from "node:assert/strict";
import test from "node:test";
import { downloadMessagePhotos } from "./message-photo-downloads";

const settle = () => new Promise<void>((resolve) => setImmediate(resolve));

test("history download concurrency is bounded and a failed file does not drop the remaining photos", async () => {
  const pending = new Map<string, (value: string | null) => void>();
  const calls: string[] = [];
  const results: string[] = [];
  const task = downloadMessagePhotos([{ id: "message", role: "user", text: "", images: [
    { fileID: "cached", source: "local" }, ...["a", "b", "c", "d", "e"].map((fileID) => ({ fileID })),
  ] }], (id) => {
    calls.push(id);
    if (id === "b") return Promise.reject(new Error("offline"));
    return new Promise((resolve) => pending.set(id, resolve));
  }, (messageID, fileID, source) => results.push(`${messageID}:${fileID}:${source}`));
  assert.deepEqual(calls, ["a", "b", "c"]);
  await settle();
  assert.deepEqual(calls, ["a", "b", "c", "d"]);
  pending.get("c")!("c-path");
  await settle();
  assert.deepEqual(calls, ["a", "b", "c", "d", "e"]);
  for (const id of ["a", "d", "e"]) pending.get(id)!(`${id}-path`);
  await task;
  assert.equal(results.length, 5);
  assert.ok(results.includes("message:b:null"));
  assert.ok(results.includes("message:a:a-path"));
});

test("leaving a conversation stops queued downloads and ignores in-flight results", async () => {
  let active = true;
  const pending: ((value: string | null) => void)[] = [];
  const task = downloadMessagePhotos([{ id: "old", role: "user", text: "", images: Array.from({ length: 9 }, (_, i) => ({ fileID: `${i}` })) }],
    () => new Promise((resolve) => pending.push(resolve)), () => assert.fail("stale download reached another conversation"), () => active);
  assert.equal(pending.length, 3);
  active = false;
  pending.forEach((resolve) => resolve("late"));
  await task;
  assert.equal(pending.length, 3);
});
