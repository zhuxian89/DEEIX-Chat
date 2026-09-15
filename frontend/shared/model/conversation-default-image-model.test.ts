import { beforeEach, describe, expect, test, vi } from "vitest";
import type { PublicModelDTO } from "@/shared/api/model.types";
import { resolveConversationDefaultImageModel } from "./conversation-default-image-model";

const { getDefault } = vi.hoisted(() => ({ getDefault: vi.fn() }));
vi.mock("@/shared/api/conversation-image-default", () => ({ getConversationDefaultImageModel: getDefault }));

const model = (name: string, kinds: string[]) => ({ platformModelName: name, kindsJSON: JSON.stringify(kinds) }) as PublicModelDTO;
const models = [model("chat", ["chat"]), model("image-a", ["image_gen"]), model("image-b", ["chat", "image_gen"])];

describe("new image conversation default", () => {
  beforeEach(() => vi.clearAllMocks());

  test("uses the configured available image model", async () => {
    getDefault.mockResolvedValue({ platformModelName: " image-b " });
    expect(await resolveConversationDefaultImageModel("token", models)).toBe("image-b");
  });

  test("empty configuration falls back only to an image generation model", async () => {
    getDefault.mockResolvedValue({ platformModelName: "" });
    expect(await resolveConversationDefaultImageModel("token", models)).toBe("image-a");
  });

  test.each(["missing", "chat"])("does not substitute another model when configured model %s is unavailable", async (name) => {
    getDefault.mockResolvedValue({ platformModelName: name });
    expect(await resolveConversationDefaultImageModel("token", models)).toBe("");
  });

  test("vision and edit-only models are not generation defaults", async () => {
    getDefault.mockResolvedValue({ platformModelName: "" });
    expect(await resolveConversationDefaultImageModel("token", [model("vision", ["chat", "vision"]), model("edit", ["image_edit"])])).toBe("");
  });

  test("configuration fetch failures do not silently pick a model", async () => {
    getDefault.mockRejectedValue(new Error("offline"));
    await expect(resolveConversationDefaultImageModel("token", models)).rejects.toThrow("offline");
  });
});
