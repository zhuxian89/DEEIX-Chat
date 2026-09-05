import assert from "node:assert/strict";
import test from "node:test";
import type { PublicModelResponse } from "@deeix/api-contract";
import {
  imageFailureMessageForRun,
  imageTaskTerminalStatus,
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

test("image terminal status never presents a failed backend task as still generating", () => {
  const upstreamError = "非常抱歉，生成的图片可能违反了关于轻度性暗示或挑逗性主题的防护限制。";
  assert.equal(imageTaskTerminalStatus("image_generation", true, "success", "正在保存图片"), "图片生成完成");
  assert.equal(imageTaskTerminalStatus("image_edit", true, "success", "正在保存图片"), "图片编辑完成");
  assert.equal(imageTaskTerminalStatus("image_generation", false, "error", "AI 正在生成图片"), "图片生成失败，请重试");
  assert.equal(imageTaskTerminalStatus("image_generation", false, "error", "AI 正在生成图片", upstreamError), upstreamError);
  assert.equal(imageTaskTerminalStatus("image_edit", false, "interrupted", "AI 正在编辑图片"), "图片编辑失败，请重试");
  assert.equal(imageTaskTerminalStatus("image_generation", false, "canceled", "AI 正在生成图片"), "本次图片生成已停止");
});

test("live image failure recovers the persisted upstream error for the exact run", () => {
  const upstreamError = "非常抱歉，生成的图片可能违反了关于轻度性暗示或挑逗性主题的防护限制。";
  assert.equal(imageFailureMessageForRun([
    { errorMessage: "其他任务失败", role: "assistant", runID: "run-other", status: "error" },
    { errorMessage: upstreamError, role: "assistant", runID: "run-image", status: "error" },
  ], "run-image", "upstream service unavailable / upstream.unavailable"), upstreamError);
  assert.equal(imageFailureMessageForRun([], "run-image", "upstream unavailable"), "upstream unavailable");
});
