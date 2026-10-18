import { messageImages, type ConversationMessage } from "./message-timeline";

/** Load history in the background without exhausting the native download queue. */
export async function downloadMessagePhotos(
  messages: readonly ConversationMessage[],
  download: (fileID: string) => Promise<string | null>,
  onResult: (messageID: string, fileID: string, source: string | null) => void,
  active: () => boolean = () => true,
): Promise<void> {
  const pending = messages.flatMap((message) => messageImages(message)
    .flatMap((photo) => photo.fileID && !photo.source ? [{ messageID: message.id, fileID: photo.fileID }] : []));
  let cursor = 0;
  const worker = async () => {
    while (active()) {
      const photo = pending[cursor++];
      if (!photo) return;
      const source = await download(photo.fileID).catch(() => null);
      if (!active()) return;
      onResult(photo.messageID, photo.fileID, source);
    }
  };
  await Promise.all(Array.from({ length: Math.min(3, pending.length) }, worker));
}
