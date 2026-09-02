export type CookieObservationSource = "response.cookies" | "set-cookie-header";

export function extractCookieLines(
  cookies: readonly string[] | undefined,
  headers: Readonly<Record<string, unknown>>,
): { lines: string[]; source: CookieObservationSource } | null {
  if (cookies && cookies.length > 0) {
    return { lines: [...cookies], source: "response.cookies" };
  }
  const entry = Object.entries(headers).find(([name]) => name.toLowerCase() === "set-cookie");
  const value = entry?.[1];
  if (typeof value === "string" && value.trim()) {
    return { lines: [value], source: "set-cookie-header" };
  }
  if (Array.isArray(value)) {
    const lines = value.filter((item): item is string => typeof item === "string" && item.trim().length > 0);
    return lines.length > 0 ? { lines, source: "set-cookie-header" } : null;
  }
  return null;
}
