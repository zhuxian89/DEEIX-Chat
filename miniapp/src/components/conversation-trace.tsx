import { Text, View } from "@tarojs/components";
import { useEffect, useState, type ReactNode } from "react";
import {
  conversationToolCalls,
  conversationToolLabel,
  conversationToolStatusLabel,
  type ConversationProcessTrace,
  type ConversationToolCall,
  isConversationTraceActive,
} from "@/product/conversation-trace";
import { Markdown } from "./markdown";

type TraceDisclosureProps = {
  active: boolean;
  children: ReactNode;
  failed?: boolean;
  icon: string;
  summary: string;
  title: string;
};

function TraceDisclosure({ active, children, failed = false, icon, summary, title }: TraceDisclosureProps) {
  const [expanded, setExpanded] = useState(active);

  useEffect(() => {
    setExpanded(active);
  }, [active]);

  return (
    <View className={`traceDisclosure ${active ? "traceDisclosureActive" : ""} ${failed ? "traceDisclosureFailed" : ""}`}>
      <View className="traceDisclosureHeader" onClick={() => setExpanded((value) => !value)}>
        <View className="traceDisclosureTitleRow">
          <Text className="traceDisclosureIcon">{icon}</Text>
          <View className="traceDisclosureHeading">
            <Text className="traceDisclosureTitle">{title}</Text>
            {summary ? <Text className="traceDisclosureSummary">{summary}</Text> : null}
          </View>
        </View>
        {active ? <View className="traceLiveDot" /> : null}
        <Text className={`traceDisclosureChevron ${expanded ? "traceDisclosureChevronOpen" : ""}`}>⌄</Text>
      </View>
      {expanded ? <View className="traceDisclosureBody">{children}</View> : null}
    </View>
  );
}

function toolCallFailed(call: ConversationToolCall): boolean {
  const status = call.status.trim().toLowerCase();
  return status === "error" || status === "failed";
}

function ToolCallItem({ call }: { call: ConversationToolCall }) {
  const active = isConversationTraceActive(call.status);
  const failed = toolCallFailed(call);
  return (
    <View className={`traceToolCall ${active ? "traceToolCallActive" : ""} ${failed ? "traceToolCallFailed" : ""}`}>
      <View className="traceToolCallHeader">
        <Text className="traceToolCallName">{conversationToolLabel(call)}</Text>
        <Text className="traceToolCallStatus">{conversationToolStatusLabel(call.status)}</Text>
      </View>
      {call.input ? (
        <View className="traceToolDetail">
          <Text className="traceToolDetailLabel">请求</Text>
          <Text className="traceToolDetailText" selectable>{call.input}</Text>
        </View>
      ) : null}
      {call.output || call.error ? (
        <View className="traceToolDetail">
          <Text className="traceToolDetailLabel">{failed ? "错误" : "结果"}</Text>
          <Text className="traceToolDetailText" selectable>{call.error || call.output}</Text>
        </View>
      ) : null}
    </View>
  );
}

function blockFailed(status: string): boolean {
  const normalized = status.trim().toLowerCase();
  return normalized === "error" || normalized === "failed";
}

export function ConversationTrace({ trace, pending }: { trace?: ConversationProcessTrace; pending: boolean }) {
  if (!trace?.enabled) {
    return null;
  }

  const thinking = trace.upstreamThink;
  const toolBlock = trace.tools;
  const toolCalls = conversationToolCalls(trace);
  const hasThinking = Boolean(thinking?.contentMarkdown.trim() || thinking?.summary.trim());
  const hasTools = toolCalls.length > 0 || Boolean(toolBlock?.contentMarkdown.trim() || toolBlock?.summary.trim());
  if (!hasThinking && !hasTools) {
    return null;
  }

  const thinkingActive = Boolean(pending && thinking && isConversationTraceActive(thinking.status));
  const toolsActive = Boolean(pending && (
    toolCalls.some((call) => isConversationTraceActive(call.status)) ||
    (toolCalls.length === 0 && toolBlock && isConversationTraceActive(toolBlock.status))
  ));

  return (
    <View className="conversationTrace">
      {hasThinking && thinking ? (
        <TraceDisclosure
          active={thinkingActive}
          failed={blockFailed(thinking.status)}
          icon="✦"
          title={thinking.title || "思考过程"}
          summary={thinking.summary || (thinkingActive ? "正在思考" : "思考完成")}
        >
          {thinking.contentMarkdown.trim()
            ? <View className="traceThinkingContent"><Markdown>{thinking.contentMarkdown}</Markdown></View>
            : <Text className="traceEmptyText">模型正在组织思路…</Text>}
        </TraceDisclosure>
      ) : null}
      {hasTools && toolBlock ? (
        <TraceDisclosure
          active={toolsActive}
          failed={blockFailed(toolBlock.status) || toolCalls.some(toolCallFailed)}
          icon="◎"
          title="工具调用"
          summary={toolBlock.summary || (toolsActive ? "正在调用工具" : "工具调用完成")}
        >
          {toolCalls.length > 0
            ? toolCalls.map((call, index) => <ToolCallItem call={call} key={`${call.name}-${index}`} />)
            : toolBlock.contentMarkdown.trim()
              ? <View className="traceToolMarkdown"><Markdown>{toolBlock.contentMarkdown}</Markdown></View>
              : null}
        </TraceDisclosure>
      ) : null}
    </View>
  );
}
