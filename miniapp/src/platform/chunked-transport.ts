import Taro from "@tarojs/taro";
import { WechatRequestError, type RecoverableStreamHandle } from "./generation-recovery";
import { ChunkedJSONParser } from "./stream-parser";
import { classifyStreamTerminal } from "./stream-terminal";

export type StreamEvent = { type: string; seq?: number; [key: string]: unknown };
export type ChunkedRequestResult = { completedData: unknown; eventCount: number; firstChunkMs: number | null; lastSeq: number; statusCode: number };
export type ChunkedRequestHandle = RecoverableStreamHandle<ChunkedRequestResult>;
export type ChunkedRequestOptions = {
  accessToken: string;
  body?: unknown;
  method?: "GET" | "POST";
  onEvent?(event: StreamEvent): void;
  timeoutMs?: number;
  url: string;
};

function normalizeEvent(value: unknown): StreamEvent {
  if (!value || typeof value !== "object") {
    throw new Error("stream event must be a JSON object");
  }
  const event = value as Record<string, unknown>;
  if (typeof event.type !== "string" || !event.type.trim()) {
    throw new Error("stream event type is missing");
  }
  return event as StreamEvent;
}

function responseEnvelopeError(value: unknown): Error | null {
  try {
    if (typeof value === "string") {
      value = JSON.parse(value);
    } else if (value instanceof ArrayBuffer) {
      const parser = new ChunkedJSONParser();
      value = [...parser.push(value), ...parser.finish()][0];
    }
  } catch {
    return null;
  }
  if (!value || typeof value !== "object") return null;
  const envelope = value as Record<string, unknown>;
  if (typeof envelope.errorMsg !== "string" || !envelope.errorMsg.trim()) return null;
  const code = typeof envelope.errorCode === "string" && envelope.errorCode.trim()
    ? `（${envelope.errorCode.trim()}）` : "";
  return new Error(`${envelope.errorMsg.trim()}${code}`);
}

export function startChunkedJSONRequest(options: ChunkedRequestOptions): ChunkedRequestHandle {
  const parser = new ChunkedJSONParser();
  const startedAt = Date.now();
  let firstChunkMs: number | null = null;
  let eventCount = 0;
  let lastSeq = 0;
  let completedData: unknown;
  let observedError: Error | null = null;
  let settled = false;
  let task: Taro.RequestTask<unknown> | null = null;

  const consume = (values: unknown[]) => {
    for (const value of values) {
      const envelopeError = responseEnvelopeError(value);
      if (envelopeError) {
        observedError = envelopeError;
        continue;
      }
      const event = normalizeEvent(value);
      eventCount += 1;
      if (typeof event.seq === "number" && Number.isFinite(event.seq) && event.seq > lastSeq) {
        lastSeq = event.seq;
      }
      const terminal = classifyStreamTerminal(event);
      if (terminal.kind === "completed") {
        completedData = terminal.data;
      } else if (terminal.kind === "error") {
        observedError = terminal.error;
      }
      options.onEvent?.(event);
    }
  };

  const promise = new Promise<ChunkedRequestResult>((resolve, reject) => {
    task = Taro.request({
      url: options.url,
      method: options.method ?? "POST",
      header: { Accept: "application/x-ndjson", Authorization: `Bearer ${options.accessToken}`, "Content-Type": "application/json" },
      data: options.body,
      dataType: "json",
      enableChunked: true,
      timeout: options.timeoutMs ?? 180_000,
      success(response) {
        if (settled) return;
        try {
          if (response.statusCode < 200 || response.statusCode >= 300) {
            settled = true;
            return reject(observedError ?? responseEnvelopeError(response.data)
              ?? new Error(`请求失败（HTTP ${response.statusCode}）`));
          }
          consume(parser.finish());
          settled = true;
          if (observedError) return reject(observedError);
          if (typeof completedData === "undefined") return reject(new Error("stream completed without a completed event"));
          resolve({ completedData, eventCount, firstChunkMs, lastSeq, statusCode: response.statusCode });
        } catch (error) {
          settled = true;
          reject(error);
        }
      },
      fail(error) {
        if (!settled) {
          settled = true;
          const errno = Number((error as { errno?: unknown }).errno);
          reject(new WechatRequestError(
            error.errMsg || "chunked request failed",
            Number.isFinite(errno) ? errno : undefined,
          ));
        }
      },
    });
    task.onChunkReceived(({ data }: { data: ArrayBuffer }) => {
      if (settled) return;
      try {
        firstChunkMs ??= Date.now() - startedAt;
        consume(parser.push(data));
      } catch (error) {
        settled = true;
        task?.abort();
        reject(error);
      }
    });
  });
  return {
    abort() { if (!settled) task?.abort(); },
    lastSeq: () => lastSeq,
    promise,
  };
}
