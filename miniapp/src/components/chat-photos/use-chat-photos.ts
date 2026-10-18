import Taro from "@tarojs/taro";
import { useEffect, useMemo, useRef, useSyncExternalStore } from "react";
import { ChatPhotos } from "@/product/chat-photos";
import type { MiniAppSession } from "@/product/session";

export function useChatPhotos(session: MiniAppSession | null, options: {
  scope: string;
  limit: number;
  onError(message: string): void;
}) {
  const latest = useRef(options.onError);
  latest.current = options.onError;
  const active = useRef<ChatPhotos | null>(null);
  const controller = useMemo(() => new ChatPhotos({
    limit: options.limit,
    readLimit: async () => {
      if (!session) throw new Error("请先登录后选择照片");
      return session.getChatPhotoLimit();
    },
    choose: async (count) => {
      const selection = await Taro.chooseMedia({ count, mediaType: ["image"], sizeType: ["compressed"], sourceType: ["album", "camera"] });
      return selection.tempFiles.map((file) => file.tempFilePath);
    },
    upload: async (path) => {
      if (!session) throw new Error("请先登录后上传照片");
      return (await session.uploadChatImage(path, path.split(/[\\/]/).at(-1) || "photo.jpg")).fileID;
    },
    onError: (message) => { if (active.current === controller) latest.current(message); },
  }), [session, options.scope, options.limit]);
  const snapshot = useSyncExternalStore(controller.subscribe, controller.snapshot, controller.snapshot);
  useEffect(() => {
    active.current = controller;
    return () => {
      active.current = null;
      // React can immediately replay mount effects in development. Dispose only
      // after a real unmount or scope change, without reviving an old draft.
      void Promise.resolve().then(() => { if (active.current !== controller) controller.dispose(); });
    };
  }, [controller]);
  return {
    ...snapshot,
    uploading: snapshot.photos.some((photo) => photo.status === "uploading"),
    blocked: snapshot.choosing || snapshot.photos.length > snapshot.limit || snapshot.photos.some((photo) => photo.status !== "ready" || !photo.fileID),
    choose: controller.choose,
    remove: controller.remove,
    retry: controller.retry,
    clear: controller.clear,
    snapshot: controller.snapshot,
  };
}
