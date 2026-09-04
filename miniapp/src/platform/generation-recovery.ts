export type RecoverableStreamHandle<T> = {
  abort(): void;
  lastSeq(): number;
  promise: Promise<T>;
};

export type RecoverableStreamOptions<T> = {
  isCanceled?(): boolean;
  maxResumeAttempts?: number;
  onHandle?(handle: RecoverableStreamHandle<T>): void;
  onInterrupted?(): void;
  onResuming?(): void;
  resume(afterSeq: number): Promise<RecoverableStreamHandle<T>> | RecoverableStreamHandle<T>;
  shouldResume(error: unknown): boolean;
  start(): Promise<RecoverableStreamHandle<T>> | RecoverableStreamHandle<T>;
  waitUntilResume(): Promise<void>;
};

export class WechatRequestError extends Error {
  readonly errno?: number;

  constructor(message: string, errno?: number) {
    super(message);
    this.name = "WechatRequestError";
    this.errno = errno;
  }
}

export function isWechatRequestInterrupted(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const candidate = error as { errno?: unknown; errMsg?: unknown; message?: unknown };
  if (Number(candidate.errno) === 600003) {
    return true;
  }
  const message = typeof candidate.errMsg === "string"
    ? candidate.errMsg
    : typeof candidate.message === "string"
      ? candidate.message
      : "";
  return message.toLowerCase().includes("request:fail interrupted");
}

export async function runRecoverableStream<T>(options: RecoverableStreamOptions<T>): Promise<T> {
  const maxResumeAttempts = options.maxResumeAttempts ?? 5;
  let resumeAttempts = 0;
  let greatestSeq = 0;
  let handle = await options.start();
  options.onHandle?.(handle);

  while (true) {
    try {
      return await handle.promise;
    } catch (error) {
      greatestSeq = Math.max(greatestSeq, handle.lastSeq());
      if (options.isCanceled?.() || !options.shouldResume(error) || resumeAttempts >= maxResumeAttempts) {
        throw error;
      }

      options.onInterrupted?.();
      await options.waitUntilResume();
      if (options.isCanceled?.()) {
        throw error;
      }

      options.onResuming?.();
      resumeAttempts += 1;
      handle = await options.resume(greatestSeq);
      if (options.isCanceled?.()) {
        handle.abort();
        throw error;
      }
      options.onHandle?.(handle);
    }
  }
}

let clientRunSequence = 0;

export function createClientRunID(): string {
  clientRunSequence = (clientRunSequence + 1) % 0x1000000;
  const timestamp = Date.now().toString(36);
  const sequence = clientRunSequence.toString(36).padStart(5, "0");
  const random = Array.from(
    { length: 4 },
    () => Math.floor(Math.random() * 0x100000000).toString(36).padStart(7, "0"),
  ).join("");
  return `run_${timestamp}_${sequence}_${random}`.slice(0, 64);
}
