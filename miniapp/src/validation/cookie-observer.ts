import { extractCookieLines, type CookieObservationSource } from "@/platform/response-cookies";

export { extractCookieLines, type CookieObservationSource };

export type CookieObservation = {
  name: string;
  action: "set" | "clear";
  httpOnly: boolean;
  secure: boolean;
  sameSite: string | null;
  path: string | null;
  rotationIndex: number;
  source: CookieObservationSource;
};

function splitAttribute(part: string): [string, string] {
  const separatorIndex = part.indexOf("=");
  if (separatorIndex < 0) {
    return [part.trim().toLowerCase(), ""];
  }
  return [part.slice(0, separatorIndex).trim().toLowerCase(), part.slice(separatorIndex + 1).trim()];
}

function parseCookieLine(cookieLine: string): Omit<CookieObservation, "rotationIndex" | "source"> | null {
  const parts = cookieLine.split(";");
  const [cookieName, cookieValue] = splitAttribute(parts.shift() ?? "");
  if (!cookieName) {
    return null;
  }

  const attributes = new Map(parts.map(splitAttribute));
  const maxAge = Number(attributes.get("max-age"));
  const action = cookieValue.length === 0 || (Number.isFinite(maxAge) && maxAge <= 0) ? "clear" : "set";

  return {
    name: cookieName,
    action,
    httpOnly: attributes.has("httponly"),
    secure: attributes.has("secure"),
    sameSite: attributes.get("samesite") || null,
    path: attributes.get("path") || null,
  };
}

export class CookieObserver {
  private readonly rotations = new Map<string, number>();

  observe(cookieLines: readonly string[], source: CookieObservationSource): CookieObservation[] {
    const observations: CookieObservation[] = [];
    for (const cookieLine of cookieLines) {
      const parsed = parseCookieLine(cookieLine);
      if (!parsed) {
        continue;
      }

      const previousRotation = this.rotations.get(parsed.name) ?? 0;
      const rotationIndex = parsed.action === "set" ? previousRotation + 1 : previousRotation;
      this.rotations.set(parsed.name, rotationIndex);
      observations.push({ ...parsed, rotationIndex, source });
    }
    return observations;
  }
}
