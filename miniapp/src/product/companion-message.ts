const TIME_MARKER = /\[历史消息时间：北京时间 \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\][ \t]*(?:\r?\n)?/gu;
const TIME_MARKER_TEMPLATE = "[历史消息时间：北京时间 0000-00-00 00:00:00]";
const TIME_MARKER_PREFIX = "[历史消息时间：北京时间 ";

// Presentation only: keep raw stream state and saved messages intact.
export function companionDisplayText({ text, role, pending }: Pick<ConversationMessage, "text" | "role" | "pending">): string {
  if (role !== "assistant") return text;
  const clean = text.replace(TIME_MARKER, "");
  const start = clean.lastIndexOf("[");
  if (start < 0) return clean;
  const tail = clean.slice(start);
  if (!pending && !tail.startsWith(TIME_MARKER_PREFIX)) return clean;
  const incompleteMarker = tail.length < TIME_MARKER_TEMPLATE.length && [...tail].every((char, index) =>
    TIME_MARKER_TEMPLATE[index] === "0" ? /[0-9]/u.test(char) : char === TIME_MARKER_TEMPLATE[index]);
  return incompleteMarker ? clean.slice(0, start) : clean;
}
import type { ConversationMessage } from "./message-timeline";
