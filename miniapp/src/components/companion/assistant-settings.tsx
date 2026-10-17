import type { CompanionAssistantState, CompanionContextConversation, CompanionFollowup } from "@deeix/api-contract";
import { Button, Input, Picker, Switch, Text, View } from "@tarojs/components";
import { useEffect, useRef, useState } from "react";
import type { CompanionClient } from "@/product/companion-client";
import { companionDecisionText, followupDateParts, followupDueAt, followupTime } from "@/product/companion-assistant";
import { useCompanionSubscriptions } from "./use-subscriptions";
import "./assistant-settings.scss";

type Editor = { item?: CompanionFollowup; title: string; date: string; time: string };

export function AssistantSettings({ client, refreshKey, quiet, onChanged }: {
  client: CompanionClient; refreshKey: unknown; quiet: boolean; onChanged(): void;
}) {
  const subscription = useCompanionSubscriptions(client);
  const [state, setState] = useState<CompanionAssistantState | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Editor | null>(null);
  const [showSources, setShowSources] = useState(false);
  const [showConversations, setShowConversations] = useState(false);
  const [conversations, setConversations] = useState<CompanionContextConversation[]>([]);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showDecisions, setShowDecisions] = useState(false);
  const mounted = useRef(true);
  const working = useRef(false);
  const loadVersion = useRef(0);

  const reload = async () => {
    const version = ++loadVersion.current;
    const next = await client.assistant();
    if (mounted.current && version === loadVersion.current) setState(next);
  };
  useEffect(() => {
    mounted.current = true;
    void reload().catch(() => { if (mounted.current) setError("跟进信息暂时没有加载成功，请重试。"); });
    return () => { mounted.current = false; loadVersion.current += 1; };
  }, [client, refreshKey]);

  const run = async (action: () => Promise<void>) => {
    if (working.current) return;
    working.current = true; setBusy(true); setError("");
    try { await action(); }
    catch (cause) {
      const message = cause instanceof Error ? cause.message : "操作未完成，请重试。";
      const readable = ({ "resource conflict": "事项可能已更新或正在处理，请刷新后重试。", "resource not found": "事项或来源已不存在，请刷新数据。", "invalid request": "内容或时间不符合要求，请检查后重试。" } as Record<string, string>)[message] ?? message;
      if (mounted.current) setError(readable);
    }
    finally { working.current = false; if (mounted.current) setBusy(false); }
  };
  const loadConversations = async (nextPage: number) => {
    const items = await client.contextConversations(query.trim(), nextPage);
    if (!mounted.current) return;
    setConversations((current) => nextPage === 1 ? items : [...current, ...items.filter((item) => !current.some((old) => old.id === item.id))]);
    setPage(nextPage); setHasMore(items.length === 50); setShowConversations(true);
  };
  const setContext = async (enabled: boolean, excluded: string[]) => {
    await client.setContextPreferences({ enabled, excluded });
    await reload(); onChanged();
  };
  const startEditor = (item?: CompanionFollowup) => {
    const parts = followupDateParts(item?.dueAt ?? new Date(Date.now() + 3600_000).toISOString());
    setEditor({ item, title: item?.title ?? "", ...parts });
  };
  const save = async () => {
    if (!editor) return;
    const dueAt = followupDueAt(editor.date, editor.time);
    if (!dueAt) throw new Error("请选择有效的日期和时间。");
    void subscription.controller.request();
    await client.saveFollowup({ id: editor.item?.id ?? "", revision: editor.item?.revision ?? 0, title: editor.title.trim(), dueAt, status: "pending" });
    if (mounted.current) setEditor(null);
    await reload();
  };
  const changeStatus = async (item: CompanionFollowup, status: "completed" | "canceled") => {
    await client.saveFollowup({ id: item.id, revision: item.revision, title: item.title, dueAt: item.dueAt, status });
    await reload();
  };
  if (!state) return <View className="assistantSettings"><Text>{error || "正在加载跟进信息…"}</Text><Button disabled={busy} onClick={() => void run(reload)}>重新加载</Button></View>;
  const pending = state.followups.filter((item) => item.status === "pending");
  const notifications = subscription.settings ?? state.notifications;
  const visible = showHistory ? state.followups : pending;
  return <View className="assistantSettings">
    {error && <View className="assistantError"><Text>{error}</Text><Button disabled={busy} onClick={() => void run(reload)}>刷新数据</Button></View>}
    <View className="assistantSection">
      <Text className="assistantTitle">跟进事项</Text>
      <Text className="assistantHint">从你明确说过的计划和有日期的待办中整理，也可以手动添加。改期、完成和取消都在这里。</Text>
      {quiet && <Text className="assistantHint">当前为“我先开口”，自动跟进已暂停。你明确设置的定时提醒继续生效。</Text>}
      <Button disabled={busy} onClick={() => startEditor()}>添加跟进</Button>
      {editor && <View className="assistantEditor">
        <Input value={editor.title} maxlength={120} placeholder="需要跟进什么？" onInput={(e) => setEditor({ ...editor, title: e.detail.value })} />
        <View className="assistantActions">
          <Picker mode="date" value={editor.date} onChange={(e) => setEditor({ ...editor, date: e.detail.value })}><Text>{editor.date} ▾</Text></Picker>
          <Picker mode="time" value={editor.time} onChange={(e) => setEditor({ ...editor, time: e.detail.value })}><Text>{editor.time} ▾</Text></Picker>
        </View>
        <View className="assistantActions"><Button disabled={busy} onClick={() => setEditor(null)}>取消编辑</Button><Button disabled={busy || !editor.title.trim()} onClick={() => void run(save)}>保存</Button></View>
      </View>}
      {!visible.length && <Text className="assistantHint">还没有跟进事项。说出具体计划和时间后，小禾会定期整理；急用时可以直接添加。</Text>}
      {visible.map((item) => <View className="assistantItem" key={item.id}>
        <Text className="assistantItemTitle">{item.title}</Text>
        <Text>{followupTime(item.dueAt)} · {item.status === "completed" ? "已完成" : item.status === "canceled" ? "已取消" : Date.parse(item.deliveredAt) > 0 ? "已提醒，待跟进" : "待提醒"}</Text>
        <Text className="assistantHint">{item.sourceKind === "todo" ? "来自待办" : item.sourceKind === "conversation" ? "来自会话" : "手动添加"}{item.evidence ? `：${item.evidence}` : ""}{item.userManaged ? " · 由你管理" : ""}</Text>
        {item.notificationStatus && <Text className="assistantHint">{companionDecisionText(item.notificationStatus)}</Text>}
        {item.status === "pending" && <View className="assistantActions">
          <Button disabled={busy} onClick={() => startEditor(item)}>修改</Button>
          <Button disabled={busy} onClick={() => void run(() => changeStatus(item, "completed"))}>已完成</Button>
          <Button disabled={busy} onClick={() => void run(() => changeStatus(item, "canceled"))}>不再跟进</Button>
        </View>}
      </View>)}
      <Text className="assistantLink" onClick={() => setShowHistory(!showHistory)}>{showHistory ? "只看待跟进" : "查看已完成和已取消"}</Text>
    </View>
    <View className="assistantSection">
      <Text className="assistantTitle">微信通知</Text>
      <Text className="assistantHint">{subscription.error || (!notifications.ready ? "通知暂未配置。到期消息会保留在小禾会话里。" : notifications.enabled ? `可用微信提醒：${notifications.credits} 次。` : "允许后，可在提醒到期时收到微信通知。")}</Text>
      <Text className="assistantHint">允许时勾选“总是保持以上选择”，后续使用小禾会免弹窗补充通知次数。</Text>
      <Button disabled={busy || subscription.busy} onClick={() => {
        if (subscription.settings?.ready) void subscription.controller.request(true);
        else void subscription.controller.refresh();
      }}>{!subscription.settings?.ready ? "重新加载通知状态" : notifications.enabled ? "补充通知次数" : "允许微信提醒"}</Button>
      {notifications.enabled && <Button disabled={busy} onClick={() => void run(async () => { await subscription.controller.disable(); await reload(); })}>关闭微信通知</Button>}
    </View>
    <View className="assistantSection">
      <View className="assistantSetting"><Text className="assistantTitle">全局理解</Text><Switch checked={state.preferences.enabled} disabled={busy} color="#548b75" onChange={(e) => void run(() => setContext(e.detail.value, state.preferences.excluded))} /></View>
      <Text className="assistantHint">允许小禾参考同账号其他会话和待办。仅检索相关片段，不代表每次读完全部历史；排除会话后，相关自动跟进会取消。</Text>
      <Text className="assistantLink" onClick={() => void run(() => loadConversations(1))}>管理会话范围（已排除 {state.preferences.excluded.length} 个）</Text>
      {showConversations && <View>
        <Input value={query} maxlength={100} placeholder="搜索会话标题" onInput={(e) => setQuery(e.detail.value)} />
        <Button disabled={busy} onClick={() => void run(() => loadConversations(1))}>搜索</Button>
        {conversations.map((item) => <View className="assistantSetting" key={item.id}>
          <Text>{item.title || "未命名会话"}</Text><Switch checked={!state.preferences.excluded.includes(item.id)} disabled={busy} color="#548b75" onChange={(e) => void run(() => setContext(state.preferences.enabled, e.detail.value ? state.preferences.excluded.filter((id) => id !== item.id) : [...new Set([...state.preferences.excluded, item.id])]))} />
        </View>)}
        {hasMore && <Button disabled={busy} onClick={() => void run(() => loadConversations(page + 1))}>加载更多会话</Button>}
        <Text className="assistantLink" onClick={() => setShowConversations(false)}>收起会话范围</Text>
      </View>}
      <Text className="assistantLink" onClick={() => setShowSources(!showSources)}>{showSources ? "收起参考资料" : "查看最近可参考的资料"}</Text>
      {showSources && <View>{state.sources.map((source) => <View className="assistantItem" key={source.id}>
        <Text className="assistantItemTitle">{source.kind === "todo" ? "待办" : "会话"}：{source.title}</Text>
        <Text className="assistantHint">{followupTime(source.at)}</Text><Text>{source.text}</Text>
      </View>)}{!state.sources.length && <Text className="assistantHint">当前没有可用资料。</Text>}</View>}
    </View>
    <View className="assistantSection">
      <Text className="assistantLink" onClick={() => setShowDecisions(!showDecisions)}>{showDecisions ? "收起主动联系状态" : "查看主动联系状态"}</Text>
      {showDecisions && <View>{state.decisions.map((decision) => <View className="assistantItem" key={decision.kind}>
        <Text>{decision.kind === "home" ? "首页招呼" : decision.kind === "idle" ? "聊天接话" : decision.kind === "notification" ? "微信通知" : "事项跟进"}：{companionDecisionText(decision.reason)}</Text><Text className="assistantHint">{followupTime(decision.at)}</Text>
      </View>)}{!state.decisions.length && <Text className="assistantHint">等待下一次检查。</Text>}<Button disabled={busy} onClick={() => void run(reload)}>刷新状态</Button></View>}
    </View>
  </View>;
}
