import type { ConversationResponse, MessageResponse, MiniAppUserResponse, PublicModelResponse } from "@deeix/api-contract";
import { Button, Image, ScrollView, Text, Textarea, View } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { useCallback, useEffect, useRef, useState } from "react";
import { Markdown } from "@/components/markdown";
import { resolveMiniAppConfig } from "@/product/runtime-config";
import {
  MiniAppSession,
  MiniAppRequestAbortedError,
  type ConversationMode,
  type ImageGenerationResult,
} from "@/product/session";
import { supportsModelKind } from "@/product/model-catalog";
import "./index.scss";

type Screen = "home" | "chat" | "image" | "models";

type ChatMessage = {
  id: string;
  imageSource?: string;
  role: "assistant" | "user";
  text: string;
};

type PendingImage = {
  fileID: string;
  fileName: string;
  localPath: string;
};

type Presets = {
  chatModel: PublicModelResponse | null;
  imageModel: PublicModelResponse | null;
};

let runtimeConfig: { apiBaseUrl: string } | null = null;
let runtimeConfigError = "";
try {
  runtimeConfig = resolveMiniAppConfig(process.env.TARO_APP_API_BASE_URL);
} catch (error) {
  runtimeConfigError = error instanceof Error ? error.message : "小程序配置无效";
}

function messageFromAPI(message: MessageResponse): ChatMessage | null {
  if (message.role !== "user" && message.role !== "assistant") {
    return null;
  }
  if (!message.content.trim()) {
    return null;
  }
  return { id: message.publicID || String(message.id), role: message.role, text: message.content };
}

function modelDescription(model: PublicModelResponse): string {
  if (model.description.trim()) {
    return model.description.trim();
  }
  if (supportsModelKind(model, "image_gen") && !supportsModelKind(model, "chat")) {
    return "图片生成";
  }
  return "智能对话";
}

export default function HomePage() {
  const sessionRef = useRef<MiniAppSession | null>(null);
  const messageCounter = useRef(0);
  const [screen, setScreen] = useState<Screen>("home");
  const [booting, setBooting] = useState(true);
  const [bootError, setBootError] = useState(runtimeConfigError);
  const [user, setUser] = useState<MiniAppUserResponse | null>(null);
  const [balanceUSD, setBalanceUSD] = useState<number | null>(null);
  const [created, setCreated] = useState(false);
  const [presets, setPresets] = useState<Presets | null>(null);
  const [models, setModels] = useState<PublicModelResponse[]>([]);
  const [conversations, setConversations] = useState<ConversationResponse[]>([]);
  const [currentConversation, setCurrentConversation] = useState<ConversationResponse | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [prompt, setPrompt] = useState("");
  const [running, setRunning] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [pendingImage, setPendingImage] = useState<PendingImage | null>(null);
  const [workspaceError, setWorkspaceError] = useState("");
  const [imageResult, setImageResult] = useState<ImageGenerationResult | null>(null);

  const bootstrap = useCallback(async () => {
    if (!runtimeConfig) {
      setBooting(false);
      return;
    }
    setBootError("");
    setBooting(true);
    sessionRef.current?.dispose();
    const session = new MiniAppSession(runtimeConfig.apiBaseUrl);
    sessionRef.current = session;
    try {
      const result = await session.bootstrap();
      setUser(result.user);
      setBalanceUSD(result.account?.balanceUSD ?? null);
      setCreated(result.created);
      setPresets(result.presets);
      setModels(result.models);
      setConversations(result.conversations);
      setScreen("home");
    } catch (error) {
      session.dispose();
      if (sessionRef.current === session) {
        sessionRef.current = null;
      }
      setBootError(error instanceof Error ? error.message : "微信登录失败，请稍后重试");
    } finally {
      setBooting(false);
    }
  }, []);

  useEffect(() => {
    void bootstrap();
    return () => {
      sessionRef.current?.dispose();
      sessionRef.current = null;
    };
  }, [bootstrap]);

  const refreshConversations = async () => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    try {
      setConversations(await session.listConversations());
    } catch {
      // Keep the last successful list; workspace actions surface their own errors.
    }
  };

  const refreshBalance = async () => {
    try {
      const account = await sessionRef.current?.getBillingAccount();
      if (account) {
        setBalanceUSD(account.balanceUSD);
      }
    } catch {
      // Balance is informational and must not hide a successful model response.
    }
  };

  const enterConversation = async (conversation: ConversationResponse, mode?: ConversationMode) => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    setWorkspaceError("");
    setImageResult(null);
    setPendingImage(null);
    setCurrentConversation(conversation);
    setScreen(mode ?? session.conversationMode(conversation));
    try {
      const history = await session.listMessages(conversation.publicID);
      setMessages(history.map(messageFromAPI).filter((item): item is ChatMessage => item !== null));
    } catch (error) {
      setMessages([]);
      setWorkspaceError(error instanceof Error ? error.message : "加载历史消息失败");
    }
  };

  const createWithModel = async (model: PublicModelResponse, mode: ConversationMode, title: string) => {
    const session = sessionRef.current;
    if (!session || running) {
      return;
    }
    setRunning(true);
    setWorkspaceError("");
    try {
      const conversation = await session.createConversation(model, title);
      setConversations((items) => [conversation, ...items]);
      setMessages([]);
      setPrompt("");
      setPendingImage(null);
      setImageResult(null);
      setCurrentConversation(conversation);
      setScreen(mode);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "创建对话失败");
    } finally {
      setRunning(false);
    }
  };

  const sendChat = async () => {
    const session = sessionRef.current;
    const conversation = currentConversation;
    const content = prompt.trim();
    const attachment = pendingImage;
    if (!session || !conversation || running || uploading || (!content && !attachment)) {
      return;
    }
    setWorkspaceError("");
    messageCounter.current += 1;
    const userID = `local-user-${messageCounter.current}`;
    messageCounter.current += 1;
    const assistantID = `local-assistant-${messageCounter.current}`;
    setMessages((items) => [
      ...items,
      { id: userID, imageSource: attachment?.localPath, role: "user", text: content },
      { id: assistantID, role: "assistant", text: "正在思考…" },
    ]);
    setPrompt("");
    setPendingImage(null);
    setRunning(true);
    try {
      const result = await session.sendChat(conversation, content, (text) => {
        setMessages((items) => items.map((item) => item.id === assistantID ? { ...item, text } : item));
      }, attachment ? [attachment.fileID] : []);
      setMessages((items) => items.map((item) => item.id === assistantID ? { ...item, text: result.text } : item));
      void refreshConversations();
      void refreshBalance();
    } catch (error) {
      const stopped = error instanceof MiniAppRequestAbortedError;
      setMessages((items) => items.map((item) => item.id === assistantID
        ? { ...item, text: stopped ? "本次回复已停止" : "回复失败，请重试" }
        : item));
      if (attachment) {
        setPendingImage((current) => current ?? attachment);
      }
      if (!stopped) {
        setWorkspaceError(error instanceof Error ? error.message : "发送失败，请重试");
      }
    } finally {
      setRunning(false);
    }
  };

  const chooseChatImage = async () => {
    const session = sessionRef.current;
    if (!session || running || uploading) {
      return;
    }
    setWorkspaceError("");
    try {
      const selection = await Taro.chooseMedia({
        count: 1,
        mediaType: ["image"],
        sizeType: ["compressed"],
        sourceType: ["album", "camera"],
      });
      const selected = selection.tempFiles[0];
      if (!selected?.tempFilePath) {
        return;
      }
      const fileName = `wechat-image-${Date.now()}.jpg`;
      setUploading(true);
      const uploaded = await session.uploadChatImage(selected.tempFilePath, fileName);
      setPendingImage({ fileID: uploaded.fileID, fileName: uploaded.fileName || fileName, localPath: selected.tempFilePath });
    } catch (error) {
      const message = error instanceof Error ? error.message : "图片上传失败，请重试";
      if (!/cancel/u.test(message)) {
        setWorkspaceError(message);
      }
    } finally {
      setUploading(false);
    }
  };

  const generateImage = async () => {
    const session = sessionRef.current;
    const conversation = currentConversation;
    const content = prompt.trim();
    if (!session || !conversation || running || !content) {
      return;
    }
    setWorkspaceError("");
    setImageResult(null);
    setPrompt("");
    setPendingImage(null);
    setRunning(true);
    try {
      setImageResult(await session.generateImage(conversation, content));
      void refreshConversations();
      void refreshBalance();
    } catch (error) {
      if (!(error instanceof MiniAppRequestAbortedError)) {
        setWorkspaceError(error instanceof Error ? error.message : "图片生成失败，请重试");
      }
    } finally {
      setRunning(false);
    }
  };

  const goHome = () => {
    sessionRef.current?.abort();
    setRunning(false);
    setWorkspaceError("");
    setCurrentConversation(null);
    setMessages([]);
    setPrompt("");
    setImageResult(null);
    setScreen("home");
    void refreshConversations();
  };

  const saveImage = async () => {
    const source = imageResult?.imageSource;
    if (!source || source.startsWith("data:")) {
      await Taro.showToast({ title: "当前图片暂不能直接保存", icon: "none" });
      return;
    }
    try {
      await Taro.saveImageToPhotosAlbum({ filePath: source });
      await Taro.showToast({ title: "已保存到相册", icon: "success" });
    } catch {
      await Taro.showToast({ title: "保存失败，请检查相册权限", icon: "none" });
    }
  };

  const openPrivacy = async () => {
    try {
      await Taro.openPrivacyContract();
    } catch {
      await Taro.showToast({ title: "请先在微信后台配置隐私保护指引", icon: "none" });
    }
  };

  const openTermsNotice = async () => {
    await Taro.showModal({
      title: "用户协议",
      content: "正式发布前需由运营主体提供并确认完整用户协议。当前构建仅用于部署和真机验收，不应在协议缺失时提交生产审核。",
      showCancel: false,
      confirmText: "我知道了",
    });
  };

  if (booting) {
    return (
      <View className="centerState">
        <View className="brandMark">D</View>
        <Text className="stateTitle">正在进入 DEEIX</Text>
        <Text className="stateHint">微信安全登录中…</Text>
      </View>
    );
  }

  if (bootError || !user || !presets) {
    return (
      <View className="centerState errorState">
        <View className="brandMark">D</View>
        <Text className="stateTitle">暂时无法进入</Text>
        <Text className="stateHint">{bootError || "登录响应不完整"}</Text>
        <Button className="primaryButton retryButton" onClick={bootstrap}>重新登录</Button>
      </View>
    );
  }

  if (screen === "models") {
    return (
      <View className="page">
        <Header title="更多模型" onBack={() => setScreen("home")} />
        <Text className="pageLead">为熟悉模型的用户保留。创建后，本次对话会固定使用所选模型。</Text>
        <View className="modelGrid">
          {models.map((model) => {
            const imageOnly = supportsModelKind(model, "image_gen") && !supportsModelKind(model, "chat");
            return (
              <View
                className="modelCard"
                key={model.platformModelName}
                onClick={() => createWithModel(model, imageOnly ? "image" : "chat", imageOnly ? "AI 生图" : "AI 对话")}
              >
                <View className="modelIcon">{imageOnly ? "画" : "聊"}</View>
                <View className="modelBody">
                  <Text className="modelName">{model.platformModelName}</Text>
                  <Text className="modelDescription">{modelDescription(model)}</Text>
                </View>
                <Text className="chevron">›</Text>
              </View>
            );
          })}
        </View>
        {workspaceError ? <Text className="errorBanner">{workspaceError}</Text> : null}
      </View>
    );
  }

  if ((screen === "chat" || screen === "image") && currentConversation) {
    return (
      <View className="workspace">
        <Header title={screen === "chat" ? "AI 对话" : "AI 生图"} onBack={goHome} />
        <Text className="conversationTitle">{currentConversation.title || (screen === "chat" ? "新对话" : "新图片")}</Text>
        {screen === "chat" ? (
          <ScrollView className="messageList" scrollY enhanced showScrollbar={false} scrollIntoView={messages.length ? `message-${messages.at(-1)?.id}` : undefined}>
            {messages.length === 0 ? (
              <View className="emptyWorkspace">
                <Text className="emptyIcon">✦</Text>
                <Text className="emptyTitle">想聊点什么？</Text>
                <Text className="emptyHint">可以提问、写作、总结或分析问题</Text>
              </View>
            ) : messages.map((message) => (
              <View id={`message-${message.id}`} className={`message message-${message.role}`} key={message.id}>
                <Text className="messageAuthor">{message.role === "user" ? "你" : "DEEIX"}</Text>
                <View className="messageContent">
                  {message.imageSource ? <Image className="messageImage" src={message.imageSource} mode="widthFix" /> : null}
                  {message.text ? <Markdown>{message.text}</Markdown> : null}
                </View>
              </View>
            ))}
          </ScrollView>
        ) : (
          <ScrollView className="imageCanvas" scrollY enhanced showScrollbar={false}>
            {imageResult?.imageSource ? (
              <View className="imageCard">
                <Image className="generatedImage" src={imageResult.imageSource} mode="widthFix" />
                <Text className="imageStatus">{imageResult.status}</Text>
                <Button className="secondaryButton" onClick={saveImage}>保存到相册</Button>
              </View>
            ) : (
              <View className="emptyWorkspace imageEmpty">
                <Text className="emptyIcon">◈</Text>
                <Text className="emptyTitle">描述你想看到的画面</Text>
                <Text className="emptyHint">例如：雨后的未来城市，电影感，暖色灯光</Text>
              </View>
            )}
          </ScrollView>
        )}
        {workspaceError ? <Text className="errorBanner workspaceError">{workspaceError}</Text> : null}
        <View className="composerSafeArea">
          {screen === "chat" && pendingImage ? (
            <View className="pendingImageRow">
              <Image className="pendingImage" src={pendingImage.localPath} mode="aspectFill" />
              <Text className="pendingImageName">{pendingImage.fileName}</Text>
              <Text className="pendingImageRemove" onClick={() => setPendingImage(null)}>移除</Text>
            </View>
          ) : null}
          <View className="composer">
            {screen === "chat" ? (
              <Button className="attachButton" disabled={running || uploading} onClick={chooseChatImage}>
                {uploading ? "上传中" : "图片"}
              </Button>
            ) : null}
            <Textarea
              className="composerInput"
              value={prompt}
              placeholder={screen === "chat" ? "输入消息…" : "描述你想生成的图片…"}
              maxlength={8000}
              autoHeight
              disabled={running || uploading}
              onInput={(event) => setPrompt(event.detail.value)}
            />
            <Button
              className={`sendButton ${running ? "stopSendButton" : ""}`}
              disabled={!running && (uploading || (!prompt.trim() && !pendingImage))}
              onClick={running ? () => sessionRef.current?.abort() : screen === "chat" ? sendChat : generateImage}
            >
              {running ? "停" : screen === "chat" ? "发送" : "生成"}
            </Button>
          </View>
          <Text className="aiNotice">内容由 AI 生成，请注意核实</Text>
        </View>
      </View>
    );
  }

  return (
    <View className="page homePage">
      <View className="homeHeader">
        <View>
          <Text className="eyebrow">DEEIX CHAT</Text>
          <Text className="homeTitle">今天想做什么？</Text>
          <Text className="welcome">{created ? "欢迎第一次来到这里" : `你好，${user.displayName || "朋友"}`}</Text>
          <Text className="accountSummary">
            {user.subscriptionPlanName || user.subscriptionTier || "标准账户"}
            {balanceUSD === null ? "" : ` · 余额 $${balanceUSD.toFixed(4)}`}
          </Text>
        </View>
        <View className="avatar">{(user.displayName || "D").slice(0, 1)}</View>
      </View>

      <View className="quickGrid">
        <View
          className={`quickCard chatQuick ${presets.chatModel ? "" : "quickCardDisabled"}`}
          onClick={() => presets.chatModel
            ? createWithModel(presets.chatModel, "chat", "AI 对话")
            : setWorkspaceError("AI 对话暂不可用，请联系管理员检查默认模型或账号权限")}
        >
          <View className="quickIcon">✦</View>
          <Text className="quickTitle">AI 对话</Text>
          <Text className="quickDescription">提问、写作与分析</Text>
          <Text className="quickAction">{presets.chatModel ? "开始对话 ›" : "暂不可用"}</Text>
        </View>
        <View
          className={`quickCard imageQuick ${presets.imageModel ? "" : "quickCardDisabled"}`}
          onClick={() => presets.imageModel
            ? createWithModel(presets.imageModel, "image", "AI 生图")
            : setWorkspaceError("AI 生图暂不可用，请联系管理员检查默认模型或账号权限")}
        >
          <View className="quickIcon">◈</View>
          <Text className="quickTitle">AI 生图</Text>
          <Text className="quickDescription">把想象变成画面</Text>
          <Text className="quickAction">{presets.imageModel ? "开始创作 ›" : "暂不可用"}</Text>
        </View>
      </View>

      <View className="advancedEntry" onClick={() => setScreen("models")}>
        <View className="advancedIcon">⌘</View>
        <View className="advancedBody">
          <Text className="advancedTitle">更多模型</Text>
          <Text className="advancedHint">为高级用户选择指定模型</Text>
        </View>
        <Text className="chevron">›</Text>
      </View>

      <View className="sectionHeader">
        <Text className="sectionTitle">最近对话</Text>
        <Text className="sectionMeta">{conversations.length} 条</Text>
      </View>
      {conversations.length === 0 ? (
        <View className="emptyHistory">
          <Text className="emptyHistoryTitle">还没有对话</Text>
          <Text className="emptyHistoryHint">从上面的快捷入口开始吧</Text>
        </View>
      ) : (
        <View className="conversationList">
          {conversations.map((conversation) => (
            <View className="conversationRow" key={conversation.publicID} onClick={() => enterConversation(conversation)}>
              <View className="conversationIcon">{sessionRef.current?.conversationMode(conversation) === "image" ? "画" : "聊"}</View>
              <View className="conversationBody">
                <Text className="conversationName">{conversation.title || "未命名对话"}</Text>
                <Text className="conversationMeta">{conversation.messageCount} 条消息</Text>
              </View>
              <Text className="chevron">›</Text>
            </View>
          ))}
        </View>
      )}
      {workspaceError ? <Text className="errorBanner">{workspaceError}</Text> : null}
      <Text className="privacyNote">微信一键登录 · 登录凭据仅保存在本次运行内存</Text>
      <View className="legalLinks">
        <Text onClick={openPrivacy}>隐私保护指引</Text>
        <Text onClick={openTermsNotice}>用户协议</Text>
      </View>
    </View>
  );
}

function Header({ title, onBack }: { title: string; onBack: () => void }) {
  return (
    <View className="topBar">
      <View className="backButton" onClick={onBack}>‹</View>
      <Text className="topBarTitle">{title}</Text>
      <View className="topBarSpacer" />
    </View>
  );
}
