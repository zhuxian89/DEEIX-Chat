import assert from "node:assert/strict";
import test from "node:test";
import { resolveMiniAppConfig } from "./runtime-config";

test("production config accepts HTTPS and trims trailing slashes", () => {
  assert.deepEqual(resolveMiniAppConfig(" https://chat.example.com/// "), { apiBaseUrl: "https://chat.example.com" });
});

test("production config rejects missing and insecure remote URLs", () => {
  assert.throws(() => resolveMiniAppConfig(undefined));
  assert.throws(() => resolveMiniAppConfig("http://chat.example.com"));
  assert.equal(resolveMiniAppConfig("http://127.0.0.1:8080").apiBaseUrl, "http://127.0.0.1:8080");
});
