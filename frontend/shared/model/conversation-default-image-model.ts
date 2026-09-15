import { getConversationDefaultImageModel } from "@/shared/api/conversation-image-default";
import type { PublicModelDTO } from "@/shared/api/model.types";
import { parseKindsJSON } from "@/shared/model/llm-schema";

export async function resolveConversationDefaultImageModel(
  accessToken: string,
  availableModels: PublicModelDTO[],
): Promise<string> {
  const candidate = await getConversationDefaultImageModel(accessToken);
  const configured = candidate.platformModelName.trim();
  const imageModels = availableModels.filter((model) => parseKindsJSON(model.kindsJSON).includes("image_gen"));
  // An explicitly configured model must not silently fall back to another price/model.
  return configured
    ? imageModels.find((model) => model.platformModelName === configured)?.platformModelName ?? ""
    : imageModels[0]?.platformModelName ?? "";
}
