import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createLoginRequest, hasUsableAccessToken } from "./api-contract";

describe("DEEIX API contract smoke", () => {
  it("creates a login payload from generated contract types", () => {
    assert.deepEqual(createLoginRequest({ username: "tester", password: "secret-value" }), {
      username: "tester",
      password: "secret-value",
    });
  });

  it("accepts only non-empty access tokens", () => {
    assert.equal(hasUsableAccessToken({ accessToken: "token" }), true);
    assert.equal(hasUsableAccessToken({ accessToken: "   " }), false);
  });
});
