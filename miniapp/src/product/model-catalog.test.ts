import assert from "node:assert/strict";
import test from "node:test";
import type { PublicModelResponse } from "@deeix/api-contract";
import { resolveFixedModel, selectableModels } from "./model-catalog";

function model(name: string, kinds: string): PublicModelResponse {
  return { platformModelName: name, kindsJSON: kinds } as PublicModelResponse;
}

test("fixed model requires exact configured name and capability", () => {
  const models = [model("chat-default", '["chat"]'), model("image-default", '["image_gen"]')];
  assert.equal(resolveFixedModel(models, "chat-default", "chat")?.platformModelName, "chat-default");
  assert.equal(resolveFixedModel(models, "image-default", "chat"), null);
  assert.equal(resolveFixedModel(models, "missing", "chat"), null);
});

test("advanced catalog excludes unsupported entries without silently substituting presets", () => {
  const models = [model("chat", '["chat"]'), model("image", '["image_gen"]'), model("embedding", '["embedding"]')];
  assert.deepEqual(selectableModels(models).map((item) => item.platformModelName), ["chat", "image"]);
});
