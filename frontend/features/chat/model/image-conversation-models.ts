import type { ChatModelOption } from "@/features/chat/types/chat-runtime";

// This entry prefers image tasks without changing request options or cached model preferences.
// The canonical catalog (and ordinary chat) retains every advertised capability.
export function imageConversationModels(models: ChatModelOption[], imageEntry: boolean): ChatModelOption[] {
  if (!imageEntry) return models;
  return models.map((model) => model.kinds.includes("image_gen") ? {
    ...model,
    kinds: model.kinds.filter((kind) => kind !== "chat" && kind !== "audio" && kind !== "video_gen"),
  } : model);
}
