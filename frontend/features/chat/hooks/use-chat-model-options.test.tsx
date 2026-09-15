import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { useChatModelOptions } from "./use-chat-model-options";

const mocks = vi.hoisted(() => ({
  image: vi.fn(), chat: vi.fn(), runs: vi.fn(),
  t: (key: string) => key,
  settings: { "chat.default_model": "personal-chat" },
}));
vi.mock("next-intl", () => ({ useTranslations: () => mocks.t }));
vi.mock("@/features/settings", () => ({ parseSendShortcut: () => "enter" }));
vi.mock("@/shared/model/user-settings-store", () => ({ useUserSettings: () => ({ settings: mocks.settings }) }));
vi.mock("@/shared/auth/resolve-access-token", () => ({ resolveAccessToken: async () => "token" }));
vi.mock("@/shared/api/billing", () => ({ getBillingConfig: async () => null }));
vi.mock("@/shared/api/settings", () => ({ getMCPPolicy: async () => null, getModelOptionPolicy: async () => null }));
vi.mock("@/shared/api/conversation", () => ({ listConversationRuns: mocks.runs }));
vi.mock("@/shared/model/conversation-default-model", () => ({ resolveConversationDefaultModel: mocks.chat }));
vi.mock("@/shared/model/conversation-default-image-model", () => ({ resolveConversationDefaultImageModel: mocks.image }));
vi.mock("@/shared/api/model", () => ({ listPublicModels: async () => [
  { platformModelName: "personal-chat", kindsJSON: '["chat"]', capabilitiesJSON: "{}", protocolsJSON: "[]" },
  { platformModelName: "image", kindsJSON: '["image_gen"]', capabilitiesJSON: "{}", protocolsJSON: "[]" },
] }));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.chat.mockResolvedValue({ platformModelName: "personal-chat" });
  mocks.image.mockResolvedValue("image");
  mocks.runs.mockResolvedValue({ results: [] });
});
afterEach(cleanup);

type Props = { conversationPublicID: string | null; conversationModel?: string; resetToken: number; imageDefault: boolean };
const initial: Props = { conversationPublicID: null, resetToken: 0, imageDefault: false };

test("image entry overrides a manually selected chat model; new chat restores personal default", async () => {
  const { result, rerender } = renderHook((props: Props) => useChatModelOptions(props), { initialProps: initial });
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("personal-chat"));
  act(() => result.current.setSelectedPlatformModelName("manual-chat"));
  rerender({ ...initial, imageDefault: true, resetToken: 1 });
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("image"));
  rerender({ ...initial, resetToken: 2 });
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("personal-chat"));
  expect(mocks.chat).toHaveBeenLastCalledWith(expect.objectContaining({ userDefaultModel: "personal-chat" }));
});

test("ordinary new chat still preserves manual selection", async () => {
  const { result, rerender } = renderHook((props: Props) => useChatModelOptions(props), { initialProps: initial });
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("personal-chat"));
  act(() => result.current.setSelectedPlatformModelName("manual-chat"));
  rerender({ ...initial, resetToken: 1 });
  expect(result.current.selectedPlatformModelName).toBe("manual-chat");
});

test("unavailable image default leaves no selected chat fallback and displays an error", async () => {
  mocks.image.mockResolvedValue("");
  const { result } = renderHook(() => useChatModelOptions({ ...initial, imageDefault: true }));
  await waitFor(() => expect(result.current.modelsErrorMsg).toBe("imageDefaultUnavailable"));
  expect(result.current.selectedPlatformModelName).toBe("");
  expect(mocks.chat).not.toHaveBeenCalled();
});

test("a slow image default cannot overwrite a subsequent new chat", async () => {
  let complete!: (value: string) => void;
  mocks.image.mockReturnValue(new Promise<string>((resolve) => { complete = resolve; }));
  const { result, rerender } = renderHook((props: Props) => useChatModelOptions(props), {
    initialProps: { ...initial, imageDefault: true },
  });
  await waitFor(() => expect(mocks.image).toHaveBeenCalled());
  rerender({ ...initial, resetToken: 1 });
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("personal-chat"));
  await act(async () => complete("image"));
  expect(result.current.selectedPlatformModelName).toBe("personal-chat");
});

test("opening an existing conversation never replaces its model with the image default", async () => {
  const { result } = renderHook(() => useChatModelOptions({
    ...initial, imageDefault: true, conversationPublicID: "existing", conversationModel: "history-model",
  }));
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("history-model"));
  expect(mocks.image).not.toHaveBeenCalled();
});

test("leaving a pending image entry clears its loading state", async () => {
  mocks.image.mockReturnValue(new Promise<string>(() => {}));
  const { result, rerender } = renderHook((props: Props) => useChatModelOptions(props), {
    initialProps: { ...initial, imageDefault: true },
  });
  await waitFor(() => expect(mocks.image).toHaveBeenCalled());
  expect(result.current.modelsLoading).toBe(true);
  rerender({ ...initial, imageDefault: true, conversationPublicID: "existing", conversationModel: "history-model" });
  await waitFor(() => expect(result.current.modelsLoading).toBe(false));
});

test.each(["unavailable", "offline"])("the %s image error does not leak into history", async (failure) => {
  if (failure === "offline") mocks.image.mockRejectedValue(new Error("offline"));
  else mocks.image.mockResolvedValue("");
  const { result, rerender } = renderHook((props: Props) => useChatModelOptions(props), {
    initialProps: { ...initial, imageDefault: true },
  });
  await waitFor(() => expect(result.current.modelsErrorMsg).not.toBe(""));
  rerender({ ...initial, imageDefault: true, conversationPublicID: "history", conversationModel: "history-model" });
  await waitFor(() => expect(result.current.selectedPlatformModelName).toBe("history-model"));
  expect(result.current.modelsErrorMsg).toBe("");
});
