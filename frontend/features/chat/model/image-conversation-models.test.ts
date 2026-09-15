import { expect, test } from "vitest";
import { imageConversationModels } from "./image-conversation-models";
import { resolveChatSubmitDecision } from "./chat-task";
import { planChatSubmission } from "./message-submit-plan";
import type { ChatModelOption } from "@/features/chat/types/chat-runtime";

const model = (kinds: string[]) => ({ platformModelName: "mixed", kinds, defaultOptions: {} }) as ChatModelOption;

test("the image entry prefers generation for a mixed model without changing the canonical catalog", () => {
  const original = model(["chat", "image_gen", "image_edit", "video_gen"]);
  const catalog = [original];
  const projected = imageConversationModels(catalog, true);
  expect(resolveChatSubmitDecision(projected[0], [], {}).task).toBe("image_generation");
  expect(projected[0].kinds).toContain("image_edit");
  expect(original.kinds).toEqual(["chat", "image_gen", "image_edit", "video_gen"]);
  expect(imageConversationModels(catalog, false)).toBe(catalog);
  expect(resolveChatSubmitDecision(original, [], {}).task).toBe("chat");
});

test.each([
  { response_format: { type: "image", aspect_ratio: "16:9", image_size: "4K" } },
  { response_format: "b64_json" },
  { response_format: { type: "text" }, temperature: 0.5 },
  {},
])("keeps all request and editor options intact: %j", (options) => {
  const original = { ...model(["chat", "image_gen"]), defaultOptions: options };
  const projected = imageConversationModels([original], true)[0];
  expect(projected.defaultOptions).toBe(options);
  expect(resolveChatSubmitDecision(projected, [], options).task).toBe("image_generation");
  expect(original.defaultOptions).toEqual(options);
});

test("chat-only models remain unchanged if manually selected from the image entry", () => {
  const original = model(["chat"]);
  expect(imageConversationModels([original], true)[0]).toBe(original);
});

test("the actual submission plan preserves nested image format parameters", () => {
  const options = { response_format: { type: "image", aspect_ratio: "16:9", image_size: "4K" } };
  const result = planChatSubmission({
    content: "draw a landscape", currentAttachments: [], attachmentFallbackContent: "", uploading: false,
    maxFilesPerMessage: 5, modelOptions: imageConversationModels([model(["chat", "image_gen"])], true),
    selectedPlatformModelName: "mixed", options, selectedToolIDs: [], selectedSkills: [], selectedKnowledgeBaseIDs: [],
    htmlVisualPromptEnabled: false, visibleConversationScopeKey: "draft", visibleBranchScopePath: [],
    visibleMessages: [], combinedMessages: [], activeStreams: [],
  });
  expect(result.ok).toBe(true);
  if (!result.ok) throw new Error("expected a valid image submission");
  expect(result.plan.submitTask).toBe("image_generation");
  expect(result.plan.sanitizedOptions).toEqual(options);
});
