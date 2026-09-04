export type ConversationTraceBlock = {
  contentMarkdown: string;
  payloadJSON?: string;
  roundID?: string;
  stage?: string;
  status: string;
  summary: string;
  title: string;
};

export type ConversationProcessTrace = {
  enabled: boolean;
  process?: ConversationTraceBlock;
  status: string;
  tools?: ConversationTraceBlock;
  upstreamThink?: ConversationTraceBlock;
};

export type ConversationToolCall = {
  error?: string;
  input?: string;
  name: string;
  output?: string;
  status: string;
  type?: string;
};

type UnknownRecord = Record<string, unknown>;

function asRecord(value: unknown): UnknownRecord | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as UnknownRecord
    : null;
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function optionalText(value: unknown): string | undefined {
  const normalized = text(value).trim();
  return normalized || undefined;
}

function normalizeTraceBlock(value: unknown): ConversationTraceBlock | undefined {
  const raw = asRecord(value);
  if (!raw) {
    return undefined;
  }
  const block: ConversationTraceBlock = {
    contentMarkdown: text(raw.contentMarkdown),
    payloadJSON: optionalText(raw.payloadJSON),
    roundID: optionalText(raw.roundID),
    stage: optionalText(raw.stage),
    status: text(raw.status).trim(),
    summary: text(raw.summary).trim(),
    title: text(raw.title).trim(),
  };
  return block.contentMarkdown || block.payloadJSON || block.summary || block.title
    ? block
    : undefined;
}

export function normalizeConversationProcessTrace(value: unknown): ConversationProcessTrace | undefined {
  const raw = asRecord(value);
  if (!raw) {
    return undefined;
  }
  const process = normalizeTraceBlock(raw.process);
  const tools = normalizeTraceBlock(raw.tools);
  const upstreamThink = normalizeTraceBlock(raw.upstreamThink);
  if (raw.enabled !== true && !process && !tools && !upstreamThink) {
    return undefined;
  }
  return {
    enabled: raw.enabled === true || Boolean(process || tools || upstreamThink),
    process,
    status: text(raw.status).trim(),
    tools,
    upstreamThink,
  };
}

function mergedTrace(
  current: ConversationProcessTrace | undefined,
  next: ConversationProcessTrace,
): ConversationProcessTrace {
  return {
    enabled: current?.enabled === true || next.enabled,
    process: next.process ?? current?.process,
    status: next.status || current?.status || "streaming",
    tools: next.tools ?? current?.tools,
    upstreamThink: next.upstreamThink ?? current?.upstreamThink,
  };
}

export function applyProcessUpdateEvent(
  current: ConversationProcessTrace | undefined,
  event: UnknownRecord,
): ConversationProcessTrace | undefined {
  const trace = normalizeConversationProcessTrace(event.trace);
  if (trace) {
    return mergedTrace(current, trace);
  }
  const block = normalizeTraceBlock(event.block);
  if (!block) {
    return current;
  }
  const stage = block.stage?.toLowerCase();
  const partial: ConversationProcessTrace = {
    enabled: true,
    status: text(event.status).trim() || block.status || "streaming",
    ...(stage === "tool" ? { tools: block } : { process: block }),
  };
  return mergedTrace(current, partial);
}

export function applyUpstreamThinkEvent(
  current: ConversationProcessTrace | undefined,
  event: UnknownRecord,
): ConversationProcessTrace {
  const fromTrace = normalizeConversationProcessTrace(event.trace);
  const base = fromTrace ? mergedTrace(current, fromTrace) : current;
  const previous = base?.upstreamThink;
  const roundID = optionalText(event.roundID) ?? previous?.roundID;
  const changedRound = Boolean(roundID && previous?.roundID && roundID !== previous.roundID);
  const previousContent = changedRound ? "" : previous?.contentMarkdown ?? "";
  const contentMarkdown = typeof event.contentMarkdown === "string"
    ? event.contentMarkdown
    : `${previousContent}${text(event.delta)}`;
  const upstreamThink: ConversationTraceBlock = {
    contentMarkdown,
    payloadJSON: previous?.payloadJSON,
    roundID,
    stage: optionalText(event.stage) ?? previous?.stage ?? "think",
    status: text(event.status).trim() || previous?.status || "streaming",
    summary: text(event.summary).trim() || previous?.summary || "",
    title: text(event.title).trim() || previous?.title || "模型思考",
  };
  return {
    enabled: true,
    process: base?.process,
    status: upstreamThink.status || base?.status || "streaming",
    tools: base?.tools,
    upstreamThink,
  };
}

export function conversationToolCalls(trace: ConversationProcessTrace | undefined): ConversationToolCall[] {
  const payloadJSON = trace?.tools?.payloadJSON;
  if (!payloadJSON) {
    return [];
  }
  try {
    const payload = asRecord(JSON.parse(payloadJSON));
    const calls = payload?.tool_calls;
    if (!Array.isArray(calls)) {
      return [];
    }
    return calls.flatMap((value): ConversationToolCall[] => {
      const call = asRecord(value);
      if (!call) {
        return [];
      }
      const name = text(call.name).trim() || text(call.type).trim();
      if (!name) {
        return [];
      }
      return [{
        error: optionalText(call.error),
        input: optionalText(call.input_detail) ?? optionalText(call.input_preview) ?? optionalText(call.input),
        name,
        output: optionalText(call.output_detail) ?? optionalText(call.output_preview) ?? optionalText(call.output_text) ?? optionalText(call.output),
        status: text(call.status).trim(),
        type: optionalText(call.type),
      }];
    });
  } catch {
    return [];
  }
}

function normalizedStatus(status: string): string {
  return status.trim().toLowerCase();
}

export function isConversationTraceActive(status: string): boolean {
  return ["requested", "streaming", "queued", "in_progress", "searching", "pending"].includes(
    normalizedStatus(status),
  );
}

export function isExaNetworkTool(name: string): boolean {
  const normalized = name.trim().toLowerCase();
  return normalized === "web_search_exa" || normalized === "web_fetch_exa" ||
    normalized.startsWith("web_search_exa_") || normalized.startsWith("web_fetch_exa_");
}

export function resolveConversationActivity(
  trace: ConversationProcessTrace | undefined,
  currentText: string,
): string {
  const calls = conversationToolCalls(trace);
  if (calls.some((call) => isExaNetworkTool(call.name) && isConversationTraceActive(call.status))) {
    return "正在搜索互联网…";
  }
  if (calls.some((call) => !isExaNetworkTool(call.name) && isConversationTraceActive(call.status))) {
    return "正在调用工具…";
  }
  const usedExa = calls.some((call) => isExaNetworkTool(call.name) &&
    ["success", "completed", "reused"].includes(normalizedStatus(call.status)));
  if (usedExa && !currentText.trim()) {
    return "正在整理搜索结果…";
  }
  if (currentText.trim()) {
    return "正在生成回复…";
  }
  if (trace?.upstreamThink && isConversationTraceActive(trace.upstreamThink.status)) {
    return "正在思考…";
  }
  return "正在思考…";
}

export function conversationToolLabel(call: ConversationToolCall): string {
  const normalized = call.name.trim().toLowerCase();
  if (normalized.startsWith("web_search_exa")) {
    return "联网搜索";
  }
  if (normalized.startsWith("web_fetch_exa")) {
    return "读取网页";
  }
  return call.name;
}

export function conversationToolStatusLabel(status: string): string {
  switch (normalizedStatus(status)) {
    case "requested":
    case "streaming":
    case "queued":
    case "in_progress":
    case "searching":
    case "pending":
      return "进行中";
    case "success":
    case "completed":
      return "已完成";
    case "reused":
      return "已复用";
    case "error":
    case "failed":
      return "失败";
    default:
      return status.trim();
  }
}
