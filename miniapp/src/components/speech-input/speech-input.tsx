import { Button, Text, View } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { useEffect, useRef, useState } from "react";
import { speechRecorder } from "@/platform/speech-recorder";
import type { SpeechClient } from "@/product/speech-client";
import { SpeechInput, type SpeechStatus } from "@/product/speech-input";
import "./speech-input.scss";

type Props = {
  client: SpeechClient;
  draft: string;
  disabled: boolean;
  onDraft(value: string): void;
  onActive(active: boolean): void;
  onError(message: string): void;
};

export function SpeechInputButton(props: Props) {
  const latest = useRef(props);
  latest.current = props;
  const [enabled, setEnabled] = useState(false);
  const [status, setStatus] = useState<SpeechStatus>("idle");
  const [seconds, setSeconds] = useState(0);
  const input = useRef<SpeechInput | null>(null);

  useEffect(() => {
    let disposed = false;
    const controller = new SpeechInput({
      recorder: speechRecorder,
      transcribe: (audio) => props.client.transcribe(audio),
      getDraft: () => latest.current.draft,
      onDraft: (value) => latest.current.onDraft(value),
      onStatus: (next) => {
        setStatus(next);
        latest.current.onActive(next !== "idle");
      },
      onError: (message) => latest.current.onError(message),
    });
    input.current = controller;
    const refresh = () => {
      void props.client.capabilities()
        .then((next) => {
          if (!disposed) setEnabled(next.enabled);
        })
        .catch(() => {
          if (!disposed) setEnabled(false);
        });
    };
    const hide = () => controller.cancel();
    refresh();
    Taro.onAppHide(hide);
    Taro.onAppShow(refresh);
    return () => {
      disposed = true;
      controller.dispose();
      input.current = null;
      latest.current.onActive(false);
      Taro.offAppHide(hide);
      Taro.offAppShow(refresh);
    };
  }, [props.client]);

  useEffect(() => {
    if (props.disabled) input.current?.cancel();
  }, [props.disabled]);

  useEffect(() => {
    if (status !== "recording") return;
    setSeconds(0);
    const started = Date.now();
    const timer = setInterval(() => {
      const elapsed = Math.floor((Date.now() - started) / 1000);
      setSeconds(Math.min(59, elapsed));
    }, 250);
    return () => clearInterval(timer);
  }, [status]);

  if (!enabled && status === "idle") return null;

  const recording = status === "recording";
  let title = "准备录音";
  let detail = "请允许使用麦克风";
  if (recording) {
    title = "正在听你说";
    detail = `${seconds} 秒 / 最长约 60 秒`;
  } else if (status === "recognizing") {
    title = "正在转成文字";
    detail = "识别后可修改，再确认发送";
  }

  return (
    <View className="speechControl">
      <Button
        className="speechButton"
        disabled={props.disabled}
        ariaLabel="语音输入"
        onClick={() => {
          void Taro.hideKeyboard().catch(() => {});
          void input.current?.start();
        }}
      >
        <View className="speechMic" />
      </Button>
      {status !== "idle" && (
        <View className="speechOverlay" catchMove>
          <View className="speechDialog">
            <Text className="speechTitle">{title}</Text>
            <View className={`speechWave ${recording ? "speechWave--active" : ""}`}>
              {[0, 1, 2, 3, 4].map((i) => (
                <View key={i} className="speechWaveBar" style={{ animationDelay: `${i * 0.12}s` }} />
              ))}
            </View>
            <Text className="speechTime">{detail}</Text>
            <Text className="speechHint">录音由腾讯云转成文字，原来的输入会保留。</Text>
            <View className="speechActions">
              <Button className="speechCancel" onClick={() => input.current?.cancel()}>取消</Button>
              {recording && (
                <Button className="speechFinish" onClick={() => input.current?.stop()}>说完了</Button>
              )}
            </View>
          </View>
        </View>
      )}
    </View>
  );
}
