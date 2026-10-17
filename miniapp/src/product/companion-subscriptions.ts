import type { CompanionNotifications } from "@deeix/api-contract";
import type { CompanionClient } from "./companion-client";
import { reminderRequestID } from "./companion-reminders";

type SavedChoices = { mainSwitch?: boolean; itemSettings?: Record<string, string> };
type SubscriptionPlatform = {
  choices(): Promise<SavedChoices>;
  request(templateID: string): Promise<string>;
};
type Consent = { templateID: string; requestID: string; automatic: boolean };
export type SubscriptionState = { settings: CompanionNotifications | null; remembered: boolean; busy: boolean; error: string };

// Shared by the entry, chat and reminder panels for one authenticated client.
// Opening a page only reads preferences. Only a real click calls request().
export class CompanionSubscriptions {
  state: SubscriptionState = { settings: null, remembered: false, busy: false, error: "" };
  private readonly listeners = new Set<(state: SubscriptionState) => void>();
  private reading: Promise<void> | null = null;
  private operation: Promise<boolean> | null = null;
  private pending: Consent | null = null;
  private version = 0;
  private stopping = false;

  constructor(private readonly client: Pick<CompanionClient, "assistant" | "setNotifications">, private readonly platform: SubscriptionPlatform) {}

  subscribe(listener: (state: SubscriptionState) => void): () => void {
    this.listeners.add(listener);
    listener(this.state);
    return () => { this.listeners.delete(listener); };
  }

  private update(patch: Partial<SubscriptionState>): void {
    this.state = { ...this.state, ...patch };
    for (const listener of this.listeners) listener(this.state);
  }

  invalidate(): void {
    this.version++;
    this.reading = null;
    this.update({ remembered: false });
  }

  refresh(): Promise<void> {
    if (this.operation) return Promise.resolve();
    if (this.reading) return this.reading;
    const version = this.version;
    const reading = Promise.all([this.client.assistant(), this.platform.choices()]).then(([assistant, choices]) => {
      if (version !== this.version) return;
      const settings = assistant.notifications;
      const obsolete = this.pending && (this.pending.templateID !== settings.templateID || this.pending.automatic && !settings.enabled);
      if (obsolete) this.pending = null;
      const error = obsolete || this.state.error === "微信授权状态暂未加载，点此重试。" ? "" : this.state.error;
      this.update({ settings, remembered: choices.mainSwitch === true && choices.itemSettings?.[settings.templateID] === "accept", error });
    }).catch(() => {
      if (version === this.version) this.update({ remembered: false, error: "微信授权状态暂未加载，点此重试。" });
    }).finally(() => { if (this.reading === reading) this.reading = null; });
    this.reading = reading;
    return reading;
  }

  // Must be entered synchronously from a click, before awaiting network work.
  request(explicit = false): Promise<boolean> {
    const settings = this.state.settings;
    if (this.operation || this.stopping || !settings?.ready || !settings.templateID) return Promise.resolve(false);
    if (!explicit && (!settings.enabled || !this.state.remembered)) return Promise.resolve(false);
    this.version++;
    this.reading = null;
    this.update({ busy: true, error: "" });
    const consent = this.pending ?? { templateID: settings.templateID, requestID: reminderRequestID(), automatic: !explicit };
    let requested: Promise<string>;
    try {
      // Retrying a saved acceptance only retries our server receipt, never the
      // WeChat call that already consumed the user's click.
      requested = this.pending ? Promise.resolve("accept") : this.platform.request(consent.templateID);
    } catch (cause) {
      requested = Promise.reject(cause);
    }
    const operation = requested.then(async (choice) => {
      if (choice !== "accept") {
        this.update({ remembered: false });
        if (choice === "reject" || choice === "ban") {
          this.update({ settings: { ...settings, enabled: false, credits: 0 } });
          await this.client.setNotifications(false);
        }
        if (explicit) this.update({ error: "未获得微信授权，小程序内提醒仍会保留。" });
        return false;
      }
      this.pending = consent;
      await this.client.setNotifications(true, true, consent.templateID, consent.requestID, consent.automatic);
      this.pending = null;
      return true;
    }).catch(() => {
      this.update({ error: this.pending ? "微信授权已获得，记录暂未同步；下次操作会重试。" : explicit ? "授权未完成，请重试。" : "" });
      return false;
    }).finally(async () => {
      this.operation = null;
      this.update({ busy: false });
      await this.refresh();
    });
    this.operation = operation;
    return operation;
  }

  async disable(): Promise<void> {
    this.stopping = true;
    this.invalidate();
    const settings = this.state.settings;
    if (settings) this.update({ settings: { ...settings, enabled: false, credits: 0 } });
    try {
      // A concurrently finishing grant must settle before the user's close.
      await this.operation;
      this.pending = null;
      this.version++;
      this.reading = null;
      await this.client.setNotifications(false);
    } finally {
      this.stopping = false;
      await this.refresh();
    }
  }
}
