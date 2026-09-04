import assert from "node:assert/strict";
import test from "node:test";
import {
  removeNativeWebSearchOptions,
  resolveExaNetworkToolIDs,
} from "./network-search";

test("resolves active Exa search and fetch tools without exposing server credentials", () => {
  assert.deepEqual(resolveExaNetworkToolIDs([
    { id: 9, name: "other_tool", status: "active" },
    { id: 12, name: "web_fetch_exa", status: "active" },
    { id: 11, name: "web_search_exa", status: "active" },
    { id: 13, name: "web_search_exa", status: "disabled" },
    { id: 11, name: "web_search_exa", status: "active" },
  ]), [12, 11]);
});

test("requires the Exa search tool before enabling network search", () => {
  assert.deepEqual(resolveExaNetworkToolIDs([
    { id: 12, name: "web_fetch_exa", status: "active" },
  ]), []);
});

test("removes duplicate provider-native web search while preserving other model options", () => {
  assert.deepEqual(removeNativeWebSearchOptions({
    reasoning: { effort: "high" },
    tools: [
      { type: "web_search_preview" },
      { type: "google_search" },
      { type: "code_interpreter" },
    ],
  }), {
    reasoning: { effort: "high" },
    tools: [{ type: "code_interpreter" }],
  });
  assert.deepEqual(removeNativeWebSearchOptions({ tools: [{ type: "web_search" }] }), undefined);
  assert.deepEqual(removeNativeWebSearchOptions({ temperature: 0.2 }), { temperature: 0.2 });
});
