import type { PublicModelResponse } from "@deeix/api-contract";

export type MiniAppModelKind = "chat" | "image_gen" | "image_edit";

export function modelKinds(model: Pick<PublicModelResponse, "kindsJSON">): string[] {
  try {
    const value = JSON.parse(model.kindsJSON) as unknown;
    return Array.isArray(value)
      ? value.filter((item): item is string => typeof item === "string").map((item) => item.trim().toLowerCase())
      : [];
  } catch {
    return [];
  }
}

export function supportsModelKind(model: PublicModelResponse, kind: MiniAppModelKind): boolean {
  return modelKinds(model).includes(kind);
}

export function modelsForKind(
  models: readonly PublicModelResponse[],
  kind: MiniAppModelKind,
): PublicModelResponse[] {
  return models.filter((model) => supportsModelKind(model, kind));
}

export function resolveFixedModel(
  models: readonly PublicModelResponse[],
  configuredName: string,
  kind: MiniAppModelKind,
): PublicModelResponse | null {
  const expected = configuredName.trim();
  if (!expected) {
    return null;
  }
  return models.find((model) => model.platformModelName === expected && supportsModelKind(model, kind)) ?? null;
}

export function resolveSelectedModel(
  models: readonly PublicModelResponse[],
  selectedName: string,
  fallbackName: string,
  kind: MiniAppModelKind,
): PublicModelResponse | null {
  return resolveFixedModel(models, selectedName, kind) ?? resolveFixedModel(models, fallbackName, kind);
}

export function selectableModels(models: readonly PublicModelResponse[]): PublicModelResponse[] {
  return models.filter((model) => model.platformModelName.trim() && (
    supportsModelKind(model, "chat") || supportsModelKind(model, "image_gen") || supportsModelKind(model, "image_edit")
  ));
}
