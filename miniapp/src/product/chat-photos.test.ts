import assert from "node:assert/strict";
import test from "node:test";
import { ChatPhotos, chatPhotoLimit } from "./chat-photos";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

const settle = () => new Promise<void>((resolve) => setImmediate(resolve));

test("multiple uploads preserve selection order, bound concurrency and retry only the failed photo", async () => {
  const uploads: { path: string; result: ReturnType<typeof deferred<string>> }[] = [];
  const photos = new ChatPhotos({
    limit: 9, readLimit: async () => 10, choose: async () => ["a", "b", "c"],
    upload: (path) => { const result = deferred<string>(); uploads.push({ path, result }); return result.promise; },
    onError: assert.fail,
  });
  await photos.choose();
  assert.deepEqual(uploads.map((item) => item.path), ["a", "b"]);
  uploads[1]!.result.reject(new Error("network"));
  await settle();
  assert.deepEqual(uploads.map((item) => item.path), ["a", "b", "c"]);
  uploads[2]!.result.resolve("id-c");
  uploads[0]!.result.resolve("id-a");
  await settle();
  assert.deepEqual(photos.snapshot().photos.map(({ path, status }) => [path, status]), [["a", "ready"], ["b", "failed"], ["c", "ready"]]);
  const failed = photos.snapshot().photos[1]!;
  await Promise.all([photos.retry(failed.id), photos.retry(failed.id)]);
  assert.deepEqual(uploads.map((item) => item.path), ["a", "b", "c", "b"]);
  uploads[3]!.result.resolve("id-b");
  await settle();
  assert.deepEqual(photos.snapshot().photos.map((photo) => photo.fileID), ["id-a", "id-b", "id-c"]);
});

test("deleting an uploading or queued photo prevents late completion from restoring it", async () => {
  const uploads: { path: string; result: ReturnType<typeof deferred<string>> }[] = [];
  const photos = new ChatPhotos({ limit: 9, readLimit: async () => 9, choose: async () => ["a", "b", "c"],
    upload: (path) => { const result = deferred<string>(); uploads.push({ path, result }); return result.promise; }, onError: assert.fail });
  await photos.choose();
  const items = photos.snapshot().photos;
  photos.remove(items[0]!.id);
  photos.remove(items[2]!.id);
  uploads[0]!.result.resolve("deleted");
  uploads[1]!.result.resolve("kept");
  await settle();
  assert.deepEqual(uploads.map((item) => item.path), ["a", "b"]);
  assert.deepEqual(photos.snapshot().photos.map((photo) => photo.fileID), ["kept"]);
});

test("clearing or disposing a draft ignores pending picker and upload results", async () => {
  const picker = deferred<readonly string[]>();
  let calls = 0;
  const photos = new ChatPhotos({ limit: 9, readLimit: async () => 9, choose: () => picker.promise,
    upload: async () => { calls++; return "unused"; }, onError: assert.fail });
  const choosing = photos.choose();
  await settle();
  photos.clear();
  picker.resolve(["old-scope"]);
  await choosing;
  assert.equal(calls, 0);
  assert.equal(photos.snapshot().photos.length, 0);

  const pending = deferred<string>();
  const next = new ChatPhotos({ limit: 9, readLimit: async () => 9, choose: async () => ["photo"], upload: () => pending.promise, onError: assert.fail });
  await next.choose();
  const snapshot = next.snapshot();
  let notifications = 0;
  next.subscribe(() => { notifications++; });
  next.dispose();
  pending.resolve("late");
  await settle();
  assert.equal(next.snapshot(), snapshot);
  assert.equal(notifications, 0);
});

test("selection appends without duplicates and uses the current configured remaining capacity", async () => {
  let limit = 4;
  let selection = ["a", "b"];
  const counts: number[] = [];
  const errors: string[] = [];
  const photos = new ChatPhotos({ limit: 9, readLimit: async () => limit,
    choose: async (count) => { counts.push(count); return selection; }, upload: async (path) => `id-${path}`, onError: (message) => errors.push(message) });
  await photos.choose();
  selection = ["b", "c", "d", "e"];
  await photos.choose();
  assert.deepEqual(counts, [4, 2]);
  assert.deepEqual(photos.snapshot().photos.map((photo) => photo.path), ["a", "b", "c", "d"]);
  limit = 2;
  await photos.choose();
  assert.equal(photos.snapshot().limit, 2);
  assert.equal(photos.snapshot().photos.length, 4);
  assert.equal(counts.length, 2);
  assert.equal(errors.length, 1);
});

test("native picker is locked during policy loading and cancellation does not clear selected photos", async () => {
  let cancel = false;
  let picks = 0;
  const photos = new ChatPhotos({ limit: 9, readLimit: async () => 9,
    choose: async () => { picks++; if (cancel) throw { errMsg: "chooseMedia:fail cancel" }; return ["a"]; },
    upload: async () => "id-a", onError: assert.fail });
  await Promise.all([photos.choose(), photos.choose()]);
  assert.equal(picks, 1);
  cancel = true;
  await photos.choose();
  assert.equal(photos.snapshot().choosing, false);
  assert.equal(photos.snapshot().photos[0]?.fileID, "id-a");
  assert.deepEqual([undefined, 0, -1, Number.NaN, 20, 1, 3].map(chatPhotoLimit), [9, 9, 9, 9, 9, 1, 3]);
});
