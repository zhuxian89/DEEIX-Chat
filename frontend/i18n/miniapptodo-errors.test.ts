import { afterEach, expect, it } from "vitest";
import { LOCALE_COOKIE_NAME } from "@/i18n/config";
import enErrors from "@/i18n/messages/en-US/errors.json";
import zhErrors from "@/i18n/messages/zh-CN/errors.json";
import { resolveLocalizedErrorMessage } from "@/i18n/resolve-error-message";

const codes = [
  ["identity_required", "identityRequired"],
  ["invalid_request", "invalidRequest"],
  ["invalid_code", "invalidCode"],
  ["version_conflict", "versionConflict"],
  ["snapshot_limit_exceeded", "snapshotLimitExceeded"],
  ["export_limit_exceeded", "exportLimitExceeded"],
] as const;

afterEach(() => {
  document.cookie = `${LOCALE_COOKIE_NAME}=; Max-Age=0; path=/`;
});

it.each([
  ["zh-CN", zhErrors.miniappTodo],
  ["en-US", enErrors.miniappTodo],
] as const)("resolves every TODO business error in %s", (locale, translations) => {
  document.cookie = `${LOCALE_COOKIE_NAME}=${locale}; path=/`;
  for (const [code, key] of codes) {
    expect(translations[key]).toBeTruthy();
    expect(resolveLocalizedErrorMessage(new Error(`errors.miniapp_todo.${code}`))).toBe(translations[key]);
  }
});
