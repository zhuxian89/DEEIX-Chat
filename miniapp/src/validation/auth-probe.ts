import type { AuthLoginRequest, AuthLoginResponse, MeResponse } from "@deeix/api-contract";
import { CookieObserver, extractCookieLines, type CookieObservation } from "./cookie-observer";
import type { ApiEnvelope, ProbeRequest, ProbeTransport, ProbeTransportResponse } from "./transport";

export type AuthProbeStepKey = "login" | "cookie-login" | "bearer-before-refresh" | "refresh-1" | "bearer-after-refresh-1" | "refresh-2" | "bearer-after-refresh-2";
export type AuthProbeStepStatus = "passed" | "warning" | "failed";

export type AuthProbeStep = {
  key: AuthProbeStepKey;
  label: string;
  status: AuthProbeStepStatus;
  detail: string;
  durationMs: number;
};

export type AuthProbeReport = {
  success: boolean;
  startedAt: string;
  completedAt: string;
  accountLabel: string | null;
  sessionId: string | null;
  steps: AuthProbeStep[];
  cookies: CookieObservation[];
};

export class ProbeHttpError extends Error {
  constructor(
    readonly statusCode: number,
    readonly errorCode: string | undefined,
    readonly requestId: string | undefined,
    message: string,
  ) {
    super(message);
    this.name = "ProbeHttpError";
  }
}

type RequestResult<T> = {
  data: T;
  cookies: CookieObservation[];
};

function headerValue(headers: Readonly<Record<string, unknown>>, name: string): string | undefined {
  const entry = Object.entries(headers).find(([headerName]) => headerName.toLowerCase() === name.toLowerCase());
  return typeof entry?.[1] === "string" ? entry[1] : undefined;
}

function safeErrorDetail(error: unknown): string {
  if (error instanceof ProbeHttpError) {
    const code = error.errorCode ? ` / ${error.errorCode}` : "";
    const requestId = error.requestId ? ` / request ${error.requestId}` : "";
    return `HTTP ${error.statusCode}${code}${requestId}: ${error.message}`;
  }
  if (error instanceof Error) {
    return error.message || error.name;
  }
  return "unknown request failure";
}

function toRequestId<T>(response: ProbeTransportResponse<T>, envelope: ApiEnvelope<T>): string | undefined {
  return envelope.requestId || headerValue(response.headers, "x-request-id");
}

export class AuthProbeRunner {
  private accessToken = "";
  private sessionId: string | null = null;
  private accountLabel: string | null = null;
  private readonly cookieObserver = new CookieObserver();

  constructor(private readonly transport: ProbeTransport) {}

  private async request<T>(request: ProbeRequest): Promise<RequestResult<T>> {
    const response = await this.transport.request<T>(request);
    const cookieInput = extractCookieLines(response.cookies, response.headers);
    const cookies = cookieInput ? this.cookieObserver.observe(cookieInput.lines, cookieInput.source) : [];
    const envelope = response.data;

    if (response.statusCode < 200 || response.statusCode >= 300 || envelope.errorMsg) {
      throw new ProbeHttpError(
        response.statusCode,
        envelope.errorCode,
        toRequestId(response, envelope),
        envelope.errorMsg?.trim() || "request failed",
      );
    }
    if (typeof envelope.data === "undefined" || envelope.data === null) {
      throw new ProbeHttpError(response.statusCode, envelope.errorCode, toRequestId(response, envelope), "response data is missing");
    }
    return { data: envelope.data, cookies };
  }

  private async executeStep<T>(
    steps: AuthProbeStep[],
    key: AuthProbeStepKey,
    label: string,
    operation: () => Promise<{ value: T; detail: string; cookies?: CookieObservation[] }>,
  ): Promise<T> {
    const startedAt = Date.now();
    try {
      const result = await operation();
      steps.push({ key, label, status: "passed", detail: result.detail, durationMs: Date.now() - startedAt });
      return result.value;
    } catch (error) {
      steps.push({ key, label, status: "failed", detail: safeErrorDetail(error), durationMs: Date.now() - startedAt });
      throw error;
    }
  }

  async run(credentials: AuthLoginRequest): Promise<AuthProbeReport> {
    const startedAt = new Date();
    const steps: AuthProbeStep[] = [];
    const cookieObservations: CookieObservation[] = [];

    try {
      await this.executeStep(steps, "login", "账号登录", async () => {
        const result = await this.request<AuthLoginResponse>({ path: "/api/v1/auth/login", method: "POST", body: credentials });
        cookieObservations.push(...result.cookies);
        if (result.data.twoFactorRequired) {
          throw new Error("该测试账号启用了两步验证；当前认证探针需要使用未启用 2FA 的专用测试账号");
        }
        if (!result.data.accessToken.trim()) {
          throw new Error("登录响应未返回 access token");
        }
        this.accessToken = result.data.accessToken;
        this.sessionId = result.data.sessionID || null;
        this.accountLabel = result.data.user.displayName || result.data.user.username || null;
        return { value: result.data, detail: `登录成功，会话 ${this.sessionId ?? "unknown"}` };
      });

      const loginCookie = cookieObservations.find((cookie) => cookie.name === "deeix_chat_refresh_token" && cookie.action === "set");
      steps.push({
        key: "cookie-login",
        label: "登录 Cookie 观察",
        status: loginCookie ? "passed" : "warning",
        detail: loginCookie
          ? `观察到 HttpOnly=${loginCookie.httpOnly} Secure=${loginCookie.secure} SameSite=${loginCookie.sameSite ?? "unknown"} Path=${loginCookie.path ?? "unknown"}`
          : "登录成功，但响应未暴露 refresh Cookie；继续用 refresh 请求验证微信原生 Cookie jar",
        durationMs: 0,
      });

      await this.executeStep(steps, "bearer-before-refresh", "首次 Bearer 鉴权", async () => {
        const result = await this.request<MeResponse>({ path: "/api/v1/me", accessToken: this.accessToken });
        this.accountLabel = result.data.user.displayName || result.data.user.username || this.accountLabel;
        return { value: result.data, detail: `鉴权成功，用户 ${this.accountLabel ?? "unknown"}` };
      });

      await this.refresh(steps, cookieObservations, 1, "refresh-1");
      await this.verifyBearer(steps, "bearer-after-refresh-1", "第一次轮换后 Bearer 鉴权");
      await this.refresh(steps, cookieObservations, 2, "refresh-2");
      await this.verifyBearer(steps, "bearer-after-refresh-2", "第二次轮换后 Bearer 鉴权");

      return this.report(startedAt, steps, cookieObservations, true);
    } catch {
      return this.report(startedAt, steps, cookieObservations, false);
    } finally {
      this.accessToken = "";
    }
  }

  private async refresh(
    steps: AuthProbeStep[],
    cookieObservations: CookieObservation[],
    expectedRotation: number,
    key: "refresh-1" | "refresh-2",
  ): Promise<void> {
    await this.executeStep(steps, key, `Refresh 第 ${expectedRotation} 次轮换`, async () => {
      const result = await this.request<AuthLoginResponse>({ path: "/api/v1/auth/refresh", method: "POST" });
      cookieObservations.push(...result.cookies);
      if (!result.data.accessToken.trim()) {
        throw new Error("refresh 响应未返回 access token");
      }
      this.accessToken = result.data.accessToken;
      this.sessionId = result.data.sessionID || this.sessionId;
      const refreshCookie = result.cookies.find((cookie) => cookie.name === "deeix_chat_refresh_token" && cookie.action === "set");
      return {
        value: undefined,
        detail: refreshCookie
          ? `轮换成功；Cookie rotation=${refreshCookie.rotationIndex}，令牌值未记录`
          : "轮换成功；响应未暴露 Cookie 值，微信原生 Cookie jar 已通过行为验证",
      };
    });
  }

  private async verifyBearer(
    steps: AuthProbeStep[],
    key: "bearer-after-refresh-1" | "bearer-after-refresh-2",
    label: string,
  ): Promise<void> {
    await this.executeStep(steps, key, label, async () => {
      const result = await this.request<MeResponse>({ path: "/api/v1/me", accessToken: this.accessToken });
      return { value: undefined, detail: `轮换后的 access token 可访问 /me（${result.data.user.username}）` };
    });
  }

  private report(
    startedAt: Date,
    steps: AuthProbeStep[],
    cookies: CookieObservation[],
    success: boolean,
  ): AuthProbeReport {
    return {
      success,
      startedAt: startedAt.toISOString(),
      completedAt: new Date().toISOString(),
      accountLabel: this.accountLabel,
      sessionId: this.sessionId,
      steps,
      cookies,
    };
  }
}

export const AUTH_PROBE_SECRET_MARKERS = ["password", "accessToken", "refreshToken"] as const;
