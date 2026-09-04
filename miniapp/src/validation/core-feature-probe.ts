import type {
  ConversationResponse,
  CreateConversationRequest,
  AuthLoginResponse,
  PublicModelResponse,
  SendMessageRequest,
} from "@deeix/api-contract";
import Taro from "@tarojs/taro";
import { startChunkedJSONRequest, type ChunkedRequestHandle, type StreamEvent } from "./chunked-transport";
import { buildApiUrl, createTaroTransport, type ProbeRequest, type ProbeTransport } from "./transport";

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

export type FeatureSessionCredentials = {
  password: string;
  username: string;
};

export type FeatureSessionInfo = {
  chatModel: string | null;
  imageModel: string | null;
};

export type ChatResult = {
  conversationID: string;
  eventCount: number;
  firstChunkMs: number | null;
  lastSeq: number;
  model: string;
  text: string;
};

export type ImageResult = {
  conversationID: string;
  eventCount: number;
  firstChunkMs: number | null;
  imageSource: string | null;
  lastSeq: number;
  model: string;
  status: string;
};

function parseKinds(rawValue: string): string[] {
  try {
    const parsed = JSON.parse(rawValue) as unknown;
    return Array.isArray(parsed)
      ? parsed.filter((value): value is string => typeof value === "string").map((value) => value.trim().toLowerCase())
      : [];
  } catch {
    return [];
  }
}

function selectModel(models: readonly PublicModelResponse[], kind: "chat" | "image_gen"): PublicModelResponse | null {
  return models.find((item) => item.platformModelName.trim() && parseKinds(item.kindsJSON).includes(kind)) ?? null;
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
  const completed = value as CompletedPayload;
  const rawAttachments = completed.assistantMessage?.attachments;
  if (!rawAttachments) {
    return null;
  }
  try {
    const attachments = JSON.parse(rawAttachments) as unknown;
    if (!Array.isArray(attachments)) {
      return null;
    }
    const image = (attachments as AttachmentSnapshot[]).find(
      (item) => item.kind === "image" && typeof item.file_id === "string" && item.file_id.trim(),
    );
    return typeof image?.file_id === "string" ? image.file_id : null;
  } catch {
    return null;
  }
}

export class CoreFeatureSession {
  private accessToken = "";
  private activeRequest: ChunkedRequestHandle | null = null;
  private chatConversationID: string | null = null;
  private chatModel = "";
  private imageConversationID: string | null = null;
  private imageModel = "";
  private readonly transport: ProbeTransport;

  constructor(private readonly baseUrl: string) {
    this.transport = createTaroTransport(baseUrl);
  }

  async connect(credentials: FeatureSessionCredentials): Promise<FeatureSessionInfo> {
    this.disposeSessionState();
    const login = await this.request<AuthLoginResponse>({
      path: "/api/v1/auth/login",
      method: "POST",
      body: credentials,
    });
    if (login.twoFactorRequired) {
      throw new Error("交互会话需要使用未启用 2FA 的专用测试账号");
    }
    this.accessToken = login.accessToken.trim();
    if (!this.accessToken) {
      throw new Error("登录响应未返回 access token");
    }

    const models = await this.request<PublicModelResponse[]>({ path: "/api/v1/models" });
    this.chatModel = selectModel(models, "chat")?.platformModelName ?? "";
    this.imageModel = selectModel(models, "image_gen")?.platformModelName ?? "";
    if (!this.chatModel && !this.imageModel) {
      throw new Error("当前账号没有可用的对话或生图模型");
    }
    return { chatModel: this.chatModel || null, imageModel: this.imageModel || null };
  }

  async sendChat(prompt: string, onText?: (text: string) => void): Promise<ChatResult> {
    this.requireAuthenticated();
    if (!this.chatModel) {
      throw new Error("当前账号没有可用的对话模型");
    }
    if (!this.chatConversationID) {
      const conversation = await this.createConversation({ model: this.chatModel, title: "小程序交互对话" });
      this.chatConversationID = conversation.publicID;
    }

    let streamedText = "";
    this.activeRequest = startChunkedJSONRequest({
      url: buildApiUrl(this.baseUrl, `/api/v1/conversations/${this.chatConversationID}/messages/stream`),
      accessToken: this.accessToken,
      body: {
        content: prompt,
        contentType: "text",
        knowledgeBaseIDs: [],
        model: this.chatModel,
      } satisfies SendMessageRequest,
      onEvent(event) {
        if (event.type === "delta" && typeof event.delta === "string") {
          streamedText += event.delta;
          onText?.(streamedText);
        }
      },
    });
    try {
      const stream = await this.activeRequest.promise;
      const completed = stream.completedData as CompletedPayload;
      const text = streamedText || completed.assistantMessage?.content?.trim() || "";
      if (!text) {
        throw new Error("对话流完成但没有返回文本");
      }
      return {
        conversationID: this.chatConversationID,
        eventCount: stream.eventCount,
        firstChunkMs: stream.firstChunkMs,
        lastSeq: stream.lastSeq,
        model: this.chatModel,
        text,
      };
    } finally {
      this.activeRequest = null;
    }
  }

  async generateImage(prompt: string): Promise<ImageResult> {
    this.requireAuthenticated();
    if (!this.imageModel) {
      throw new Error("当前账号没有可用的生图模型");
    }
    if (!this.imageConversationID) {
      const conversation = await this.createConversation({ model: this.imageModel, title: "小程序交互生图" });
      this.imageConversationID = conversation.publicID;
    }

    let imageSource: string | null = null;
    let status = "等待生成";
    this.activeRequest = startChunkedJSONRequest({
      url: buildApiUrl(this.baseUrl, `/api/v1/conversations/${this.imageConversationID}/media/images/generations/stream`),
      accessToken: this.accessToken,
      body: { prompt, model: this.imageModel },
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
      const imageFileID = imageFileIDFromCompleted(stream.completedData);
      if (imageFileID) {
        try {
          const download = await Taro.downloadFile({
            url: buildApiUrl(this.baseUrl, `/api/v1/files/${encodeURIComponent(imageFileID)}/content`),
            header: { Authorization: `Bearer ${this.accessToken}` },
            timeout: 120_000,
          });
          if (download.statusCode >= 200 && download.statusCode < 300 && download.tempFilePath) {
            imageSource = download.tempFilePath;
          }
        } catch {
          // Keep the inline stream image when downloadFile is unavailable or not allowlisted.
        }
      }
      return {
        conversationID: this.imageConversationID,
        eventCount: stream.eventCount,
        firstChunkMs: stream.firstChunkMs,
        imageSource,
        lastSeq: stream.lastSeq,
        model: this.imageModel,
        status: imageSource ? "图片数据接收成功" : `${status}；流已完成但未返回可显示图片`,
      };
    } finally {
      this.activeRequest = null;
    }
  }

  abort(): void {
    this.activeRequest?.abort();
  }

  dispose(): void {
    this.abort();
    this.disposeSessionState();
    this.transport.dispose?.();
  }

  private disposeSessionState(): void {
    this.accessToken = "";
    this.chatConversationID = null;
    this.chatModel = "";
    this.imageConversationID = null;
    this.imageModel = "";
  }

  private requireAuthenticated(): void {
    if (!this.accessToken) {
      throw new Error("请先登录并加载模型");
    }
  }

  private createConversation(body: CreateConversationRequest): Promise<ConversationResponse> {
    return this.request<ConversationResponse>({ path: "/api/v1/conversations", method: "POST", body });
  }

  private async request<T>(request: ProbeRequest): Promise<T> {
    const response = await this.transport.request<T>({ ...request, accessToken: this.accessToken || undefined });
    if (response.statusCode < 200 || response.statusCode >= 300 || response.data.errorMsg) {
      const code = response.data.errorCode ? ` / ${response.data.errorCode}` : "";
      throw new Error(`HTTP ${response.statusCode}${code}: ${response.data.errorMsg || "request failed"}`);
    }
    if (typeof response.data.data === "undefined" || response.data.data === null) {
      throw new Error("API 响应缺少 data");
    }
    return response.data.data;
  }
}
