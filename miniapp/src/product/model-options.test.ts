import assert from "node:assert/strict";
import test from "node:test";
import type { PublicModelResponse } from "@deeix/api-contract";
import {
  type NativeToolDefinition,
  resolveModelRequestOptions,
} from "./model-options";

function model(capabilities: unknown, protocols = ["openai_responses"]): PublicModelResponse {
  return {
    capabilitiesJSON: JSON.stringify(capabilities),
    protocolsJSON: JSON.stringify(protocols),
  } as PublicModelResponse;
}

function nativeTool(
  toolKey: string,
  protocol: string,
  payload: Record<string, unknown>,
): NativeToolDefinition {
  return {
    protocol,
    provider: "OpenAI",
    type: String(payload.type ?? ""),
    toolKey,
    label: toolKey,
    description: toolKey,
    payload,
    defaultEnabled: false,
    billable: false,
    billingUnit: "",
    priceNanousd: 0,
    priceLabel: "",
    riskLevel: "",
    usageAliases: [],
  };
}

test("model defaults and default-enabled native tools mirror the Web request options", () => {
  const catalog = [
    nativeTool("web_search", "openai_responses", { type: "web_search_preview" }),
    nativeTool("web_search", "openai_chat_completions", { type: "web_search" }),
  ];
  const options = resolveModelRequestOptions(model({
    defaultOptions: {
      reasoning: { effort: "high" },
      tools: [{ type: "code_interpreter", container: { type: "auto" } }],
    },
    nativeTools: [
      { key: "web_search", enabled: true, defaultEnabled: true, payload: {} },
      { key: "disabled", enabled: false, defaultEnabled: true, payload: { type: "disabled_tool" } },
      { key: "manual", enabled: true, defaultEnabled: false, payload: { type: "manual_tool" } },
    ],
  }), catalog);

  assert.deepEqual(options, {
    reasoning: { effort: "high" },
    tools: [
      { type: "code_interpreter", container: { type: "auto" } },
      { type: "web_search_preview" },
    ],
  });
});

test("default native tool payloads are deduplicated independent of object key order", () => {
  const options = resolveModelRequestOptions(model({
    defaultOptions: { tools: [{ search_context_size: "medium", type: "web_search_preview" }] },
    nativeTools: [{
      key: "web_search",
      enabled: true,
      defaultEnabled: true,
      payload: { type: "web_search_preview", search_context_size: "medium" },
    }],
  }));

  assert.deepEqual(options, {
    tools: [{ search_context_size: "medium", type: "web_search_preview" }],
  });
});

test("inline default tool payload still works when the policy catalog is unavailable", () => {
  const options = resolveModelRequestOptions(model({
    nativeTools: [{
      key: "url_reader",
      enabled: true,
      defaultEnabled: true,
      payload: { type: "url_reader" },
    }],
  }));
  assert.deepEqual(options, { tools: [{ type: "url_reader" }] });
});

test("invalid capability JSON is ignored and reserved request fields are removed", () => {
  assert.equal(resolveModelRequestOptions({ capabilitiesJSON: "not-json", protocolsJSON: "[]" }), undefined);
  assert.deepEqual(resolveModelRequestOptions(model({
    defaultOptions: { model: "must-not-pass", messages: [], temperature: 0.2 },
  })), { temperature: 0.2 });
});
