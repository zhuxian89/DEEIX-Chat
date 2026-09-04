import { View } from "@tarojs/components";

export function Markdown({ children }: { children: string }) {
  return (
    <View className="markdownRenderer">
      <mp-html
        content={children}
        markdown
        selectable="force"
        scrollTable
        copyLink
        previewImg
        showImgMenu
        setTitle={false}
      />
    </View>
  );
}
