import { Button, Text, Textarea, View } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { SpeechHoldGesture, speechTouches, type SpeechGestureTarget } from "@/platform/speech-gesture";
import { speechRecorder } from "@/platform/speech-recorder";
import { composerKeyboardStyle } from "@/product/keyboard-layout";
import type { SpeechClient } from "@/product/speech-client";
import { SpeechInput, type SpeechStatus } from "@/product/speech-input";
import "./speech-input.scss";

type Props = {
  client?: SpeechClient;
  children: ReactNode;
  draft: string;
  disabled: boolean;
  onSend(text: string): Promise<boolean>;
  onActive(active: boolean): void;
  onError(message: string): void;
};

export function SpeechComposer(props: Props) {
  const latest = useRef(props);
  latest.current = props;
  const [enabled, setEnabled] = useState(false);
  const [voiceMode, setVoiceMode] = useState(false);
  const [status, setStatus] = useState<SpeechStatus>("idle");
  const [target, setTarget] = useState<SpeechGestureTarget>("record");
  const [seconds, setSeconds] = useState(0);
  const [preview, setPreview] = useState<string | null>(null);
  const [previewError, setPreviewError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [keyboard, setKeyboard] = useState(0);
  const previewRef = useRef<string | null>(null);
  const previewVersion = useRef(0);
  const submittingRef = useRef(false);
  const input = useRef<SpeechInput | null>(null);
  const hold = useRef<SpeechHoldGesture | null>(null);

  function updatePreview(value: string | null) {
    previewRef.current = value;
    previewVersion.current++;
    setPreview(value);
    setPreviewError("");
    setSubmitting(false);
    submittingRef.current = false;
    latest.current.onActive(value !== null || (input.current !== null && input.current.status !== "idle"));
  }

  function closePreview() {
    updatePreview(null);
    void Taro.hideKeyboard().catch(() => {});
  }

  useEffect(() => {
    const client = props.client;
    if (!client) {
      setEnabled(false);
      setVoiceMode(false);
      return;
    }
    let disposed = false;
    const controller = new SpeechInput({
      recorder: speechRecorder,
      transcribe: (audio) => client.transcribe(audio),
      getDraft: () => latest.current.draft,
      onDraft: (value) => updatePreview(value),
      onStatus: (next) => {
        if (next === "recognizing" && hold.current?.recordingStopped()) return;
        if (next === "idle") hold.current?.reset();
        setStatus(next);
        latest.current.onActive(next !== "idle" || previewRef.current !== null);
      },
      onError: (message) => latest.current.onError(message),
    });
    input.current = controller;
    const gesture = new SpeechHoldGesture(controller, (next) => {
      if (!disposed) setTarget(next);
    });
    hold.current = gesture;
    const refresh = () => {
      void client.capabilities().then((next) => {
        if (!disposed) setEnabled(next.enabled);
      }).catch(() => {
        if (!disposed) setEnabled(false);
      });
    };
    const hide = () => {
      gesture.cancel();
      updatePreview(null);
      setKeyboard(0);
    };
    const keyboardChanged = (event: { height: number }) => setKeyboard(Math.max(0, event.height));
    refresh();
    Taro.onAppHide(hide);
    Taro.onAppShow(refresh);
    Taro.onKeyboardHeightChange(keyboardChanged);
    return () => {
      disposed = true;
      gesture.reset();
      controller.dispose();
      input.current = null;
      hold.current = null;
      previewRef.current = null;
      previewVersion.current++;
      latest.current.onActive(false);
      Taro.offAppHide(hide);
      Taro.offAppShow(refresh);
      Taro.offKeyboardHeightChange(keyboardChanged);
    };
  }, [props.client]);

  useEffect(() => {
    if (props.disabled) {
      hold.current?.cancel();
      updatePreview(null);
    }
  }, [props.disabled]);

  useEffect(() => {
    if (status !== "recording") return;
    setSeconds(0);
    const started = Date.now();
    const timer = setInterval(() => setSeconds(Math.min(59, Math.floor((Date.now() - started) / 1000))), 250);
    return () => clearInterval(timer);
  }, [status]);

  async function sendPreview() {
    const text = previewRef.current?.trim();
    if (!text || props.disabled || submittingRef.current) return;
    const version = previewVersion.current;
    submittingRef.current = true;
    setSubmitting(true);
    void Taro.hideKeyboard().catch(() => {});
    try {
      const accepted = await latest.current.onSend(text);
      if (version !== previewVersion.current) return;
      if (accepted) {
        closePreview();
      } else {
        setPreviewError("暂时无法发送，请检查当前模型或图片要求");
      }
    } catch {
      if (version === previewVersion.current) setPreviewError("发送失败，文字已保留，请重试");
    } finally {
      if (version === previewVersion.current) {
        submittingRef.current = false;
        setSubmitting(false);
      }
    }
  }

  const recording = status === "recording";
  const recognizing = status === "recognizing";
  const showVoice = voiceMode && (enabled || status !== "idle" || preview !== null);
  let holdText = "按住 说话";
  let hint = "松手 转文字";
  if (status === "starting") {
    holdText = "正在准备…";
    hint = "准备录音，请允许麦克风权限";
  } else if (recognizing) {
    holdText = "正在转文字…";
    hint = "正在转文字…";
  } else if (recording) {
    holdText = target === "cancel" ? "松开 取消" : "松开 转文字";
    hint = target === "cancel" ? "松手 取消" : "松手 转文字";
  }

  return (
    <View className="speechComposer">
      {enabled && (
        <Button className="speechButton" disabled={props.disabled || status !== "idle" || preview !== null}
          ariaLabel={showVoice ? "切换键盘输入" : "切换语音输入"}
          onClick={() => { setVoiceMode(!showVoice); void Taro.hideKeyboard().catch(() => {}); }}>
          <View className={showVoice ? "speechKeyboard" : "speechMic"} />
        </Button>
      )}
      {showVoice ? (
        <View className={`speechHold ${status !== "idle" ? "speechHold--active" : ""} ${props.disabled ? "speechHold--disabled" : ""}`}
          ariaRole="button" ariaLabel="按住说话，松手转文字，左上滑取消" catchMove
          onTouchStart={(event) => {
            if (latest.current.disabled || previewRef.current !== null) return;
            hold.current?.begin(speechTouches(event)[0], Taro.getWindowInfo().windowWidth);
          }}
          onTouchMove={(event) => hold.current?.move(speechTouches(event))}
          onTouchEnd={(event) => hold.current?.release(speechTouches(event, true))}
          onTouchCancel={() => hold.current?.cancel()}>
          <Text>{holdText}</Text>
        </View>
      ) : props.children}
      {status !== "idle" && preview === null && (
        <View className={`speechOverlay ${recognizing ? "speechOverlay--recognizing" : ""}`} catchMove={recognizing}>
          <View className={`speechBubble speechBubble--${target}`}>
            <View className={`speechWave ${recording ? "speechWave--active" : ""}`}>
              {[12, 20, 32, 18, 42, 28, 54, 34, 64, 38, 48, 24, 36, 18, 28, 16, 12].map((height, i) => (
                <View key={i} className="speechWaveBar" style={{ height: `${height / 2}px`, animationDelay: `${i * 0.08}s` }} />
              ))}
            </View>
            <Text className="speechBubbleTime">{recording ? `${seconds}s` : ""}</Text>
          </View>
          <Text className="speechGestureHint">{hint}</Text>
          {recognizing ? (
            <Button className="speechRecognizingCancel" onClick={() => hold.current?.cancel()}>取消</Button>
          ) : (
            <>
              <View className="speechGestureActions">
                <View className={`speechGestureCancel ${target === "cancel" ? "speechGestureCancel--active" : ""}`}><Text>取消</Text></View>
                <View className={`speechGestureText ${target === "text" ? "speechGestureText--active" : ""}`}><Text>滑到这里 转文字</Text></View>
              </View>
              <View className="speechHoldSurface" />
            </>
          )}
        </View>
      )}
      {preview !== null && (
        <View className="speechPreviewOverlay" catchMove style={composerKeyboardStyle(keyboard)}>
          <View className="speechPreviewSheet">
            <Text className="speechPreviewTitle">转文字</Text>
            <Textarea className="speechPreviewInput" value={preview} maxlength={8000} disabled={submitting}
              adjustPosition={false} showConfirmBar={false} placeholder="可以修改识别结果…"
              onInput={(event) => { previewRef.current = event.detail.value; setPreview(event.detail.value); setPreviewError(""); }} />
            {previewError && <Text className="speechPreviewError">{previewError}</Text>}
            <View className="speechPreviewActions">
              <Button className="speechPreviewCancel" disabled={submitting} onClick={closePreview}>取消</Button>
              <Button className="speechPreviewSend" disabled={props.disabled || submitting || !preview.trim()} onClick={() => void sendPreview()}>{submitting ? "发送中…" : "发送"}</Button>
            </View>
          </View>
        </View>
      )}
    </View>
  );
}
