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

// One opportunity per page visit. Invalidating a visit never aborts the user's
// ordinary chat; it only prevents a delayed proactive candidate from appearing.
export class CompanionInitiativeVisit {
  readonly id = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
  private attempted = false;
  private revision = 0;

  invalidate(): void { this.revision += 1; }

  async consider(
    client: Pick<CompanionClient, "prepareInitiative" | "acceptInitiative">,
    kind: "home" | "idle",
    afterMessageID: number,
    isEligible: () => boolean,
    onAccepted: (message: CompanionInitiative) => void,
  ): Promise<void> {
    if (this.attempted || !isEligible()) return;
    this.attempted = true;
    const revision = this.revision;
    const isCurrent = () => this.revision === revision && isEligible();
    try {
      const candidate = await client.prepareInitiative(kind, this.id, afterMessageID);
      if (!candidate || !isCurrent() || !(Date.parse(candidate.expiresAt) > Date.now())) return;
      const accepted = await client.acceptInitiative(candidate.id, this.id);
      if (accepted?.id === candidate.id && Date.parse(accepted.acceptedAt) > 0 && isCurrent()) onAccepted(accepted);
    } catch { /* Silence is a valid outcome; proactive work never blocks chatting. */ }
  }
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
