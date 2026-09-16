import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  getAdminReferenceData: vi.fn(),
  listAdminSettings: vi.fn(),
}));

vi.mock("next-intl", () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock("motion/react", () => ({
  motion: {
    div: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  },
}));

vi.mock("@/features/admin/api", () => ({
  exportAllConversations: vi.fn(),
  getAdminReferenceData: mocks.getAdminReferenceData,
  listAdminSettings: mocks.listAdminSettings,
  patchAdminSettings: vi.fn(),
}));

vi.mock("@/features/admin/components/sections/conversation/conversation-prompt-presets", () => ({
  ConversationPromptPresetsSection: () => null,
}));

vi.mock("@/shared/auth/resolve-access-token", () => ({
  resolveAccessToken: vi.fn().mockResolvedValue("token"),
}));

import { AdminConversationSettingsPage } from "./admin-conversation";

afterEach(cleanup);

test("shows the saved default image model after reference data loads", async () => {
  mocks.listAdminSettings.mockResolvedValue({
    chat: [
      {
        key: "conversation_default_image_model",
        value: "gpt-image-1.5",
        valueType: "string",
        description: "",
        sensitive: false,
        configured: true,
      },
    ],
  });
  mocks.getAdminReferenceData.mockResolvedValue({
    billingConfig: {},
    billingPlans: [],
    modelPricing: [],
    models: [
      {
        platformModelName: "gpt-image-1.5",
        kindsJSON: '["image_gen"]',
        status: "active",
        activeSourceCount: 1,
        vendor: "openai",
        icon: "",
      },
    ],
  });

  const { container } = render(<AdminConversationSettingsPage />);

  await waitFor(() => {
    const trigger = container.querySelector("#chat\\.conversation_default_image_model");
    const visibleValue = trigger?.querySelector(":scope > span > span:last-child");
    expect(visibleValue?.textContent).toBe("gpt-image-1.5");
  });
});
