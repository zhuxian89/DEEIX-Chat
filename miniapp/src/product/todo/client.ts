import type { AuthLoginResponse, WechatminiappLoginResponse } from "@deeix/api-contract";
import zhErrors from "../../../../frontend/i18n/messages/zh-CN/errors.json";
import type { ApiRequest, ApiTransport } from "../../platform/transport";
import type { EntryStatus, Operation, Snapshot, SyncResult, TaskPage, TodoExport } from "./types";

const todoErrorMessages: Record<string, string> = {
  "miniapp_todo.identity_required": zhErrors.miniappTodo.identityRequired,
  "miniapp_todo.invalid_request": zhErrors.miniappTodo.invalidRequest,
  "miniapp_todo.invalid_code": zhErrors.miniappTodo.invalidCode,
  "miniapp_todo.version_conflict": zhErrors.miniappTodo.versionConflict,
  "miniapp_todo.snapshot_limit_exceeded": zhErrors.miniappTodo.snapshotLimitExceeded,
  "miniapp_todo.export_limit_exceeded": zhErrors.miniappTodo.exportLimitExceeded,
};

export class TodoClient {
  private accessToken = "";
  private expiresAt = 0;
  private refreshing?: Promise<void>;
  constructor(private readonly transport: ApiTransport, private readonly loginCode: () => Promise<string>) {}
  async connect(): Promise<EntryStatus> {
    this.transport.dispose?.();
    this.accessToken = "";
    const code = await this.loginCode();
    if (!code.trim()) throw new Error("微信登录失败，请重试");
    const login = await this.raw<WechatminiappLoginResponse>({
      path: "/api/v1/auth/wechat-miniapp/login", method: "POST", body: { code },
    });
    this.applyAuth(login.auth);
    return this.status();
  }
  status(): Promise<EntryStatus> { return this.request({ path: "/api/v1/miniapp-entry/status" }); }
  snapshot(): Promise<Snapshot> { return this.request({ path: "/api/v1/miniapp-todo/snapshot" }); }
  sync(operations: Operation[]): Promise<SyncResult> {
    return this.request({ path: "/api/v1/miniapp-todo/sync", method: "POST", body: { operations } });
  }
  query(q: string, completed = "", page = 1, listID = ""): Promise<TaskPage> {
    return this.request({ path: `/api/v1/miniapp-todo/tasks?q=${encodeURIComponent(q)}&completed=${completed}&page=${page}&pageSize=50&listID=${encodeURIComponent(listID)}` });
  }
  export(format: "text" | "csv", listID = "", completed = ""): Promise<TodoExport> {
    return this.request({ path: `/api/v1/miniapp-todo/export?format=${format}&listID=${encodeURIComponent(listID)}&completed=${completed}` });
  }
  feedback(content: string): Promise<EntryStatus> {
    return this.request({ path: "/api/v1/miniapp-todo/feedback", method: "POST", body: { content: content.trim() } });
  }
  dispose(): void { this.accessToken = ""; this.expiresAt = 0; this.transport.dispose?.(); }
  private applyAuth(auth: Pick<AuthLoginResponse, "accessToken" | "expiresAt">): void {
    if (!auth.accessToken?.trim() || !Number.isFinite(Date.parse(auth.expiresAt))) throw new Error("登录响应无效，请重试");
    this.accessToken = auth.accessToken.trim(); this.expiresAt = Date.parse(auth.expiresAt);
  }
  private refresh(): Promise<void> {
    if (!this.refreshing) {
      this.refreshing = this.raw<AuthLoginResponse>({ path: "/api/v1/auth/refresh", method: "POST" })
        .then((auth) => this.applyAuth(auth)).finally(() => { this.refreshing = undefined; });
    }
    return this.refreshing;
  }
  private async request<T>(request: ApiRequest): Promise<T> {
    if (!this.accessToken) throw new Error("请重新连接微信登录");
    if (Date.now() >= this.expiresAt - 60_000) await this.refresh();
    let response = await this.transport.request<T>({ ...request, accessToken: this.accessToken });
    if (response.statusCode === 401) {
      await this.refresh();
      response = await this.transport.request<T>({ ...request, accessToken: this.accessToken });
    }
    return unwrap(response);
  }
  private async raw<T>(request: ApiRequest): Promise<T> { return unwrap(await this.transport.request<T>(request)); }
}

function unwrap<T>(response: { statusCode: number; data: { data?: T; errorCode?: string; errorMsg?: string } }): T {
  if (response.statusCode < 200 || response.statusCode >= 300 || response.data.errorMsg) {
    throw new Error(todoErrorMessages[response.data.errorCode ?? ""] || response.data.errorMsg || `请求失败（${response.statusCode}）`);
  }
  if (response.data.data == null) throw new Error("服务响应缺少数据");
  return response.data.data;
}
