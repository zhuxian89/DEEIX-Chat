import type { ConversationResponse, MessageResponse, ReminderState, ReminderPlan } from "@deeix/api-contract";
import { Button, Image, ScrollView, Switch, Text, Textarea, View } from "@tarojs/components";
import Taro from "@tarojs/taro";
import { useCallback, useEffect, useRef, useState } from "react";
import { Markdown } from "@/components/markdown";
import { SpeechComposer } from "@/components/speech-input/speech-input";
import { canOfferCompanionGreeting, companionBeijingDate, companionImageIDs, type CompanionMemory, type CompanionProactivity, type CompanionState } from "@/product/companion-client";
import { canOfferCompanionContinuation, companionEntryPreview, CompanionInitiativeVisit, CompanionReadTracker, withCompanionInitiative } from "@/product/companion-initiative";
import { composerKeyboardHandlers, composerKeyboardStyle } from "@/product/keyboard-layout";
import { companionDisplayText } from "@/product/companion-message";
import { nextChatBottomScrollTop, shouldReleaseChatAutoFollow } from "@/product/chat-auto-scroll";
import { latestVisibleMessages, messageFromAPI, type ConversationMessage } from "@/product/message-timeline";
import { MessageTime } from "@/components/message-time";
import { MiniAppRequestAbortedError, type ChatGenerationProgress, type MiniAppSession } from "@/product/session";
import companionAvatar from "./assets/xiaohe-portrait.png";
import "./companion.scss";
import { AssistantSettings } from "./assistant-settings";
import { ReminderCards, type ReminderAction } from "./reminder-cards";
import { reminderMessages, reminderRequestID, reminderError } from "@/product/companion-reminders";

const COMPANION_NAME = "小禾";
const COMPANION_BOTTOM_THRESHOLD_PX = 80;

type SharedProps = { session: MiniAppSession; onState(state: CompanionState): void };

function CompanionAvatar({ size }: { size: "entry" | "header" | "message" }) {
  return <Image className={`companionAvatar companionAvatar--${size}`} src={companionAvatar} mode="aspectFill" />;
}

// Refresh persisted greetings and server-generated follow-ups while foregrounded.
export function CompanionEntry({ session, onState, onOpen, lastInteraction }: SharedProps & {
  onOpen(): void;
  lastInteraction: { current: number };
}) {
  const [state, setState] = useState<CompanionState | null>(null);
  const [reminderPreview, setReminderPreview] = useState("");
  const foreground = useRef(true);
  const working = useRef(false);
  const refreshAt = useRef(0);
  const stateCallback = useRef(onState);
  stateCallback.current = onState;

  useEffect(() => {
    let disposed = false;
    let visit = new CompanionInitiativeVisit();
    const hide = () => {
      foreground.current = false;
      visit.invalidate();
    };
    const show = () => {
      foreground.current = true;
      lastInteraction.current = Date.now();
      refreshAt.current = 0;
      visit = new CompanionInitiativeVisit();
    };
    Taro.onAppHide(hide);
    Taro.onAppShow(show);
    const timer = setInterval(() => {
      if (Date.now() < refreshAt.current || working.current) return;
      if (!canOfferCompanionGreeting({
        foreground: foreground.current,
        typing: false,
        replying: false,
        lastInteractionAt: lastInteraction.current,
      }, Date.now())) return;
      working.current = true;
      refreshAt.current = Date.now() + 30_000;
      const interactionAt = lastInteraction.current;
      void session.companion.open(false)
        .then(async (next) => {
          if (disposed || !foreground.current) return;
          setState(next);
          stateCallback.current(next);
          const reminderState = await session.companion.reminders(next.conversationPublicID).catch(() => null);
          if (disposed || !foreground.current) return;
          if (reminderState) {
            const due = reminderState.occurrences.filter((item) => item.status === "due")
              .sort((a, b) => Date.parse(b.dueAt) - Date.parse(a.dueAt))[0];
            setReminderPreview(due ? `提醒：${due.title}已到时间` : "");
          }
          const eligible = () => !disposed && foreground.current && lastInteraction.current === interactionAt && !next.quiet;
          if (!eligible()) return;
          if (!next.initiativeVersion) {
            const legacy = await session.companion.open(true);
            if (eligible()) { setState(legacy); stateCallback.current(legacy); }
            return;
          }
          await visit.consider(session.companion, "home", 0, eligible, (message) => {
            const updated = withCompanionInitiative(next, message);
            setState(updated);
            stateCallback.current(updated);
          });
        })
        .catch(() => { /* The entry stays hidden until the backend enables it. */ })
        .finally(() => { working.current = false; });
    }, 1000);
    return () => {
      disposed = true;
      visit.invalidate();
      clearInterval(timer);
      Taro.offAppHide(hide);
      Taro.offAppShow(show);
    };
  }, [session, lastInteraction]);

  if (!state) return null;

  return (
    <View className="companionEntryFrame" onClick={onOpen}>
      <View className="companionEntry">
        <View className="companionEntryPortrait">
          <CompanionAvatar size="entry" />
        </View>
        <View className="companionEntryBody">
          <Text className="companionEntryTitle">{COMPANION_NAME} <Text className="companionTag">AI 聊天伙伴</Text></Text>
          <Text className="companionEntryText">{reminderPreview || companionEntryPreview(state)}</Text>
          <Text className="companionEntryAction">和{COMPANION_NAME}聊聊 ›</Text>
        </View>
      </View>
    </View>
  );
}

type DisplayMessage = ConversationMessage & {
  images?: string[];
  fileIDs?: string[];
  serverID?: number;
  topic?: CompanionState["topic"];
  reminderIDs?: string[];
  occurrenceID?: string;
};

function toDisplay(message: MessageResponse): DisplayMessage | null {
  const normalized = messageFromAPI(message);
  if (!normalized) return null;
  const fileIDs = companionImageIDs(message.attachments);
  return {
    ...normalized,
    fileIDs,
    serverID: message.id,
    createdAt: message.createdAt,
    text: fileIDs.length ? normalized.text.replace(/!\[[^\]]*\]\([^)]*\)/gu, "").trim() : normalized.text,
  };
}

function visibleMessages(items: MessageResponse[]): DisplayMessage[] {
  const messages = items.map(toDisplay).filter((item): item is DisplayMessage => item !== null);
  const visible = new Set(latestVisibleMessages(messages).map((item) => item.id));
  return messages.filter((item) => visible.has(item.id));
}

function errorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : "暂时没连上，请稍后再试";
  if (message === "resource conflict") return `${COMPANION_NAME}还在回复，等这轮结束后再试`;
  if (message === "resource not found") return `${COMPANION_NAME}暂时不可用，请稍后重试`;
  return message;
}

export function CompanionPanel({ session, onState, onBack, initial }: SharedProps & { onBack(): void; initial: CompanionState | null }) {
  const [state, setState] = useState(initial);
  const [conversation, setConversation] = useState<ConversationResponse | null>(null);
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [reminders, setReminders] = useState<ReminderState | null>(null);
  const [reminderOpen, setReminderOpen] = useState(false);
  const [reminderNow, setReminderNow] = useState(Date.now());
  const reminderOffset = useRef(0);
  const reminderVersion = useRef(0);
  const reminderRetry = useRef<{ input: string; requestID: string } | null>(null);
  const [draft, setDraft] = useState("");
  const [speechActive, setSpeechActive] = useState(false);
  const [attachment, setAttachment] = useState<{ path: string; fileID: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [memoryOpen, setMemoryOpen] = useState(false);
  const [memoryBusy, setMemoryBusy] = useState(false);
  const [editor, setEditor] = useState<{ memory: CompanionMemory; value: string } | null>(null);
  const [keyboard, setKeyboard] = useState(0);
  const [scrollTop, setScrollTop] = useState(0);
  const [following, setFollowing] = useState(true);
  const mounted = useRef(true);
  const autoFollow = useRef(true);
  const scrollingAt = useRef(0);
  const touching = useRef(false);
  const previousScrollTop = useRef(0);
  const historyHeight = useRef(0);
  const busyRef = useRef(false);
  const composerComposingRef = useRef(false);
  const readThrough = useRef(0);
  const readTracker = useRef(new CompanionReadTracker());
  const initiativeVisit = useRef(new CompanionInitiativeVisit());
  const initiativeForeground = useRef(true);
  const stateRefreshAt = useRef(0);
  const stateRefreshing = useRef(false);
  const lastInteractionAt = useRef(Date.now());
  const replyVisible = useRef({ id: 0, at: 0 });
  const initiativeContext = useRef({ state, messages, draft, attachment, busy, uploading, loading, memoryOpen: memoryOpen || reminderOpen, speechActive });
  initiativeContext.current = { state, messages, draft, attachment, busy, uploading, loading, memoryOpen: memoryOpen || reminderOpen, speechActive };

  const recordCompanionInteraction = useCallback(() => {
    lastInteractionAt.current = Date.now();
    initiativeVisit.current.invalidate();
  }, []);

  const measureHistory = useCallback(() => {
    Taro.nextTick(() => {
      if (!mounted.current) return;
      Taro.createSelectorQuery().select("#companionMessages").boundingClientRect((rect) => {
        if (rect && !Array.isArray(rect)) historyHeight.current = rect.height;
      }).exec();
    });
  }, []);

  useEffect(() => {
    if (!memoryOpen) measureHistory();
  }, [memoryOpen, keyboard, attachment, error, loading, measureHistory]);

  const updateState = (next: CompanionState) => {
    if (!mounted.current) return;
    setState(next);
    onState(next);
  };

  const updateStreamMessage = (messageID: string, progress: ChatGenerationProgress) => {
    if (!mounted.current) return;
    setMessages((current) => current.map((message) => {
      if (message.id !== messageID) return message;
      return {
        ...message,
        text: progress.text,
        activityStatus: progress.status,
        images: progress.imageSource ? [progress.imageSource] : message.images,
      };
    }));
  };

  const loadImages = async (items: DisplayMessage[]) => {
    // Bound concurrent authenticated downloads. Text is already visible.
    for (const item of items) {
      if (!mounted.current) return;
      if (!item.fileIDs?.length) continue;
      const results = await Promise.all(item.fileIDs.map((id) => session.downloadMessageImage(id).catch(() => null)));
      if (mounted.current) {
        setMessages((current) => current.map((message) => {
          if (message.id !== item.id) return message;
          return {
            ...message,
            images: results.filter((path): path is string => Boolean(path)),
            imageStatus: results.some(Boolean) ? undefined : "图片加载失败，点此重试",
          };
        }));
      }
    }
  };

  const loadHistory = async (id: string) => {
    const items = visibleMessages(await session.listMessages(id));
    if (mounted.current) {
      setMessages(items);
      void loadImages(items);
    }
    await refreshReminders(id);
    return items;
  };

  const acceptReminders = (next: ReminderState) => {
    if (!mounted.current) return;
    reminderOffset.current = Date.parse(next.serverTime) - Date.now();
    setReminderNow(Date.now() + reminderOffset.current);
    setReminders(next);
  };
  const refreshReminders = async (id: string) => {
    const version = ++reminderVersion.current;
    const next = await session.companion.reminders(id);
    if (version === reminderVersion.current) acceptReminders(next);
  };

  useEffect(() => {
    if (!conversation) return;
    let disposed = false;
    let refreshing = false;
    let refreshAt = 0;
    const tick = () => {
      if (disposed || !initiativeForeground.current) return;
      setReminderNow(Date.now() + reminderOffset.current);
      if (busyRef.current || refreshing || Date.now() < refreshAt) return;
      refreshing = true; refreshAt = Date.now() + 10_000;
      void refreshReminders(conversation.publicID).catch(() => {}).finally(() => { refreshing = false; });
    };
    const show = () => { refreshAt = 0; tick(); };
    const timer = setInterval(tick, 1000);
    Taro.onAppShow(show);
    return () => { disposed = true; clearInterval(timer); Taro.offAppShow(show); reminderVersion.current++; };
  }, [session, conversation?.publicID]);

  const initialize = async () => {
    setLoading(true);
    setError("");
    try {
      const next = await session.companion.open(false).catch(() => session.companion.state());
      if (!mounted.current) return;
      updateState(next);
      const conv = await session.getConversation(next.conversationPublicID);
      if (!mounted.current) return;
      setConversation(conv);
      const items = await loadHistory(conv.publicID);
      const pending = [...items].reverse().find((item) => item.pending && item.runID);
      if (pending?.runID && mounted.current) {
        busyRef.current = true;
        setBusy(true);
        setLoading(false);
        try {
          await session.resumeGeneration(
            pending.runID,
            { text: pending.text, imageSource: null, processTrace: pending.processTrace },
            (progress) => updateStreamMessage(pending.id, progress),
          );
          if (mounted.current) await loadHistory(conv.publicID);
        } finally {
          busyRef.current = false;
          if (mounted.current) setBusy(false);
        }
      }
    } catch (cause) {
      if (mounted.current && !(cause instanceof MiniAppRequestAbortedError)) setError(errorMessage(cause));
    } finally {
      if (mounted.current) setLoading(false);
    }
  };

  useEffect(() => {
    mounted.current = true;
    const show = () => { initiativeForeground.current = true; stateRefreshAt.current = 0; recordCompanionInteraction(); };
    const hide = () => { initiativeForeground.current = false; recordCompanionInteraction(); setKeyboard(0); };
    Taro.onAppShow(show);
    Taro.onAppHide(hide);
    void initialize();
    return () => {
      mounted.current = false;
      initiativeVisit.current.invalidate();
      Taro.offAppShow(show);
      Taro.offAppHide(hide);
      if (busyRef.current) session.abort();
    };
  }, [session]);

  useEffect(() => {
    if (!autoFollow.current || Date.now() - scrollingAt.current < 800) return;
    setScrollTop(nextChatBottomScrollTop);
  }, [messages, keyboard, state?.initiatives, reminders?.turns.length, reminders?.occurrences.length]);

  useEffect(() => {
    const latest = messages.at(-1);
    if (!busy && !loading && latest?.serverID && !latest.pending && latest.role === "assistant" && replyVisible.current.id !== latest.serverID) {
      replyVisible.current = { id: latest.serverID, at: Date.now() };
    }
  }, [messages, busy, loading]);

  useEffect(() => {
    const timer = setInterval(() => {
      const after = initiativeContext.current.messages.at(-1)?.serverID ?? 0;
      const snapshot = initiativeContext.current;
      const latest = snapshot.messages.at(-1);
      if (mounted.current && initiativeForeground.current && !snapshot.busy && !snapshot.loading && !snapshot.memoryOpen &&
        !snapshot.draft && !snapshot.attachment && !snapshot.speechActive && !touching.current &&
        Date.now() - lastInteractionAt.current >= 2000 && Date.now() >= stateRefreshAt.current && !stateRefreshing.current) {
        stateRefreshing.current = true;
        stateRefreshAt.current = Date.now() + 15_000;
        const previous = snapshot.state;
        void session.companion.state().then((next) => {
          if (mounted.current && initiativeForeground.current && initiativeContext.current.state === previous &&
            !initiativeContext.current.busy && !initiativeContext.current.memoryOpen) updateState(next);
        }).catch(() => { /* Next foreground tick retries. */ }).finally(() => { stateRefreshing.current = false; });
      }
      if (mounted.current && initiativeForeground.current && !snapshot.memoryOpen && !snapshot.busy &&
        !snapshot.loading && autoFollow.current && latest?.serverID && !latest.pending) {
        void readTracker.current.acknowledge(session.companion, latest.serverID).then((confirmed) => {
          if (mounted.current) readThrough.current = Math.max(readThrough.current, confirmed);
        });
      }
      const eligible = () => {
        const current = initiativeContext.current;
        if (!mounted.current || !current.state?.initiativeVersion || current.messages.at(-1)?.serverID !== after) return false;
        return canOfferCompanionContinuation({
          foreground: initiativeForeground.current,
          typing: Boolean(current.draft.length || current.attachment || current.speechActive),
          replying: current.busy || busyRef.current,
          readingHistory: !autoFollow.current || touching.current,
          unavailable: current.loading || current.uploading || current.memoryOpen,
          mode: current.state.quiet ? "off" : current.state.proactivity === "less" ? "less" : "normal",
          lastInteractionAt: lastInteractionAt.current,
          replyVisibleAt: replyVisible.current.at,
          latestReplyID: after,
          readThroughID: readThrough.current,
        }, Date.now());
      };
      void initiativeVisit.current.consider(session.companion, "idle", after, eligible, (message) => {
        const current = initiativeContext.current.state;
        if (current) updateState(withCompanionInitiative(current, message));
      });
    }, 1000);
    return () => { clearInterval(timer); initiativeVisit.current.invalidate(); };
  }, [session]);

  const send = async (submittedText?: string) => {
    const content = (submittedText ?? draft).trim();
    if (!conversation || !state || busyRef.current || uploading || (speechActive && submittedText === undefined) || (!content && !attachment)) return false;
    recordCompanionInteraction();
    busyRef.current = true;
    setBusy(true);
    setError("");
    autoFollow.current = true;
    setFollowing(true);
    const image = attachment;
    const id = `companion-local-${Date.now()}`;
    const pendingID = `${id}-reply`;
    setDraft("");
    setAttachment(null);
    setMessages((current) => [...current,
      { id, role: "user", text: content, images: image ? [image.path] : undefined, createdAt: new Date().toISOString() },
      { id: pendingID, role: "assistant", text: "", pending: true, activityStatus: `${COMPANION_NAME}正在想怎么接着聊…`, createdAt: new Date().toISOString() },
    ]);
    try {
      if (!image) {
        const request = reminderRetry.current?.input === content ? reminderRetry.current : { input: content, requestID: reminderRequestID() };
        reminderRetry.current = request;
        reminderVersion.current++;
        const handled = await session.companion.reminderCommand({ ...request, conversationID: conversation.publicID });
        reminderRetry.current = null;
        if (handled.handled) {
          acceptReminders(handled.snapshot);
          if (mounted.current) setMessages((current) => current.filter((item) => item.id !== id && item.id !== pendingID));
          return true;
        }
      }
      const result = await session.sendChat(
        conversation,
        state.model,
        content,
        (progress) => updateStreamMessage(pendingID, progress),
        image ? [image.fileID] : [],
        false,
        { branchReason: "default" },
        true,
      );
      if (!mounted.current) return true;
      const assistant = result.assistantMessage ? toDisplay(result.assistantMessage) : null;
      const user = result.userMessage ? toDisplay(result.userMessage) : null;
      setMessages((current) => current.map((item) => {
        if (item.id === id) return { ...item, ...user, images: item.images };
        if (item.id !== pendingID) return item;
        return {
          ...item,
          ...assistant,
          text: assistant?.text ?? result.text,
          pending: false,
          activityStatus: undefined,
          images: result.imageSource ? [result.imageSource] : item.images,
        };
      }));
      if (assistant) void loadImages([assistant]);
    } catch (cause) {
      if (mounted.current) {
        if (reminderRetry.current) setDraft(content);
        setMessages((current) => current.map((item) => item.id === pendingID
          ? { ...item, pending: false, activityStatus: "这条回复没有完成" } : item));
        if (!(cause instanceof MiniAppRequestAbortedError)) setError(errorMessage(cause));
        // Reconcile persisted turns so a retry cannot duplicate a charged run.
        await loadHistory(conversation.publicID).catch(() => {});
      }
    } finally {
      busyRef.current = false;
      if (mounted.current) setBusy(false);
    }
    return true;
  };

  const chooseImage = async () => {
    if (busyRef.current || uploading) return;
    setUploading(true);
    setError("");
    try {
      const selection = await Taro.chooseMedia({ count: 1, mediaType: ["image"], sourceType: ["album", "camera"] });
      const path = selection.tempFiles[0]?.tempFilePath;
      if (!path) return;
      const uploaded = await session.uploadChatImage(path, path.split("/").at(-1) || "photo.jpg");
      if (mounted.current) setAttachment({ path, fileID: uploaded.fileID });
    } catch (cause) {
      if (mounted.current && !String((cause as { errMsg?: string })?.errMsg ?? "").includes("cancel")) setError(errorMessage(cause));
    } finally {
      if (mounted.current) setUploading(false);
    }
  };

  const runMemoryAction = async (action: () => Promise<void>) => {
    setMemoryBusy(true);
    try {
      await action();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setMemoryBusy(false);
    }
  };

  const openMemories = async () => {
    recordCompanionInteraction();
    setKeyboard(0);
    composerComposingRef.current = false;
    setMemoryOpen(true);
    await runMemoryAction(async () => {
      setError("");
      updateState(await session.companion.state());
    });
  };

  const forget = async (memory?: CompanionMemory) => {
    const choice = await Taro.showModal({
      title: memory ? "忘记这条记忆？" : "忘记全部记忆？",
      content: "会同时清除续聊摘要和从旧资料整理的跟进。旧会话、旧待办不再用于回忆，从接下来的内容重新了解你。历史聊天仍保留供你查看。",
      confirmText: "忘记",
      confirmColor: "#b54b45",
    });
    if (!choice.confirm) return;
    await runMemoryAction(async () => {
      await session.companion.forget(memory?.id);
      updateState(await session.companion.state());
    });
  };

  const saveMemory = async () => {
    if (!editor?.value.trim()) return;
    await runMemoryAction(async () => {
      await session.companion.editMemory(editor.memory.id, editor.value.trim());
      updateState(await session.companion.state());
      setEditor(null);
    });
  };

  const toggleQuiet = async (quiet: boolean) => {
    recordCompanionInteraction();
    await runMemoryAction(async () => {
      await session.companion.setQuiet(quiet);
      updateState(await session.companion.state());
    });
  };

  const setProactivity = async (mode: CompanionProactivity) => {
    recordCompanionInteraction();
    await runMemoryAction(async () => {
      await session.companion.setProactivity(mode);
      updateState(await session.companion.state());
    });
  };

  const topicFeedback = async (topic: NonNullable<CompanionState["topic"]>, preference: "like" | "avoid") => {
    if (memoryBusy || busyRef.current) return;
    const topicURL = topic.url;
    await runMemoryAction(async () => {
      await session.companion.topicFeedback(topicURL, preference);
      updateState(await session.companion.state());
      void Taro.showToast({ title: preference === "like" ? "记下了，可以在记忆里修改" : "以后少聊这类", icon: "none" });
    });
  };

  const reminderAction = async (action: ReminderAction) => {
    if (!conversation || busyRef.current) return;
    busyRef.current = true; setBusy(true); setError("");
    reminderVersion.current++;
    try {
      const result = await session.companion.reminderCommand({ ...action, requestID: reminderRequestID(), conversationID: conversation.publicID });
      acceptReminders(result.snapshot);
    } catch (cause) {
      if (mounted.current) setError(reminderError(cause));
      await refreshReminders(conversation.publicID).catch(() => {});
    } finally { busyRef.current = false; if (mounted.current) setBusy(false); }
  };
  const editReminder = (plan: ReminderPlan) => {
    setMemoryOpen(false); setReminderOpen(false);
    setDraft(`把“${plan.title}”提醒改到`);
  };

  const timeline: DisplayMessage[] = messages.map((message) => ({ ...message, text: companionDisplayText(message) }));
  for (const initiative of state?.initiatives ?? []) {
    if (messages.length >= 100 && initiative.afterMessageID < (messages[0].serverID ?? 0)) continue;
    const next = timeline.findIndex((message) => (message.serverID ?? 0) > initiative.afterMessageID);
    timeline.splice(next < 0 ? timeline.length : next, 0, {
      id: `initiative-${initiative.id}`, role: "assistant", text: initiative.text,
      createdAt: initiative.acceptedAt, topic: initiative.topic,
    });
  }
  if (state?.greetingID && state.greeting && !state.initiatives?.some((item) => item.id === state.greetingID)) {
    const greetingAt = Date.parse(state.greetingAt);
    if (!messages.length || greetingAt >= Date.parse(messages[0].createdAt ?? "")) {
      const next = timeline.findIndex((item) => Date.parse(item.createdAt ?? "") > greetingAt);
      timeline.splice(next < 0 ? timeline.length : next, 0, { id: `greeting-${state.greetingID}`, role: "assistant", text: state.greeting, topic: state.topic, createdAt: state.greetingAt });
    }
  }
  if (reminders) {
    for (const item of reminderMessages(reminders)) {
      const next = timeline.findIndex((message) => Date.parse(message.createdAt ?? "") > Date.parse(item.createdAt ?? ""));
      timeline.splice(next < 0 ? timeline.length : next, 0, item);
    }
  }

  return (
    <View className="companionPage" onTouchStart={recordCompanionInteraction}>
      <View className="companionHeader">
        <Text className="companionBack" onClick={reminderOpen ? () => setReminderOpen(false) : memoryOpen ? () => setMemoryOpen(false) : onBack}>‹ 返回</Text>
        <View className="companionHeading">
          <CompanionAvatar size="header" />
          <View className="companionHeadingText">
            <Text className="companionName">{reminderOpen ? "我的提醒" : memoryOpen ? "记忆与跟进" : COMPANION_NAME}</Text>
            <Text className="companionSubtitle">{memoryOpen ? "由你查看、纠正和删除" : "AI 聊天伙伴 · 随时聊聊"}</Text>
          </View>
        </View>
        {!memoryOpen && !reminderOpen && <Text className="companionMemoryLink" onClick={() => void openMemories()}>记忆与跟进</Text>}
      </View>
      {error && <View className="companionError"><Text>{error}</Text>{!busy && <Text onClick={() => void initialize()}>重新连接</Text>}</View>}
      {reminderOpen ? <ScrollView scrollY enhanced bounces={false} scrollAnchoring={false} className="companionHistory">
        <View className="reminderManage">
          <Text className="reminderHint">可以在对话中创建、查询、改期或删除。每天重复的提醒，完成本次后仍会继续。</Text>
          <Text className="reminderHint">关闭小程序后的微信通知，需要在“记忆与跟进”中授权。</Text>
          {reminders && <ReminderCards state={reminders} now={reminderNow} busy={busy} onAction={(action) => void reminderAction(action)} onEdit={editReminder} />}
        </View>
      </ScrollView> : memoryOpen ? (
        <ScrollView scrollY enhanced bounces={false} scrollAnchoring={false} className="companionHistory">
          <View className="companionMemoryIntro">{COMPANION_NAME}会逐步记下你明确说过的兴趣、偏好和近期计划。记错了可以改；过期的信息会淡出。</View>
          {state?.initiativeVersion ? (
            <View className="companionProactivity">
              <Text>小禾主动找你聊天</Text>
              <Text className="companionEvidence">闲聊只在打开时出现；“我先开口”会暂停主动闲聊和自动跟进。你明确设置的定时提醒仍会生效，微信通知需要另行授权。</Text>
              <View className="companionProactivityOptions">
                {([{ mode: "normal", label: "自然一点" }, { mode: "less", label: "少主动一点" }, { mode: "off", label: "我先开口" }] as const).map(({ mode, label }) => (
                  <Button key={mode} className={(state.quiet ? "off" : state.proactivity) === mode ? "companionProactivitySelected" : ""}
                    disabled={memoryBusy} onClick={() => void setProactivity(mode)}>{label}</Button>
                ))}
              </View>
            </View>
          ) : <View className="companionSetting"><Text>打开时主动打个招呼</Text><Switch checked={!state?.quiet} disabled={memoryBusy} color="#548b75" onChange={(event) => void toggleQuiet(!event.detail.value)} /></View>}
          <AssistantSettings client={session.companion} refreshKey={state} quiet={Boolean(state?.quiet)} onChanged={() => void session.companion.state().then(updateState).catch((cause) => setError(errorMessage(cause)))} />
          <Text className="reminderManageLink" onClick={() => setReminderOpen(true)}>我的提醒 · 查看倒计时与管理 ›</Text>
          {editor && <View className="companionMemory">
            <Text className="companionMemoryValue">纠正这条记忆</Text>
            <Textarea className="companionMemoryEditor" value={editor.value} maxlength={120} onInput={(event) => setEditor({ ...editor, value: event.detail.value })} />
            <Text className="companionEvidence">修改后会重新整理续聊摘要，旧聊天不再用于回忆。</Text>
            <View className="companionMemoryActions"><Button disabled={memoryBusy} onClick={() => setEditor(null)}>取消</Button><Button disabled={memoryBusy || !editor.value.trim()} onClick={() => void saveMemory()}>保存</Button></View>
          </View>}
          {state?.memories.map((memory) => <View className="companionMemory" key={memory.id}>
            <Text className="companionMemoryValue">{memory.value}</Text>
            <Text className="companionEvidence">依据：{memory.evidence}</Text>
            <View className="companionMemoryActions"><Button disabled={memoryBusy || busy} onClick={() => setEditor({ memory, value: memory.value })}>纠正</Button><Button disabled={memoryBusy || busy} onClick={() => void forget(memory)}>忘记</Button></View>
          </View>)}
          {!state?.memories.length && <View className="companionEmpty">{memoryBusy ? "正在整理记忆…" : "还没有长期记忆。先自然聊几轮，不用特意介绍自己。"}</View>}
          <Button className="companionForgetAll" disabled={memoryBusy || busy} onClick={() => void forget()}>全部忘记，重新认识</Button>
        </ScrollView>
      ) : <>
        <Text className="reminderManageLink" onClick={() => setReminderOpen(true)}>我的提醒{reminders ? ` · ${reminders.plans.filter((p) => p.status === "active").length} 个进行中` : ""} ›</Text>
        <View className="companionHistoryShell">
          <ScrollView id="companionMessages" scrollY enhanced bounces={false} scrollAnchoring={false} className="companionHistory companionChatHistory" scrollTop={scrollTop} scrollWithAnimation={false}
            lowerThreshold={COMPANION_BOTTOM_THRESHOLD_PX}
            onTouchStart={() => { touching.current = true; measureHistory(); }}
            onTouchEnd={() => { touching.current = false; }}
            onTouchCancel={() => { touching.current = false; }}
            onTouchMove={() => { scrollingAt.current = Date.now(); }}
            onScroll={(event) => {
              recordCompanionInteraction();
              const next = event.detail.scrollTop;
              const nearBottom = historyHeight.current > 0 &&
                event.detail.scrollHeight - next - historyHeight.current <= COMPANION_BOTTOM_THRESHOLD_PX;
              if (nearBottom) { autoFollow.current = true; setFollowing(true); }
              else if (shouldReleaseChatAutoFollow(previousScrollTop.current, next, touching.current)) { autoFollow.current = false; setFollowing(false); }
              previousScrollTop.current = next;
            }}
            onScrollToLower={() => { autoFollow.current = true; setFollowing(true); }}>
            {loading && <View className="companionEmpty">{COMPANION_NAME}正在过来…</View>}
            {!timeline.length && !loading && <View className="companionRow">
              <CompanionAvatar size="message" />
              <View className="companionBubble companionAssistant"><Text>嗨，我是{COMPANION_NAME}，一个 AI 聊天伙伴。今天有什么想聊的吗？</Text></View>
            </View>}
            {timeline.map((message) => <View className={`companionRow ${message.role === "user" ? "companionUserRow" : ""}`} key={message.id}>
              {message.role !== "user" && <CompanionAvatar size="message" />}
              <View className={`companionBubble ${message.role === "user" ? "companionUser" : "companionAssistant"}`}>
                <MessageTime createdAt={message.createdAt} />
                {message.text && <Markdown>{message.text}</Markdown>}
                {reminders && Boolean(message.reminderIDs?.length) && <ReminderCards state={reminders} ids={message.reminderIDs} occurrenceID={message.occurrenceID} now={reminderNow} busy={busy} onAction={(action) => void reminderAction(action)} onEdit={editReminder} />}
                {message.topic && (
                  <View className="companionTopicSource">
                    <Text className="companionSourceLink" onClick={() => void Taro.setClipboardData({ data: message.topic!.url })}>
                      来源：{message.topic.title}（点此复制链接）
                    </Text>
                    <Text>报道日期：{companionBeijingDate(message.topic.publishedAt)} · 北京时间</Text>
                    <View className="companionMemoryActions">
                      <Button disabled={memoryBusy || busy} onClick={() => void topicFeedback(message.topic!, "like")}>喜欢这类</Button>
                      <Button disabled={memoryBusy || busy} onClick={() => void topicFeedback(message.topic!, "avoid")}>少聊这类</Button>
                    </View>
                  </View>
                )}
                {message.images?.map((source, index) => <Image key={`${message.id}-${index}`} className="companionImage" src={source} mode="widthFix" onLoad={() => { if (autoFollow.current) setScrollTop(nextChatBottomScrollTop); }} onClick={() => void Taro.previewImage({ current: source, urls: message.images! })} />)}
                {!message.images?.length && Boolean(message.fileIDs?.length) && <Text className="companionImageRetry" onClick={() => void loadImages([message])}>{message.imageStatus || "正在加载图片…"}</Text>}
                {message.pending && <Text className="companionActivity">{message.activityStatus || `${COMPANION_NAME}正在回复…`}</Text>}
              </View>
            </View>)}
            <View className="companionBottomSpace" />
          </ScrollView>
          <Text className="companionFollow" style={{ display: following ? "none" : "block" }} onClick={() => { autoFollow.current = true; setFollowing(true); setScrollTop(nextChatBottomScrollTop); }}>回到最新消息 ↓</Text>
        </View>
        <View className="companionComposer" style={composerKeyboardStyle(keyboard)}>
          {attachment && <View className="companionAttachment"><Image src={attachment.path} mode="aspectFill" /><Text onClick={() => setAttachment(null)}>移除图片</Text></View>}
          <View className="companionInputRow">
            <Button className="companionPhoto" disabled={busy || uploading || loading || speechActive} onClick={() => void chooseImage()}>{uploading ? "…" : "图片"}</Button>
            <SpeechComposer client={session.speech} draft={draft} disabled={busy || uploading || loading || memoryOpen}
              onSend={(text) => send(text)}
              onActive={(active) => { recordCompanionInteraction(); setSpeechActive(active); }} onError={setError}>
              <Textarea className="companionInput"
                {...composerKeyboardHandlers((height) => { recordCompanionInteraction(); setKeyboard(height); })}
                confirmType="send" confirmHold
                onKeyboardCompositionStart={() => { composerComposingRef.current = true; }}
                onKeyboardCompositionEnd={() => { composerComposingRef.current = false; }}
                onBlur={() => { composerComposingRef.current = false; recordCompanionInteraction(); setKeyboard(0); }}
                onConfirm={(event) => {
                  if (composerComposingRef.current || speechActive || loading || memoryOpen) return;
                  return send(event.detail.value);
                }}
                value={draft} placeholder="随口说点什么…" maxlength={8000} autoHeight adjustPosition={false} showConfirmBar={false} disabled={busy || loading || speechActive} onLineChange={measureHistory} onInput={(event) => { recordCompanionInteraction(); setDraft(event.detail.value); }} />
            </SpeechComposer>
            <Button className="companionSend" disabled={loading || uploading || speechActive || (!busy && !draft.trim() && !attachment)} onClick={() => busy ? void session.cancelActiveGeneration().catch((cause) => setError(errorMessage(cause))) : void send()}>{busy ? "停止" : "发送"}</Button>
          </View>
          {!keyboard && <Text className="companionFootnote">聊天及联网按平台用量计费 · 主动招呼不扣余额</Text>}
        </View>
      </>}
    </View>
  );
}
