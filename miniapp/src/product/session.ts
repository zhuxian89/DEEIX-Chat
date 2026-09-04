import type {
  AuthLoginResponse,
  BillingAccountDataResponse,
  BillingOverviewDataResponse,
  BillingOverviewResponse,
  ConversationDeleteResponse,
  ConversationResponse,
  CreateConversationRequest,
  FileObjectResponse,
  FileUploadResponse,
  MessageResponse,
  PublicModelResponse,
  RenameConversationRequest,
  ToolListResponse,
  UsageMonthlyResponse,
  WechatminiappLoginResponse,
} from "@deeix/api-contract";
import Taro from "@tarojs/taro";
import { startChunkedJSONRequest, type ChunkedRequestHandle, type StreamEvent } from "@/platform/chunked-transport";
import {
  createClientRunID,
  isWechatRequestInterrupted,
  runRecoverableStream,
} from "@/platform/generation-recovery";
import { buildApiUrl, createTaroTransport, type ApiRequest, type ApiTransport } from "@/platform/transport";
import {
  cancelConversationRunPath,
  chatMessageStreamPath,
  createChatRunRequest,
  createImageRunRequest,
  deleteConversationPath,
  imageEditStreamPath,
  imageGenerationStreamPath,
  renameConversationPath,
  resumeConversationRunPath,
} from "./conversation-contract";
import {
  applyConversationStreamEvent,
  emptyConversationStreamState,
  type ConversationStreamState,
} from "./conversation-stream-state";
import {
  type ConversationProcessTrace,
  normalizeConversationProcessTrace,
} from "./conversation-trace";
import { resolveFixedModel, selectableModels, supportsModelKind, type MiniAppModelKind } from "./model-catalog";
import {
  type ModelOptionPolicyResponse,
  type NativeToolDefinition,
  resolveModelRequestOptions,
} from "./model-options";
import type { ImageSubmitTask } from "./image-task";
import { removeNativeWebSearchOptions, resolveExaNetworkToolIDs } from "./network-search";

export type PagePayload<T> = { results: T[]; total: number };

type CompletedMessage = {
  attachments?: string;
  content?: string;
  processTrace?: unknown;
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
  conversationTotal: number;
  created: boolean;
  models: PublicModelResponse[];
  networkSearchAvailable: boolean;
  presets: { chatModel: PublicModelResponse | null; imageModel: PublicModelResponse | null };
  user: WechatminiappLoginResponse["auth"]["user"];
};

export type ChatStreamResult = {
  processTrace?: ConversationProcessTrace;
  text: string;
};

export type ChatGenerationProgress = Pick<ConversationStreamState, "processTrace" | "status" | "text">;

export type ImageGenerationResult = {
  imageFileID?: string;
  imageSource: string | null;
  status: string;
};

export type ImageGenerationProgress = ImageGenerationResult;

export type ResumeGenerationProgress = ConversationStreamState;

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
  private activeRunID = "";
  private cancelRequestedRunID = "";
  private cancelSettlementTimer: ReturnType<typeof setTimeout> | null = null;
  private appForeground = true;
  private abortRequested = false;
  private disposed = false;
  private readonly foregroundWaiters = new Set<() => void>();
  private models: PublicModelResponse[] = [];
  private nativeToolCatalog: NativeToolDefinition[] = [];
  private exaNetworkToolIDs: number[] = [];
  private readonly transport: ApiTransport;
  private readonly handleAppHide = () => {
    this.appForeground = false;
  };
  private readonly handleAppShow = () => {
    this.appForeground = true;
    this.resolveForegroundWaiters();
  };

  constructor(private readonly baseUrl: string) {
    this.transport = createTaroTransport(baseUrl);
    Taro.onAppHide(this.handleAppHide);
    Taro.onAppShow(this.handleAppShow);
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
    const [models, modelOptionPolicy, mcpTools] = await Promise.all([
      this.request<PublicModelResponse[]>({ path: "/api/v1/models" }),
      this.request<ModelOptionPolicyResponse>({ path: "/api/v1/settings/model-option-policy" })
        .catch((): null => null),
      this.request<ToolListResponse>({ path: "/api/v1/mcp/tools" })
        .catch((): null => null),
    ]);
    this.models = selectableModels(models);
    this.nativeToolCatalog = modelOptionPolicy?.nativeTools ?? [];
    this.exaNetworkToolIDs = resolveExaNetworkToolIDs(mcpTools?.results ?? []);
    const chatModel = resolveFixedModel(this.models, login.presets.chatModel, "chat");
    const imageModel = resolveFixedModel(this.models, login.presets.imageModel, "image_gen");
    if (!chatModel && !imageModel) {
      throw new Error("AI 服务暂不可用，请联系管理员检查小程序默认模型和用户权限");
    }
    const conversationPage = await this.listConversationPage();
    const account = await this.getBillingAccount().catch(() => null);
    return {
      account,
      conversations: conversationPage.results,
      conversationTotal: conversationPage.total,
      created: login.created,
      models: this.models,
      networkSearchAvailable: this.exaNetworkToolIDs.length > 0,
      presets: { chatModel, imageModel },
      user: login.auth.user,
    };
  }

  async listConversationPage(page = 1, pageSize = 50): Promise<PagePayload<ConversationResponse>> {
    const result = await this.request<PagePayload<ConversationResponse>>({
      path: `/api/v1/conversations?page=${Math.max(1, Math.floor(page))}&page_size=${Math.max(1, Math.floor(pageSize))}&status=active&starred=all&share=all&project=all`,
    });
    return { results: result.results ?? [], total: result.total ?? 0 };
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

  async getBillingOverview(): Promise<BillingOverviewResponse> {
    const result = await this.request<BillingOverviewDataResponse>({ path: "/api/v1/billing/overview" });
    return result.overview;
  }

  async listMonthlyUsage(months = 6): Promise<UsageMonthlyResponse[]> {
    const normalizedMonths = Math.min(24, Math.max(1, Math.floor(months)));
    return this.request<UsageMonthlyResponse[]>({ path: `/api/v1/billing/usage/monthly?months=${normalizedMonths}` });
  }

  async createConversation(model: PublicModelResponse, title: string): Promise<ConversationResponse> {
    const body: CreateConversationRequest = { model: model.platformModelName, title };
    return this.request<ConversationResponse>({ path: "/api/v1/conversations", method: "POST", body });
  }

  async getConversation(conversationID: string): Promise<ConversationResponse> {
    return this.request<ConversationResponse>({
      path: `/api/v1/conversations/${encodeURIComponent(conversationID)}`,
    });
  }

  async renameConversation(conversationID: string, title: string): Promise<ConversationResponse> {
    const body: RenameConversationRequest = { title: title.trim() };
    return this.request<ConversationResponse>({
      path: renameConversationPath(conversationID),
      method: "PATCH",
      body,
    });
  }

  async deleteConversation(conversationID: string, deleteFiles = false): Promise<ConversationDeleteResponse> {
    return this.request<ConversationDeleteResponse>({
      path: deleteConversationPath(conversationID, deleteFiles),
      method: "DELETE",
    });
  }

  conversationMode(conversation: ConversationResponse): ConversationMode {
    const model = this.models.find((item) => item.platformModelName === conversation.model);
    return model && (supportsModelKind(model, "image_gen") || supportsModelKind(model, "image_edit")) &&
      !supportsModelKind(model, "chat") ? "image" : "chat";
  }

  modelsFor(kind?: MiniAppModelKind): PublicModelResponse[] {
    return kind ? this.models.filter((model) => supportsModelKind(model, kind)) : [...this.models];
  }

  async sendChat(
    conversation: ConversationResponse,
    selectedModelName: string,
    prompt: string,
    onProgress: (progress: ChatGenerationProgress) => void,
    fileIDs: readonly string[] = [],
    useNetworkSearch = false,
  ): Promise<ChatStreamResult> {
    await this.ensureAccessToken();
    const model = selectedModelName.trim();
    if (!model) {
      throw new Error("请选择本次回复使用的模型");
    }
    const clientRunID = createClientRunID();
    let streamState = emptyConversationStreamState();
    const selectedToolIDs = useNetworkSearch ? this.exaNetworkToolIDs : [];
    const body = createChatRunRequest(
      prompt,
      model,
      clientRunID,
      fileIDs,
      this.requestOptionsForModel(model, selectedToolIDs.length > 0),
      selectedToolIDs,
    );
    try {
      const stream = await this.runConversationStream({
        runID: clientRunID,
        start: (onEvent) => startChunkedJSONRequest({
          url: buildApiUrl(this.baseUrl, chatMessageStreamPath(conversation.publicID)),
          accessToken: this.accessToken,
          body,
          onEvent,
        }),
        onEvent: (event) => {
          streamState = applyConversationStreamEvent(streamState, event);
          if ([
            "delta",
            "file_proc",
            "moderation_checking",
            "process_update",
            "rag_search",
            "upstream_think_delta",
          ].includes(event.type)) {
            onProgress(streamState);
          }
        },
        onInterrupted: () => {
          streamState.status = this.appForeground
            ? "连接被微信中断，正在恢复回复"
            : "已切到后台，返回后继续接收回复";
          onProgress(streamState);
        },
        onResuming: () => {
          streamState.status = "正在恢复回复";
          onProgress(streamState);
        },
      });
      const completed = stream.completedData as CompletedPayload;
      const text = completed.assistantMessage?.content?.trim() || streamState.text;
      if (!text) {
        throw new Error("AI 已完成响应，但没有返回文本");
      }
      return {
        processTrace: normalizeConversationProcessTrace(completed.assistantMessage?.processTrace) ?? streamState.processTrace,
        text,
      };
    } catch (error) {
      if (this.abortRequested) {
        throw new MiniAppRequestAbortedError();
      }
      throw error;
    } finally {
      this.finishRun(clientRunID);
    }
  }

  async generateImage(
    conversation: ConversationResponse,
    selectedModelName: string,
    task: ImageSubmitTask,
    prompt: string,
    onProgress?: (progress: ImageGenerationProgress) => void,
    fileIDs: readonly string[] = [],
  ): Promise<ImageGenerationResult> {
    await this.ensureAccessToken();
    const model = selectedModelName.trim();
    if (!model) {
      throw new Error("请选择本次图片任务使用的模型");
    }
    const editing = task === "image_edit";
    let streamState = { ...emptyConversationStreamState(), status: editing ? "正在编辑图片" : "正在生成图片" };
    const clientRunID = createClientRunID();
    const generationUrl = buildApiUrl(
      this.baseUrl,
      editing ? imageEditStreamPath(conversation.publicID) : imageGenerationStreamPath(conversation.publicID),
    );
    const consumeEvent = (event: StreamEvent) => {
      streamState = applyConversationStreamEvent(streamState, event);
      onProgress?.({ imageSource: streamState.imageSource, status: streamState.status });
    };
    try {
      const stream = await this.runConversationStream({
        runID: clientRunID,
        start: (onEvent) => startChunkedJSONRequest({
          url: generationUrl,
          accessToken: this.accessToken,
          body: createImageRunRequest(
            prompt,
            model,
            clientRunID,
            fileIDs,
            this.requestOptionsForModel(model),
          ),
          timeoutMs: 300_000,
          onEvent,
        }),
        onEvent: consumeEvent,
        onInterrupted: () => {
          streamState.status = this.appForeground
            ? "连接被微信中断，正在恢复生图进度"
            : "已切到后台，返回后继续接收图片";
          onProgress?.({ imageSource: streamState.imageSource, status: streamState.status });
        },
        onResuming: () => {
          streamState.status = "正在恢复生图进度";
          onProgress?.({ imageSource: streamState.imageSource, status: streamState.status });
        },
      });
      const fileID = imageFileIDFromCompleted(stream.completedData);
      if (fileID) {
        streamState.imageSource = await this.downloadMessageImage(fileID).catch(() => streamState.imageSource);
      }
      return {
        imageFileID: fileID || undefined,
        imageSource: streamState.imageSource,
        status: streamState.imageSource
          ? editing ? "图片编辑完成" : "图片生成完成"
          : `${streamState.status}，但没有收到可显示的图片`,
      };
    } catch (error) {
      if (this.abortRequested) {
        throw new MiniAppRequestAbortedError();
      }
      throw error;
    } finally {
      this.finishRun(clientRunID);
    }
  }

  async resumeGeneration(
    runID: string,
    initial: Pick<ResumeGenerationProgress, "imageSource" | "processTrace" | "text">,
    onProgress: (progress: ResumeGenerationProgress) => void,
  ): Promise<ResumeGenerationProgress> {
    await this.ensureAccessToken();
    const normalizedRunID = runID.trim();
    let streamState = emptyConversationStreamState(initial.text, initial.imageSource, initial.processTrace);
    const consumeEvent = (event: StreamEvent) => {
      streamState = applyConversationStreamEvent(streamState, event);
      onProgress(streamState);
    };
    try {
      const stream = await this.runConversationStream({
        runID: normalizedRunID,
        start: (onEvent) => startChunkedJSONRequest({
          url: buildApiUrl(this.baseUrl, resumeConversationRunPath(normalizedRunID, 0)),
          accessToken: this.accessToken,
          method: "GET",
          timeoutMs: 300_000,
          onEvent,
        }),
        onEvent: consumeEvent,
        onInterrupted: () => {
          streamState.status = this.appForeground ? "连接被微信中断，正在恢复" : "返回小程序后继续接收";
          onProgress(streamState);
        },
        onResuming: () => {
          streamState.status = "正在恢复生成进度";
          onProgress(streamState);
        },
      });
      const completed = stream.completedData as CompletedPayload;
      const completedText = completed.assistantMessage?.content?.trim();
      if (completedText) {
        streamState.text = completedText;
      }
      const fileID = imageFileIDFromCompleted(stream.completedData);
      if (fileID) {
        streamState.imageSource = await this.downloadMessageImage(fileID).catch(() => streamState.imageSource);
      }
      return streamState;
    } catch (error) {
      if (this.abortRequested) {
        throw new MiniAppRequestAbortedError();
      }
      throw error;
    } finally {
      this.finishRun(normalizedRunID);
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

  async downloadMessageImage(fileID: string): Promise<string | null> {
    await this.ensureAccessToken();
    let download = await this.performDownload(fileID);
    if (download.statusCode === 401) {
      await this.refresh();
      download = await this.performDownload(fileID);
    }
    return download.statusCode >= 200 && download.statusCode < 300 && download.tempFilePath
      ? download.tempFilePath
      : null;
  }

  async cancelActiveGeneration(): Promise<boolean> {
    const runID = this.activeRunID.trim();
    if (!runID) {
      return false;
    }
    if (this.cancelRequestedRunID === runID) {
      return true;
    }
    this.cancelRequestedRunID = runID;
    try {
      const result = await this.request<{ canceled: boolean }>({
        path: cancelConversationRunPath(runID),
        method: "POST",
      });
      if (result.canceled && this.activeRunID === runID) {
        this.clearCancelSettlementTimer();
        this.cancelSettlementTimer = setTimeout(() => {
          if (this.activeRunID === runID) {
            this.abort();
          }
        }, 25_000);
      }
      if (!result.canceled && this.cancelRequestedRunID === runID) {
        this.cancelRequestedRunID = "";
      }
      return result.canceled;
    } catch (error) {
      if (this.cancelRequestedRunID === runID) {
        this.cancelRequestedRunID = "";
      }
      if (this.activeRunID === runID) {
        this.abort();
      }
      throw error;
    }
  }

  abort(): void {
    if (this.activeRequest) {
      this.abortRequested = true;
      this.activeRequest.abort();
    }
  }

  dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    this.abort();
    this.clearCancelSettlementTimer();
    this.resolveForegroundWaiters();
    Taro.offAppHide(this.handleAppHide);
    Taro.offAppShow(this.handleAppShow);
    this.disposeSessionState();
    this.transport.dispose?.();
  }

  private waitUntilAppForeground(): Promise<void> {
    if (this.appForeground || this.abortRequested || this.disposed) {
      return Promise.resolve();
    }
    return new Promise((resolve) => {
      this.foregroundWaiters.add(resolve);
    });
  }

  private resolveForegroundWaiters(): void {
    for (const resolve of this.foregroundWaiters) {
      resolve();
    }
    this.foregroundWaiters.clear();
  }

  private async runConversationStream(options: {
    onEvent(event: StreamEvent): void;
    onInterrupted(): void;
    onResuming(): void;
    runID: string;
    start(onEvent: (event: StreamEvent) => void): ChunkedRequestHandle;
  }) {
    const runID = options.runID.trim();
    this.abortRequested = false;
    this.cancelRequestedRunID = "";
    this.clearCancelSettlementTimer();
    this.activeRunID = runID;
    return runRecoverableStream({
      start: () => options.start(options.onEvent),
      resume: async (afterSeq) => {
        await this.ensureAccessToken();
        return startChunkedJSONRequest({
          url: buildApiUrl(this.baseUrl, resumeConversationRunPath(runID, afterSeq)),
          accessToken: this.accessToken,
          method: "GET",
          timeoutMs: 300_000,
          onEvent: options.onEvent,
        });
      },
      shouldResume: isWechatRequestInterrupted,
      waitUntilResume: () => this.waitUntilAppForeground(),
      isCanceled: () => this.abortRequested || this.disposed,
      onHandle: (handle) => {
        this.activeRequest = handle;
      },
      onInterrupted: options.onInterrupted,
      onResuming: options.onResuming,
    });
  }

  private finishRun(runID: string): void {
    if (this.activeRunID === runID) {
      this.activeRunID = "";
      this.activeRequest = null;
      this.abortRequested = false;
      this.cancelRequestedRunID = "";
      this.clearCancelSettlementTimer();
    }
  }

  private clearCancelSettlementTimer(): void {
    if (this.cancelSettlementTimer !== null) {
      clearTimeout(this.cancelSettlementTimer);
      this.cancelSettlementTimer = null;
    }
  }

  private disposeSessionState(): void {
    this.accessToken = "";
    this.accessExpiresAt = 0;
    this.models = [];
    this.nativeToolCatalog = [];
    this.exaNetworkToolIDs = [];
  }

  private requestOptionsForModel(
    platformModelName: string,
    removeNativeSearch = false,
  ): Record<string, unknown> | undefined {
    const model = this.models.find((item) => item.platformModelName === platformModelName);
    const options = model ? resolveModelRequestOptions(model, this.nativeToolCatalog) : undefined;
    return removeNativeSearch ? removeNativeWebSearchOptions(options) : options;
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

  private performDownload(fileID: string) {
    return Taro.downloadFile({
      url: buildApiUrl(this.baseUrl, `/api/v1/files/${encodeURIComponent(fileID)}/content`),
      header: { Authorization: `Bearer ${this.accessToken}` },
      timeout: 120_000,
    });
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
