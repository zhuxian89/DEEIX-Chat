import type { Initiative as CompanionInitiative, InitiativeResult, State as CompanionState } from "@deeix/api-contract";
import type { ApiRequest } from "@/platform/transport";
import type { CompanionAssistantState, CompanionContextConversation, CompanionContextPreferences, CompanionFollowup, CompanionFollowupRequest } from "@deeix/api-contract";
import type { ReminderState, ReminderCommandRequest, ReminderCommandResult } from "@deeix/api-contract";

export type { State as CompanionState, Memory as CompanionMemory, Initiative as CompanionInitiative } from "@deeix/api-contract";

export type CompanionProactivity = "normal" | "less" | "off";

// Authentication, refresh cookies and error envelopes stay in MiniAppSession.
export class CompanionClient {
  private reminderReadVersion = 0;
  private reminderWrites = 0;
  private readonly reminderListeners = new Set<(state: ReminderState) => void>();

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

  assistant(): Promise<CompanionAssistantState> {
    return this.request({ path: "/api/v1/companion/assistant" });
  }

  subscribeReminders(listener: (state: ReminderState) => void): () => void {
    this.reminderListeners.add(listener);
    return () => { this.reminderListeners.delete(listener); };
  }

  async reminders(conversationID = ""): Promise<ReminderState> {
    const version = ++this.reminderReadVersion;
    const state = await this.request<ReminderState>({ path: `/api/v1/companion/reminders?conversationID=${encodeURIComponent(conversationID)}` });
    if (version === this.reminderReadVersion && this.reminderWrites === 0) this.publishReminders(state);
    return state;
  }

  async reminderCommand(body: Partial<ReminderCommandRequest> & { requestID: string }): Promise<ReminderCommandResult> {
    this.reminderReadVersion++;
    this.reminderWrites++;
    try {
      const result = await this.request<ReminderCommandResult>({ path: "/api/v1/companion/reminders/command", method: "POST", body });
      // A successful mutation supersedes reads that began before it committed.
      this.reminderReadVersion++;
      if (result.handled) this.publishReminders(result.snapshot);
      return result;
    } finally {
      this.reminderWrites--;
    }
  }

  private publishReminders(state: ReminderState): void {
    for (const listener of this.reminderListeners) listener(state);
  }

  contextConversations(query = "", page = 1): Promise<CompanionContextConversation[]> {
    return this.request({ path: `/api/v1/companion/assistant/conversations?q=${encodeURIComponent(query)}&page=${page}` });
  }

  async setContextPreferences(body: CompanionContextPreferences): Promise<void> {
    await this.request({ path: "/api/v1/companion/assistant/context", method: "PATCH", body });
  }

  saveFollowup(body: CompanionFollowupRequest): Promise<CompanionFollowup> {
    return this.request({ path: "/api/v1/companion/assistant/followups", method: "POST", body });
  }

  async setNotifications(enabled: boolean, accepted = false, templateID = "", requestID = "", automatic = false): Promise<void> {
    await this.request({ path: "/api/v1/companion/assistant/notifications", method: "POST", body: { enabled, accepted, templateID, requestID, automatic } });
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
      (String(item.kind ?? "").toLowerCase() === "image" || String(item.file_category ?? "").toLowerCase() === "image" || String(item.mime_type ?? "").toLowerCase().startsWith("image/")),
    );
    const fileIDs = images
      .map((item) => typeof item.file_id === "string" ? item.file_id.trim() : "")
      .filter(Boolean);
    return [...new Set(fileIDs)];
  } catch {
    return [];
  }
}
