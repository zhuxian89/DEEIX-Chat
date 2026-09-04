import assert from "node:assert/strict";
import test from "node:test";
import type { PublicModelResponse } from "@deeix/api-contract";
import {
  modelsForKind,
  resolveFixedModel,
  resolveSelectedModel,
  selectableModels,
} from "./model-catalog";

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

test("chat model picker exposes only chat-capable models", () => {
  const models = [
    model("chat", '["chat"]'),
    model("multimodal", '["chat","image_gen"]'),
    model("image", '["image_gen"]'),
  ];
  assert.deepEqual(modelsForKind(models, "chat").map((item) => item.platformModelName), ["chat", "multimodal"]);
});

test("conversation reopens with its last model and falls back to the recommended model", () => {
  const models = [model("recommended", '["chat"]'), model("previous", '["chat"]')];
  assert.equal(resolveSelectedModel(models, "previous", "recommended", "chat")?.platformModelName, "previous");
  assert.equal(resolveSelectedModel(models, "removed", "recommended", "chat")?.platformModelName, "recommended");
  assert.equal(resolveSelectedModel(models, "removed", "missing", "chat"), null);
});
