import type { CompanionClient, CompanionInitiative, CompanionProactivity, CompanionState } from "./companion-client";

export type CompanionIdlePresence = {
  foreground: boolean;
  typing: boolean;
  replying: boolean;
  readingHistory: boolean;
  unavailable: boolean;
  mode: CompanionProactivity;
  lastInteractionAt: number;
  replyVisibleAt: number;
  latestReplyID: number;
  readThroughID: number;
};

export function canOfferCompanionContinuation(presence: CompanionIdlePresence, now: number): boolean {
  const delay = presence.mode === "less" ? 90_000 : 45_000;
  return presence.mode !== "off" && presence.foreground && !presence.typing && !presence.replying &&
    !presence.readingHistory && !presence.unavailable && presence.latestReplyID > 0 &&
    presence.readThroughID >= presence.latestReplyID && presence.replyVisibleAt > 0 &&
    now - Math.max(presence.lastInteractionAt, presence.replyVisibleAt) >= delay;
}

// Keep concurrent ticks single-flight, but let transient failures recover.
// A delivered turn is never published twice during this visit.
export class CompanionInitiativeVisit {
  readonly id = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
  private working = false;
  private retryAt = 0;
  private delivered = new Set<string>();
  private revision = 0;

  invalidate(): void { this.revision += 1; }

  async consider(
    client: Pick<CompanionClient, "prepareInitiative" | "acceptInitiative">,
    kind: "home" | "idle",
    afterMessageID: number,
    isEligible: () => boolean,
    onAccepted: (message: CompanionInitiative) => void,
  ): Promise<void> {
    const key = `${kind}:${afterMessageID}`;
    if (this.working || this.delivered.has(key) || Date.now() < this.retryAt || !isEligible()) return;
    this.working = true;
    this.retryAt = Date.now() + 30_000;
    const revision = this.revision;
    const isCurrent = () => this.revision === revision && isEligible();
    try {
      const candidate = await client.prepareInitiative(kind, this.id, afterMessageID);
      if (!candidate) { this.retryAt = Date.now() + 5 * 60_000; return; }
      if (!isCurrent() || !(Date.parse(candidate.expiresAt) > Date.now())) return;
      const accepted = await client.acceptInitiative(candidate.id, this.id);
      if (accepted?.id === candidate.id && Date.parse(accepted.acceptedAt) > 0) {
        this.delivered.add(key);
        if (isCurrent()) onAccepted(accepted);
      }
    } catch { /* Retry after a bounded delay; ordinary chat stays available. */ }
    finally { this.working = false; }
  }
}

export class CompanionReadTracker {
  confirmed = 0;
  private working = false;
  private retryAt = 0;

  async acknowledge(client: Pick<CompanionClient, "markRead">, messageID: number): Promise<number> {
    if (messageID <= this.confirmed || this.working || Date.now() < this.retryAt) return this.confirmed;
    this.working = true;
    try {
      await client.markRead(messageID);
      this.confirmed = Math.max(this.confirmed, messageID);
    } catch {
      this.retryAt = Date.now() + 10_000;
    } finally { this.working = false; }
    return this.confirmed;
  }
}

export function companionEntryPreview(state: CompanionState): string {
  if (state.quiet) return "不用想好问题，随口聊聊也可以。";
  const latest = [...(state.initiatives ?? [])].reverse().find((item) => item.kind === "followup");
  if (latest && (!state.greeting || Date.parse(latest.acceptedAt) >= Date.parse(state.greetingAt))) return latest.text;
  return state.greeting || "不用想好问题，随口聊聊也可以。";
}

export function withCompanionInitiative(state: CompanionState, item: CompanionInitiative): CompanionState {
  const initiatives = [...(state.initiatives ?? []).filter((entry) => entry.id !== item.id), item]
    .sort((left, right) => Date.parse(left.acceptedAt) - Date.parse(right.acceptedAt)).slice(-100);
  return {
    ...state, initiatives,
    ...(item.kind === "home" ? {
      greeting: item.text, greetingID: item.id, greetingAt: item.acceptedAt, greetingOffered: true, topic: item.topic,
    } : {}),
  };
}
