import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { resolveValidationConfig, ValidationConfigError } from "./runtime-config";

describe("resolveValidationConfig", () => {
  it("defaults to an offline configuration without a backend", () => {
    assert.deepEqual(resolveValidationConfig({}), {
      mode: "offline",
      apiBaseUrl: null,
    });
  });

  it("ignores a backend URL while explicitly offline", () => {
    assert.deepEqual(
      resolveValidationConfig({
        TARO_APP_VALIDATION_MODE: "offline",
        TARO_APP_API_BASE_URL: "https://should-not-be-used.example",
      }),
      { mode: "offline", apiBaseUrl: null },
    );
  });

  it("normalizes an HTTPS integration URL", () => {
    assert.deepEqual(
      resolveValidationConfig({
        TARO_APP_VALIDATION_MODE: "integration",
        TARO_APP_API_BASE_URL: "https://api.example.com/",
      }),
      { mode: "integration", apiBaseUrl: "https://api.example.com" },
    );
  });

  it("allows HTTP only for loopback development", () => {
    assert.deepEqual(
      resolveValidationConfig({
        TARO_APP_VALIDATION_MODE: "integration",
        TARO_APP_API_BASE_URL: "http://127.0.0.1:8080/",
      }),
      { mode: "integration", apiBaseUrl: "http://127.0.0.1:8080" },
    );
  });

  const invalidConfigurations = [
    ["unknown mode", { TARO_APP_VALIDATION_MODE: "production" }],
    ["missing URL", { TARO_APP_VALIDATION_MODE: "integration" }],
    ["embedded credentials", { TARO_APP_VALIDATION_MODE: "integration", TARO_APP_API_BASE_URL: "https://user:password@example.com" }],
    ["insecure remote URL", { TARO_APP_VALIDATION_MODE: "integration", TARO_APP_API_BASE_URL: "http://api.example.com" }],
  ] as const;

  for (const [name, environment] of invalidConfigurations) {
    it(`rejects ${name}`, () => {
      assert.throws(() => resolveValidationConfig(environment), ValidationConfigError);
    });
  }
});
