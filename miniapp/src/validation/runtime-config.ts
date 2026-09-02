export type ValidationMode = "offline" | "integration";

export type ValidationEnvironment = {
  TARO_APP_VALIDATION_MODE?: string;
  TARO_APP_API_BASE_URL?: string;
};

export type ValidationConfig = {
  mode: ValidationMode;
  apiBaseUrl: string | null;
};

export class ValidationConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ValidationConfigError";
  }
}

function isLoopback(hostname: string): boolean {
  return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1";
}

function normalizeApiBaseUrl(rawValue: string | undefined): string {
  const rawUrl = rawValue?.trim();
  if (!rawUrl) {
    throw new ValidationConfigError("integration mode requires TARO_APP_API_BASE_URL");
  }

  const match = /^(https?):\/\/([^/?#]+)(\/[^?#]*)?$/iu.exec(rawUrl);
  if (!match) {
    throw new ValidationConfigError("TARO_APP_API_BASE_URL must be an absolute URL");
  }
  const [, protocol, authority] = match;
  if (authority.includes("@")) {
    throw new ValidationConfigError("credentials must not be embedded in TARO_APP_API_BASE_URL");
  }
  const hostname = authority.startsWith("[")
    ? authority.slice(1, authority.indexOf("]"))
    : (authority.split(":", 1)[0] ?? "");
  if (protocol.toLowerCase() !== "https" && !(protocol.toLowerCase() === "http" && isLoopback(hostname))) {
    throw new ValidationConfigError("integration mode requires HTTPS except for loopback development");
  }
  return rawUrl.replace(/\/+$/u, "");
}

export function resolveValidationConfig(environment: ValidationEnvironment): ValidationConfig {
  const rawMode = environment.TARO_APP_VALIDATION_MODE?.trim() || "offline";
  if (rawMode === "offline") {
    return { mode: "offline", apiBaseUrl: null };
  }
  if (rawMode !== "integration") {
    throw new ValidationConfigError("TARO_APP_VALIDATION_MODE must be offline or integration");
  }

  return {
    mode: "integration",
    apiBaseUrl: normalizeApiBaseUrl(environment.TARO_APP_API_BASE_URL),
  };
}
