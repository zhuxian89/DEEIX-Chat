import type { Initiative as CompanionInitiative, InitiativeResult, State as CompanionState } from "@deeix/api-contract";
import type { ApiRequest } from "@/platform/transport";

export type { State as CompanionState, Memory as CompanionMemory, Initiative as CompanionInitiative } from "@deeix/api-contract";

export type CompanionProactivity = "normal" | "less" | "off";

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

  async setProactivity(proactivity: CompanionProactivity): Promise<void> {
    await this.request({ path: "/api/v1/companion/preferences", method: "PATCH", body: { proactivity } });
  }

  async prepareInitiative(kind: "home" | "idle", visitID: string, afterMessageID: number): Promise<CompanionInitiative | null> {
    const result = await this.request<InitiativeResult>({
      path: "/api/v1/companion/initiatives", method: "POST", body: { kind, visitID, afterMessageID },
    });
    return result.message;
  }

  async acceptInitiative(id: string, visitID: string): Promise<CompanionInitiative | null> {
    const result = await this.request<InitiativeResult>({
      path: `/api/v1/companion/initiatives/${encodeURIComponent(id)}/accept`, method: "POST", body: { visitID },
    });
    return result.message;
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

  async topicFeedback(topicURL: string, preference: "like" | "avoid"): Promise<void> {
    await this.request({ path: "/api/v1/companion/topics/feedback", method: "POST", body: { topicURL, preference } });
  }
}

export function companionBeijingDate(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return "日期未知";
  const local = new Date(timestamp + 8 * 60 * 60 * 1000);
  return `${local.getUTCFullYear()}-${String(local.getUTCMonth() + 1).padStart(2, "0")}-${String(local.getUTCDate()).padStart(2, "0")}`;
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
