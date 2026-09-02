import { extractCookieLines } from "./response-cookies";

const refreshCookieName = "deeix_chat_refresh_token";
const refreshEndpoint = "/api/v1/auth/refresh";

function parseRefreshCookie(cookieLine: string): { action: "set" | "clear"; value: string } | null {
  const parts = cookieLine.split(";");
  const pair = parts.shift()?.trim() ?? "";
  const separatorIndex = pair.indexOf("=");
  if (separatorIndex < 1 || pair.slice(0, separatorIndex).trim() !== refreshCookieName) {
    return null;
  }
  const value = pair.slice(separatorIndex + 1).trim();
  const maxAgePart = parts.find((part) => part.trim().toLowerCase().startsWith("max-age="));
  const maxAge = maxAgePart ? Number(maxAgePart.slice(maxAgePart.indexOf("=") + 1).trim()) : null;
  if (!value || (maxAge !== null && Number.isFinite(maxAge) && maxAge <= 0)) {
    return { action: "clear", value: "" };
  }
  if (!/^[\x21-\x3A\x3C-\x7E]+$/u.test(value)) {
    return { action: "clear", value: "" };
  }
  return { action: "set", value };
}

export class MemoryRefreshCookieBridge {
  private refreshToken: string | null = null;

  clear(): void {
    this.refreshToken = null;
  }

  capture(cookies: readonly string[] | undefined, headers: Readonly<Record<string, unknown>>): void {
    const cookieLines = extractCookieLines(cookies, headers)?.lines ?? [];
    for (const cookieLine of cookieLines) {
      const parsed = parseRefreshCookie(cookieLine);
      if (parsed) {
        this.refreshToken = parsed.action === "set" ? parsed.value : null;
      }
    }
  }

  cookieHeaderFor(path: string): string | null {
    return path === refreshEndpoint && this.refreshToken
      ? `${refreshCookieName}=${this.refreshToken}`
      : null;
  }
}
