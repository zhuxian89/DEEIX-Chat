import { Text } from "@tarojs/components";
import { formatMessageTime } from "@/product/message-time";
import "./message-time.scss";

export function MessageTime({ createdAt }: { createdAt?: string }) {
  const label = formatMessageTime(createdAt);
  return label ? <Text className="messageTime">{label}</Text> : null;
}
