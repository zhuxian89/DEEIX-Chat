import type { PublicModelResponse } from "@deeix/api-contract";
import { supportsModelKind } from "./model-catalog";

export type ImageSubmitTask = "image_generation" | "image_edit";
export type ImageSubmitBlockReason = "image_edit_input_required" | "image_edit_unsupported";

export type ImageSubmitDecision = {
  blockedReason: ImageSubmitBlockReason | null;
  task: ImageSubmitTask | null;
};

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
