import { Image, ScrollView, Text, View } from "@tarojs/components";
import Taro from "@tarojs/taro";
import type { ChatPhoto } from "@/product/chat-photos";
import "./photo-tray.scss";

export function PhotoTray({ photos, limit, disabled, tone = "chat", onAdd, onRemove, onRetry }: {
  photos: readonly ChatPhoto[];
  limit: number;
  disabled?: boolean;
  tone?: "chat" | "companion";
  onAdd(): void;
  onRemove(id: string): void;
  onRetry(id: string): void;
}) {
  if (!photos.length) return null;
  const failed = photos.filter((photo) => photo.status === "failed").length;
  const uploading = photos.filter((photo) => photo.status === "uploading").length;
  return <View className={`photoTray photoTray--${tone}`}>
    <View className="photoTrayHeader">
      <Text>已选 {photos.length} 张照片</Text>
      <Text>{photos.length < limit ? `最多 ${limit} 张` : "已达数量上限"}</Text>
    </View>
    <ScrollView scrollX className="photoTrayScroll">
      <View className="photoTrayItems">
        {photos.map((photo, index) => <View key={photo.id} className={`photoTrayItem ${photo.status === "failed" ? "photoTrayItem--failed" : ""}`}>
          <Image className="photoTrayImage" src={photo.path} mode="aspectFill" onClick={() => void Taro.previewImage({ current: photo.path, urls: photos.map((item) => item.path) }).catch(() => { void Taro.showToast({ title: "照片预览失败，请重试", icon: "none" }); })} />
          <Text className="photoTrayNumber">{index + 1}</Text>
          {photo.status !== "ready" && <View className="photoTrayStatus" onClick={() => { if (!disabled && photo.status === "failed") onRetry(photo.id); }}>
            <Text>{photo.status === "failed" ? "点击重试" : "上传中…"}</Text>
          </View>}
          {!disabled && <View className="photoTrayRemove" onClick={(event) => { event.stopPropagation(); onRemove(photo.id); }}><Text>×</Text></View>}
        </View>)}
        {!disabled && photos.length < limit && <View className="photoTrayAdd" onClick={onAdd}><Text className="photoTrayPlus">+</Text><Text>添加</Text></View>}
      </View>
    </ScrollView>
    {photos.length > limit ? <Text className="photoTrayError">每条消息最多 {limit} 张，请移除多余照片后发送</Text>
      : failed > 0 ? <Text className="photoTrayError">{failed} 张上传失败，点照片重试或移除后发送</Text>
        : uploading > 0 ? <Text className="photoTrayHint">正在上传 {uploading} 张照片…</Text> : null}
  </View>;
}
