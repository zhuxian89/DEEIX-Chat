import { expect, test } from "vitest";
import type { AdminLLMModelDTO } from "@/features/admin/api/llm.types";
import { buildImageModelOptions } from "./image-model-options";

test("image defaults list only active, routable generation models and deduplicate names", () => {
  const model = (name: string, kinds: string[], status = "active", activeSourceCount = 1) => ({
    platformModelName: name, kindsJSON: JSON.stringify(kinds), status, activeSourceCount,
  }) as AdminLLMModelDTO;
  const options = buildImageModelOptions([
    model("chat", ["chat"]), model("image", ["image_gen"]), model("image", ["image_gen"]),
    model("edit", ["image_edit"]), model("disabled", ["image_gen"], "disabled"),
    model("no-route", ["image_gen"], "active", 0), model("mixed", ["chat", "image_gen"]),
  ], "Automatic");
  expect(options.map((item) => item.value)).toEqual(["", "image", "mixed"]);
});
