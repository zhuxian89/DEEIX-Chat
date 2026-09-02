export type MiniAppRuntimeConfig = {
  apiBaseUrl: string;
};

export class MiniAppConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "MiniAppConfigError";
  }
}

function isLoopback(hostname: string): boolean {
  return hostname === "localhost" || hostname === "127.0.0.1" || hostname === "::1";
}

export function resolveMiniAppConfig(rawValue: string | undefined): MiniAppRuntimeConfig {
  const rawUrl = rawValue?.trim();
  if (!rawUrl) {
    throw new MiniAppConfigError("尚未配置小程序后端地址");
  }
  const match = /^(https?):\/\/([^/?#]+)(\/[^?#]*)?$/iu.exec(rawUrl);
  if (!match) {
    throw new MiniAppConfigError("小程序后端地址必须是完整 URL");
  }
  const [, protocol, authority] = match;
  if (authority.includes("@")) {
    throw new MiniAppConfigError("小程序后端地址不能包含账号或密码");
  }
  const hostname = authority.startsWith("[")
    ? authority.slice(1, authority.indexOf("]"))
    : (authority.split(":", 1)[0] ?? "");
  if (protocol.toLowerCase() !== "https" && !(protocol.toLowerCase() === "http" && isLoopback(hostname))) {
    throw new MiniAppConfigError("真机必须使用 HTTPS 后端地址");
  }
  return { apiBaseUrl: rawUrl.replace(/\/+$/u, "") };
}
