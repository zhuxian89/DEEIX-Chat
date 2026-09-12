import type { SpeechInput } from "@/product/speech-input";

export type SpeechTouch = { identifier: number; x: number; y: number };
export type SpeechGestureTarget = "record" | "cancel" | "text";

export function speechTouches(event: unknown, changed = false): SpeechTouch[] {
  type Touch = { identifier?: number; clientX?: number; clientY?: number; pageX?: number; pageY?: number };
  type TouchEvent = { touches?: Touch[]; changedTouches?: Touch[]; nativeEvent?: TouchEvent };
  const source = event as TouchEvent;
  const key = changed ? "changedTouches" : "touches";
  return (source[key] ?? source.nativeEvent?.[key] ?? []).map((touch) => ({
    identifier: touch.identifier ?? 0,
    x: Number(touch.clientX ?? touch.pageX),
    y: Number(touch.clientY ?? touch.pageY),
  })).filter((touch) => Number.isFinite(touch.x) && Number.isFinite(touch.y));
}

// Touch ownership stays with the original finger, including while native
// microphone authorization is pending or the recording reaches its time limit.
export class SpeechHoldGesture {
  target: SpeechGestureTarget = "record";
  private origin: SpeechTouch | null = null;
  private width = 0;

  constructor(
    private readonly input: Pick<SpeechInput, "status" | "start" | "stop" | "cancel">,
    private readonly onTarget: (target: SpeechGestureTarget) => void,
  ) {}

  begin(touch: SpeechTouch | undefined, width: number) {
    if (!touch || this.origin || this.input.status !== "idle" || !Number.isFinite(width) || width <= 0) return;
    this.origin = touch;
    this.width = width;
    this.setTarget("record");
    void this.input.start();
  }

  move(touches: SpeechTouch[]) {
    if (!this.origin) return;
    const touch = touches.find((item) => item.identifier === this.origin?.identifier);
    if (!touch) return;
    const threshold = this.target === "record" ? 64 : 48;
    let target: SpeechGestureTarget = "record";
    if (this.origin.y - touch.y >= threshold) {
      if (touch.x < this.width * 0.46) target = "cancel";
      else if (touch.x > this.width * 0.54) target = "text";
    }
    this.setTarget(target);
  }

  release(touches: SpeechTouch[]) {
    if (!this.origin) return;
    if (touches.length === 0) {
      this.cancel();
      return;
    }
    if (!touches.some((touch) => touch.identifier === this.origin?.identifier)) return;
    this.move(touches);
    const discard = this.target === "cancel" || this.input.status === "starting";
    this.reset();
    if (discard) this.input.cancel();
    else this.input.stop();
  }

  // An automatic stop in the cancel zone must not upload the recording.
  recordingStopped(): boolean {
    if (this.target === "cancel") {
      this.cancel();
      return true;
    }
    this.reset();
    return false;
  }

  cancel() {
    this.reset();
    this.input.cancel();
  }

  reset() {
    this.origin = null;
    this.setTarget("record");
  }

  private setTarget(target: SpeechGestureTarget) {
    if (this.target === target) return;
    this.target = target;
    this.onTarget(target);
  }
}
