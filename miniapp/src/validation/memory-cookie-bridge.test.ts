import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { MemoryRefreshCookieBridge } from "./memory-cookie-bridge";

const loginCookie =
  "deeix_chat_refresh_token=refresh-secret-zero; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax";

describe("MemoryRefreshCookieBridge", () => {
  it("sends the captured refresh cookie only to the refresh endpoint", () => {
    const bridge = new MemoryRefreshCookieBridge();
    bridge.capture([loginCookie], {});

    assert.equal(
      bridge.cookieHeaderFor("/api/v1/auth/refresh"),
      "deeix_chat_refresh_token=refresh-secret-zero",
    );
    assert.equal(bridge.cookieHeaderFor("/api/v1/me"), null);
    assert.equal(bridge.cookieHeaderFor("/api/v1/auth/login"), null);
  });

  it("replaces the in-memory token after every rotation", () => {
    const bridge = new MemoryRefreshCookieBridge();
    bridge.capture([loginCookie], {});
    bridge.capture(
      ["deeix_chat_refresh_token=refresh-secret-one; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax"],
      {},
    );

    assert.equal(
      bridge.cookieHeaderFor("/api/v1/auth/refresh"),
      "deeix_chat_refresh_token=refresh-secret-one",
    );
  });

  it("reads the Set-Cookie header fallback and clears expired state", () => {
    const bridge = new MemoryRefreshCookieBridge();
    bridge.capture([], { "Set-Cookie": loginCookie });
    assert.equal(
      bridge.cookieHeaderFor("/api/v1/auth/refresh"),
      "deeix_chat_refresh_token=refresh-secret-zero",
    );

    bridge.capture([], {
      "set-cookie": "deeix_chat_refresh_token=; Path=/api/v1/auth; Max-Age=0; HttpOnly; Secure; SameSite=Lax",
    });
    assert.equal(bridge.cookieHeaderFor("/api/v1/auth/refresh"), null);
  });

  it("ignores unrelated cookies", () => {
    const bridge = new MemoryRefreshCookieBridge();
    bridge.capture(["analytics=value; Path=/"], {});

    assert.equal(bridge.cookieHeaderFor("/api/v1/auth/refresh"), null);
  });

  it("drops the previous token when a rotation contains unsafe characters", () => {
    const bridge = new MemoryRefreshCookieBridge();
    bridge.capture([loginCookie], {});
    bridge.capture(["deeix_chat_refresh_token=unsafe\r\nvalue; Path=/api/v1/auth"], {});

    assert.equal(bridge.cookieHeaderFor("/api/v1/auth/refresh"), null);
  });

  it("clears the token when the owning session is disposed", () => {
    const bridge = new MemoryRefreshCookieBridge();
    bridge.capture([loginCookie], {});

    bridge.clear();

    assert.equal(bridge.cookieHeaderFor("/api/v1/auth/refresh"), null);
  });
});
