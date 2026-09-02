import type { PublicModelResponse } from "@deeix/api-contract";

export type MiniAppModelKind = "chat" | "image_gen";

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

export function selectableModels(models: readonly PublicModelResponse[]): PublicModelResponse[] {
  return models.filter((model) => model.platformModelName.trim() && (
    supportsModelKind(model, "chat") || supportsModelKind(model, "image_gen")
  ));
}
