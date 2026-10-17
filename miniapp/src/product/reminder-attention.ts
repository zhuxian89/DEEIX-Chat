import type { ReminderEvent, ReminderState } from "@deeix/api-contract";

export function dueReminderEvents(state: ReminderState): ReminderEvent[] {
  const active = new Set(state.plans.filter((plan) => plan.status === "active").map((plan) => plan.id));
  const now = Date.parse(state.serverTime);
  return state.occurrences.filter((event) => active.has(event.reminderID) && event.status === "due" &&
    Date.parse(event.dueAt) <= now && Date.parse(event.notifiedAt) > 0)
    .sort((a, b) => Date.parse(b.dueAt) - Date.parse(a.dueAt) || a.id.localeCompare(b.id));
}

function attentionKey(event: ReminderEvent): string {
  // Snoozing reuses an occurrence ID but represents a new alert when it is due again.
  return `${event.id}:${event.revision}:${event.dueAt}`;
}

export class ReminderAttention {
  private announced = new Set<string>();
  private dismissed = new Set<string>();
  due: ReminderEvent[] = [];

  accept(state: ReminderState): boolean {
    this.due = dueReminderEvents(state);
    const keys = new Set(this.due.map(attentionKey));
    this.dismissed = new Set([...this.dismissed].filter((key) => keys.has(key)));
    const fresh = this.due.some((event) => !this.announced.has(attentionKey(event)));
    for (const key of keys) this.announced.add(key);
    // Keep memory bounded while retaining identities across temporary empty snapshots.
    if (this.announced.size > 1000) this.announced = new Set([...this.announced].slice(-500));
    return fresh;
  }

  get visible(): ReminderEvent[] {
    return this.due.filter((event) => !this.dismissed.has(attentionKey(event)));
  }

  dismiss(): void {
    for (const event of this.visible) this.dismissed.add(attentionKey(event));
  }
}
