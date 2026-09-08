import assert from "node:assert/strict";
import test from "node:test";
import { SpeechInput, type SpeechRecorder, type SpeechRecording } from "./speech-input";

const file: SpeechRecording = { path: "temporary.mp3", duration: 1500, size: 5000 };
const flush = () => new Promise<void>((resolve) => setImmediate(resolve));
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>((r) => { resolve = r; }); return { promise, resolve }; }

function setup(options: { authorize?: () => Promise<void>; transcribe?: () => Promise<{text:string}> } = {}) {
  let draft = "原有的文字";
  let events!: Parameters<SpeechRecorder["start"]>[0];
  let requests = 0;
  let cancellations = 0;
  let starts = 0;
  const removed: string[] = [];
  const errors: string[] = [];
  const input = new SpeechInput({
    recorder: {
      authorize: options.authorize ?? (async () => {}),
      start(next) { starts++; events = next; return { stop: () => events.onStop(file), cancel: () => { cancellations++; } }; },
      read: async () => "encoded-audio", remove: (next) => { removed.push(next.path); },
    },
    transcribe: async () => { requests++; return options.transcribe ? options.transcribe() : {text:"语音内容"}; },
    getDraft: () => draft, onDraft: (next) => { draft = next; }, onStatus: () => {}, onError: (message) => { errors.push(message); },
  });
  return { input, errors, removed, get draft() { return draft; }, get events() { return events; }, get requests() { return requests; }, get cancellations() { return cancellations; }, get starts() { return starts; } };
}

test("voice input appends to the existing draft only after recognition", async () => {
  const result = setup();
  await result.input.start();
  result.events.onStart();
  assert.equal(result.input.status, "recording");
  assert.equal(result.draft, "原有的文字");
  result.input.stop();
  await flush();
  assert.equal(result.draft, "原有的文字 语音内容");
  assert.equal(result.requests, 1);
  assert.deepEqual(result.removed, [file.path]);
  assert.equal(result.input.status, "idle");
});

test("cancelling permission prevents recording from starting later", async () => {
  const permission = deferred<void>();
  const result = setup({ authorize: () => permission.promise });
  const starting = result.input.start();
  result.input.cancel();
  permission.resolve();
  await starting;
  assert.equal(result.starts, 0);
  assert.equal(result.requests, 0);
});

test("cancelled and disposed recordings discard late stop events", async () => {
  for (const cancel of ["cancel", "dispose"] as const) {
    const result = setup();
    await result.input.start();
    result.events.onStart();
    result.input[cancel]();
    result.events.onStop(file);
    await flush();
    assert.equal(result.requests, 0);
    assert.equal(result.draft, "原有的文字");
    assert.equal(result.cancellations, 1);
    assert.deepEqual(result.removed, [file.path]);
  }
});

test("recognition cannot write into another visit after cancel or dispose", async () => {
  for (const cancel of ["cancel", "dispose"] as const) {
    const response = deferred<{text:string}>();
    const result = setup({ transcribe: () => response.promise });
    await result.input.start();
    result.events.onStart(); result.input.stop();
    await flush();
    result.input[cancel]();
    response.resolve({text:"迟到的结果"});
    await flush();
    assert.equal(result.draft, "原有的文字");
    assert.deepEqual(result.removed, [file.path]);
  }
});

test("silent and failed recognition preserve draft and remove temporary audio", async () => {
  for (const transcribe of [async () => ({text:"  "}), async () => { throw new Error("speech_quota_exhausted"); }]) {
    const result = setup({transcribe});
    await result.input.start();
    result.events.onStart(); result.input.stop(); await flush();
    assert.equal(result.draft, "原有的文字");
    assert.equal(result.errors.length, 1);
    assert.deepEqual(result.removed, [file.path]);
    assert.equal(result.input.status, "idle");
  }
});

test("double start and stop do not duplicate recognition calls", async () => {
  const result = setup();
  await Promise.all([result.input.start(), result.input.start()]);
  assert.equal(result.starts, 1);
  result.events.onStart(); result.input.stop(); result.input.stop();
  await flush();
  assert.equal(result.requests, 1);
});

test("invalid recordings are rejected before upload", async () => {
  for (const invalid of [{...file, duration:100}, {...file, duration:61000}, {...file, size:600000}]) {
    const result = setup();
    await result.input.start();
    result.events.onStop(invalid); await flush();
    assert.equal(result.requests, 0);
    assert.equal(result.draft, "原有的文字");
    assert.deepEqual(result.removed, [file.path]);
  }
});
