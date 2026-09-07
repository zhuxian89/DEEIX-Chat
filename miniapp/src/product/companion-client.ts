import type { State as CompanionState } from "@deeix/api-contract";
import type { ApiRequest } from "@/platform/transport";

export type { State as CompanionState, Memory as CompanionMemory } from "@deeix/api-contract";

// Authentication, refresh cookies and error envelopes stay in MiniAppSession.
export class CompanionClient {
  constructor(private readonly request: <T>(request: ApiRequest) => Promise<T>) {}

  state(): Promise<CompanionState> {
    return this.request({ path: "/api/v1/companion" });
  }

  open(allowGreeting: boolean): Promise<CompanionState> {
    return this.request({ path: "/api/v1/companion/open", method: "POST", body: { allowGreeting } });
  }

  async setQuiet(quiet: boolean): Promise<void> {
    await this.request({ path: "/api/v1/companion/preferences", method: "PATCH", body: { quiet } });
  }

  async forget(id?: string): Promise<void> {
    await this.request({
      path: `/api/v1/companion/memories${id ? `/${encodeURIComponent(id)}` : ""}`,
      method: "DELETE",
    });
  }

  async editMemory(id: string, value: string): Promise<void> {
    await this.request({ path: `/api/v1/companion/memories/${encodeURIComponent(id)}`, method: "PATCH", body: { value } });
  }

  async markRead(messageID: number): Promise<void> {
    await this.request({ path: "/api/v1/companion/read", method: "POST", body: { messageID } });
  }
}

export function companionStreamPath(conversationID: string): string {
  return `/api/v1/companion/conversations/${encodeURIComponent(conversationID.trim())}/messages/stream`;
}

export type CompanionPresence = {
  foreground: boolean;
  typing: boolean;
  replying: boolean;
  lastInteractionAt: number;
};

export function canOfferCompanionGreeting(presence: CompanionPresence, now: number): boolean {
  return presence.foreground && !presence.typing && !presence.replying && now - presence.lastInteractionAt >= 2000;
}

// File URLs remain authenticated. Only persisted file IDs are used for reloads.
export function companionImageIDs(attachments: string | undefined): string[] {
  if (!attachments) return [];
  try {
    const items: unknown = JSON.parse(attachments);
    if (!Array.isArray(items)) return [];
    const images = items.filter((item): item is Record<string, unknown> =>
      Boolean(item) && typeof item === "object" &&
      (item.kind === "image" || item.file_category === "image" || String(item.mime_type ?? "").startsWith("image/")),
    );
    const fileIDs = images
      .map((item) => typeof item.file_id === "string" ? item.file_id.trim() : "")
      .filter(Boolean);
    return [...new Set(fileIDs)].slice(0, 8);
  } catch {
    return [];
  }
}
