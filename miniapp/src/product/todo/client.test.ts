import assert from "node:assert/strict";
import test from "node:test";
import type { ApiRequest, ApiTransport } from "../../platform/transport";
import { TodoClient } from "./client";
import { newDraft } from "./types";

function fixture(codeError?: { errorCode: string; errorMsg: string }) {
  const requests: ApiRequest[] = [];
  let refreshes = 0;
  const auth = { accessToken: "memory-only", expiresAt: new Date(Date.now() + 3600000).toISOString() };
  const transport: ApiTransport = {
    async request<T>(request: ApiRequest) {
      requests.push(request);
      if (request.path.endsWith("/unlock") && codeError) {
        return { statusCode: 400, data: codeError, headers: {}, cookies: [] };
      }
      let data: unknown = { ownerKey: "verified-owner", unlocked: false };
      if (request.path.endsWith("/login")) data = { auth };
      if (request.path.endsWith("/refresh")) { refreshes += 1; data = auth; }
      if (request.path.endsWith("/unlock")) data = { ownerKey: "verified-owner", unlocked: true };
      return { statusCode: 200, data: { data: data as T }, headers: {}, cookies: [] };
    },
  };
  return { requests, auth, client: new TodoClient(transport, async () => "wx-code"), refreshes: () => refreshes };
}
test("TODO startup uses existing WeChat login and only asks the new entry status", async () => {
  const { client, requests } = fixture();
  assert.equal((await client.connect()).ownerKey, "verified-owner");
  assert.deepEqual(requests.map((item) => item.path), ["/api/v1/auth/wechat-miniapp/login", "/api/v1/miniapp-entry/status"]);
  assert.deepEqual(requests[0].body, { code: "wx-code" });
  assert.equal(requests[1].accessToken, "memory-only");
});
test("shared code is trimmed and sent separately from feedback", async () => {
  const { client, requests } = fixture();
  await client.connect(); assert.equal((await client.unlock(" 666 ")).unlocked, true);
  assert.deepEqual(requests.at(-1)?.body, { code: "666" });
  await client.feedback("建议"); assert.deepEqual(requests.at(-1)?.body, { content: "建议" });
});
test("TODO business error codes produce localized messages instead of English fallbacks", async () => {
  const { client } = fixture({ errorCode: "miniapp_todo.invalid_code", errorMsg: "invalid TODO experience code" });
  await client.connect();
  await assert.rejects(() => client.unlock("wrong"), { message: "体验码不正确，请重新输入。" });
});
test("offline retry sends the exact operation identifier and payload", async () => {
  const { client, requests } = fixture();
  await client.connect();
  const task = { ...newDraft(), title: "记下此刻" };
  const operation = { id: "operation", kind: "task.save" as const, entityID: task.id, baseVersion: 0, task };
  await client.sync([operation]); await client.sync([operation]);
  assert.deepEqual(requests.at(-1)?.body, requests.at(-2)?.body);
});
test("concurrent requests share one token refresh", async () => {
  const f = fixture(); await f.client.connect();
  // Connect again with an expiring token: status refreshes before it is used.
  f.auth.expiresAt = new Date(Date.now() + 1000).toISOString();
  await f.client.connect();
  f.auth.expiresAt = new Date(Date.now() + 3600000).toISOString();
  const before = f.refreshes();
  await Promise.all([f.client.status(), f.client.status()]);
  assert.equal(f.refreshes() - before, 1);
  f.client.dispose();
  await assert.rejects(() => f.client.status(), /微信登录/);
});
