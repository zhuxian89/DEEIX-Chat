import assert from "node:assert/strict";
import test from "node:test";
import type { PublicModelResponse } from "@deeix/api-contract";
import {
  imageModelOptions,
  resolveImageEditModel,
  resolveImageSubmitDecision,
} from "./image-task";

function model(name: string, kinds: string): PublicModelResponse {
  return { platformModelName: name, kindsJSON: kinds } as PublicModelResponse;
}

test("image workspace exposes generation and editing capable models", () => {
  const models = [
    model("chat", '["chat"]'),
    model("generate", '["image_gen"]'),
    model("edit", '["image_edit"]'),
    model("both", '["image_gen","image_edit"]'),
  ];
  assert.deepEqual(imageModelOptions(models).map((item) => item.platformModelName), ["generate", "edit", "both"]);
});

test("an attached image follows the mature web image edit task contract", () => {
  const generation = model("generate", '["image_gen"]');
  const edit = model("edit", '["image_edit"]');
  const both = model("both", '["image_gen","image_edit"]');

  assert.deepEqual(resolveImageSubmitDecision(generation, false), { blockedReason: null, task: "image_generation" });
  assert.deepEqual(resolveImageSubmitDecision(generation, true), { blockedReason: "image_edit_unsupported", task: null });
  assert.deepEqual(resolveImageSubmitDecision(edit, false), { blockedReason: "image_edit_input_required", task: null });
  assert.deepEqual(resolveImageSubmitDecision(edit, true), { blockedReason: null, task: "image_edit" });
  assert.deepEqual(resolveImageSubmitDecision(both, false), { blockedReason: null, task: "image_generation" });
  assert.deepEqual(resolveImageSubmitDecision(both, true), { blockedReason: null, task: "image_edit" });
});

test("editing keeps the current model, then source model, then falls back to an editing model", () => {
  const models = [
    model("generate", '["image_gen"]'),
    model("source-edit", '["image_edit"]'),
    model("fallback-edit", '["image_edit"]'),
  ];
  assert.equal(resolveImageEditModel(models, "source-edit", "")?.platformModelName, "source-edit");
  assert.equal(resolveImageEditModel(models, "generate", "source-edit")?.platformModelName, "source-edit");
  assert.equal(resolveImageEditModel(models, "generate", "missing")?.platformModelName, "source-edit");
});
