import type {
  AuthLoginResponse,
  BillingAccountDataResponse,
  ConversationResponse,
  CreateConversationRequest,
  FileObjectResponse,
  FileUploadResponse,
  MessageResponse,
  PublicModelResponse,
  SendMessageRequest,
  WechatminiappLoginResponse,
} from "@deeix/api-contract";
import Taro from "@tarojs/taro";
import { startChunkedJSONRequest, type ChunkedRequestHandle, type StreamEvent } from "@/platform/chunked-transport";
import { buildApiUrl, createTaroTransport, type ApiRequest, type ApiTransport } from "@/platform/transport";
import { resolveFixedModel, selectableModels, supportsModelKind, type MiniAppModelKind } from "./model-catalog";

type PagePayload<T> = { results: T[]; total: number };

type CompletedMessage = {
  attachments?: string;
  content?: string;
};

type CompletedPayload = {
  assistantMessage?: CompletedMessage;
};

type AttachmentSnapshot = {
  file_id?: unknown;
  kind?: unknown;
};

export type ConversationMode = "chat" | "image";

export type BootstrapResult = {
  account: BillingAccountDataResponse["account"] | null;
  conversations: ConversationResponse[];
  created: boolean;
  models: PublicModelResponse[];
  presets: { chatModel: PublicModelResponse | null; imageModel: PublicModelResponse | null };
  user: WechatminiappLoginResponse["auth"]["user"];
};

export type ChatStreamResult = {
  text: string;
};

export type ImageGenerationResult = {
  imageSource: string | null;
  status: string;
};

export class MiniAppRequestAbortedError extends Error {
  constructor() {
    super("request aborted by user");
    this.name = "MiniAppRequestAbortedError";
  }
}

export class MiniAppSession {
  private accessToken = "";
  private accessExpiresAt = 0;
  private activeRequest: ChunkedRequestHandle | null = null;
  private abortRequested = false;
  private models: PublicModelResponse[] = [];
  private readonly transport: ApiTransport;

  constructor(private readonly baseUrl: string) {
    this.transport = createTaroTransport(baseUrl);
  }

  async bootstrap(): Promise<BootstrapResult> {
    this.disposeSessionState();
    const wxLogin = await Taro.login({ timeout: 10_000 });
    if (!wxLogin.code?.trim()) {
      throw new Error("微信登录失败，请重新打开小程序");
    }
    const login = await this.rawRequest<WechatminiappLoginResponse>({
      path: "/api/v1/auth/wechat-miniapp/login",
      method: "POST",
      body: { code: wxLogin.code },
    });
    this.applyAuth(login.auth.accessToken, login.auth.expiresAt);
    this.models = selectableModels(await this.request<PublicModelResponse[]>({ path: "/api/v1/models" }));
    const chatModel = resolveFixedModel(this.models, login.presets.chatModel, "chat");
    const imageModel = resolveFixedModel(this.models, login.presets.imageModel, "image_gen");
    if (!chatModel && !imageModel) {
      throw new Error("AI 服务暂不可用，请联系管理员检查小程序默认模型和用户权限");
    }
    const conversations = await this.listConversations();
    const account = await this.getBillingAccount().catch(() => null);
    return { account, conversations, created: login.created, models: this.models, presets: { chatModel, imageModel }, user: login.auth.user };
  }

  async listConversations(): Promise<ConversationResponse[]> {
    const page = await this.request<PagePayload<ConversationResponse>>({
      path: "/api/v1/conversations?page=1&page_size=50&status=active&starred=all&share=all&project=all",
    });
    return page.results ?? [];
  }

  async listMessages(conversationID: string): Promise<MessageResponse[]> {
    const page = await this.request<PagePayload<MessageResponse>>({
      path: `/api/v1/conversations/${encodeURIComponent(conversationID)}/messages?page=1&page_size=100&tail=true`,
    });
    return page.results ?? [];
  }

  async getBillingAccount(): Promise<BillingAccountDataResponse["account"]> {
    const result = await this.request<BillingAccountDataResponse>({ path: "/api/v1/billing/account" });
    return result.account;
  }

  async createConversation(model: PublicModelResponse, title: string): Promise<ConversationResponse> {
    const body: CreateConversationRequest = { model: model.platformModelName, title };
    return this.request<ConversationResponse>({ path: "/api/v1/conversations", method: "POST", body });
  }

  conversationMode(conversation: ConversationResponse): ConversationMode {
    const model = this.models.find((item) => item.platformModelName === conversation.model);
    return model && supportsModelKind(model, "image_gen") && !supportsModelKind(model, "chat") ? "image" : "chat";
  }

  modelsFor(kind?: MiniAppModelKind): PublicModelResponse[] {
    return kind ? this.models.filter((model) => supportsModelKind(model, kind)) : [...this.models];
  }

  async sendChat(
    conversation: ConversationResponse,
    prompt: string,
    onText: (text: string) => void,
    fileIDs: readonly string[] = [],
  ): Promise<ChatStreamResult> {
    await this.ensureAccessToken();
    const model = conversation.model.trim();
    if (!model) {
      throw new Error("当前对话没有固定模型");
    }
    let streamedText = "";
    const body: SendMessageRequest = {
      content: prompt,
      contentType: fileIDs.length > 0 ? "mixed" : "text",
      fileIDs: fileIDs.length > 0 ? [...fileIDs] : undefined,
      model,
    };
    this.activeRequest = startChunkedJSONRequest({
      url: buildApiUrl(this.baseUrl, `/api/v1/conversations/${conversation.publicID}/messages/stream`),
      accessToken: this.accessToken,
      body,
      onEvent(event) {
        if (event.type === "delta" && typeof event.delta === "string") {
          streamedText += event.delta;
          onText(streamedText);
        }
      },
    });
    try {
      const stream = await this.activeRequest.promise;
      const completed = stream.completedData as CompletedPayload;
      const text = streamedText || completed.assistantMessage?.content?.trim() || "";
      if (!text) {
        throw new Error("AI 已完成响应，但没有返回文本");
      }
      return { text };
    } catch (error) {
      if (this.abortRequested) {
        throw new MiniAppRequestAbortedError();
      }
      throw error;
    } finally {
      this.activeRequest = null;
      this.abortRequested = false;
    }
  }

  async generateImage(conversation: ConversationResponse, prompt: string): Promise<ImageGenerationResult> {
    await this.ensureAccessToken();
    const model = conversation.model.trim();
    if (!model) {
      throw new Error("当前生图会话没有固定模型");
    }
    let imageSource: string | null = null;
    let status = "正在生成图片";
    this.activeRequest = startChunkedJSONRequest({
      url: buildApiUrl(this.baseUrl, `/api/v1/conversations/${conversation.publicID}/media/images/generations/stream`),
      accessToken: this.accessToken,
      body: { prompt, model },
      timeoutMs: 300_000,
      onEvent(event) {
        imageSource = imageSourceFromEvent(event) ?? imageSource;
        if (event.type === "media_status" && typeof event.message === "string") {
          status = event.message;
        }
      },
    });
    try {
      const stream = await this.activeRequest.promise;
      const fileID = imageFileIDFromCompleted(stream.completedData);
      if (fileID) {
        imageSource = await this.downloadFile(fileID).catch(() => imageSource);
      }
      return { imageSource, status: imageSource ? "图片生成完成" : `${status}，但没有收到可显示的图片` };
    } catch (error) {
      if (this.abortRequested) {
        throw new MiniAppRequestAbortedError();
      }
      throw error;
    } finally {
      this.activeRequest = null;
      this.abortRequested = false;
    }
  }

  async uploadChatImage(filePath: string, fileName: string): Promise<FileObjectResponse> {
    await this.ensureAccessToken();
    let upload = await this.performUpload(filePath, fileName);
    if (upload.statusCode === 401) {
      await this.refresh();
      upload = await this.performUpload(filePath, fileName);
    }
    let envelope: { data?: FileUploadResponse; errorCode?: string; errorMsg?: string };
    try {
      envelope = JSON.parse(upload.data) as typeof envelope;
    } catch {
      throw new Error(`图片上传响应无效（HTTP ${upload.statusCode}）`);
    }
    return unwrapResponse(upload.statusCode, envelope).file;
  }

  abort(): void {
    if (this.activeRequest) {
      this.abortRequested = true;
      this.activeRequest.abort();
    }
  }

  dispose(): void {
    this.abort();
    this.disposeSessionState();
    this.transport.dispose?.();
  }

  private disposeSessionState(): void {
    this.accessToken = "";
    this.accessExpiresAt = 0;
    this.models = [];
  }

  private applyAuth(accessToken: string, expiresAt: string): void {
    this.accessToken = accessToken.trim();
    this.accessExpiresAt = Date.parse(expiresAt);
    if (!this.accessToken || !Number.isFinite(this.accessExpiresAt)) {
      throw new Error("登录响应无效，请重试");
    }
  }

  private async ensureAccessToken(): Promise<void> {
    if (!this.accessToken) {
      throw new Error("登录状态已失效，请重新打开小程序");
    }
    if (Date.now() < this.accessExpiresAt - 60_000) {
      return;
    }
    await this.refresh();
  }

  private async refresh(): Promise<void> {
    const refreshed = await this.rawRequest<AuthLoginResponse>({ path: "/api/v1/auth/refresh", method: "POST" });
    this.applyAuth(refreshed.accessToken, refreshed.expiresAt);
  }

  private async request<T>(request: ApiRequest): Promise<T> {
    await this.ensureAccessToken();
    let response = await this.transport.request<T>({ ...request, accessToken: this.accessToken });
    if (response.statusCode === 401) {
      await this.refresh();
      response = await this.transport.request<T>({ ...request, accessToken: this.accessToken });
    }
    return unwrapResponse(response.statusCode, response.data);
  }

  private async rawRequest<T>(request: ApiRequest): Promise<T> {
    const response = await this.transport.request<T>(request);
    return unwrapResponse(response.statusCode, response.data);
  }

  private async downloadFile(fileID: string): Promise<string | null> {
    const download = await Taro.downloadFile({
      url: buildApiUrl(this.baseUrl, `/api/v1/files/${encodeURIComponent(fileID)}/content`),
      header: { Authorization: `Bearer ${this.accessToken}` },
      timeout: 120_000,
    });
    return download.statusCode >= 200 && download.statusCode < 300 && download.tempFilePath
      ? download.tempFilePath
      : null;
  }

  private performUpload(filePath: string, fileName: string) {
    return Taro.uploadFile({
      url: buildApiUrl(this.baseUrl, "/api/v1/files"),
      filePath,
      name: "file",
      formData: { purpose: "chat_attachment", fileName },
      header: { Authorization: `Bearer ${this.accessToken}` },
      timeout: 120_000,
    });
  }
}

function unwrapResponse<T>(statusCode: number, envelope: { data?: T; errorCode?: string; errorMsg?: string }): T {
  if (statusCode < 200 || statusCode >= 300 || envelope.errorMsg) {
    const code = envelope.errorCode ? `（${envelope.errorCode}）` : "";
    throw new Error(envelope.errorMsg ? `${envelope.errorMsg}${code}` : `请求失败（HTTP ${statusCode}）`);
  }
  if (typeof envelope.data === "undefined" || envelope.data === null) {
    throw new Error("服务响应缺少数据");
  }
  return envelope.data;
}

function imageSourceFromEvent(event: StreamEvent): string | null {
  if (event.type !== "media_image_delta" || typeof event.b64_json !== "string") {
    return null;
  }
  const base64 = event.b64_json.trim();
  if (!base64) {
    return null;
  }
  if (base64.startsWith("data:image/")) {
    return base64;
  }
  const mimeType = typeof event.mime_type === "string" && event.mime_type.startsWith("image/")
    ? event.mime_type
    : "image/png";
  return `data:${mimeType};base64,${base64}`;
}

function imageFileIDFromCompleted(value: unknown): string | null {
  const rawAttachments = (value as CompletedPayload)?.assistantMessage?.attachments;
  if (!rawAttachments) {
    return null;
  }
  try {
    const attachments = JSON.parse(rawAttachments) as AttachmentSnapshot[];
    const image = attachments.find((item) => item.kind === "image" && typeof item.file_id === "string");
    return typeof image?.file_id === "string" ? image.file_id : null;
  } catch {
    return null;
  }
}
