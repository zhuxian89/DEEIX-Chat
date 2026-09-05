import type {
  BillingOverviewResponse,
  ConversationResponse,
  DailyCheckinStatusResponse,
  MiniAppUserResponse,
  PublicModelResponse,
  PublicSharedConversationResponse,
  UsageMonthlyResponse,
  UserMemoryResponse,
} from "@deeix/api-contract";
import { Button, Image, Input, KeyboardAccessory, ScrollView, Text, Textarea, View } from "@tarojs/components";
import Taro, { useDidShow, useRouter, useShareAppMessage } from "@tarojs/taro";
import { type ComponentProps, type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { DailyCheckinEntry, DailyCheckinWheel } from "@/components/daily-checkin-wheel";
import { Markdown } from "@/components/markdown";
import { ConversationTrace } from "@/components/conversation-trace";
import { resolveMiniAppConfig } from "@/product/runtime-config";
import {
  MiniAppSession,
  MiniAppRequestAbortedError,
  type ConversationMode,
} from "@/product/session";
import { modelsForKind, resolveSelectedModel, supportsModelKind } from "@/product/model-catalog";
import { formatAccountDate, formatUSD, periodUsagePercent } from "@/product/account-summary";
import {
  imageModelOptions,
  resolveImageEditModel,
  resolveImageSubmitDecision,
} from "@/product/image-task";
import { composerKeyboardStyle } from "@/product/keyboard-layout";
import { MINIAPP_BUILD_VERSION } from "@/product/build-version";
import { nextChatBottomScrollTop, shouldReleaseChatAutoFollow } from "@/product/chat-auto-scroll";
import { wheelRotationForPrize } from "@/product/daily-checkin";
import { miniAppSharedConversationPath } from "@/product/retention-contract";
import {
  conversationSwipeOffset,
  conversationRefreshPageSize,
  mergeConversationPage,
  settleConversationSwipe,
} from "@/product/conversation-list";
import {
  conversationTitleFromFirstUserMessage,
  isPlaceholderConversationTitle,
  preserveOptimisticConversationTitle,
} from "@/product/conversation-metadata";
import {
  applyImageProgress,
  type ConversationMessage,
  createPendingImageTurn,
  latestVisibleMessages,
  messageFromAPI,
} from "@/product/message-timeline";
import "./index.scss";

type Screen = "home" | "chat" | "image" | "account" | "checkin" | "history" | "memories" | "shared";

type ConversationListItem = Pick<
  ConversationResponse,
  "isStarred" | "messageCount" | "publicID" | "title" | "updatedAt"
> & Partial<Pick<ConversationResponse, "model">>;

type MemoryEditor = {
  memoryKey: string;
  originalKey?: string;
  value: string;
};

type PreparedShare = {
  shareID: string;
  title: string;
};

type HistoryPageResult = {
  hasMore: boolean;
  results: ConversationListItem[];
};

const HISTORY_LIST_PAGE_SIZE = 50;
const HISTORY_SEARCH_PAGE_SIZE = 30;
const MAX_PREFERENCE_MEMORIES = 20;

function historyEmptyTitle(query: string, favoritesOnly: boolean): string {
  if (query.trim()) {
    return "没有找到相关对话";
  }
  return favoritesOnly ? "还没有收藏的对话" : "还没有对话";
}

function historyEmptyHint(favoritesOnly: boolean): string {
  return favoritesOnly ? "收藏重要会话，稍后可以快速找到" : "换个关键词再试试";
}

async function fetchHistoryPage(
  session: MiniAppSession,
  query: string,
  page: number,
  favoritesOnly: boolean,
): Promise<HistoryPageResult> {
  if (query) {
    const result = await session.searchConversations(query, page, HISTORY_SEARCH_PAGE_SIZE);
    return {
      hasMore: result.hasMore,
      results: favoritesOnly
        ? result.results.filter((conversation) => conversation.isStarred)
        : result.results,
    };
  }

  const result = await session.listConversationPage(
    page,
    HISTORY_LIST_PAGE_SIZE,
    favoritesOnly ? "starred" : "all",
  );
  return {
    hasMore: page === 1
      ? result.results.length < result.total
      : page * HISTORY_LIST_PAGE_SIZE < result.total,
    results: result.results,
  };
}

function mergeHistoryPage(
  current: readonly ConversationListItem[],
  incoming: readonly ConversationListItem[],
): ConversationListItem[] {
  const existingIDs = new Set(current.map((conversation) => conversation.publicID));
  return [...current, ...incoming.filter((conversation) => !existingIDs.has(conversation.publicID))];
}

function chatGenerationFailureText(currentText: string, stopped: boolean): string {
  if (stopped) {
    return currentText ? `${currentText}\n\n（本次回复已停止）` : "本次回复已停止";
  }
  return currentText
    ? `${currentText}\n\n（回复中断，可重新进入会话恢复）`
    : "回复失败，请重试";
}

function imageLoadingPhaseClass(status: string | undefined): string {
  if (status?.includes("排队")) {
    return "imageLoadingFrameQueued";
  }
  if (status?.includes("保存") || status?.includes("加载")) {
    return "imageLoadingFrameSaving";
  }
  return "imageLoadingFrameRunning";
}

type PendingImage = {
  fileID: string;
  fileName: string;
  localPath: string;
};

type Presets = {
  chatModel: PublicModelResponse | null;
  imageModel: PublicModelResponse | null;
};

type TouchPoint = {
  clientX?: number;
  clientY?: number;
  pageX?: number;
  pageY?: number;
};

type TouchEventLike = {
  changedTouches?: TouchPoint[];
  nativeEvent?: TouchEventLike;
  touches?: TouchPoint[];
};

type ScrollEventLike = {
  detail?: { scrollTop?: number };
  nativeEvent?: ScrollEventLike;
};

type ViewTouchHandler = NonNullable<ComponentProps<typeof View>["onTouchStart"]>;

function readTouchPoint(event: unknown, changed = false): { x: number; y: number } | null {
  const candidate = event as TouchEventLike;
  const touches = changed
    ? candidate.changedTouches ?? candidate.nativeEvent?.changedTouches
    : candidate.touches ?? candidate.nativeEvent?.touches;
  const touch = touches?.[0];
  const x = Number(touch?.clientX ?? touch?.pageX);
  const y = Number(touch?.clientY ?? touch?.pageY);
  return Number.isFinite(x) && Number.isFinite(y) ? { x, y } : null;
}

function readScrollTop(event: unknown): number | null {
  const candidate = event as ScrollEventLike;
  const value = Number(candidate.detail?.scrollTop ?? candidate.nativeEvent?.detail?.scrollTop);
  return Number.isFinite(value) ? value : null;
}

function conversationActionWidthPx(): number {
  return Taro.getWindowInfo().windowWidth * 432 / 750;
}

let runtimeConfig: { apiBaseUrl: string } | null = null;
let runtimeConfigError = "";
try {
  runtimeConfig = resolveMiniAppConfig(process.env.TARO_APP_API_BASE_URL);
} catch (error) {
  runtimeConfigError = error instanceof Error ? error.message : "小程序配置无效";
}

export default function HomePage() {
  const router = useRouter();
  const incomingShareID = typeof router.params.share === "string" ? router.params.share.trim() : "";
  const sessionRef = useRef<MiniAppSession | null>(null);
  const messageCounter = useRef(0);
  const historyLoadCounter = useRef(0);
  const dailyCheckinRevealTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isDailyCheckinClaimingRef = useRef(false);
  const [screen, setScreen] = useState<Screen>("home");
  const [booting, setBooting] = useState(true);
  const [bootError, setBootError] = useState(runtimeConfigError);
  const [user, setUser] = useState<MiniAppUserResponse | null>(null);
  const [balanceUSD, setBalanceUSD] = useState<number | null>(null);
  const [dailyCheckin, setDailyCheckin] = useState<DailyCheckinStatusResponse | null>(null);
  const [isDailyCheckinClaiming, setIsDailyCheckinClaiming] = useState(false);
  const [showDailyCheckinResult, setShowDailyCheckinResult] = useState(false);
  const [dailyCheckinRotation, setDailyCheckinRotation] = useState(0);
  const [presets, setPresets] = useState<Presets | null>(null);
  const [models, setModels] = useState<PublicModelResponse[]>([]);
  const [selectedChatModel, setSelectedChatModel] = useState<PublicModelResponse | null>(null);
  const [selectedImageModel, setSelectedImageModel] = useState<PublicModelResponse | null>(null);
  const [conversations, setConversations] = useState<ConversationResponse[]>([]);
  const [conversationPage, setConversationPage] = useState(1);
  const [conversationTotal, setConversationTotal] = useState(0);
  const [currentConversation, setCurrentConversation] = useState<ConversationResponse | null>(null);
  const [messages, setMessages] = useState<ConversationMessage[]>([]);
  const [prompt, setPrompt] = useState("");
  const [running, setRunning] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [pendingImage, setPendingImage] = useState<PendingImage | null>(null);
  const [workspaceError, setWorkspaceError] = useState("");
  const [keyboardHeight, setKeyboardHeight] = useState(0);
  const [loadingMoreConversations, setLoadingMoreConversations] = useState(false);
  const [managingConversationID, setManagingConversationID] = useState("");
  const [openSwipeConversationID, setOpenSwipeConversationID] = useState("");
  const [billingOverview, setBillingOverview] = useState<BillingOverviewResponse | null>(null);
  const [monthlyUsage, setMonthlyUsage] = useState<UsageMonthlyResponse[]>([]);
  const [accountLoading, setAccountLoading] = useState(false);
  const [accountError, setAccountError] = useState("");
  const [modelPickerMode, setModelPickerMode] = useState<ConversationMode | null>(null);
  const [networkSearchAvailable, setNetworkSearchAvailable] = useState(false);
  const [networkSearchEnabled, setNetworkSearchEnabled] = useState(false);
  const chatAutoFollowRef = useRef(true);
  const chatTouchingRef = useRef(false);
  const chatScrollTopRef = useRef(0);
  const [chatAutoFollow, setChatAutoFollow] = useState(true);
  const [chatScrollTop, setChatScrollTop] = useState(0);
  const historyRequestCounter = useRef(0);
  const [historyQuery, setHistoryQuery] = useState("");
  const [historyItems, setHistoryItems] = useState<ConversationListItem[]>([]);
  const [historyFavoritesOnly, setHistoryFavoritesOnly] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyLoadingMore, setHistoryLoadingMore] = useState(false);
  const [historyPage, setHistoryPage] = useState(1);
  const [historyHasMore, setHistoryHasMore] = useState(false);
  const [memories, setMemories] = useState<UserMemoryResponse[]>([]);
  const [memoriesLoading, setMemoriesLoading] = useState(false);
  const [memorySaving, setMemorySaving] = useState(false);
  const [memoryEditor, setMemoryEditor] = useState<MemoryEditor | null>(null);
  const [preparedShare, setPreparedShare] = useState<PreparedShare | null>(null);
  const [shareSheetOpen, setShareSheetOpen] = useState(false);
  const [shareWorking, setShareWorking] = useState(false);
  const [sharedConversation, setSharedConversation] = useState<PublicSharedConversationResponse | null>(null);
  const [sharedMessages, setSharedMessages] = useState<ConversationMessage[]>([]);
  const [sharedLoading, setSharedLoading] = useState(false);

  useShareAppMessage(() => ({
    title: preparedShare?.title ? `${preparedShare.title}｜AI省着用` : "AI省着用",
    path: preparedShare?.shareID
      ? miniAppSharedConversationPath(preparedShare.shareID)
      : "/pages/index/index",
  }));

  const enableChatAutoFollow = useCallback((scrollNow = false) => {
    chatAutoFollowRef.current = true;
    setChatAutoFollow(true);
    if (scrollNow) {
      setChatScrollTop(nextChatBottomScrollTop);
    }
  }, []);

  const handleChatScroll = useCallback((event: unknown) => {
    const nextScrollTop = readScrollTop(event);
    if (nextScrollTop === null) {
      return;
    }
    if (shouldReleaseChatAutoFollow(chatScrollTopRef.current, nextScrollTop, chatTouchingRef.current)) {
      chatAutoFollowRef.current = false;
      setChatAutoFollow(false);
    }
    chatScrollTopRef.current = nextScrollTop;
  }, []);

  const handleChatScrollToLower = useCallback(() => {
    enableChatAutoFollow();
  }, [enableChatAutoFollow]);

  const handleConversationImageLoad = useCallback(() => {
    if (chatAutoFollowRef.current) {
      setChatScrollTop(nextChatBottomScrollTop);
    }
  }, []);

  const applyDailyCheckinStatus = useCallback((status: DailyCheckinStatusResponse | null) => {
    setDailyCheckin(status);
    setShowDailyCheckinResult(Boolean(status?.claimed));
    if (status?.claimed) {
      setDailyCheckinRotation((current) =>
        wheelRotationForPrize(status.prizes, status.prizeKey, current),
      );
    }
  }, []);

  const clearDailyCheckinRevealTimer = useCallback(() => {
    if (dailyCheckinRevealTimerRef.current !== null) {
      clearTimeout(dailyCheckinRevealTimerRef.current);
      dailyCheckinRevealTimerRef.current = null;
    }
  }, []);

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
      applyDailyCheckinStatus(result.dailyCheckin);
      setPresets(result.presets);
      setModels(result.models);
      setNetworkSearchAvailable(result.networkSearchAvailable);
      setNetworkSearchEnabled(result.networkSearchAvailable);
      setSelectedChatModel(result.presets.chatModel);
      setSelectedImageModel(result.presets.imageModel);
      setConversations(result.conversations);
      setConversationPage(1);
      setConversationTotal(result.conversationTotal);
      if (incomingShareID) {
        setSharedLoading(true);
        setSharedConversation(null);
        setSharedMessages([]);
        setScreen("shared");
        try {
          const shared = await session.getSharedConversation(incomingShareID);
          const allMessages = shared.messages
            .map(messageFromAPI)
            .filter((item): item is ConversationMessage => item !== null);
          const defaultIDs = new Set(shared.defaultMessagePublicIDs);
          const visibleMessages = defaultIDs.size > 0
            ? allMessages.filter((message) => defaultIDs.has(message.id))
            : latestVisibleMessages(allMessages);
          setSharedConversation(shared);
          setSharedMessages(visibleMessages);
          setPreparedShare({ shareID: shared.shareID, title: shared.title || "AI 对话分享" });
          for (const item of visibleMessages) {
            if (!item.imageFileID) {
              continue;
            }
            void session.downloadSharedImage(shared.shareID, item.imageFileID).then((imageSource) => {
              if (!imageSource) {
                return;
              }
              setSharedMessages((current) => current.map((message) => message.id === item.id
                ? { ...message, imageSource }
                : message));
            });
          }
        } catch (error) {
          setWorkspaceError(error instanceof Error ? error.message : "分享内容加载失败");
        } finally {
          setSharedLoading(false);
        }
      } else {
        setScreen("home");
      }
    } catch (error) {
      session.dispose();
      if (sessionRef.current === session) {
        sessionRef.current = null;
      }
      setBootError(error instanceof Error ? error.message : "微信登录失败，请稍后重试");
    } finally {
      setBooting(false);
    }
  }, [applyDailyCheckinStatus, incomingShareID]);

  useEffect(() => {
    void bootstrap();
    return () => {
      clearDailyCheckinRevealTimer();
      sessionRef.current?.dispose();
      sessionRef.current = null;
    };
  }, [bootstrap, clearDailyCheckinRevealTimer]);

  useEffect(() => {
    if ((screen === "chat" || screen === "image") && chatAutoFollowRef.current) {
      setChatScrollTop(nextChatBottomScrollTop);
    }
  }, [messages, screen]);

  useEffect(() => {
    const handleKeyboardHeightChange = (event: { height: number }) => {
      setKeyboardHeight(Math.max(0, Number(event.height) || 0));
    };
    Taro.onKeyboardHeightChange(handleKeyboardHeightChange);
    return () => Taro.offKeyboardHeightChange(handleKeyboardHeightChange);
  }, []);

  useEffect(() => {
    if (screen !== "history") {
      return;
    }
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    const requestID = ++historyRequestCounter.current;
    const query = historyQuery.trim();
    setHistoryLoading(true);
    setHistoryPage(1);
    setHistoryHasMore(false);
    const timer = setTimeout(() => {
      void (async () => {
        try {
          const result = await fetchHistoryPage(session, query, 1, historyFavoritesOnly);
          if (requestID !== historyRequestCounter.current) {
            return;
          }
          setHistoryItems(result.results);
          setHistoryHasMore(result.hasMore);
          setWorkspaceError("");
        } catch (error) {
          if (requestID === historyRequestCounter.current) {
            setHistoryItems([]);
            setWorkspaceError(error instanceof Error ? error.message : "历史会话加载失败");
          }
        } finally {
          if (requestID === historyRequestCounter.current) {
            setHistoryLoading(false);
          }
        }
      })();
    }, query ? 250 : 0);
    return () => clearTimeout(timer);
  }, [historyFavoritesOnly, historyQuery, screen]);

  const refreshConversations = async () => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    try {
      const pageSize = conversationRefreshPageSize(conversations.length);
      const page = await session.listConversationPage(1, pageSize);
      setConversations((current) => page.results.map((item) => preserveOptimisticConversationTitle(
        current.find((existing) => existing.publicID === item.publicID),
        item,
      )));
      setConversationPage(Math.max(1, Math.ceil(page.results.length / 50)));
      setConversationTotal(page.total);
      setCurrentConversation((current) => {
        if (!current) {
          return current;
        }
        const refreshed = page.results.find((item) => item.publicID === current.publicID);
        return refreshed ? preserveOptimisticConversationTitle(current, refreshed) : current;
      });
    } catch {
      // Keep the last successful list; workspace actions surface their own errors.
    }
  };

  const loadMoreConversations = async () => {
    const session = sessionRef.current;
    if (!session || loadingMoreConversations || conversations.length >= conversationTotal) {
      return;
    }
    setLoadingMoreConversations(true);
    try {
      const nextPage = conversationPage + 1;
      const page = await session.listConversationPage(nextPage);
      setConversations((current) => mergeConversationPage(current, page.results));
      setConversationPage(nextPage);
      setConversationTotal(page.total);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "加载更多会话失败");
    } finally {
      setLoadingMoreConversations(false);
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

  const refreshDailyCheckinStatus = useCallback(async () => {
    const session = sessionRef.current;
    if (!session || isDailyCheckinClaimingRef.current) {
      return;
    }
    try {
      const status = await session.getDailyCheckinStatus();
      applyDailyCheckinStatus(status);
    } catch {
      // Daily benefits must never prevent the core chat workspace from opening.
    }
  }, [applyDailyCheckinStatus]);

  useDidShow(() => {
    void refreshDailyCheckinStatus();
  });

  const claimDailyCheckin = async () => {
    const session = sessionRef.current;
    if (!session || !dailyCheckin || dailyCheckin.claimed || isDailyCheckinClaimingRef.current) {
      return;
    }
    isDailyCheckinClaimingRef.current = true;
    setIsDailyCheckinClaiming(true);
    setShowDailyCheckinResult(false);
    setWorkspaceError("");
    try {
      const result = await session.claimDailyCheckin();
      setDailyCheckinRotation((current) =>
        wheelRotationForPrize(dailyCheckin.prizes, result.prizeKey, current),
      );
      clearDailyCheckinRevealTimer();
      dailyCheckinRevealTimerRef.current = setTimeout(() => {
        setDailyCheckin((current) => {
          if (!current) {
            return current;
          }
          return {
            ...current,
            claimed: true,
            awardedCalls: result.awardedCalls,
            businessDate: result.businessDate,
            prizeKey: result.prizeKey,
            rewardUsd: result.rewardUsd,
            streakDays: result.streakDays,
            unitPriceUsd: result.unitPriceUsd,
          };
        });
        setShowDailyCheckinResult(true);
        isDailyCheckinClaimingRef.current = false;
        setIsDailyCheckinClaiming(false);
        dailyCheckinRevealTimerRef.current = null;
        void refreshBalance();
      }, 2_300);
    } catch (error) {
      isDailyCheckinClaimingRef.current = false;
      setIsDailyCheckinClaiming(false);
      setWorkspaceError(error instanceof Error ? error.message : "签到领取失败，请稍后重试");
    }
  };

  const loadAccountCenter = async () => {
    const session = sessionRef.current;
    if (!session || accountLoading) {
      return;
    }
    setAccountLoading(true);
    setAccountError("");
    const [overviewResult, monthlyResult] = await Promise.allSettled([
      session.getBillingOverview(),
      session.listMonthlyUsage(6),
    ]);
    if (overviewResult.status === "fulfilled") {
      setBillingOverview(overviewResult.value);
      setBalanceUSD(overviewResult.value.account?.balanceUSD ?? null);
    }
    if (monthlyResult.status === "fulfilled") {
      setMonthlyUsage(monthlyResult.value);
    }
    if (overviewResult.status === "rejected" && monthlyResult.status === "rejected") {
      setAccountError("账户信息暂时加载失败，请稍后重试");
    } else if (overviewResult.status === "rejected") {
      setAccountError("订阅信息暂时加载失败，已显示可用的用量数据");
    } else if (monthlyResult.status === "rejected") {
      setAccountError("用量趋势暂时加载失败，已显示账户和订阅信息");
    }
    setAccountLoading(false);
  };

  const openAccountCenter = () => {
    setScreen("account");
    void loadAccountCenter();
  };

  const openDailyCheckin = () => {
    setWorkspaceError("");
    setScreen("checkin");
    void refreshDailyCheckinStatus();
  };

  const openHistory = () => {
    setWorkspaceError("");
    setHistoryQuery("");
    setHistoryFavoritesOnly(false);
    setScreen("history");
  };

  const loadMoreHistory = async () => {
    const session = sessionRef.current;
    if (!session || historyLoading || historyLoadingMore || !historyHasMore) {
      return;
    }
    const nextPage = historyPage + 1;
    const query = historyQuery.trim();
    setHistoryLoadingMore(true);
    try {
      const result = await fetchHistoryPage(session, query, nextPage, historyFavoritesOnly);
      setHistoryItems((current) => mergeHistoryPage(current, result.results));
      setHistoryPage(nextPage);
      setHistoryHasMore(result.hasMore);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "加载更多会话失败");
    } finally {
      setHistoryLoadingMore(false);
    }
  };

  const loadUserMemories = async () => {
    const session = sessionRef.current;
    if (!session || memoriesLoading) {
      return;
    }
    setMemoriesLoading(true);
    setWorkspaceError("");
    try {
      const result = await session.listUserMemories();
      setMemories(result.filter((item) => item.scope === "preference"));
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "AI 偏好记忆加载失败");
    } finally {
      setMemoriesLoading(false);
    }
  };

  const openUserMemories = () => {
    setMemoryEditor(null);
    setScreen("memories");
    void loadUserMemories();
  };

  const saveUserMemory = async () => {
    const session = sessionRef.current;
    const editor = memoryEditor;
    const memoryKey = editor?.memoryKey.trim() ?? "";
    const value = editor?.value.trim() ?? "";
    if (!session || !editor || !memoryKey || !value || memorySaving) {
      return;
    }
    setMemorySaving(true);
    setWorkspaceError("");
    try {
      await session.upsertUserMemory(memoryKey, value);
      const now = new Date().toISOString();
      setMemories((current) => {
        const existing = current.find((item) => item.memoryKey === memoryKey);
        if (existing) {
          return current.map((item) => item.memoryKey === memoryKey
            ? { ...item, updatedAt: now, value }
            : item);
        }
        return [{
          createdAt: now,
          id: Date.now(),
          memoryKey,
          scope: "preference",
          updatedAt: now,
          updatedBy: "user",
          userID: 0,
          value,
        }, ...current];
      });
      setMemoryEditor(null);
      await Taro.showToast({ title: editor.originalKey ? "偏好已更新" : "偏好已记住", icon: "success" });
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "保存偏好失败");
    } finally {
      setMemorySaving(false);
    }
  };

  const deleteUserMemory = async (memory: UserMemoryResponse) => {
    const session = sessionRef.current;
    if (!session || memorySaving) {
      return;
    }
    const confirmation = await Taro.showModal({
      title: "删除这条偏好？",
      content: `AI 将不再记住“${memory.memoryKey}”。`,
      confirmText: "删除",
      confirmColor: "#d14343",
    });
    if (!confirmation.confirm) {
      return;
    }
    setMemorySaving(true);
    setWorkspaceError("");
    try {
      await session.deleteUserMemory(memory.memoryKey);
      setMemories((current) => current.filter((item) => item.memoryKey !== memory.memoryKey));
      await Taro.showToast({ title: "已删除", icon: "success" });
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "删除偏好失败");
    } finally {
      setMemorySaving(false);
    }
  };

  const prepareConversationShare = async (conversation: ConversationListItem) => {
    const session = sessionRef.current;
    if (!session || shareWorking || conversation.messageCount <= 0) {
      if (conversation.messageCount <= 0) {
        await Taro.showToast({ title: "发送一条消息后才能分享", icon: "none" });
      }
      return;
    }
    setShareWorking(true);
    setWorkspaceError("");
    try {
      const share = await session.createConversationShare(conversation.publicID);
      setPreparedShare({ shareID: share.shareID, title: conversation.title || "AI 对话分享" });
      setShareSheetOpen(true);
      setConversations((current) => current.map((item) => item.publicID === conversation.publicID
        ? { ...item, shareID: share.shareID, shareStatus: share.status, sharedAt: share.createdAt }
        : item));
      await Taro.showShareMenu({ withShareTicket: true });
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "创建分享失败");
    } finally {
      setShareWorking(false);
    }
  };

  const copyPreparedShareLink = async () => {
    if (!preparedShare?.shareID || !runtimeConfig) {
      return;
    }
    const url = `${runtimeConfig.apiBaseUrl}/share?conversation_id=${encodeURIComponent(preparedShare.shareID)}`;
    await Taro.setClipboardData({ data: url });
  };

  const updateConversation = (updated: ConversationResponse) => {
    setConversations((items) => items.map((item) => item.publicID === updated.publicID ? updated : item));
    setHistoryItems((items) => items.map((item) => item.publicID === updated.publicID ? updated : item));
    setCurrentConversation((current) => current?.publicID === updated.publicID ? updated : current);
  };

  const applyOptimisticConversationTitle = (conversation: ConversationResponse, content: string): string => {
    if (!isPlaceholderConversationTitle(conversation.title)) {
      return "";
    }
    const title = conversationTitleFromFirstUserMessage(content);
    if (title) {
      updateConversation({ ...conversation, title });
    }
    return title;
  };

  const refreshGeneratedConversationTitle = async (conversationID: string) => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    let elapsedMs = 0;
    let delayMs = 800;
    while (elapsedMs < 45_000 && sessionRef.current === session) {
      const nextDelay = Math.min(delayMs, 45_000 - elapsedMs);
      await new Promise<void>((resolve) => setTimeout(resolve, nextDelay));
      elapsedMs += nextDelay;
      if (sessionRef.current !== session) {
        return;
      }
      const latest = await session.getConversation(conversationID).catch(() => null);
      if (latest && !isPlaceholderConversationTitle(latest.title)) {
        updateConversation(latest);
        return;
      }
      delayMs = Math.min(Math.round(delayMs * 1.5), 5_000);
    }
  };

  const enterConversation = async (conversation: ConversationResponse, mode?: ConversationMode) => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    const historyLoadID = ++historyLoadCounter.current;
    let resolvedMode = mode ?? session.conversationMode(conversation);
    enableChatAutoFollow(true);
    if (resolvedMode === "chat") {
      setSelectedChatModel(resolveSelectedModel(
        models,
        conversation.model,
        presets?.chatModel?.platformModelName ?? "",
        "chat",
      ));
    } else {
      const imageOptions = imageModelOptions(models);
      setSelectedImageModel(
        imageOptions.find((model) => model.platformModelName === conversation.model) ??
        imageOptions.find((model) => model.platformModelName === presets?.imageModel?.platformModelName) ??
        imageOptions[0] ??
        null,
      );
    }
    setWorkspaceError("");
    setPendingImage(null);
    setCurrentConversation(conversation);
    setScreen(resolvedMode);
    const applyHistory = (history: Awaited<ReturnType<MiniAppSession["listMessages"]>>) => {
      const timeline = latestVisibleMessages(
        history.map(messageFromAPI).filter((item): item is ConversationMessage => item !== null),
      );
      setMessages(timeline);
      for (const item of timeline) {
        if (!item.imageFileID) {
          continue;
        }
        void session.downloadMessageImage(item.imageFileID).then((imageSource) => {
          if (historyLoadCounter.current !== historyLoadID) {
            return;
          }
          if (!imageSource) {
            setMessages((current) => current.map((message) => message.id === item.id
              ? {
                  ...message,
                  imageFileID: undefined,
                  imageStatus: "历史图片加载失败，请重新进入对话",
                  pending: false,
                }
              : message));
            return;
          }
          setMessages((current) => current.map((message) => message.id === item.id
            ? applyImageProgress(message, { imageSource, pending: false, status: "图片生成完成" })
            : message));
        }).catch(() => {
          if (historyLoadCounter.current !== historyLoadID) {
            return;
          }
          setMessages((current) => current.map((message) => message.id === item.id
            ? {
                ...message,
                imageFileID: undefined,
                imageStatus: "历史图片加载失败，请重新进入对话",
                pending: false,
              }
            : message));
        });
      }
      return timeline;
    };
    try {
      const history = await session.listMessages(conversation.publicID);
      const timeline = applyHistory(history);
      if (!mode && timeline.some((item) => item.role === "assistant" && (item.contentType === "image" || item.imageFileID))) {
        resolvedMode = "image";
        const imageOptions = imageModelOptions(models);
        setSelectedImageModel(
          imageOptions.find((model) => model.platformModelName === conversation.model) ??
          imageOptions.find((model) => model.platformModelName === presets?.imageModel?.platformModelName) ??
          imageOptions[0] ??
          null,
        );
        setScreen("image");
      }
      const pending = [...timeline].reverse().find((item) => item.role === "assistant" && item.pending && item.runID);
      if (!pending?.runID || historyLoadCounter.current !== historyLoadID) {
        return;
      }
      const pendingIndex = timeline.findIndex((item) => item.id === pending.id);
      const pendingParent = timeline.slice(0, pendingIndex).reverse().find((item) => item.role === "user");
      setRunning(true);
      setStopping(false);
      try {
        const resumed = await session.resumeGeneration(
          pending.runID,
          {
            imageSource: pending.imageSource ?? null,
            imageTask: resolvedMode === "image"
              ? pendingParent?.imageFileID ? "image_edit" : "image_generation"
              : undefined,
            processTrace: pending.processTrace,
            text: pending.text,
          },
          (progress) => {
            if (historyLoadCounter.current !== historyLoadID) {
              return;
            }
            setMessages((current) => current.map((message) => message.id !== pending.id
              ? message
              : resolvedMode === "image"
                ? applyImageProgress(message, {
                    imageSource: progress.imageSource,
                    pending: true,
                    status: progress.status || message.imageStatus,
                  })
                : {
                    ...message,
                    activityStatus: progress.status || "正在生成回复",
                    pending: true,
                    processTrace: progress.processTrace ?? message.processTrace,
                    text: progress.text || message.text,
                  }));
          },
        );
        setMessages((current) => current.map((message) => message.id !== pending.id
          ? message
          : resolvedMode === "image"
            ? applyImageProgress(message, {
                imageSource: resumed.imageSource,
                pending: false,
                status: resumed.imageSource
                  ? resumed.imageTask === "image_edit" ? "图片编辑完成" : "图片生成完成"
                  : resumed.status,
              })
            : {
                ...message,
                activityStatus: undefined,
                pending: false,
                processTrace: resumed.processTrace ?? message.processTrace,
                text: resumed.text || message.text,
              }));
      } catch (error) {
        if (historyLoadCounter.current === historyLoadID && !(error instanceof MiniAppRequestAbortedError)) {
          setWorkspaceError(error instanceof Error ? error.message : "恢复生成进度失败，请稍后重新进入");
        }
      } finally {
        if (historyLoadCounter.current === historyLoadID) {
          setRunning(false);
          setStopping(false);
          const refreshed = await session.listMessages(conversation.publicID).catch(() => null);
          if (refreshed && historyLoadCounter.current === historyLoadID) {
            applyHistory(refreshed);
          }
          void refreshBalance();
          void refreshConversations();
        }
      }
    } catch (error) {
      setMessages([]);
      setWorkspaceError(error instanceof Error ? error.message : "加载历史消息失败");
    }
  };

  const createWithModel = async (model: PublicModelResponse, mode: ConversationMode) => {
    const session = sessionRef.current;
    if (!session || running) {
      return;
    }
    setRunning(true);
    setStopping(false);
    setWorkspaceError("");
    try {
      const conversation = await session.createConversation(model, "新对话");
      setConversations((items) => [conversation, ...items]);
      setConversationTotal((total) => total + 1);
      setMessages([]);
      setPrompt("");
      setPendingImage(null);
      setCurrentConversation(conversation);
      enableChatAutoFollow(true);
      if (mode === "chat") {
        setSelectedChatModel(model);
      } else {
        setSelectedImageModel(model);
      }
      setScreen(mode);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "创建对话失败");
    } finally {
      setRunning(false);
      setStopping(false);
    }
  };

  const sendChat = async () => {
    const session = sessionRef.current;
    const conversation = currentConversation;
    const selectedModel = selectedChatModel;
    const content = prompt.trim();
    const attachment = pendingImage;
    if (!session || !conversation || !selectedModel || running || uploading || (!content && !attachment)) {
      return;
    }
    enableChatAutoFollow(true);
    setWorkspaceError("");
    messageCounter.current += 1;
    const userID = `local-user-${messageCounter.current}`;
    messageCounter.current += 1;
    const assistantID = `local-assistant-${messageCounter.current}`;
    setMessages((items) => [
      ...items,
      { id: userID, imageSource: attachment?.localPath, role: "user", text: content },
      {
        activityStatus: "正在思考…",
        id: assistantID,
        pending: true,
        role: "assistant",
        text: "",
      },
    ]);
    setPrompt("");
    setPendingImage(null);
    setRunning(true);
    setStopping(false);
    const shouldRefreshGeneratedTitle = Boolean(applyOptimisticConversationTitle(conversation, content));
    try {
      const result = await session.sendChat(conversation, selectedModel.platformModelName, content, (progress) => {
        setMessages((items) => items.map((item) => item.id === assistantID
          ? {
              ...item,
              activityStatus: progress.status || (progress.text ? "正在生成回复…" : "正在思考…"),
              pending: true,
              processTrace: progress.processTrace ?? item.processTrace,
              text: progress.text || item.text,
            }
          : item));
      }, attachment ? [attachment.fileID] : [], networkSearchAvailable && networkSearchEnabled);
      const persistedUser = result.userMessage ? messageFromAPI(result.userMessage) : null;
      const persistedAssistant = result.assistantMessage ? messageFromAPI(result.assistantMessage) : null;
      setMessages((items) => items.map((item) => {
        if (item.id === userID && persistedUser) {
          return { ...persistedUser, imageSource: attachment?.localPath ?? persistedUser.imageSource };
        }
        if (item.id === assistantID) {
          return {
            ...(persistedAssistant ?? item),
            activityStatus: undefined,
            pending: false,
            processTrace: result.processTrace ?? persistedAssistant?.processTrace ?? item.processTrace,
            text: result.text,
          };
        }
        return item;
      }));
      updateConversation({ ...conversation, model: selectedModel.platformModelName });
      void refreshConversations();
      void refreshBalance();
      if (shouldRefreshGeneratedTitle) {
        void refreshGeneratedConversationTitle(conversation.publicID);
      }
    } catch (error) {
      const stopped = error instanceof MiniAppRequestAbortedError;
      setMessages((items) => items.map((item) => item.id === assistantID
          ? {
            ...item,
            activityStatus: undefined,
            pending: false,
            text: chatGenerationFailureText(item.text, stopped),
          }
        : item));
      if (attachment) {
        setPendingImage((current) => current ?? attachment);
      }
      if (!stopped) {
        setWorkspaceError(error instanceof Error ? error.message : "发送失败，请重试");
      }
    } finally {
      setRunning(false);
      setStopping(false);
    }
  };

  const stopGeneration = async () => {
    const session = sessionRef.current;
    if (!session || !running || stopping) {
      return;
    }
    setStopping(true);
    setWorkspaceError("");
    try {
      const requested = await session.cancelActiveGeneration();
      if (!requested) {
        setStopping(false);
      }
    } catch (error) {
      setStopping(false);
      setWorkspaceError(error instanceof Error
        ? `停止请求失败：${error.message}。任务可能仍在后端运行，请稍后重新进入会话查看。`
        : "停止请求失败，任务可能仍在后端运行，请稍后重新进入会话查看。");
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

  const chooseImageEditInput = async () => {
    const session = sessionRef.current;
    if (!session || running || uploading) {
      return;
    }
    const editModel = resolveImageEditModel(models, selectedImageModel?.platformModelName ?? "", "");
    if (!editModel) {
      setWorkspaceError("当前账号没有支持图片编辑的模型");
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
      const fileName = `wechat-edit-${Date.now()}.jpg`;
      setUploading(true);
      const uploaded = await session.uploadChatImage(selected.tempFilePath, fileName);
      setSelectedImageModel(editModel);
      setPendingImage({
        fileID: uploaded.fileID,
        fileName: uploaded.fileName || fileName,
        localPath: selected.tempFilePath,
      });
      await Taro.showToast({ title: "已进入图片编辑模式", icon: "none" });
    } catch (error) {
      const message = error instanceof Error ? error.message : "图片上传失败，请重试";
      if (!/cancel/u.test(message)) {
        setWorkspaceError(message);
      }
    } finally {
      setUploading(false);
    }
  };

  const continueEditingImage = async (message: ConversationMessage) => {
    if (!message.imageFileID || !message.imageSource || running || uploading) {
      return;
    }
    const editModel = resolveImageEditModel(
      models,
      selectedImageModel?.platformModelName ?? "",
      message.modelName ?? "",
    );
    if (!editModel) {
      await Taro.showToast({ title: "当前账号没有支持图片编辑的模型", icon: "none" });
      return;
    }
    setSelectedImageModel(editModel);
    setPendingImage({
      fileID: message.imageFileID,
      fileName: "已生成的图片",
      localPath: message.imageSource,
    });
    setPrompt("");
    setWorkspaceError("");
    await Taro.showToast({ title: "已加入编辑区，请描述修改要求", icon: "none" });
  };

  const removePendingImage = () => {
    setPendingImage(null);
    setWorkspaceError("");
    if (screen !== "image" || !selectedImageModel || supportsModelKind(selectedImageModel, "image_gen")) {
      return;
    }
    const generationModels = modelsForKind(models, "image_gen");
    setSelectedImageModel(
      generationModels.find((model) => model.platformModelName === presets?.imageModel?.platformModelName) ??
      generationModels[0] ??
      selectedImageModel,
    );
  };

  const generateImage = async () => {
    const session = sessionRef.current;
    const conversation = currentConversation;
    const selectedModel = selectedImageModel;
    const content = prompt.trim();
    const attachment = pendingImage;
    if (!session || !conversation || !selectedModel || running || !content) {
      return;
    }
    const decision = resolveImageSubmitDecision(selectedModel, Boolean(attachment));
    if (!decision.task) {
      setWorkspaceError(decision.blockedReason === "image_edit_input_required"
        ? "该模型用于图片编辑，请先上传需要编辑的图片"
        : "当前模型不支持图片编辑，请移除图片或切换模型");
      return;
    }
    setWorkspaceError("");
    messageCounter.current += 1;
    const userID = `local-image-user-${messageCounter.current}`;
    messageCounter.current += 1;
    const assistantID = `local-image-assistant-${messageCounter.current}`;
    enableChatAutoFollow(true);
    setMessages((items) => [
      ...items,
      ...createPendingImageTurn(
        content,
        userID,
        assistantID,
        attachment?.localPath,
        decision.task === "image_edit" ? "AI 正在编辑图片" : "AI 正在生成图片",
      ),
    ]);
    setPrompt("");
    setPendingImage(null);
    setRunning(true);
    setStopping(false);
    const shouldRefreshGeneratedTitle = Boolean(applyOptimisticConversationTitle(conversation, content));
    try {
      const result = await session.generateImage(
        conversation,
        selectedModel.platformModelName,
        decision.task,
        content,
        (progress) => {
          setMessages((items) => items.map((item) => item.id === assistantID
            ? applyImageProgress(item, progress)
            : item));
        },
        attachment ? [attachment.fileID] : [],
      );
      const persistedUser = result.userMessage ? messageFromAPI(result.userMessage) : null;
      const persistedAssistant = result.assistantMessage ? messageFromAPI(result.assistantMessage) : null;
      setMessages((items) => items.map((item) => {
        if (item.id === userID && persistedUser) {
          return { ...persistedUser, imageSource: attachment?.localPath ?? persistedUser.imageSource };
        }
        if (item.id === assistantID) {
          return {
            ...applyImageProgress(persistedAssistant ?? item, { ...result, pending: false }),
            modelName: selectedModel.platformModelName,
          };
        }
        return item;
      }));
      updateConversation({ ...conversation, model: selectedModel.platformModelName });
      void refreshConversations();
      void refreshBalance();
      if (shouldRefreshGeneratedTitle) {
        void refreshGeneratedConversationTitle(conversation.publicID);
      }
    } catch (error) {
      const stopped = error instanceof MiniAppRequestAbortedError;
      const failureMessage = error instanceof Error && error.message.trim()
        ? error.message.trim()
        : "图片生成失败，请重试";
      setMessages((items) => items.map((item) => item.id === assistantID
        ? applyImageProgress(item, {
            pending: false,
            status: stopped ? "本次图片生成已停止" : failureMessage,
          })
        : item));
      setWorkspaceError("");
      if (attachment) {
        setPendingImage((current) => current ?? attachment);
      }
    } finally {
      setRunning(false);
      setStopping(false);
    }
  };

  const requestRenameConversation = async (conversation: ConversationListItem) => {
    const session = sessionRef.current;
    if (!session || managingConversationID) {
      return;
    }
    const modal = await Taro.showModal({
      title: "重命名对话",
      content: conversation.title || "",
      editable: true,
      placeholderText: "请输入对话名称",
      confirmText: "保存",
    } as Parameters<typeof Taro.showModal>[0]) as Awaited<ReturnType<typeof Taro.showModal>> & { content?: string };
    const title = modal.content?.trim() || "";
    if (!modal.confirm || !title || title === conversation.title.trim()) {
      setOpenSwipeConversationID("");
      return;
    }
    setManagingConversationID(conversation.publicID);
    setWorkspaceError("");
    try {
      const updated = await session.renameConversation(conversation.publicID, title);
      updateConversation(updated);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "重命名失败，请重试");
    } finally {
      setManagingConversationID("");
      setOpenSwipeConversationID("");
    }
  };

  const requestDeleteConversation = async (conversation: ConversationListItem) => {
    const session = sessionRef.current;
    if (!session || managingConversationID) {
      return;
    }
    const selected = await Taro.showActionSheet({
      alertText: "选择删除方式",
      itemList: ["仅删除会话（保留文件）", "删除会话并清理未引用文件"],
    }).catch(() => null);
    if (!selected) {
      setOpenSwipeConversationID("");
      return;
    }
    const deleteFiles = selected.tapIndex === 1;
    const confirmation = await Taro.showModal({
      title: "确认删除对话？",
      content: deleteFiles
        ? `将删除“${conversation.title || "未命名对话"}”，并清理未被其他会话使用的文件。`
        : `将删除“${conversation.title || "未命名对话"}”，已上传和生成的文件会保留。`,
      confirmText: "删除",
      confirmColor: "#d14343",
    });
    if (!confirmation.confirm) {
      setOpenSwipeConversationID("");
      return;
    }
    setManagingConversationID(conversation.publicID);
    setWorkspaceError("");
    try {
      if (running && currentConversation?.publicID === conversation.publicID) {
        await session.cancelActiveGeneration();
      }
      const result = await session.deleteConversation(conversation.publicID, deleteFiles);
      if (result.deleted) {
        setConversations((items) => items.filter((item) => item.publicID !== conversation.publicID));
        setHistoryItems((items) => items.filter((item) => item.publicID !== conversation.publicID));
        setConversationTotal((total) => Math.max(0, total - 1));
        if (currentConversation?.publicID === conversation.publicID) {
          goHome();
        }
      }
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "删除失败，请重试");
    } finally {
      setManagingConversationID("");
      setOpenSwipeConversationID("");
    }
  };

  const toggleConversationStar = async (conversation: ConversationListItem) => {
    const session = sessionRef.current;
    if (!session || managingConversationID) {
      return;
    }
    setManagingConversationID(conversation.publicID);
    setWorkspaceError("");
    try {
      const updated = await session.setConversationStar(conversation.publicID, !conversation.isStarred);
      updateConversation(updated);
      if (historyFavoritesOnly && !updated.isStarred) {
        setHistoryItems((items) => items.filter((item) => item.publicID !== updated.publicID));
      }
      await Taro.showToast({ title: updated.isStarred ? "已收藏" : "已取消收藏", icon: "none" });
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "收藏操作失败");
    } finally {
      setManagingConversationID("");
      setOpenSwipeConversationID("");
    }
  };

  const enterHistoryConversation = async (conversation: ConversationListItem) => {
    const session = sessionRef.current;
    if (!session) {
      return;
    }
    setManagingConversationID(conversation.publicID);
    try {
      const fullConversation = await session.getConversation(conversation.publicID);
      updateConversation(fullConversation);
      await enterConversation(fullConversation);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "打开会话失败");
    } finally {
      setManagingConversationID("");
    }
  };

  const showConversationActions = async (conversation: ConversationListItem) => {
    const selected = await Taro.showActionSheet({
      itemList: [conversation.isStarred ? "取消收藏" : "收藏会话", "重命名", "分享给好友", "删除"],
    }).catch(() => null);
    switch (selected?.tapIndex) {
      case 0:
        await toggleConversationStar(conversation);
        break;
      case 1:
        await requestRenameConversation(conversation);
        break;
      case 2:
        await prepareConversationShare(conversation);
        break;
      case 3:
        await requestDeleteConversation(conversation);
        break;
    }
  };

  const goHome = () => {
    sessionRef.current?.abort();
    historyLoadCounter.current += 1;
    setRunning(false);
    setStopping(false);
    setWorkspaceError("");
    setCurrentConversation(null);
    setMessages([]);
    setPrompt("");
    setKeyboardHeight(0);
    setModelPickerMode(null);
    setScreen("home");
    void refreshConversations();
  };

  const saveImage = async (source?: string) => {
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

  const previewImage = async (source?: string) => {
    if (!source) {
      return;
    }
    await Taro.previewImage({ current: source, urls: [source] });
  };

  const copyAssistantAnswer = async (message: ConversationMessage) => {
    if (!message.text.trim()) {
      return;
    }
    try {
      await Taro.setClipboardData({ data: message.text });
    } catch {
      await Taro.showToast({ title: "复制失败，请重试", icon: "none" });
    }
  };

  const regenerateChatAnswer = async (message: ConversationMessage) => {
    const session = sessionRef.current;
    const conversation = currentConversation;
    const model = selectedChatModel;
    const sourceIndex = messages.findIndex((item) => item.id === message.id);
    const parentUser = messages.find((item) => item.id === message.parentPublicID && item.role === "user");
    if (!session || !conversation || !model || running || sourceIndex < 0 || !parentUser ||
      message.id.startsWith("local-")) {
      await Taro.showToast({ title: "这条回答暂时无法重新生成", icon: "none" });
      return;
    }
    const previousMessages = messages;
    messageCounter.current += 1;
    const pendingID = `local-retry-${messageCounter.current}`;
    enableChatAutoFollow(true);
    setMessages([
      ...messages.slice(0, sourceIndex),
      {
        activityStatus: "正在重新思考…",
        id: pendingID,
        parentPublicID: parentUser.id,
        pending: true,
        role: "assistant",
        sourcePublicID: message.id,
        text: "",
      },
    ]);
    setRunning(true);
    setStopping(false);
    setWorkspaceError("");
    try {
      const result = await session.sendChat(
        conversation,
        model.platformModelName,
        parentUser.text,
        (progress) => {
          setMessages((items) => items.map((item) => item.id === pendingID
            ? {
                ...item,
                activityStatus: progress.status || (progress.text ? "正在生成新回答…" : "正在重新思考…"),
                processTrace: progress.processTrace ?? item.processTrace,
                text: progress.text || item.text,
              }
            : item));
        },
        parentUser.imageFileID ? [parentUser.imageFileID] : [],
        networkSearchAvailable && networkSearchEnabled,
        {
          branchReason: "retry",
          parentMessagePublicID: parentUser.id,
          sourceMessagePublicID: message.id,
        },
      );
      const persisted = result.assistantMessage ? messageFromAPI(result.assistantMessage) : null;
      setMessages((items) => items.map((item) => item.id === pendingID
        ? {
            ...(persisted ?? item),
            activityStatus: undefined,
            pending: false,
            processTrace: result.processTrace ?? persisted?.processTrace ?? item.processTrace,
            text: result.text,
          }
        : item));
      void refreshBalance();
      void refreshConversations();
    } catch (error) {
      setMessages(previousMessages);
      if (!(error instanceof MiniAppRequestAbortedError)) {
        setWorkspaceError(error instanceof Error ? error.message : "重新生成失败，请重试");
      }
    } finally {
      setRunning(false);
      setStopping(false);
    }
  };

  const regenerateImageAnswer = async (message: ConversationMessage) => {
    const session = sessionRef.current;
    const conversation = currentConversation;
    const sourceIndex = messages.findIndex((item) => item.id === message.id);
    const parentUser = messages.find((item) => item.id === message.parentPublicID && item.role === "user");
    const editing = Boolean(parentUser?.imageFileID);
    const modelFromMessage = models.find((item) =>
      item.platformModelName === message.modelName &&
      supportsModelKind(item, editing ? "image_edit" : "image_gen"));
    let fallbackModel = selectedImageModel;
    if (editing) {
      fallbackModel = resolveImageEditModel(models, selectedImageModel?.platformModelName ?? "", "");
    }
    const model = modelFromMessage ?? fallbackModel;
    if (!session || !conversation || !parentUser || !model || running || sourceIndex < 0 ||
      message.id.startsWith("local-")) {
      await Taro.showToast({ title: "这张图片暂时无法重新生成", icon: "none" });
      return;
    }
    const decision = resolveImageSubmitDecision(model, editing);
    if (!decision.task) {
      await Taro.showToast({ title: "当前没有可用的图片模型", icon: "none" });
      return;
    }
    const previousMessages = messages;
    messageCounter.current += 1;
    const pendingID = `local-image-retry-${messageCounter.current}`;
    setSelectedImageModel(model);
    enableChatAutoFollow(true);
    setMessages([
      ...messages.slice(0, sourceIndex),
      {
        id: pendingID,
        imageStatus: editing ? "AI 正在编辑图片" : "AI 正在生成图片",
        parentPublicID: parentUser.id,
        pending: true,
        role: "assistant",
        sourcePublicID: message.id,
        text: "",
      },
    ]);
    setRunning(true);
    setStopping(false);
    setWorkspaceError("");
    try {
      const result = await session.generateImage(
        conversation,
        model.platformModelName,
        decision.task,
        parentUser.text,
        (progress) => {
          setMessages((items) => items.map((item) => item.id === pendingID
            ? applyImageProgress(item, progress)
            : item));
        },
        parentUser.imageFileID ? [parentUser.imageFileID] : [],
        {
          branchReason: "retry",
          parentMessagePublicID: parentUser.id,
          sourceMessagePublicID: message.id,
        },
      );
      const persisted = result.assistantMessage ? messageFromAPI(result.assistantMessage) : null;
      setMessages((items) => items.map((item) => item.id === pendingID
        ? {
            ...applyImageProgress(persisted ?? item, { ...result, pending: false }),
            modelName: model.platformModelName,
          }
        : item));
      void refreshBalance();
      void refreshConversations();
    } catch (error) {
      setMessages(previousMessages);
      if (!(error instanceof MiniAppRequestAbortedError)) {
        setWorkspaceError(error instanceof Error ? error.message : "重新生成图片失败");
      }
    } finally {
      setRunning(false);
      setStopping(false);
    }
  };

  const cloneSharedConversation = async () => {
    const session = sessionRef.current;
    if (!session || !sharedConversation || shareWorking) {
      return;
    }
    setShareWorking(true);
    setWorkspaceError("");
    try {
      const cloned = await session.cloneSharedConversation(sharedConversation.shareID);
      setConversations((current) => [cloned, ...current.filter((item) => item.publicID !== cloned.publicID)]);
      setConversationTotal((total) => total + 1);
      await enterConversation(cloned);
    } catch (error) {
      setWorkspaceError(error instanceof Error ? error.message : "保存到我的对话失败");
    } finally {
      setShareWorking(false);
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

  const shareOverlay = shareSheetOpen && preparedShare ? (
    <ShareSheet
      share={preparedShare}
      onClose={() => setShareSheetOpen(false)}
      onCopy={() => void copyPreparedShareLink()}
    />
  ) : null;

  if (booting) {
    return (
      <View className="centerState">
        <View className="brandMark">AI</View>
        <Text className="stateTitle">正在进入 AI省着用</Text>
        <Text className="stateHint">微信安全登录中…</Text>
      </View>
    );
  }

  if (bootError || !user || !presets) {
    return (
      <View className="centerState errorState">
        <View className="brandMark">AI</View>
        <Text className="stateTitle">暂时无法进入</Text>
        <Text className="stateHint">{bootError || "登录响应不完整"}</Text>
        <Button className="primaryButton retryButton" onClick={bootstrap}>重新登录</Button>
      </View>
    );
  }

  if (screen === "checkin") {
    return (
      <ScrollView className="checkinPage" scrollY enhanced showScrollbar={false}>
        <Header title="每日签到" onBack={() => setScreen("home")} />
        {dailyCheckin?.enabled ? (
          <DailyCheckinWheel
            status={dailyCheckin}
            isClaiming={isDailyCheckinClaiming}
            rotation={dailyCheckinRotation}
            revealResult={showDailyCheckinResult}
            onClaim={() => void claimDailyCheckin()}
          />
        ) : (
          <View className="checkinUnavailable">
            <Text className="checkinUnavailableTitle">签到暂未开放</Text>
            <Text className="checkinUnavailableHint">稍后再回来看看吧</Text>
          </View>
        )}
        {workspaceError ? <Text className="errorBanner checkinError">{workspaceError}</Text> : null}
      </ScrollView>
    );
  }

  if (screen === "history") {
    let historyContent: ReactNode;
    if (historyLoading) {
      historyContent = (
        <View className="historyState"><View className="imageSpinner" /><Text>正在查找对话…</Text></View>
      );
    } else if (historyItems.length === 0) {
      historyContent = (
        <View className="historyState">
          <Text className="historyStateIcon">{historyFavoritesOnly ? "☆" : "⌕"}</Text>
          <Text className="historyStateTitle">{historyEmptyTitle(historyQuery, historyFavoritesOnly)}</Text>
          <Text className="historyStateHint">{historyEmptyHint(historyFavoritesOnly)}</Text>
          {historyHasMore ? (
            <Button className="historyContinueButton" loading={historyLoadingMore} disabled={historyLoadingMore} onClick={() => void loadMoreHistory()}>
              继续查找收藏
            </Button>
          ) : null}
        </View>
      );
    } else {
      historyContent = (
        <View className="conversationList historyConversationList">
          {historyItems.map((conversation) => (
            <ConversationHistoryRow
              busy={managingConversationID === conversation.publicID}
              conversation={conversation}
              key={conversation.publicID}
              mode={sessionRef.current?.conversationMode(conversation) ?? "chat"}
              open={openSwipeConversationID === conversation.publicID}
              onDelete={() => void requestDeleteConversation(conversation)}
              onEnter={() => void enterHistoryConversation(conversation)}
              onMore={() => void showConversationActions(conversation)}
              onOpen={() => setOpenSwipeConversationID(conversation.publicID)}
              onRename={() => void requestRenameConversation(conversation)}
              onReset={() => setOpenSwipeConversationID("")}
              onStar={() => void toggleConversationStar(conversation)}
            />
          ))}
          {historyHasMore ? (
            <Button className="loadMoreButton" loading={historyLoadingMore} disabled={historyLoadingMore} onClick={() => void loadMoreHistory()}>
              {historyLoadingMore ? "正在加载…" : "加载更多"}
            </Button>
          ) : null}
        </View>
      );
    }
    return (
      <View className="historyPage">
        <Header title="全部对话" onBack={() => setScreen("home")} />
        <View className="historySearchBar">
          <Text className="historySearchIcon">⌕</Text>
          <Input
            className="historySearchInput"
            value={historyQuery}
            placeholder="搜索标题和对话内容"
            confirmType="search"
            onInput={(event) => setHistoryQuery(event.detail.value)}
          />
          {historyQuery ? <Text className="historySearchClear" onClick={() => setHistoryQuery("")}>×</Text> : null}
        </View>
        <View className="historyFilterRow">
          <View
            className={`historyFilter ${!historyFavoritesOnly ? "historyFilterActive" : ""}`}
            onClick={() => setHistoryFavoritesOnly(false)}
          >全部会话</View>
          <View
            className={`historyFilter ${historyFavoritesOnly ? "historyFilterActive" : ""}`}
            onClick={() => setHistoryFavoritesOnly(true)}
          >★ 我的收藏</View>
        </View>
        <ScrollView className="historyResults" scrollY enhanced showScrollbar={false}>
          {historyContent}
          {workspaceError ? <Text className="errorBanner historyError">{workspaceError}</Text> : null}
        </ScrollView>
        {shareOverlay}
      </View>
    );
  }

  if (screen === "memories") {
    const atLimit = memories.length >= MAX_PREFERENCE_MEMORIES;
    let memoryContent: ReactNode;
    if (memoriesLoading) {
      memoryContent = (
        <View className="memoryState"><View className="imageSpinner" /><Text>正在读取记忆…</Text></View>
      );
    } else if (memories.length === 0) {
      memoryContent = (
        <View className="memoryState">
          <Text className="memoryStateTitle">还没有保存偏好</Text>
          <Text className="memoryStateHint">例如“回复风格：请使用简洁中文”</Text>
        </View>
      );
    } else {
      memoryContent = (
        <View className="memoryList">
          {memories.map((memory) => (
            <View className="memoryCard" key={memory.memoryKey}>
              <View className="memoryCardBody">
                <Text className="memoryCardKey">{memory.memoryKey}</Text>
                <Text className="memoryCardValue">{memory.value}</Text>
              </View>
              <View className="memoryCardActions">
                <Text onClick={() => setMemoryEditor({ originalKey: memory.memoryKey, memoryKey: memory.memoryKey, value: memory.value })}>编辑</Text>
                <Text className="memoryDelete" onClick={() => void deleteUserMemory(memory)}>删除</Text>
              </View>
            </View>
          ))}
        </View>
      );
    }
    return (
      <ScrollView className="memoryPage" scrollY enhanced showScrollbar={false}>
        <Header title="AI 偏好记忆" onBack={() => setScreen("account")} />
        <View className="memoryHero">
          <Text className="memoryHeroIcon">✦</Text>
          <Text className="memoryHeroTitle">让 AI 更懂你</Text>
          <Text className="memoryHeroHint">保存回复风格、身份背景等长期偏好，之后的每次对话都会自动参考。</Text>
        </View>
        <View className="memorySectionHeader">
          <Text>我的偏好</Text>
          <Text className="memoryCount">{memories.length} / {MAX_PREFERENCE_MEMORIES}</Text>
        </View>
        {memoryContent}
        <Button
          className="memoryAddButton"
          disabled={atLimit || memoriesLoading}
          onClick={() => setMemoryEditor({ memoryKey: "", value: "" })}
        >{atLimit ? `已达到 ${MAX_PREFERENCE_MEMORIES} 条上限` : "＋ 添加一条偏好"}</Button>
        {workspaceError ? <Text className="errorBanner memoryError">{workspaceError}</Text> : null}
        {memoryEditor ? (
          <MemoryEditorSheet
            editor={memoryEditor}
            saving={memorySaving}
            onChange={setMemoryEditor}
            onClose={() => setMemoryEditor(null)}
            onSave={() => void saveUserMemory()}
          />
        ) : null}
      </ScrollView>
    );
  }

  if (screen === "shared") {
    let sharedContent: ReactNode;
    if (sharedLoading) {
      sharedContent = (
        <View className="sharedState"><View className="imageSpinner" /><Text>正在打开分享…</Text></View>
      );
    } else if (!sharedConversation) {
      sharedContent = (
        <View className="sharedState">
          <Text className="sharedStateTitle">分享内容无法打开</Text>
          <Text className="sharedStateHint">链接可能已经失效或被分享者撤销</Text>
        </View>
      );
    } else {
      sharedContent = (
        <>
          <View className="sharedHero">
            <Text className="sharedEyebrow">AI 对话分享</Text>
            <Text className="sharedTitle">{sharedConversation.title || "未命名对话"}</Text>
            <Text className="sharedMeta">共 {sharedMessages.length} 条消息 · 内容为分享时快照</Text>
          </View>
          <View className="sharedMessageList">
            {sharedMessages.map((message) => (
              <View className={`message message-${message.role}`} key={message.id}>
                <Text className="messageAuthor">{message.role === "user" ? "提问" : "AI"}</Text>
                <View className="messageContent">
                  {message.imageSource ? <Image className="messageImage" src={message.imageSource} mode="widthFix" onClick={() => void previewImage(message.imageSource)} /> : null}
                  {message.text ? <Markdown>{message.text}</Markdown> : null}
                </View>
              </View>
            ))}
          </View>
          <Button className="sharedCloneButton" loading={shareWorking} disabled={shareWorking} onClick={() => void cloneSharedConversation()}>
            保存到我的对话
          </Button>
          <Button className="sharedReshareButton" openType="share">转发给微信好友</Button>
        </>
      );
    }
    return (
      <View className="sharedPage">
        <Header title="好友分享" onBack={() => { setPreparedShare(null); setScreen("home"); }} />
        <ScrollView className="sharedContent" scrollY enhanced showScrollbar={false}>
          {sharedContent}
          {workspaceError ? <Text className="errorBanner sharedError">{workspaceError}</Text> : null}
        </ScrollView>
      </View>
    );
  }

  if (screen === "account") {
    const planName = billingOverview?.plan?.name || user.subscriptionPlanName || user.subscriptionTier || "标准账户";
    const usagePercent = periodUsagePercent(
      billingOverview?.periodUsedUSD ?? 0,
      billingOverview?.periodCreditUSD ?? 0,
    );
    const latestMonthly = [...monthlyUsage].sort((left, right) =>
      right.monthStartAt.localeCompare(left.monthStartAt))[0] ?? null;
    const subscriptionStatus = user.subscriptionStatus === "active"
      ? "使用中"
      : user.subscriptionStatus === "canceled"
        ? "已取消"
        : user.subscriptionStatus || "正常";
    return (
      <ScrollView className="accountPage" scrollY enhanced showScrollbar={false}>
        <Header title="我的账户" onBack={() => setScreen("home")} />
        <View className="accountHero">
          <View className="accountAvatar">{(user.displayName || "友").slice(0, 1)}</View>
          <View className="accountIdentity">
            <Text className="accountName">{user.displayName || "微信用户"}</Text>
            <Text className="accountPlan">{planName} · {subscriptionStatus}</Text>
          </View>
          <Text className="accountID">ID {user.publicID.slice(0, 8)}</Text>
        </View>

        {accountLoading && !billingOverview ? (
          <View className="accountLoadingCard">
            <View className="imageSpinner" />
            <Text>正在加载账户信息…</Text>
          </View>
        ) : null}
        {accountError ? <Text className="errorBanner accountError">{accountError}</Text> : null}

        <View className="accountSection">
          <Text className="accountSectionTitle">账户资产</Text>
          <View className="balanceCard">
            <Text className="accountCardLabel">可用余额</Text>
            <Text className="balanceValue">{formatUSD(billingOverview?.account?.balanceUSD ?? balanceUSD ?? 0)}</Text>
            <Text className="accountCardHint">模型调用结算后自动更新</Text>
          </View>
        </View>

        <View className="accountSection">
          <Text className="accountSectionTitle">订阅使用情况</Text>
          <View className="usageCard">
            <View className="usageHeader">
              <View>
                <Text className="accountCardLabel">{planName}</Text>
                <Text className="usageHeadline">
                  {billingOverview?.mode === "period" ? `已使用 ${usagePercent}%` : "本月使用情况"}
                </Text>
              </View>
              <Text className="usageRemaining">
                {billingOverview?.mode === "period"
                  ? `剩余 ${formatUSD(billingOverview.periodRemainingUSD)}`
                  : `${latestMonthly?.callCount ?? 0} 次调用`}
              </Text>
            </View>
            {billingOverview?.mode === "period" ? (
              <>
                <View className="usageProgressTrack">
                  <View className="usageProgressFill" style={{ width: `${usagePercent}%` }} />
                </View>
                <View className="usageStats">
                  <AccountMetric label="本期额度" value={formatUSD(billingOverview.periodCreditUSD)} />
                  <AccountMetric label="已使用" value={formatUSD(billingOverview.periodUsedUSD)} />
                  <AccountMetric label="重置日期" value={formatAccountDate(billingOverview.periodEndAt)} />
                </View>
              </>
            ) : (
              <View className="usageStats">
                <AccountMetric label="本月消费" value={formatUSD(latestMonthly?.billedUSD ?? 0)} />
                <AccountMetric label="Tokens" value={(latestMonthly?.totalTokens ?? 0).toLocaleString()} />
                <AccountMetric label="调用次数" value={String(latestMonthly?.callCount ?? 0)} />
              </View>
            )}
          </View>
        </View>

        <View className="accountSection">
          <Text className="accountSectionTitle">账户信息</Text>
          <View className="accountInfoCard">
            <AccountInfoRow label="订阅状态" value={subscriptionStatus} />
            <AccountInfoRow label="计费方式" value={billingOverview?.mode === "period" ? "订阅额度" : billingOverview?.mode === "usage" ? "按量计费" : "用量记录"} />
            <AccountInfoRow label="周期开始" value={formatAccountDate(billingOverview?.periodStartAt)} />
            <AccountInfoRow label="周期结束" value={formatAccountDate(billingOverview?.periodEndAt)} />
          </View>
        </View>

        <View className="accountSection">
          <Text className="accountSectionTitle">个性化</Text>
          <View className="accountLinkCard" onClick={openUserMemories}>
            <View className="accountLinkIcon">✦</View>
            <View className="accountLinkBody">
              <Text className="accountLinkTitle">AI 偏好记忆</Text>
              <Text className="accountLinkHint">让 AI 在后续对话中记住你的习惯</Text>
            </View>
            <Text className="accountLinkArrow">›</Text>
          </View>
        </View>

        <Button className="accountRefreshButton" loading={accountLoading} disabled={accountLoading} onClick={() => void loadAccountCenter()}>
          刷新账户信息
        </Button>
        <View className="accountLegal">
          <Text onClick={openPrivacy}>隐私保护指引</Text>
          <Text onClick={openTermsNotice}>用户协议</Text>
          <Text>版本 {MINIAPP_BUILD_VERSION}</Text>
        </View>
      </ScrollView>
    );
  }

  if ((screen === "chat" || screen === "image") && currentConversation) {
    const chatModelOptions = modelsForKind(models, "chat");
    const mediaModelOptions = imageModelOptions(models);
    const openModelPicker = (mode: ConversationMode, optionCount: number) => {
      if (running || uploading || optionCount === 0) {
        return;
      }
      void Taro.hideKeyboard();
      setModelPickerMode(mode);
    };
    const toggleNetworkSearch = () => {
      if (running || uploading) {
        return;
      }
      if (!networkSearchAvailable) {
        void Taro.showToast({ title: "联网搜索尚未配置或未启用", icon: "none" });
        return;
      }
      setNetworkSearchEnabled((enabled) => !enabled);
    };
    const selectWorkspaceModel = (mode: ConversationMode, nextModel: PublicModelResponse) => {
      setModelPickerMode(null);
      if (mode === "chat") {
        if (nextModel.platformModelName === selectedChatModel?.platformModelName) {
          return;
        }
        setSelectedChatModel(nextModel);
        void Taro.showToast({ title: "已切换，下条消息生效", icon: "none" });
        return;
      }
      if (nextModel.platformModelName === selectedImageModel?.platformModelName) {
        return;
      }
      setSelectedImageModel(nextModel);
      const decision = resolveImageSubmitDecision(nextModel, Boolean(pendingImage));
      if (decision.blockedReason) {
        setWorkspaceError(decision.blockedReason === "image_edit_input_required"
          ? "该模型用于图片编辑，请先上传需要编辑的图片"
          : "该模型不支持当前图片编辑，请移除原图或切换模型");
      } else {
        setWorkspaceError("");
      }
    };
    return (
      <View className="workspace">
        <Header
          title={screen === "chat" ? "AI 对话" : "AI 生图"}
          onBack={goHome}
          onMore={() => void showConversationActions(currentConversation)}
        />
        <Text className="conversationTitle">{currentConversation.title || (screen === "chat" ? "新对话" : "新图片")}</Text>
        {screen === "chat" ? (
          <View className="messageListShell">
            <ScrollView
              className="messageList"
              scrollY
              enhanced
              bounces={false}
              lowerThreshold={80}
              scrollAnchoring
              showScrollbar={false}
              scrollTop={chatScrollTop}
              onScroll={handleChatScroll}
              onScrollToLower={handleChatScrollToLower}
              onTouchStart={() => { chatTouchingRef.current = true; }}
              onTouchEnd={() => { chatTouchingRef.current = false; }}
              onTouchCancel={() => { chatTouchingRef.current = false; }}
            >
              {messages.length === 0 ? (
                <View className="emptyWorkspace">
                  <Text className="emptyIcon">✦</Text>
                  <Text className="emptyTitle">想聊点什么？</Text>
                  <Text className="emptyHint">可以提问、写作、总结或分析问题</Text>
                </View>
              ) : messages.map((message) => (
                <View id={`message-${message.id}`} className={`message message-${message.role}`} key={message.id}>
                  <Text className="messageAuthor">{message.role === "user" ? "你" : "AI"}</Text>
                  <View className="messageContent">
                    {message.imageSource ? (
                      <Image className="messageImage" src={message.imageSource} mode="widthFix" onLoad={handleConversationImageLoad} />
                    ) : null}
                    {message.role === "assistant" ? (
                      <ConversationTrace trace={message.processTrace} pending={Boolean(message.pending)} />
                    ) : null}
                    {message.text ? <Markdown>{message.text}</Markdown> : null}
                    {message.pending ? (
                      <View className="chatActivity">
                        <View className="chatActivityDot" />
                        <Text>{message.activityStatus || "正在生成回复"}</Text>
                      </View>
                    ) : null}
                  </View>
                  {message.role === "assistant" && !message.pending && message.text ? (
                    <View className="messageActions">
                      <Text onClick={() => void copyAssistantAnswer(message)}>复制</Text>
                      <Text onClick={() => void regenerateChatAnswer(message)}>重新生成</Text>
                    </View>
                  ) : null}
                </View>
              ))}
            </ScrollView>
            {!chatAutoFollow && messages.length > 0 ? (
              <View className="scrollToBottomButton" onClick={() => enableChatAutoFollow(true)}>
                <Text>↓</Text>
              </View>
            ) : null}
          </View>
        ) : (
          <View className="messageListShell">
            <ScrollView
              className="imageCanvas"
              scrollY
              enhanced
              bounces={false}
              lowerThreshold={80}
              scrollAnchoring
              showScrollbar={false}
              scrollTop={chatScrollTop}
              onScroll={handleChatScroll}
              onScrollToLower={handleChatScrollToLower}
              onTouchStart={() => { chatTouchingRef.current = true; }}
              onTouchEnd={() => { chatTouchingRef.current = false; }}
              onTouchCancel={() => { chatTouchingRef.current = false; }}
            >
            {messages.length === 0 ? (
              <View className="emptyWorkspace imageEmpty">
                <Text className="emptyIcon">◈</Text>
                <Text className="emptyTitle">描述你想看到的画面</Text>
                <Text className="emptyHint">例如：雨后的未来城市，电影感，暖色灯光</Text>
              </View>
            ) : messages.map((message) => message.role === "user" ? (
              <View id={`message-${message.id}`} className="message message-user imagePromptMessage" key={message.id}>
                <Text className="messageAuthor">你</Text>
                <View className="messageContent">
                  {message.imageSource ? (
                    <Image
                      className="messageImage"
                      src={message.imageSource}
                      mode="widthFix"
                      onLoad={handleConversationImageLoad}
                      onClick={() => void previewImage(message.imageSource)}
                    />
                  ) : null}
                  {message.text ? <Markdown>{message.text}</Markdown> : null}
                </View>
              </View>
            ) : (
              <View id={`message-${message.id}`} className="imageTimelineItem" key={message.id}>
                <Text className="messageAuthor">AI</Text>
                {message.pending || (!message.imageSource && Boolean(message.imageFileID)) ? (
                  <View className="imageLoadingCard">
                    <View className={`imageLoadingFrame ${imageLoadingPhaseClass(message.imageStatus)}`}>
                      <View className="imageLoadingGlow imageLoadingGlowPrimary" />
                      <View className="imageLoadingGlow imageLoadingGlowSecondary" />
                      <View className="imageLoadingOrbit" />
                      <View className="imageLoadingContent">
                        <View className="imageSpinner" />
                        <Text className="imageLoadingLabel">{message.imageStatus || "AI 正在生成图片"}</Text>
                      </View>
                    </View>
                  </View>
                ) : message.imageSource ? (
                  <View className="imageCard">
                    <Image
                      className="generatedImage"
                      src={message.imageSource}
                      mode="widthFix"
                      onLoad={handleConversationImageLoad}
                      onClick={() => void previewImage(message.imageSource)}
                    />
                    <Text className="imageStatus">{message.imageStatus || "图片生成完成"}</Text>
                    <View className="imageActions">
                      <Button className="imageActionButton" disabled={running} onClick={() => void regenerateImageAnswer(message)}>
                        重新生成
                      </Button>
                      {message.imageFileID ? (
                        <Button className="imageActionButton imageEditButton" onClick={() => void continueEditingImage(message)}>
                          继续编辑
                        </Button>
                      ) : null}
                      <Button className="imageActionButton" onClick={() => void saveImage(message.imageSource)}>保存到相册</Button>
                    </View>
                  </View>
                ) : (
                  <View className="imageResultStatus">
                    <Text className="imageResultStatusText">{message.imageStatus || message.text || "未收到可显示的图片"}</Text>
                    {message.parentPublicID && !message.id.startsWith("local-") ? (
                      <View className="imageResultActions">
                        <Button className="imageActionButton" disabled={running} onClick={() => void regenerateImageAnswer(message)}>
                          重试
                        </Button>
                      </View>
                    ) : null}
                  </View>
                )}
              </View>
              ))}
            </ScrollView>
            {!chatAutoFollow && messages.length > 0 ? (
              <View className="scrollToBottomButton" onClick={() => enableChatAutoFollow(true)}>
                <Text>↓</Text>
              </View>
            ) : null}
          </View>
        )}
        {workspaceError ? <Text className="errorBanner workspaceError">{workspaceError}</Text> : null}
        <View className="composerSafeArea" style={composerKeyboardStyle(keyboardHeight)}>
          {screen === "chat" ? (
            <View className="composerToolbar">
              <View className="composerToolbarStart">
                <Text className="composerToolbarLabel">本次回复</Text>
                <View
                  className={`networkSearchToggle ${networkSearchEnabled ? "networkSearchToggleActive" : ""} ${!networkSearchAvailable ? "networkSearchToggleUnavailable" : ""}`}
                  onClick={toggleNetworkSearch}
                >
                  <Text className="networkSearchIcon">◎</Text>
                  <Text>{networkSearchEnabled && networkSearchAvailable ? "联网可用" : "联网"}</Text>
                </View>
              </View>
              <View
                className={`chatModelPicker ${running || uploading ? "chatModelPickerDisabled" : ""}`}
                onClick={() => openModelPicker("chat", chatModelOptions.length)}
              >
                <Text className="chatModelPickerName">
                  {selectedChatModel?.platformModelName || "暂无可用模型"}
                </Text>
                <Text className="chatModelPickerChevron">⌄</Text>
              </View>
            </View>
          ) : (
            <View className="composerToolbar">
              <Text className={`imageModeBadge ${pendingImage ? "imageModeBadgeEdit" : ""}`}>
                {pendingImage ? "图片编辑" : "文字生图"}
              </Text>
              <View
                className={`chatModelPicker ${running || uploading ? "chatModelPickerDisabled" : ""}`}
                onClick={() => openModelPicker("image", mediaModelOptions.length)}
              >
                <Text className="chatModelPickerName">{selectedImageModel?.platformModelName || "暂无可用模型"}</Text>
                <Text className="chatModelPickerChevron">⌄</Text>
              </View>
            </View>
          )}
          {pendingImage ? (
            <View className="pendingImageRow">
              <Image className="pendingImage" src={pendingImage.localPath} mode="aspectFill" />
              <View className="pendingImageBody">
                <Text className="pendingImageName">{pendingImage.fileName}</Text>
                {screen === "image" ? <Text className="pendingImageMode">将根据提示编辑这张图片</Text> : null}
              </View>
              <Text className="pendingImageRemove" onClick={removePendingImage}>移除</Text>
            </View>
          ) : null}
          <View className="composer">
            <Button
              className="attachButton"
              disabled={running || uploading}
              onClick={screen === "chat" ? chooseChatImage : chooseImageEditInput}
            >
              {uploading ? "…" : "＋"}
            </Button>
            <Textarea
              className="composerInput"
              value={prompt}
              placeholder={screen === "chat"
                ? "输入消息…"
                : pendingImage
                  ? "描述你想怎样修改这张图片…"
                  : "描述你想生成的图片…"}
              maxlength={8000}
              autoHeight
              adjustPosition={false}
              cursorSpacing={0}
              showConfirmBar={false}
              disabled={running || uploading}
              onInput={(event) => setPrompt(event.detail.value)}
            >
              <KeyboardAccessory style={{ height: "1px" }} />
            </Textarea>
            <Button
              className={`sendButton ${running ? "stopSendButton" : ""}`}
              disabled={stopping || (!running && (
                uploading ||
                (screen === "chat" && (!selectedChatModel || (!prompt.trim() && !pendingImage))) ||
                (screen === "image" && (!selectedImageModel || !prompt.trim()))
              ))}
              onClick={running
                ? () => void stopGeneration()
                : screen === "chat"
                  ? sendChat
                  : generateImage}
            >
              {stopping ? "停止中" : running ? "停" : screen === "chat" ? "发送" : "生成"}
            </Button>
          </View>
          {keyboardHeight <= 0 ? (
            <Text className="aiNotice">内容由 AI 生成，请注意核实</Text>
          ) : null}
        </View>
        {modelPickerMode ? (
          <ModelPickerSheet
            title={modelPickerMode === "chat" ? "选择对话模型" : "选择生图模型"}
            options={modelPickerMode === "chat" ? chatModelOptions : mediaModelOptions}
            selectedName={modelPickerMode === "chat"
              ? selectedChatModel?.platformModelName ?? ""
              : selectedImageModel?.platformModelName ?? ""}
            onClose={() => setModelPickerMode(null)}
            onSelect={(model) => selectWorkspaceModel(modelPickerMode, model)}
          />
        ) : null}
        {shareOverlay}
      </View>
    );
  }

  return (
    <View className="page homePage">
      <View className="homeHeader">
        <Text className="eyebrow">AI省着用</Text>
        <View className="avatar" onClick={openAccountCenter}>
          <Text>{(user.displayName || "友").slice(0, 1)}</Text>
        </View>
      </View>
      <Text className="homeTitle">今天想做什么？</Text>

      {dailyCheckin?.enabled ? (
        <DailyCheckinEntry
          status={dailyCheckin}
          onOpen={openDailyCheckin}
        />
      ) : null}

      <View className="quickGrid">
        <View
          className={`quickCard chatQuick ${presets.chatModel ? "" : "quickCardDisabled"}`}
          onClick={() => presets.chatModel
            ? createWithModel(presets.chatModel, "chat")
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
            ? createWithModel(presets.imageModel, "image")
            : setWorkspaceError("AI 生图暂不可用，请联系管理员检查默认模型或账号权限")}
        >
          <View className="quickIcon">◈</View>
          <Text className="quickTitle">AI 生图</Text>
          <Text className="quickDescription">把想象变成画面</Text>
          <Text className="quickAction">{presets.imageModel ? "开始创作 ›" : "暂不可用"}</Text>
        </View>
      </View>

      <View className="sectionHeader">
        <Text className="sectionTitle">最近对话</Text>
        <Text className="sectionMeta" onClick={openHistory}>搜索与收藏 · {conversationTotal} 条 ›</Text>
      </View>
      {conversations.length === 0 ? (
        <View className="emptyHistory">
          <Text className="emptyHistoryTitle">还没有对话</Text>
          <Text className="emptyHistoryHint">从上面的快捷入口开始吧</Text>
        </View>
      ) : (
        <View className="conversationList">
          {conversations.map((conversation) => (
            <ConversationHistoryRow
              busy={managingConversationID === conversation.publicID}
              conversation={conversation}
              key={conversation.publicID}
              mode={sessionRef.current?.conversationMode(conversation) ?? "chat"}
              open={openSwipeConversationID === conversation.publicID}
              onDelete={() => void requestDeleteConversation(conversation)}
              onEnter={() => void enterConversation(conversation)}
              onMore={() => void showConversationActions(conversation)}
              onOpen={() => setOpenSwipeConversationID(conversation.publicID)}
              onRename={() => void requestRenameConversation(conversation)}
              onReset={() => setOpenSwipeConversationID("")}
              onStar={() => void toggleConversationStar(conversation)}
            />
          ))}
          {conversations.length < conversationTotal ? (
            <Button
              className="loadMoreButton"
              disabled={loadingMoreConversations}
              onClick={() => void loadMoreConversations()}
            >
              {loadingMoreConversations ? "正在加载…" : "加载更多对话"}
            </Button>
          ) : null}
        </View>
      )}
      {workspaceError ? <Text className="errorBanner">{workspaceError}</Text> : null}
      <Text className="privacyNote">微信一键登录 · 登录凭据仅保存在本次运行内存</Text>
      <View className="legalLinks">
        <Text onClick={openPrivacy}>隐私保护指引</Text>
        <Text onClick={openTermsNotice}>用户协议</Text>
      </View>
      <Text className="versionNote">版本 {MINIAPP_BUILD_VERSION}</Text>
      {shareOverlay}
    </View>
  );
}

function AccountMetric({ label, value }: { label: string; value: string }) {
  return (
    <View className="accountMetric">
      <Text className="accountMetricValue">{value}</Text>
      <Text className="accountMetricLabel">{label}</Text>
    </View>
  );
}

function AccountInfoRow({ label, value }: { label: string; value: string }) {
  return (
    <View className="accountInfoRow">
      <Text className="accountInfoLabel">{label}</Text>
      <Text className="accountInfoValue">{value}</Text>
    </View>
  );
}

function Header({ title, onBack, onMore }: { title: string; onBack: () => void; onMore?: () => void }) {
  return (
    <View className="topBar">
      <View className="backButton" onClick={onBack}>‹</View>
      <Text className="topBarTitle">{title}</Text>
      {onMore ? <View className="moreButton" onClick={onMore}>•••</View> : <View className="topBarSpacer" />}
    </View>
  );
}

function ModelPickerSheet({
  title,
  options,
  selectedName,
  onClose,
  onSelect,
}: {
  title: string;
  options: PublicModelResponse[];
  selectedName: string;
  onClose(): void;
  onSelect(model: PublicModelResponse): void;
}) {
  return (
    <View className="modelPickerBackdrop" onClick={onClose}>
      <View className="modelPickerSheet" onClick={(event) => event.stopPropagation()}>
        <View className="modelPickerHeader">
          <View>
            <Text className="modelPickerTitle">{title}</Text>
            <Text className="modelPickerCount">共 {options.length} 个可用模型</Text>
          </View>
          <Text className="modelPickerClose" onClick={onClose}>×</Text>
        </View>
        <ScrollView className="modelPickerList" scrollY enhanced showScrollbar={false}>
          {options.map((model) => {
            const selected = model.platformModelName === selectedName;
            return (
              <View
                className={`modelPickerOption ${selected ? "modelPickerOptionSelected" : ""}`}
                key={model.platformModelName}
                onClick={() => onSelect(model)}
              >
                <View className="modelPickerOptionBody">
                  <Text className="modelPickerName">{model.platformModelName}</Text>
                  {model.vendor ? <Text className="modelPickerVendor">{model.vendor}</Text> : null}
                </View>
                <Text className="modelPickerCheck">{selected ? "✓" : ""}</Text>
              </View>
            );
          })}
        </ScrollView>
      </View>
    </View>
  );
}

function ShareSheet({
  share,
  onClose,
  onCopy,
}: {
  share: PreparedShare;
  onClose(): void;
  onCopy(): void;
}) {
  return (
    <View className="shareBackdrop" onClick={onClose}>
      <View className="shareSheet" onClick={(event) => event.stopPropagation()}>
        <View className="shareHandle" />
        <Text className="shareSheetTitle">分享这段对话</Text>
        <Text className="shareSheetHint">将发送当前对话的公开快照，后续新消息不会自动加入。</Text>
        <Button className="shareWechatButton" openType="share">发送给微信好友</Button>
        <Button className="shareCopyButton" onClick={onCopy}>复制网页分享链接</Button>
        <Text className="shareSheetID">分享编号 {share.shareID.slice(0, 8)}</Text>
      </View>
    </View>
  );
}

function MemoryEditorSheet({
  editor,
  saving,
  onChange,
  onClose,
  onSave,
}: {
  editor: MemoryEditor;
  saving: boolean;
  onChange(editor: MemoryEditor): void;
  onClose(): void;
  onSave(): void;
}) {
  const editing = Boolean(editor.originalKey);
  const canSave = Boolean(editor.memoryKey.trim() && editor.value.trim() && !saving);
  return (
    <View className="memoryEditorBackdrop" onClick={onClose}>
      <View className="memoryEditorSheet" onClick={(event) => event.stopPropagation()}>
        <View className="memoryEditorHeader">
          <View>
            <Text className="memoryEditorTitle">{editing ? "编辑偏好" : "添加偏好"}</Text>
            <Text className="memoryEditorHint">AI 会在之后的对话中自动参考</Text>
          </View>
          <Text className="memoryEditorClose" onClick={onClose}>×</Text>
        </View>
        <Text className="memoryEditorLabel">偏好名称</Text>
        <Input
          className={`memoryEditorInput ${editing ? "memoryEditorInputDisabled" : ""}`}
          disabled={editing || saving}
          maxlength={128}
          value={editor.memoryKey}
          placeholder="例如：回复风格"
          onInput={(event) => onChange({ ...editor, memoryKey: event.detail.value })}
        />
        <Text className="memoryEditorLabel">偏好内容</Text>
        <Textarea
          className="memoryEditorTextarea"
          disabled={saving}
          maxlength={10000}
          value={editor.value}
          placeholder="例如：请使用简洁的中文回复，并先给出结论"
          onInput={(event) => onChange({ ...editor, value: event.detail.value })}
        />
        <View className="memoryEditorActions">
          <Button className="memoryEditorCancel" disabled={saving} onClick={onClose}>取消</Button>
          <Button className="memoryEditorSave" disabled={!canSave} loading={saving} onClick={onSave}>
            {saving ? "保存中" : "保存"}
          </Button>
        </View>
      </View>
    </View>
  );
}

function ConversationHistoryRow({
  busy,
  conversation,
  mode,
  open,
  onDelete,
  onEnter,
  onMore,
  onOpen,
  onRename,
  onReset,
  onStar,
}: {
  busy: boolean;
  conversation: ConversationListItem;
  mode: ConversationMode;
  open: boolean;
  onDelete(): void;
  onEnter(): void;
  onMore(): void;
  onOpen(): void;
  onRename(): void;
  onReset(): void;
  onStar(): void;
}) {
  const touchStart = useRef<{
    actionWidth: number;
    axis: "horizontal" | "vertical" | null;
    lastX: number;
    lastY: number;
    x: number;
    y: number;
  } | null>(null);
  const swiped = useRef(false);
  const [dragOffset, setDragOffset] = useState<number | null>(null);

  const handleTouchStart: ViewTouchHandler = (event) => {
    const touch = readTouchPoint(event);
    touchStart.current = touch ? {
      actionWidth: conversationActionWidthPx(),
      axis: null,
      lastX: touch.x,
      lastY: touch.y,
      x: touch.x,
      y: touch.y,
    } : null;
    swiped.current = false;
  };

  const handleTouchMove: ViewTouchHandler = (event) => {
    const start = touchStart.current;
    const touch = readTouchPoint(event);
    if (!start || !touch) {
      return;
    }
    start.lastX = touch.x;
    start.lastY = touch.y;
    const deltaX = touch.x - start.x;
    const deltaY = touch.y - start.y;
    if (!start.axis && Math.max(Math.abs(deltaX), Math.abs(deltaY)) >= 6) {
      start.axis = Math.abs(deltaX) > Math.abs(deltaY) ? "horizontal" : "vertical";
    }
    if (start.axis !== "horizontal") {
      return;
    }
    event.preventDefault();
    swiped.current = Math.abs(deltaX) >= 8;
    setDragOffset(conversationSwipeOffset(deltaX, start.actionWidth, open));
  };

  const handleTouchEnd: ViewTouchHandler = (event) => {
    const start = touchStart.current;
    const touch = readTouchPoint(event, true) ?? (start ? { x: start.lastX, y: start.lastY } : null);
    touchStart.current = null;
    setDragOffset(null);
    if (!start || !touch) {
      return;
    }
    const deltaX = touch.x - start.x;
    const deltaY = touch.y - start.y;
    const swipe = settleConversationSwipe(deltaX, deltaY, open);
    swiped.current = start.axis === "horizontal" && Math.abs(deltaX) >= 8;
    if (swipe === "open") {
      onOpen();
    } else {
      onReset();
    }
  };

  const handleTouchCancel = () => {
    touchStart.current = null;
    swiped.current = false;
    setDragOffset(null);
  };

  const handleClick = () => {
    if (swiped.current) {
      swiped.current = false;
      return;
    }
    if (open) {
      onReset();
    } else {
      onEnter();
    }
  };

  return (
    <View className={`conversationSwipe ${busy ? "conversationSwipeBusy" : ""}`}>
      <View className="conversationSwipeActions">
        <View className="conversationSwipeAction favoriteAction" onClick={onStar}>
          {conversation.isStarred ? "取消收藏" : "收藏"}
        </View>
        <View className="conversationSwipeAction renameAction" onClick={onRename}>重命名</View>
        <View className="conversationSwipeAction deleteAction" onClick={onDelete}>删除</View>
      </View>
      <View
        className={`conversationRow conversationSwipeContent ${open ? "conversationSwipeOpen" : ""}`}
        style={dragOffset === null ? undefined : { transform: `translateX(${dragOffset}px)`, transition: "none" }}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
        onTouchCancel={handleTouchCancel}
        onClick={handleClick}
      >
        <View className={`conversationIcon conversationIcon-${mode}`}>
          {mode === "image" ? "◇" : "✦"}
        </View>
        <View className="conversationBody">
          <View className="conversationNameRow">
            <Text className="conversationName">{conversation.title || "未命名对话"}</Text>
            {conversation.isStarred ? <Text className="conversationStar">★</Text> : null}
          </View>
          <Text className="conversationMeta">
            {mode === "image" ? "AI 生图" : "AI 对话"} · {conversation.messageCount} 条消息
          </Text>
        </View>
        <Text
          className="conversationMore"
          onClick={(event) => {
            event.stopPropagation();
            onMore();
          }}
        >•••</Text>
      </View>
    </View>
  );
}
