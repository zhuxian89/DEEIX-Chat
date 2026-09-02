import { Button, RichText, Text, View } from "@tarojs/components";
import Taro from "@tarojs/taro";

function escapeHTML(value: string): string {
  return value.replace(/&/gu, "&amp;").replace(/</gu, "&lt;").replace(/>/gu, "&gt;");
}

function simpleMarkdown(value: string): string {
  let html = escapeHTML(value);
  html = html.replace(/`([^`\n]+)`/gu, '<code class="inline-code">$1</code>');
  html = html.replace(/\*\*([^*]+)\*\*/gu, "<strong>$1</strong>");
  html = html.replace(/^###\s+(.+)$/gmu, "<h3>$1</h3>");
  html = html.replace(/^##\s+(.+)$/gmu, "<h2>$1</h2>");
  html = html.replace(/^#\s+(.+)$/gmu, "<h1>$1</h1>");
  return html.replace(/\n/gu, "<br/>");
}

export function Markdown({ children }: { children: string }) {
  const parts = children.split(/```([^\n]*)\n([\s\S]*?)```/gu);
  if (parts.length === 1) {
    return <RichText nodes={simpleMarkdown(children)} />;
  }
  const nodes = [];
  for (let index = 0; index < parts.length; index += 3) {
    const prose = parts[index] ?? "";
    const language = parts[index + 1]?.trim() ?? "";
    const code = parts[index + 2];
    if (prose) {
      nodes.push(<RichText key={`prose-${index}`} nodes={simpleMarkdown(prose)} />);
    }
    if (typeof code === "string") {
      nodes.push(
        <View className="codeBlock" key={`code-${index}`}>
          <View className="codeHeader">
            <Text>{language || "代码"}</Text>
            <Button className="codeCopy" onClick={() => Taro.setClipboardData({ data: code })}>复制</Button>
          </View>
          <Text className="codeContent" userSelect>{code}</Text>
        </View>,
      );
    }
  }
  return <View>{nodes}</View>;
}
