import type { MessageResponse, PublicModelResponse } from "@deeix/api-contract";
import { supportsModelKind } from "./model-catalog";

export type ImageSubmitTask = "image_generation" | "image_edit";
export type ImageSubmitBlockReason = "image_edit_input_required" | "image_edit_unsupported";

export type ImageSubmitDecision = {
  blockedReason: ImageSubmitBlockReason | null;
  task: ImageSubmitTask | null;
};

type ImageFailureMessage = Pick<MessageResponse, "errorMessage" | "role" | "runID" | "status">;

export function imageFailureMessageForRun(
  messages: readonly ImageFailureMessage[],
  runID: string,
  fallback: string,
): string {
  const normalizedRunID = runID.trim();
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];
    const status = message?.status.trim().toLowerCase();
    if (
      message?.role === "assistant" &&
      message.runID.trim() === normalizedRunID &&
      (status === "error" || status === "interrupted")
    ) {
      return message.errorMessage.trim() || fallback.trim();
    }
  }
  return fallback.trim();
}

export function imageTaskTerminalStatus(
  task: ImageSubmitTask,
  hasImage: boolean,
  assistantStatus: string | undefined,
  lastStatus: string,
  assistantErrorMessage?: string,
): string {
  const editing = task === "image_edit";
  if (hasImage) {
    return editing ? "图片编辑完成" : "图片生成完成";
  }

  switch (assistantStatus?.trim().toLowerCase()) {
    case "canceled":
      return editing ? "本次图片编辑已停止" : "本次图片生成已停止";
    case "error":
    case "interrupted":
      return assistantErrorMessage?.trim() || (editing ? "图片编辑失败，请重试" : "图片生成失败，请重试");
    default:
      return `${lastStatus.trim() || "正在处理图片"}，但没有收到可显示的图片`;
  }
}

export function imageModelOptions(models: readonly PublicModelResponse[]): PublicModelResponse[] {
  return models.filter((model) => model.platformModelName.trim() && (
    supportsModelKind(model, "image_gen") || supportsModelKind(model, "image_edit")
  ));
}

export function resolveImageSubmitDecision(
  model: PublicModelResponse | null,
  hasInputImage: boolean,
): ImageSubmitDecision {
  const supportsGeneration = Boolean(model && supportsModelKind(model, "image_gen"));
  const supportsEdit = Boolean(model && supportsModelKind(model, "image_edit"));
  if (hasInputImage) {
    return supportsEdit
      ? { blockedReason: null, task: "image_edit" }
      : { blockedReason: "image_edit_unsupported", task: null };
  }
  if (supportsGeneration) {
    return { blockedReason: null, task: "image_generation" };
  }
  return { blockedReason: "image_edit_input_required", task: null };
}

export function resolveImageEditModel(
  models: readonly PublicModelResponse[],
  selectedName: string,
  sourceModelName: string,
): PublicModelResponse | null {
  const candidates = [selectedName.trim(), sourceModelName.trim()];
  for (const name of candidates) {
    const model = models.find((item) => item.platformModelName === name);
    if (model && supportsModelKind(model, "image_edit")) {
      return model;
    }
  }
  return models.find((model) => model.platformModelName.trim() && supportsModelKind(model, "image_edit")) ?? null;
}
