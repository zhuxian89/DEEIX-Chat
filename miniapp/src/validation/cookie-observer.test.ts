import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { CookieObserver, extractCookieLines } from "./cookie-observer";

describe("CookieObserver", () => {
  it("records refresh rotation metadata without retaining cookie values", () => {
    const observer = new CookieObserver();
    const first = observer.observe(
      ["deeix_chat_refresh_token=refresh-secret-one; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax"],
      "response.cookies",
    );
    const second = observer.observe(
      ["deeix_chat_refresh_token=refresh-secret-two; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Lax"],
      "response.cookies",
    );

    assert.deepEqual(first[0], {
      name: "deeix_chat_refresh_token",
      action: "set",
      httpOnly: true,
      secure: true,
      sameSite: "Lax",
      path: "/api/v1/auth",
      rotationIndex: 1,
      source: "response.cookies",
    });
    assert.equal(second[0]?.rotationIndex, 2);
    const serialized = JSON.stringify([first, second]);
    assert.equal(serialized.includes("refresh-secret-one"), false);
    assert.equal(serialized.includes("refresh-secret-two"), false);
  });

  it("recognizes a cleared cookie without incrementing rotation", () => {
    const observer = new CookieObserver();
    observer.observe(["deeix_chat_refresh_token=value; HttpOnly"], "response.cookies");
    const cleared = observer.observe(["deeix_chat_refresh_token=; Max-Age=-1; HttpOnly"], "response.cookies");
    assert.equal(cleared[0]?.action, "clear");
    assert.equal(cleared[0]?.rotationIndex, 1);
  });

  it("prefers the official response cookies collection over headers", () => {
    assert.deepEqual(extractCookieLines(["preferred=value"], { "Set-Cookie": "fallback=value" }), {
      lines: ["preferred=value"],
      source: "response.cookies",
    });
  });
});
