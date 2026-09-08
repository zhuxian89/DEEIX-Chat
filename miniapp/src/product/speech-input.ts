export type SpeechStatus = "idle" | "starting" | "recording" | "recognizing";
export type SpeechRecording = { path: string; duration: number; size: number };
export type SpeechCapture = { stop(): void; cancel(): void };
export interface SpeechRecorder {
  authorize(): Promise<void>;
  start(events: { onStart(): void; onStop(file: SpeechRecording): void; onError(error: unknown): void }): SpeechCapture;
  read(file: SpeechRecording): Promise<string>;
  remove(file: SpeechRecording): void;
}

const SPEECH_ERROR_MESSAGES: Readonly<Record<string, string>> = {
  speech_no_speech: "没有听清你说的话，请再试一次",
  speech_invalid_audio: "这段录音无法识别，请重新录制",
  speech_audio_too_large: "录音过长，请分成几段说",
  speech_quota_exhausted: "语音识别额度暂时用完了，可以继续打字",
  speech_busy: "语音识别正忙，请稍后再试",
  speech_unavailable: "语音输入暂时不可用，可以继续打字",
};

export function speechErrorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : "";
  for (const [code, text] of Object.entries(SPEECH_ERROR_MESSAGES)) {
    if (message.includes(code)) return text;
  }
  if (["录音", "请", "输入", "没有"].some((prefix) => message.startsWith(prefix))) {
    return message;
  }
  return "语音识别失败，原来的文字已保留，请重试";
}

// Platform adaptation of Web's draft append behavior. Recording and ASR are
// asynchronous on WeChat; a visit token prevents late results crossing screens.
export class SpeechInput {
  status: SpeechStatus = "idle";
  private version = 0;
  private capture: SpeechCapture | null = null;
  private disposed = false;

  constructor(private readonly options: {
    recorder: SpeechRecorder;
    transcribe(audio: string): Promise<{ text: string }>;
    getDraft(): string;
    onDraft(value: string): void;
    onStatus(status: SpeechStatus): void;
    onError(message: string): void;
  }) {}

  private setStatus(status: SpeechStatus) {
    this.status = status;
    if (!this.disposed) this.options.onStatus(status);
  }

  private isCurrent(version: number) {
    return !this.disposed && version === this.version;
  }

  async start() {
    if (this.disposed || this.status !== "idle") return;
    const version = ++this.version;
    this.setStatus("starting");
    try {
      await this.options.recorder.authorize();
      if (!this.isCurrent(version)) return;
      this.capture = this.options.recorder.start({
        onStart: () => {
          if (this.isCurrent(version)) this.setStatus("recording");
        },
        onStop: (file) => {
          void this.recognize(file, version);
        },
        onError: (error) => {
          if (!this.isCurrent(version)) return;
          this.capture = null;
          this.setStatus("idle");
          this.options.onError(speechErrorMessage(error));
        },
      });
    } catch (error) {
      if (!this.isCurrent(version)) return;
      this.setStatus("idle");
      this.options.onError(speechErrorMessage(error));
    }
  }

  stop() {
    if (this.status !== "recording") return;
    this.setStatus("recognizing");
    this.capture?.stop();
  }

  cancel() {
    ++this.version;
    const capture = this.capture;
    this.capture = null;
    capture?.cancel();
    this.setStatus("idle");
  }

  dispose() {
    this.disposed = true;
    this.cancel();
  }

  private async recognize(file: SpeechRecording, version: number) {
    try {
      if (!this.isCurrent(version)) return;
      this.capture = null;
      this.setStatus("recognizing");
      if (file.duration < 500 || file.size === 0) throw new Error("录音太短，请说完一句话再结束");
      if (file.duration > 60000 || file.size > 512 * 1024) throw new Error("录音过长，请分成几段说");
      const audio = await this.options.recorder.read(file);
      if (!this.isCurrent(version)) return;
      const result = await this.options.transcribe(audio);
      if (!this.isCurrent(version)) return;
      const transcript = result.text.trim();
      if (!transcript) throw new Error("没有听清你说的话，请再试一次");
      const next = [this.options.getDraft().trimEnd(), transcript].filter(Boolean).join(" ");
      if (next.length > 8000) throw new Error("输入内容已达长度上限，请先发送已有文字");
      this.options.onDraft(next);
    } catch (error) {
      if (this.isCurrent(version)) this.options.onError(speechErrorMessage(error));
    } finally {
      this.options.recorder.remove(file);
      if (this.isCurrent(version)) this.setStatus("idle");
    }
  }
}
