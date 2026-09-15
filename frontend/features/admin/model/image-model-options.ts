import type { AdminLLMModelDTO } from "@/features/admin/api/llm.types";
import type { ModelSelectOption } from "@/shared/components/model-select";
import { isRoutablePlatformModel, resolveModelOptionIconUrl, resolveModelOptionLabel } from "@/shared/lib/model-option-display";
import { parseKindsJSON } from "@/shared/model/llm-schema";

export function buildImageModelOptions(models: AdminLLMModelDTO[], automaticLabel: string): ModelSelectOption[] {
  const seen = new Set<string>();
  const options: ModelSelectOption[] = [{ label: automaticLabel, value: "", iconUrl: null }];
  for (const model of models) {
    if (!isRoutablePlatformModel(model) || !parseKindsJSON(model.kindsJSON).includes("image_gen")) continue;
    const name = model.platformModelName.trim();
    if (seen.has(name)) continue;
    seen.add(name);
    options.push({
      label: resolveModelOptionLabel(name),
      value: name,
      iconUrl: resolveModelOptionIconUrl({ platformModelName: name, vendor: model.vendor ?? "", icon: model.icon ?? "" }),
    });
  }
  return options;
}
