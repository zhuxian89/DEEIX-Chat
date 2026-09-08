import Taro from "@tarojs/taro";
import type { SpeechRecorder, SpeechRecording } from "@/product/speech-input";

type Events = Parameters<SpeechRecorder["start"]>[0];
type Capture = { events: Events; cancelled: boolean; timer: ReturnType<typeof setTimeout> };
let manager: Taro.RecorderManager | null = null;
let capture: Capture | null = null;

function remove(file: SpeechRecording) {
  if (file.path) Taro.getFileSystemManager().unlink({ filePath: file.path, fail: () => {} });
}

function getManager() {
  if (manager) return manager;
  const recorder = Taro.getRecorderManager();
  manager = recorder;
  // Bind once: RecorderManager is a singleton and Taro does not expose offStop.
  recorder.onStart(() => {
    if (!capture) return;
    if (capture.cancelled) {
      recorder.stop();
      return;
    }
    clearTimeout(capture.timer);
    capture.events.onStart();
  });
  recorder.onStop((result) => {
    const owner = capture;
    capture = null;
    const file = { path: result.tempFilePath, duration: result.duration, size: result.fileSize };
    if (owner) clearTimeout(owner.timer);
    if (!owner || owner.cancelled) {
      remove(file);
      return;
    }
    owner.events.onStop(file);
  });
  recorder.onError(() => {
    const owner = capture;
    capture = null;
    if (owner) clearTimeout(owner.timer);
    if (owner && !owner.cancelled) owner.events.onError(new Error("录音失败，请检查麦克风权限后重试"));
  });
  recorder.onInterruptionBegin(() => {
    if (!capture || capture.cancelled) return;
    capture.events.onError(new Error("录音被来电或其他音频中断，请重新录制"));
    capture.cancelled = true;
    recorder.stop();
  });
  return recorder;
}

export const speechRecorder: SpeechRecorder = {
  async authorize() {
    if (typeof Taro.requirePrivacyAuthorize === "function") {
      await new Promise<void>((resolve, reject) => Taro.requirePrivacyAuthorize({
        success: () => resolve(),
        fail: () => reject(new Error("请先同意隐私保护指引，再使用语音输入")),
      }));
    }
    const settings = await Taro.getSetting();
    if (settings.authSetting["scope.record"] === false) {
      const answer = await Taro.showModal({
        title: "开启麦克风权限",
        content: "语音输入需要麦克风权限，可在设置中开启。",
        confirmText: "去设置",
      });
      if (!answer.confirm) throw new Error("录音已取消，原来的文字已保留");
      const next = await Taro.openSetting();
      if (!next.authSetting["scope.record"]) throw new Error("请开启麦克风权限后重试");
      return;
    }
    try {
      await Taro.authorize({ scope: "scope.record" });
    } catch {
      throw new Error("请允许使用麦克风，也可以继续打字");
    }
  },
  start(events) {
    const recorder = getManager();
    if (capture) throw new Error("录音正在结束，请稍后再试");
    const owner: Capture = {
      events,
      cancelled: false,
      timer: setTimeout(() => {
        if (capture !== owner || owner.cancelled) return;
        owner.cancelled = true;
        events.onError(new Error("录音启动超时，请重试"));
        recorder.stop();
      }, 10000),
    };
    capture = owner;
    try {
      recorder.start({ duration: 59000, sampleRate: 16000, numberOfChannels: 1, encodeBitRate: 48000, format: "mp3" });
    } catch {
      capture = null;
      clearTimeout(owner.timer);
      throw new Error("录音启动失败，请重试");
    }
    return {
      stop() {
        if (capture === owner) recorder.stop();
      },
      cancel() {
        if (capture !== owner) return;
        owner.cancelled = true;
        clearTimeout(owner.timer);
        recorder.stop();
      },
    };
  },
  read(file) {
    return new Promise<string>((resolve, reject) => {
      const fail = () => reject(new Error("录音读取失败，请重新录制"));
      Taro.getFileSystemManager().readFile({
        filePath: file.path,
        encoding: "base64",
        success: (result) => {
          if (typeof result.data === "string") {
            resolve(result.data);
          } else {
            fail();
          }
        },
        fail,
      });
    });
  },
  remove,
};
