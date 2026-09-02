import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { AuthLoginResponse, AuthUserResponse, MeResponse } from "@deeix/api-contract";
import { AuthProbeRunner } from "./auth-probe";
import type { ProbeRequest, ProbeTransport, ProbeTransportResponse } from "./transport";

const testUser = {
  displayName: "Validation User",
  username: "validation-user",
} as AuthUserResponse;

function loginResponse(accessToken: string, sessionID = "session-1"): AuthLoginResponse {
  return {
    accessToken,
    expiresAt: "2026-09-01T01:00:00Z",
    refreshExpiresAt: "2026-10-01T01:00:00Z",
    sessionID,
    twoFactorRequired: false,
    user: testUser,
  };
}

function success<T>(data: T, cookies: string[] = []): ProbeTransportResponse<T> {
  return { statusCode: 200, data: { data, errorMsg: "" }, headers: {}, cookies };
}

class QueueTransport implements ProbeTransport {
  readonly requests: ProbeRequest[] = [];

  constructor(private readonly responses: ProbeTransportResponse<unknown>[]) {}

  async request<T>(request: ProbeRequest): Promise<ProbeTransportResponse<T>> {
    this.requests.push(request);
    const response = this.responses.shift();
    if (!response) {
      throw new Error("unexpected request");
    }
    return response as ProbeTransportResponse<T>;
  }
}

describe("AuthProbeRunner", () => {
  it("runs login, bearer verification, and two refresh rotations without exposing secrets", async () => {
    const transport = new QueueTransport([
      success(loginResponse("access-secret-zero"), [
        "deeix_chat_refresh_token=refresh-secret-zero; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax",
      ]),
      success<MeResponse>({ user: testUser }),
      success(loginResponse("access-secret-one"), [
        "deeix_chat_refresh_token=refresh-secret-one; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax",
      ]),
      success<MeResponse>({ user: testUser }),
      success(loginResponse("access-secret-two"), [
        "deeix_chat_refresh_token=refresh-secret-two; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax",
      ]),
      success<MeResponse>({ user: testUser }),
    ]);

    const report = await new AuthProbeRunner(transport).run({ username: "validation-user", password: "password-secret" });

    assert.equal(report.success, true);
    assert.equal(report.steps.length, 7);
    assert.deepEqual(
      report.cookies.map((cookie) => cookie.rotationIndex),
      [1, 2, 3],
    );
    assert.deepEqual(
      transport.requests.map(({ path, accessToken }) => ({ path, accessToken })),
      [
        { path: "/api/v1/auth/login", accessToken: undefined },
        { path: "/api/v1/me", accessToken: "access-secret-zero" },
        { path: "/api/v1/auth/refresh", accessToken: undefined },
        { path: "/api/v1/me", accessToken: "access-secret-one" },
        { path: "/api/v1/auth/refresh", accessToken: undefined },
        { path: "/api/v1/me", accessToken: "access-secret-two" },
      ],
    );

    const serializedReport = JSON.stringify(report);
    for (const secret of ["password-secret", "access-secret", "refresh-secret"]) {
      assert.equal(serializedReport.includes(secret), false);
    }
  });

  it("reports a refresh rejection without retrying or exposing the refresh token", async () => {
    const transport = new QueueTransport([
      success(loginResponse("access-secret-zero"), [
        "deeix_chat_refresh_token=refresh-secret-zero; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax",
      ]),
      success<MeResponse>({ user: testUser }),
      {
        statusCode: 401,
        data: { errorMsg: "invalid refresh token", errorCode: "auth.invalid_refresh_token", requestId: "request-1" },
        headers: {},
        cookies: [],
      },
    ]);

    const report = await new AuthProbeRunner(transport).run({ username: "validation-user", password: "password-secret" });

    assert.equal(report.success, false);
    assert.equal(report.steps.at(-1)?.key, "refresh-1");
    assert.match(report.steps.at(-1)?.detail ?? "", /HTTP 401 \/ auth\.invalid_refresh_token/u);
    assert.equal(JSON.stringify(report).includes("refresh-secret-zero"), false);
  });
});
