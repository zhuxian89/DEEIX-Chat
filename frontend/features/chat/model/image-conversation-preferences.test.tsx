import * as React from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { useChatModelOptionState } from "@/features/chat/hooks/use-chat-model-option-state";
import type { ChatModelOption } from "@/features/chat/types/chat-runtime";
import { readCachedModelOptions, removeCachedModelOptions, writeCachedModelOptions } from "./chat-model-options-cache";
import { resolveChatSubmitDecision } from "./chat-task";
import { cloneConversationOptions } from "./conversation-options";
import { imageConversationModels } from "./image-conversation-models";

const original = {
  platformModelName: "mixed-entry-regression", kinds: ["chat", "image_gen"], defaultOptions: {},
} as ChatModelOption;
const catalog = [original];
const refreshModelOption = async () => original;

function useEntryOptions(imageEntry: boolean) {
  const selectedModel = React.useMemo(() => imageConversationModels(catalog, imageEntry)[0], [imageEntry]);
  const state = useChatModelOptionState({
    selectedModel, selectedPlatformModelName: original.platformModelName, chatPreferencesLoaded: true,
    reuseModelOptions: true, refreshModelOption,
  });
  return { ...state, selectedModel };
}

afterEach(() => { cleanup(); removeCachedModelOptions(original.platformModelName); });

test.each([false, true])("saving the image parameter editor (modified=%s) cannot turn ordinary chat into generation", async (modified) => {
  writeCachedModelOptions(original.platformModelName, { temperature: 0.5 });
  const image = renderHook(() => useEntryOptions(true));
  await waitFor(() => expect(image.result.current.options.temperature).toBe(0.5));
  expect(resolveChatSubmitDecision(image.result.current.selectedModel, [], image.result.current.options).task).toBe("image_generation");
  // The editor clones and saves the entire supplied options object.
  const editorDraft = cloneConversationOptions(image.result.current.options);
  if (modified) editorDraft.temperature = 0.8;
  act(() => image.result.current.setModelOptions(editorDraft));
  expect(readCachedModelOptions(original.platformModelName)).toEqual({ temperature: modified ? 0.8 : 0.5 });
  image.unmount();

  const chat = renderHook(() => useEntryOptions(false));
  await waitFor(() => expect(chat.result.current.options.temperature).toBe(modified ? 0.8 : 0.5));
  expect(resolveChatSubmitDecision(chat.result.current.selectedModel, [], chat.result.current.options).task).toBe("chat");
  expect(chat.result.current.options.response_format).toBeUndefined();
});
