import Taro from "@tarojs/taro";
import { ChunkedJSONParser } from "./stream-parser";

export type StreamEvent = { type: string; seq?: number; [key: string]: unknown };
export type ChunkedRequestResult = { completedData: unknown; eventCount: number; firstChunkMs: number | null; lastSeq: number; statusCode: number };
export type ChunkedRequestHandle = { abort(): void; promise: Promise<ChunkedRequestResult> };
export type ChunkedRequestOptions = { accessToken: string; body: unknown; onEvent?(event: StreamEvent): void; timeoutMs?: number; url: string };

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

function streamError(event: StreamEvent): Error | null {
  if (event.type !== "error") {
    return null;
  }
  const errorCode = typeof event.errorCode === "string" ? ` / ${event.errorCode}` : "";
  return new Error(`${typeof event.message === "string" ? event.message : "stream failed"}${errorCode}`);
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
      const event = normalizeEvent(value);
      eventCount += 1;
      if (typeof event.seq === "number" && Number.isFinite(event.seq) && event.seq > lastSeq) {
        lastSeq = event.seq;
      }
      if (event.type === "completed") {
        completedData = event.data;
      }
      observedError = streamError(event) ?? observedError;
      options.onEvent?.(event);
    }
  };

  const promise = new Promise<ChunkedRequestResult>((resolve, reject) => {
    task = Taro.request({
      url: options.url,
      method: "POST",
      header: { Accept: "application/x-ndjson", Authorization: `Bearer ${options.accessToken}`, "Content-Type": "application/json" },
      data: options.body,
      dataType: "json",
      enableChunked: true,
      timeout: options.timeoutMs ?? 180_000,
      success(response) {
        if (settled) return;
        try {
          consume(parser.finish());
          settled = true;
          if (observedError) return reject(observedError);
          if (response.statusCode < 200 || response.statusCode >= 300) return reject(new Error(`stream request failed with HTTP ${response.statusCode}`));
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
          reject(new Error(error.errMsg || "chunked request failed"));
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
  return { abort() { if (!settled) task?.abort(); }, promise };
}
