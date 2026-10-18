export type ChatPhoto = {
  id: string;
  path: string;
  fileID?: string;
  status: "uploading" | "ready" | "failed";
};

export type ChatPhotoSnapshot = {
  photos: readonly ChatPhoto[];
  choosing: boolean;
  limit: number;
};

export function chatPhotoLimit(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? Math.min(9, Math.max(1, Math.floor(value))) : 9;
}

type Dependencies = {
  limit: number;
  readLimit(): Promise<number>;
  choose(count: number): Promise<readonly string[]>;
  upload(path: string): Promise<string>;
  onError(message: string): void;
};

/** Keeps upload results tied to their original draft, even after deletion or navigation. */
export class ChatPhotos {
  private state: ChatPhotoSnapshot;
  private listeners = new Set<() => void>();
  private version = 0;
  private sequence = 0;
  private active = 0;
  private disposed = false;
  private queue: { id: string; version: number }[] = [];

  constructor(private readonly dependencies: Dependencies) {
    this.state = { photos: [], choosing: false, limit: chatPhotoLimit(dependencies.limit) };
  }

  snapshot = (): ChatPhotoSnapshot => this.state;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };

  private publish(next: ChatPhotoSnapshot) {
    if (this.disposed) return;
    this.state = next;
    for (const listener of this.listeners) listener();
  }

  choose = async (): Promise<void> => {
    if (this.disposed || this.state.choosing) return;
    const version = this.version;
    this.publish({ ...this.state, choosing: true });
    try {
      const configured = await this.dependencies.readLimit();
      if (this.disposed || version !== this.version) return;
      const limit = Math.min(chatPhotoLimit(this.dependencies.limit), chatPhotoLimit(configured));
      this.publish({ ...this.state, limit });
      const remaining = limit - this.state.photos.length;
      if (remaining <= 0) {
        this.dependencies.onError(`每条消息最多发送 ${limit} 张照片`);
        return;
      }
      const paths = await this.dependencies.choose(remaining);
      if (this.disposed || version !== this.version) return;
      const seen = new Set(this.state.photos.map((photo) => photo.path));
      const additions: ChatPhoto[] = [];
      for (const path of paths) {
        if (!path || seen.has(path) || this.state.photos.length + additions.length >= limit) continue;
        seen.add(path);
        additions.push({ id: `photo-${++this.sequence}`, path, status: "uploading" });
      }
      this.publish({ ...this.state, photos: [...this.state.photos, ...additions] });
      this.queue.push(...additions.map(({ id }) => ({ id, version })));
      this.pump();
    } catch (cause) {
      const detail = String((cause as { errMsg?: string })?.errMsg ?? "");
      if (!this.disposed && version === this.version && !/cancel/i.test(detail)) {
        this.dependencies.onError(cause instanceof Error ? cause.message : detail || "选择照片失败，请重试");
      }
    } finally {
      if (!this.disposed && version === this.version) this.publish({ ...this.state, choosing: false });
    }
  };

  remove = (id: string): void => {
    this.queue = this.queue.filter((job) => job.id !== id);
    this.publish({ ...this.state, photos: this.state.photos.filter((photo) => photo.id !== id) });
  };

  retry = async (id: string): Promise<void> => {
    if (this.disposed || !this.state.photos.some((photo) => photo.id === id && photo.status === "failed")) return;
    this.publish({ ...this.state, photos: this.state.photos.map((photo) => photo.id === id ? { ...photo, status: "uploading" } : photo) });
    this.queue.push({ id, version: this.version });
    this.pump();
  };

  clear = (): void => {
    this.version++;
    this.queue = [];
    this.publish({ ...this.state, photos: [], choosing: false });
  };

  dispose = (): void => {
    this.disposed = true;
    this.version++;
    this.queue = [];
    this.listeners.clear();
  };

  private pump() {
    while (!this.disposed && this.active < 2 && this.queue.length) {
      const job = this.queue.shift()!;
      const photo = this.state.photos.find((item) => item.id === job.id);
      if (!photo || job.version !== this.version) continue;
      this.active++;
      void this.upload(photo, job.version);
    }
  }

  private async upload(photo: ChatPhoto, version: number) {
    try {
      const fileID = (await this.dependencies.upload(photo.path)).trim();
      if (!fileID) throw new Error("图片上传结果为空");
      if (!this.disposed && version === this.version) {
        this.publish({ ...this.state, photos: this.state.photos.map((item) => item.id === photo.id ? { ...item, fileID, status: "ready" } : item) });
      }
    } catch {
      if (!this.disposed && version === this.version) {
        this.publish({ ...this.state, photos: this.state.photos.map((item) => item.id === photo.id ? { ...item, status: "failed" } : item) });
      }
    } finally {
      this.active--;
      this.pump();
    }
  }
}
