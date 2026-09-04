import type { ToolResponse } from "@deeix/api-contract";

export type NetworkToolSummary = Pick<ToolResponse, "id" | "name" | "status">;

const EXA_SEARCH_TOOL_NAME = "web_search_exa";
const EXA_NETWORK_TOOL_NAMES = new Set([EXA_SEARCH_TOOL_NAME, "web_fetch_exa"]);

function normalizedToolName(value: string): string {
  return value.trim().toLocaleLowerCase();
}

/** Resolves server-owned tool IDs only; credentials never enter the miniapp. */
export function resolveExaNetworkToolIDs(tools: readonly NetworkToolSummary[]): number[] {
  const selected: number[] = [];
  const seen = new Set<number>();
  let hasSearchTool = false;
  for (const tool of tools) {
    const name = normalizedToolName(tool.name);
    if (
      tool.status.trim().toLocaleLowerCase() !== "active" ||
      !EXA_NETWORK_TOOL_NAMES.has(name) ||
      !Number.isSafeInteger(tool.id) ||
      tool.id <= 0 ||
      seen.has(tool.id)
    ) {
      continue;
    }
    seen.add(tool.id);
    selected.push(tool.id);
    hasSearchTool ||= name === EXA_SEARCH_TOOL_NAME;
  }
  return hasSearchTool ? selected : [];
}

function isProviderNativeWebSearchTool(value: unknown): boolean {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const type = (value as Record<string, unknown>).type;
  if (typeof type !== "string") {
    return false;
  }
  const normalized = type.trim().toLocaleLowerCase();
  return normalized.includes("web_search") || normalized.startsWith("google_search");
}

/** Keeps model-native tools except search, avoiding duplicate searches when Exa is selected. */
export function removeNativeWebSearchOptions(
  options: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined {
  if (!options || !Array.isArray(options.tools)) {
    return options;
  }
  const tools = options.tools.filter((tool) => !isProviderNativeWebSearchTool(tool));
  if (tools.length === options.tools.length) {
    return options;
  }
  const next = { ...options };
  if (tools.length > 0) {
    next.tools = tools;
  } else {
    delete next.tools;
  }
  return Object.keys(next).length > 0 ? next : undefined;
}
