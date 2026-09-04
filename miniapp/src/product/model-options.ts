import type { PublicModelResponse } from "@deeix/api-contract";

export type ConversationOptions = Record<string, unknown>;

export type NativeToolDefinition = {
  protocol: string;
  provider: string;
  type: string;
  toolKey: string;
  label: string;
  description: string;
  payload: Record<string, unknown>;
  defaultEnabled: boolean;
  billable: boolean;
  billingUnit: string;
  priceNanousd: number;
  priceLabel: string;
  riskLevel: string;
  usageAliases: string[];
};

export type ModelOptionPolicyResponse = {
  mode: string;
  allowedPathsJSON: string;
  deniedPathsJSON: string;
  nativeTools?: NativeToolDefinition[];
};

type ModelNativeToolConfig = {
  key: string;
  protocol: string;
  protocols: string[];
  provider?: string;
  type: string;
  label: string;
  description?: string;
  enabled: boolean;
  defaultEnabled: boolean;
  payload: Record<string, unknown>;
};

const RESERVED_CONVERSATION_OPTION_KEYS = new Set([
  "contents",
  "instructions",
  "input",
  "messages",
  "model",
  "prompt",
  "stream",
  "system",
  "systemInstruction",
]);

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function parseJSONObject(raw: string): Record<string, unknown> | null {
  const normalized = raw.trim();
  if (!normalized) {
    return null;
  }
  try {
    const parsed = JSON.parse(normalized) as unknown;
    return isPlainObject(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function sanitizeConversationOptions(options: ConversationOptions): ConversationOptions {
  return Object.fromEntries(
    Object.entries(options).filter(([key]) => !RESERVED_CONVERSATION_OPTION_KEYS.has(key)),
  );
}

function normalizeString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function normalizeStrings(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return Array.from(new Set(value.map(normalizeString).filter(Boolean)));
}

function parseProtocolsJSON(raw: string): string[] {
  try {
    return normalizeStrings(JSON.parse(raw) as unknown);
  } catch {
    return [];
  }
}

function resolveModelOptionPolicyProtocol(protocol: string): string {
  switch (protocol.trim().toLowerCase()) {
    case "openai":
    case "openai_responses":
      return "openai_responses";
    case "openrouter_chat_completions":
      return "openrouter_chat_completions";
    case "openrouter":
    case "openrouter_responses":
      return "openrouter_responses";
    case "openai_chat_completions":
      return "openai_chat_completions";
    case "openai_image_generations":
      return "openai_image_generations";
    case "openai_image_edits":
      return "openai_image_edits";
    case "anthropic":
    case "claude":
    case "anthropic_messages":
      return "anthropic_messages";
    case "xai":
    case "grok":
    case "xai_responses":
      return "xai_responses";
    case "xai_image":
      return "xai_image";
    case "xai_image_edits":
      return "xai_image_edits";
    case "xai_video":
      return "xai_video";
    case "xai_video_extensions":
      return "xai_video_extensions";
    case "google":
    case "gemini":
    case "google_generate_content":
    case "gemini_generate_content":
      return "gemini_generate_content";
    case "google_image_generation":
      return "google_image_generation";
    case "gemini_interactions":
      return "gemini_interactions";
    default:
      return "openai_responses";
  }
}

function nativeToolPayloadSignature(value: unknown): string {
  if (Array.isArray(value)) {
    return `[${value.map(nativeToolPayloadSignature).join(",")}]`;
  }
  if (isPlainObject(value)) {
    return `{${Object.keys(value)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${nativeToolPayloadSignature(value[key])}`)
      .join(",")}}`;
  }
  return JSON.stringify(value) ?? String(value);
}

function resolveNativeTools(raw: string): ModelNativeToolConfig[] {
  const parsed = parseJSONObject(raw);
  if (!parsed) {
    return [];
  }
  const rawTools = parsed.nativeTools;
  if (Array.isArray(rawTools)) {
    return rawTools.flatMap((item): ModelNativeToolConfig[] => {
      if (!isPlainObject(item)) {
        return [];
      }
      const key = normalizeString(item.key ?? item.toolKey);
      const payload = isPlainObject(item.payload) ? item.payload : {};
      const type = normalizeString(item.type) || normalizeString(payload.type);
      const protocol = normalizeString(item.protocol);
      const protocols = normalizeStrings(item.protocols);
      if (!key && !type && Object.keys(payload).length === 0) {
        return [];
      }
      return [{
        key,
        protocol,
        protocols: protocols.length > 0 ? protocols : (protocol ? [protocol] : []),
        provider: normalizeString(item.provider) || undefined,
        type,
        label: normalizeString(item.label) || type || key,
        description: normalizeString(item.description) || undefined,
        enabled: item.enabled !== false,
        defaultEnabled: item.defaultEnabled === true,
        payload,
      }];
    }).filter((item) => item.enabled);
  }
  return normalizeStrings(parsed.nativeToolKeys).map((key) => ({
    key,
    protocol: "",
    protocols: [],
    type: "",
    label: key,
    enabled: true,
    defaultEnabled: false,
    payload: {},
  }));
}

function nativeToolPayloadMatchesShape(
  configuredPayload: Record<string, unknown>,
  catalogPayload: Record<string, unknown>,
): boolean {
  const catalogType = normalizeString(catalogPayload.type);
  if (catalogType) {
    return normalizeString(configuredPayload.type) === catalogType;
  }
  const identityKeys = Object.keys(catalogPayload).filter((key) => key !== "type");
  return identityKeys.length > 0
    && identityKeys.some((key) => Object.prototype.hasOwnProperty.call(configuredPayload, key));
}

function nativeToolDefinitionFromConfig(
  config: ModelNativeToolConfig,
  catalog: readonly NativeToolDefinition[],
  modelProtocols: readonly string[],
): NativeToolDefinition | null {
  const key = config.key.trim();
  const protocols = config.protocols.length > 0
    ? config.protocols
    : (config.protocol.trim() ? [config.protocol.trim()] : []);
  const type = config.type.trim() || normalizeString(config.payload.type);
  const configuredProtocolSet = new Set(protocols.map(resolveModelOptionPolicyProtocol).filter(Boolean));
  const modelProtocolSet = new Set(modelProtocols.map(resolveModelOptionPolicyProtocol).filter(Boolean));
  const matched = (key && configuredProtocolSet.size > 0
    ? catalog.find((tool) => tool.toolKey === key && configuredProtocolSet.has(resolveModelOptionPolicyProtocol(tool.protocol)))
    : undefined)
    ?? (key && modelProtocolSet.size > 0
      ? catalog.find((tool) => tool.toolKey === key && modelProtocolSet.has(resolveModelOptionPolicyProtocol(tool.protocol)))
      : undefined)
    ?? catalog.find((tool) => tool.toolKey === key)
    ?? (type && configuredProtocolSet.size > 0
      ? catalog.find((tool) => tool.type === type && configuredProtocolSet.has(resolveModelOptionPolicyProtocol(tool.protocol)))
      : undefined)
    ?? (type && modelProtocolSet.size > 0
      ? catalog.find((tool) => tool.type === type && modelProtocolSet.has(resolveModelOptionPolicyProtocol(tool.protocol)))
      : undefined)
    ?? (type ? catalog.find((tool) => tool.type === type) : undefined);
  if (!matched && !key && !type && Object.keys(config.payload).length === 0) {
    return null;
  }
  return {
    protocol: matched?.protocol || protocols[0] || "",
    provider: config.provider || matched?.provider || "Provider",
    type: type || matched?.type || key,
    toolKey: key || matched?.toolKey || type,
    label: config.label || matched?.label || type || key,
    description: config.description || matched?.description || type || key,
    payload: Object.keys(config.payload).length > 0 ? config.payload : (matched?.payload ?? {}),
    defaultEnabled: config.defaultEnabled,
    billable: matched?.billable ?? false,
    billingUnit: matched?.billingUnit ?? "",
    priceNanousd: matched?.priceNanousd ?? 0,
    priceLabel: matched?.priceLabel ?? "",
    riskLevel: matched?.riskLevel ?? "",
    usageAliases: matched?.usageAliases ?? [],
  };
}

function nativeToolDefinitionVariantsFromConfig(
  config: ModelNativeToolConfig,
  catalog: readonly NativeToolDefinition[],
  modelProtocols: readonly string[],
): NativeToolDefinition[] {
  const base = nativeToolDefinitionFromConfig(config, catalog, modelProtocols);
  if (!base) {
    return [];
  }
  const matchingDefinitions = catalog.filter((tool) => tool.toolKey === base.toolKey);
  const explicitProtocols = config.protocols.length > 0
    ? config.protocols
    : [config.protocol].filter(Boolean);
  const modelProtocolSet = new Set(modelProtocols.map(resolveModelOptionPolicyProtocol));
  const inferredProtocols = matchingDefinitions
    .filter((tool) => modelProtocolSet.has(resolveModelOptionPolicyProtocol(tool.protocol)))
    .map((tool) => tool.protocol);
  const sourceProtocols = explicitProtocols.length > 0
    ? explicitProtocols
    : (inferredProtocols.length > 0 ? inferredProtocols : [base.protocol]);
  const protocols = Array.from(new Set(sourceProtocols.map((protocol) => protocol.trim()).filter(Boolean)));
  if (protocols.length === 0) {
    return [base];
  }
  return protocols.map((protocol) => {
    const catalogDefinition = matchingDefinitions.find((tool) =>
      resolveModelOptionPolicyProtocol(tool.protocol) === resolveModelOptionPolicyProtocol(protocol)
    );
    if (!catalogDefinition) {
      return { ...base, protocol };
    }
    const configuredPayload = config.payload;
    const payload = Object.keys(configuredPayload).length > 0
      && nativeToolPayloadMatchesShape(configuredPayload, catalogDefinition.payload)
      ? configuredPayload
      : catalogDefinition.payload;
    return {
      ...catalogDefinition,
      provider: config.provider || catalogDefinition.provider,
      label: config.label || catalogDefinition.label,
      description: config.description || catalogDefinition.description,
      payload,
      defaultEnabled: config.defaultEnabled,
    };
  });
}

/** Mirrors the mature Web client's capabilitiesJSON -> request options behavior. */
export function resolveModelRequestOptions(
  model: Pick<PublicModelResponse, "capabilitiesJSON" | "protocolsJSON">,
  nativeToolCatalog: readonly NativeToolDefinition[] = [],
): ConversationOptions | undefined {
  const capabilities = parseJSONObject(model.capabilitiesJSON);
  if (!capabilities) {
    return undefined;
  }
  const defaults = isPlainObject(capabilities.defaultOptions)
    ? sanitizeConversationOptions(capabilities.defaultOptions)
    : {};
  const currentTools = Array.isArray(defaults.tools)
    ? defaults.tools.filter(isPlainObject)
    : [];
  const seenPayloads = new Set(currentTools.map(nativeToolPayloadSignature));
  const defaultToolPayloads = resolveNativeTools(model.capabilitiesJSON)
    .filter((tool) => tool.enabled && tool.defaultEnabled)
    .flatMap((tool) => nativeToolDefinitionVariantsFromConfig(
      tool,
      nativeToolCatalog,
      parseProtocolsJSON(model.protocolsJSON),
    ))
    .flatMap((tool) => {
      if (Object.keys(tool.payload).length === 0) {
        return [];
      }
      const signature = nativeToolPayloadSignature(tool.payload);
      if (seenPayloads.has(signature)) {
        return [];
      }
      seenPayloads.add(signature);
      return [{ ...tool.payload }];
    });
  const options = defaultToolPayloads.length > 0
    ? sanitizeConversationOptions({ ...defaults, tools: [...currentTools, ...defaultToolPayloads] })
    : defaults;
  return Object.keys(options).length > 0 ? options : undefined;
}
