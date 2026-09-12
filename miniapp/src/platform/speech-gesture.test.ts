import assert from "node:assert/strict";
import test from "node:test";
import { SpeechInput, type SpeechRecorder } from "../product/speech-input";
import { SpeechHoldGesture, speechTouches } from "./speech-gesture";

const finger = (x = 200, y = 700, identifier = 1) => ({ x, y, identifier });
const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

function setup(authorize = async () => {}) {
  let events!: Parameters<SpeechRecorder["start"]>[0];
  let starts = 0;
  let stops = 0;
  let requests = 0;
  const drafts: string[] = [];
  const removed: string[] = [];
  const file = { path: "hold.mp3", duration: 1500, size: 5000 };
  const input = new SpeechInput({
    recorder: {
      authorize,
      start(next) {
        starts++;
        events = next;
        return { stop() { stops++; events.onStop(file); }, cancel() { events.onStop(file); } };
      },
      read: async () => "audio",
      remove: (recording) => { removed.push(recording.path); },
    },
    transcribe: async () => { requests++; return { text: "识别文字" }; },
    getDraft: () => "",
    onDraft: (text) => { drafts.push(text); },
    onStatus: (status) => {
      if (status === "recognizing") gesture.recordingStopped();
      if (status === "idle") gesture.reset();
    },
    onError: (error) => assert.fail(error),
  });
  const gesture = new SpeechHoldGesture(input, () => {});
  return {
    input, gesture, drafts, removed,
    async record() { gesture.begin(finger(), 400); await flush(); events.onStart(); },
    autoStop() { events.onStop(file); },
    get starts() { return starts; }, get stops() { return stops; }, get requests() { return requests; },
  };
}

test("ordinary release and upper-right release each recognize exactly once", async () => {
  for (const releaseAt of [finger(), finger(330, 570)]) {
    const state = setup();
    await state.record();
    state.gesture.release([releaseAt]);
    state.gesture.release([releaseAt]);
    await flush();
    assert.equal(state.stops, 1);
    assert.equal(state.requests, 1);
    assert.deepEqual(state.drafts, ["识别文字"]);
  }
});

test("release uses the final finger position and discards upper-left audio", async () => {
  const state = setup();
  await state.record();
  state.gesture.release([finger(70, 570)]);
  await flush();
  assert.equal(state.input.status, "idle");
  assert.equal(state.requests, 0);
  assert.deepEqual(state.drafts, []);
  assert.deepEqual(state.removed, ["hold.mp3"]);
});

test("sliding back restores recording and a second finger cannot finish it", async () => {
  const state = setup();
  await state.record();
  state.gesture.move([finger(70, 570)]);
  assert.equal(state.gesture.target, "cancel");
  state.gesture.move([finger(70, 642)]);
  assert.equal(state.gesture.target, "cancel", "small edge movement should not flicker");
  state.gesture.move([finger(200, 690)]);
  assert.equal(state.gesture.target, "record");
  state.gesture.release([finger(200, 690, 2)]);
  assert.equal(state.input.status, "recording");
  state.gesture.release([finger()]);
  await flush();
  assert.equal(state.requests, 1);
});

test("letting go while authorization is pending cannot start a ghost recording", async () => {
  let authorize!: () => void;
  const permission = new Promise<void>((resolve) => { authorize = resolve; });
  const state = setup(() => permission);
  state.gesture.begin(finger(), 400);
  state.gesture.release([finger()]);
  authorize();
  await flush();
  assert.equal(state.starts, 0);
  assert.equal(state.input.status, "idle");
});

test("interrupted touches and automatic stop in the cancel zone never upload", async () => {
  for (const interrupt of ["missing-end", "touchcancel", "time-limit"]) {
    const state = setup();
    await state.record();
    if (interrupt === "missing-end") state.gesture.release([]);
    if (interrupt === "touchcancel") state.gesture.cancel();
    if (interrupt === "time-limit") {
      state.gesture.move([finger(70, 570)]);
      state.autoStop();
    }
    await flush();
    assert.equal(state.requests, 0, interrupt);
    assert.equal(state.input.status, "idle", interrupt);
    assert.deepEqual(state.removed, ["hold.mp3"], interrupt);
  }
});

test("native and React touch events retain finger identity and viewport coordinates", () => {
  assert.deepEqual(speechTouches({ touches: [{ identifier: 4, clientX: 20, clientY: 40, pageY: 900 }] }), [finger(20, 40, 4)]);
  assert.deepEqual(speechTouches({ nativeEvent: { changedTouches: [{ identifier: 3, pageX: 10, pageY: 30 }] } }, true), [finger(10, 30, 3)]);
  assert.deepEqual(speechTouches({ changedTouches: [{ identifier: 1 }] }, true), []);
});
