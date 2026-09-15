import type { ReactNode } from "react";
import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ChatSessionProvider, useChatSession } from "@/features/chat/context/chat-session-context";
import { NewImageConversation } from "./new-image-conversation";

const nav = vi.hoisted(() => ({ pathname: "/chat", push: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => nav.pathname, useRouter: () => ({ push: nav.push }) }));
vi.mock("next-intl", () => ({ useTranslations: () => () => "生图" }));
vi.mock("@/components/ui/sidebar", () => ({
  SidebarMenuItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  SidebarTransitionContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: () => null,
}));

function SessionState() {
  const state = useChatSession();
  return <output data-testid="session">{`${state.newConversationRevision}:${state.newConversationImageDefault}:${state.newConversationProjectID}`}</output>;
}

beforeEach(() => { vi.clearAllMocks(); nav.pathname = "/chat"; });
afterEach(cleanup);

test("the image button starts a fresh image draft, clears project context, and closes the mobile sidebar", () => {
  const close = vi.fn();
  render(<ChatSessionProvider><NewImageConversation isCollapsed onCloseMobileSidebar={close} /><SessionState /></ChatSessionProvider>);
  fireEvent.click(screen.getByRole("button", { name: "生图" }));
  expect(screen.getByTestId("session").textContent).toBe("1:true:");
  expect(close).toHaveBeenCalledOnce();
  expect(window.location.pathname).toBe("/chat");
  fireEvent.click(screen.getByRole("button", { name: "生图" }));
  expect(screen.getByTestId("session").textContent).toBe("2:true:");
});

test("the image button navigates to chat from another page", () => {
  nav.pathname = "/files";
  render(<ChatSessionProvider><NewImageConversation isCollapsed={false} onCloseMobileSidebar={() => {}} /></ChatSessionProvider>);
  fireEvent.click(screen.getByRole("button", { name: "生图" }));
  expect(nav.push).toHaveBeenCalledWith("/chat");
});

test("an ordinary new conversation clears the image entry flag", () => {
  const { result } = renderHook(useChatSession, { wrapper: ChatSessionProvider });
  act(() => result.current.requestNewConversation({ imageDefault: true }));
  expect(result.current.newConversationImageDefault).toBe(true);
  act(() => result.current.requestNewConversation({ projectID: " project " }));
  expect(result.current.newConversationImageDefault).toBe(false);
  expect(result.current.newConversationProjectID).toBe("project");
  expect(result.current.newConversationRevision).toBe(2);
});
