import { useEffect, useMemo, useRef, useState } from "react";
import Taro from "@tarojs/taro";
import { Button, Input, Picker, ScrollView, Text, Textarea, View } from "@tarojs/components";
import { TaskEditor } from "../../components/todo/task-editor";
import { aiPage, messageOf, useTodo } from "../../product/todo/use-todo";
import { newDraft, newID, taskDraft, type TaskDraft, type TodoList, type TodoTask } from "../../product/todo/types";
import { beijingDate, isOverdue } from "../../product/todo/dates";
import "./index.scss";

type Tab = "tasks" | "lists" | "mine";
type Period = "today" | "week" | "all";
const emptyHint = {
  today: "今天的空间，留给重要的事",
  week: "提前记下，接下来更从容",
  all: "把脑海里的事，轻轻放在这里",
};
function dateLabel(task: TodoTask, now: number): string {
  if (!task.dueDate) return "";
  let label = task.dueDate === beijingDate(0, now) ? "今天" : task.dueDate.slice(5);
  if (isOverdue(task, now)) label = `已逾期 · ${label}`;
  if (task.dueTime) label += ` ${task.dueTime}`;
  if (task.repeatKind !== "none") label += " · 重复";
  return label;
}

function pageTitle(tab: Tab, history: boolean, listID: string, lists: TodoList[]): string {
  if (tab === "lists") return "每件事，各有归处";
  if (tab === "mine") return "我的小工具";
  if (history) return "已完成记录";
  if (listID === "inbox") return "收件箱";
  if (listID) return lists.find((list) => list.id === listID)?.name || "待办";
  return "给今天减点负担";
}

export default function TodoEntry() {
  const todo = useTodo();
  if (todo.loading)
    return (
      <View className="todo-loading">
        <View className="todo-mark">✓</View>
        <Text>正在准备你的待办…</Text>
      </View>
    );
  if (!todo.store.current)
    return (
      <View className="todo-loading">
        <Text className="todo-heading">暂时无法打开</Text>
        <Text className="todo-muted">{todo.error}</Text>
        <Button className="todo-primary" onClick={() => void todo.connect()}>
          重新连接
        </Button>
      </View>
    );
  // Switching a verified owner also discards transient search/editor/feedback
  // state, so a draft or late query from the previous account cannot leak.
  return <TodoWorkspace key={todo.store.current.ownerKey} todo={todo} />;
}

function TodoWorkspace({ todo }: { todo: ReturnType<typeof useTodo> }) {
  const activeWorkspace = useRef(true);
  useEffect(() => {
    activeWorkspace.current = true;
    return () => {
      activeWorkspace.current = false;
    };
  }, []);
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60000);
    return () => clearInterval(timer);
  }, []);
  const [tab, setTab] = useState<Tab>("tasks");
  const [period, setPeriod] = useState<Period>("today");
  const [listID, setListID] = useState("");
  const [important, setImportant] = useState(false);
  const [showDone, setShowDone] = useState(false);
  const [history, setHistory] = useState(false);
  const [query, setQuery] = useState("");
  const [remote, setRemote] = useState<TodoTask[]>();
  const [remoteTotal, setRemoteTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [limit, setLimit] = useState(60);
  const [editor, setEditor] = useState<{ draft: TaskDraft; version: number }>();
  const [listEditor, setListEditor] = useState<TodoList>();
  const [listError, setListError] = useState("");
  const [feedback, setFeedback] = useState(false);
  const [body, setBody] = useState("");
  const [feedbackError, setFeedbackError] = useState("");
  const [sending, setSending] = useState(false);
  const [undo, setUndo] = useState<{ task: TodoTask; kind: "task.restore" | "task.complete" }>();
  const [exportList, setExportList] = useState("");
  const [exportRange, setExportRange] = useState(0);
  const [exporting, setExporting] = useState(false);
  const pending = todo.store.current?.pending ?? [];
  const problem = pending.find((item) => item.problem);
  const lists = useMemo(
    () =>
      todo.snapshot.lists
        .filter((list) => list.id !== "inbox" && !list.deletedAt)
        .sort((a, b) => a.position - b.position),
    [todo.snapshot.lists],
  );

  useEffect(() => {
    setPage(1);
    setLimit(60);
    setRemote(undefined);
  }, [query, history, listID, period, important]);
  useEffect(() => {
    if ((!query.trim() && !history) || !todo.online || pending.length || !todo.client.current) {
      setRemote(undefined);
      return;
    }
    let current = true;
    setSearching(true);
    const timer = setTimeout(() => {
      void todo.client
        .current!.query(query.trim(), history ? "true" : "", page, listID)
        .then((result) => {
          if (!current) return;
          setRemote((previous) => (page === 1 ? result.results : [...(previous ?? []), ...result.results]));
          setRemoteTotal(result.total);
          setSearchError("");
        })
        .catch((failure: unknown) => {
          if (current) {
            setRemote(undefined);
            setSearchError(messageOf(failure));
          }
        })
        .finally(() => {
          if (current) setSearching(false);
        });
    }, 250);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [query, history, listID, page, todo.online, pending.length, todo.client]);

  const allTasks = todo.snapshot.tasks.filter((task) => !task.deletedAt);
  const source = remote ?? allTasks;
  const today = beijingDate(0, now);
  const weekEnd = new Date(`${today}T00:00:00Z`);
  weekEnd.setUTCDate(weekEnd.getUTCDate() + 6);
  const visible = source
    .filter((task) => {
      if (task.parentID || task.deletedAt || (listID && task.listID !== listID) || (important && !task.important))
        return false;
      if (
        query.trim() &&
        !`${task.title}\n${task.notes}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase())
      )
        return false;
      if (history) return !!task.completedAt;
      if (query.trim()) return true;
      if (period === "today") return !!task.dueDate && task.dueDate <= today;
      if (period === "week") return !!task.dueDate && task.dueDate <= weekEnd.toISOString().slice(0, 10);
      return true;
    })
    .sort(
      (a, b) =>
        Number(b.important) - Number(a.important) ||
        (a.dueDate || "9999").localeCompare(b.dueDate || "9999") ||
        a.position - b.position,
    );
  const active = visible.filter((task) => !task.completedAt);
  const completed = visible.filter((task) => task.completedAt);
  const overdue = active.filter((task) => isOverdue(task, now));
  const rest = active.filter((task) => !isOverdue(task, now));
  let syncMessage = `${pending.length} 项修改待同步`;
  if (todo.syncing) syncMessage = "正在同步…";
  else if (!todo.online) syncMessage = "暂时离线，修改保存在本机";

  let syncStatus = "暂时离线";
  if (todo.syncing) syncStatus = "同步中";
  else if (pending.length) syncStatus = `${pending.length} 项待同步`;
  else if (todo.online) syncStatus = "已同步";

  let searchStatus = "正在显示本机缓存";
  if (searching) searchStatus = "正在查找…";
  else if (remote) searchStatus = `找到 ${remoteTotal} 项`;

  let activeHeading = "待办事项";
  if (query) activeHeading = "待完成";
  else if (period === "today") activeHeading = "今天要做";

  let emptyTitle = "清清爽爽";
  if (query) emptyTitle = "没有找到这件事";
  else if (history) emptyTitle = "完成的事会留在这里";

  let exportListName: string | undefined = "全部清单";
  if (exportList === "inbox") exportListName = "收件箱";
  else if (exportList) exportListName = lists.find((list) => list.id === exportList)?.name;

  const run = async (action: () => void | Promise<void>) => {
    try {
      await action();
    } catch (failure) {
      void Taro.showToast({ title: messageOf(failure), icon: "none", duration: 3000 });
    }
  };
  const findTask = (task: TodoTask): TodoTask => todo.snapshot.tasks.find((item) => item.id === task.id) ?? task;
  const toggle = async (original: TodoTask) => {
    const task = findTask(original);
    if (!task.completedAt && allTasks.some((item) => item.parentID === task.id && !item.completedAt)) {
      const answer = await Taro.showModal({
        title: "一起完成小步骤？",
        content: "这项任务还有未完成的步骤，完成任务会一并勾选。",
        confirmText: "一起完成",
      });
      if (!answer.confirm) return;
    }
    todo.enqueue({ kind: "task.complete", entityID: task.id, baseVersion: task.version, completed: !task.completedAt });
    if (!task.completedAt) setUndo({ task: { ...task, version: task.version + 1 }, kind: "task.complete" });
    if (history || query) setRemote(undefined);
  };
  const remove = (task: TodoTask) => {
    const latest = findTask(task);
    todo.enqueue({ kind: "task.delete", entityID: latest.id, baseVersion: latest.version });
    setUndo({ task: { ...latest, version: latest.version + 1 }, kind: "task.restore" });
    setEditor(undefined);
  };
  const edit = (task: TodoTask) => setEditor({ draft: taskDraft(task), version: task.version });
  const save = (draft: TaskDraft) => {
    const version = editor?.version ?? 0;
    todo.enqueue({ kind: "task.save", entityID: draft.id, baseVersion: version, task: draft });
    setEditor(undefined);
  };
  const bumpEditorParent = (parentID: string) =>
    setEditor((current) => (current?.draft.id === parentID ? { ...current, version: current.version + 1 } : current));
  const addTask = () => {
    const draft = newDraft(listID || "inbox");
    if (period === "today") draft.dueDate = today;
    setEditor({ draft, version: 0 });
  };
  const manageList = (list?: TodoList) => {
    setListEditor(list ?? { id: newID(), name: "", position: Date.now(), version: 0 });
    setListError("");
  };
  const saveList = () => {
    if (!listEditor) return;
    const name = listEditor.name.trim();
    if (!name) {
      setListError("给清单起个名字");
      return;
    }
    try {
      todo.enqueue({
        kind: "list.save",
        entityID: listEditor.id,
        baseVersion: listEditor.version,
        list: { id: listEditor.id, name, position: listEditor.position },
      });
      setListEditor(undefined);
    } catch (failure) {
      setListError(messageOf(failure));
    }
  };
  const addChild = (title: string) => {
    const parent = todo.snapshot.tasks.find((task) => task.id === editor?.draft.id);
    if (!parent) throw new Error("请先保存任务");
    const draft = { ...newDraft(parent.listID, parent.id), title };
    todo.enqueue({ kind: "task.save", entityID: draft.id, baseVersion: 0, task: draft });
    bumpEditorParent(parent.id);
  };
  const deleteChild = (task: TodoTask) => {
    todo.enqueue({ kind: "task.delete", entityID: task.id, baseVersion: task.version });
    bumpEditorParent(task.parentID);
    setUndo({ task: { ...task, version: task.version + 1 }, kind: "task.restore" });
  };
  const toggleImportant = (task: TodoTask) => {
    const latest = findTask(task);
    todo.enqueue({
      kind: "task.save",
      entityID: task.id,
      baseVersion: latest.version,
      task: { ...taskDraft(latest), important: !task.important },
    });
  };
  const submitFeedback = () => {
    if (sending || !body.trim()) return;
    setSending(true);
    setFeedbackError("");
    void todo.client
      .current!.feedback(body.trim())
      .then(async (status) => {
        if (!activeWorkspace.current) return;
        setBody("");
        if (status.unlocked) {
          await Taro.reLaunch({ url: aiPage });
          return;
        }
        void Taro.showToast({ title: "反馈成功", icon: "success" });
      })
      .catch((failure: unknown) => {
        if (activeWorkspace.current) setFeedbackError(messageOf(failure));
      })
      .finally(() => {
        if (activeWorkspace.current) setSending(false);
      });
  };
  const deleteList = async (list: TodoList) => {
    const answer = await Taro.showModal({
      title: `删除「${list.name}」？`,
      content: "清单中的任务会移回收件箱。",
      confirmText: "删除清单",
    });
    if (answer.confirm) {
      todo.enqueue({ kind: "list.delete", entityID: list.id, baseVersion: list.version });
      if (listID === list.id) setListID("");
    }
  };
  const enterList = (id: string) => {
    setListID(id);
    setPeriod("all");
    setHistory(false);
    setTab("tasks");
    setQuery("");
  };
  const exportData = async (format: "text" | "csv") => {
    if (pending.length) throw new Error("请先同步待办并处理冲突，再导出完整数据");
    setExporting(true);
    try {
      const data = await todo.client.current!.export(format, exportList, ["", "false", "true"][exportRange]);
      if (!activeWorkspace.current) return;
      if (format === "text") {
        await Taro.setClipboardData({ data: data.content });
        return;
      }
      const path = `${Taro.env.USER_DATA_PATH}/${data.filename.replace(/[^a-zA-Z0-9._-]/g, "_")}`;
      await new Promise<void>((resolve, reject) =>
        Taro.getFileSystemManager().writeFile({
          filePath: path,
          data: data.content,
          encoding: "utf8",
          success: () => resolve(),
          fail: reject,
        }),
      );
      if (!activeWorkspace.current) return;
      await Taro.shareFileMessage({ filePath: path, fileName: data.filename });
    } finally {
      setExporting(false);
    }
  };
  const renderTask = (task: TodoTask) => {
    const label = dateLabel(task, now);
    const children = allTasks.filter((item) => item.parentID === task.id);
    const completedChildren = children.filter((item) => item.completedAt).length;
    return (
      <View className={`todo-task ${task.completedAt ? "is-completed" : ""}`} key={task.id}>
        <Button
          className={`todo-check ${task.completedAt ? "is-done" : ""}`}
          ariaLabel={task.completedAt ? "取消完成" : "完成任务"}
          onClick={() => void run(() => toggle(task))}
        >
          {task.completedAt ? "✓" : ""}
        </Button>
        <View className="todo-task-content" onClick={() => edit(task)}>
          <Text className="todo-task-title">{task.title}</Text>
          <View className="todo-task-meta">
            {label && <Text className={isOverdue(task, now) ? "todo-overdue" : ""}>{label}</Text>}
            {children.length > 0 && (
              <Text>
                {" "}
                · {completedChildren}/{children.length} 步骤
              </Text>
            )}
          </View>
        </View>
        <Button
          className={`todo-star ${task.important ? "is-important" : ""}`}
          ariaLabel={task.important ? "取消重要标记" : "标为重要"}
          onClick={() => void run(() => toggleImportant(task))}
        >
          {task.important ? "★" : "☆"}
        </Button>
      </View>
    );
  };

  if (listEditor)
    return (
      <View className="todo-page">
        <View className="todo-toolbar">
          <Button className="todo-link" onClick={() => setListEditor(undefined)}>
            取消
          </Button>
          <Text className="todo-heading">{listEditor.version ? "清单名称" : "新建清单"}</Text>
          <View className="todo-toolbar-end" />
        </View>
        <View className="todo-card">
          <Text className="todo-label">清单名称</Text>
          <Input
            className="todo-input"
            value={listEditor.name}
            maxlength={100}
            placeholder="例如：生活、工作、想做的事"
            onInput={(event) => setListEditor({ ...listEditor, name: event.detail.value })}
          />
          {listError && <Text className="todo-error">{listError}</Text>}
          <Button className="todo-primary" onClick={saveList}>
            保存清单
          </Button>
        </View>
      </View>
    );
  if (editor)
    return (
      <TaskEditor
        key={editor.draft.id}
        initial={editor.draft}
        lists={lists}
        childrenTasks={allTasks.filter((task) => task.parentID === editor.draft.id)}
        onClose={() => setEditor(undefined)}
        onSave={save}
        onDelete={
          editor.version
            ? () => void run(() => remove(findTask({ ...editor.draft, version: editor.version } as TodoTask)))
            : undefined
        }
        onChild={addChild}
        onToggleChild={(task) =>
          void run(async () => {
            await toggle(task);
            bumpEditorParent(task.parentID);
          })
        }
        onDeleteChild={(task) => void run(() => deleteChild(task))}
      />
    );

  if (feedback)
    return (
      <View className="todo-page">
        <View className="todo-toolbar">
          <Button className="todo-link" onClick={() => setFeedback(false)}>
            ‹ 返回
          </Button>
          <Text className="todo-heading">反馈与建议</Text>
          <View className="todo-toolbar-end" />
        </View>
        <ScrollView scrollY className="todo-main">
          <View className="todo-card">
            <Text className="todo-heading">让它更顺手一点</Text>
            <Text className="todo-muted">遇到的问题，或你希望增加的小功能。</Text>
            <Textarea
              className="todo-notes"
              placeholder="写下你的建议"
              value={body}
              maxlength={2000}
              disabled={sending}
              onInput={(event) => {
                setBody(event.detail.value);
                setFeedbackError("");
              }}
            />
            {feedbackError && <Text className="todo-error">{feedbackError}</Text>}
            <Button
              className="todo-primary"
              loading={sending}
              disabled={sending || !body.trim()}
              onClick={submitFeedback}
            >
              提交反馈
            </Button>
          </View>
          <View className="todo-spacer" />
        </ScrollView>
      </View>
    );

  return (
    <View className="todo-page">
      <View className="todo-header">
        <View>
          <Text className="todo-eyebrow">AI省着用 · 日常清单</Text>
          <Text className="todo-page-title">{pageTitle(tab, history, listID, lists)}</Text>
        </View>
        <Text className="todo-date">{today.slice(5).replace("-", "/")}</Text>
      </View>
      {(!todo.online || todo.error || pending.length > 0) && (
        <View className="todo-sync">
          <Text>{syncMessage}</Text>
          <Button className="todo-link" onClick={() => void (todo.online ? todo.sync() : todo.connect())}>
            {todo.syncing ? "同步中" : "重试"}
          </Button>
        </View>
      )}
      {problem && (
        <View className="todo-conflict">
          <Text className="todo-heading">有一项修改需要你决定</Text>
          <Text>{problem.operation.task?.title || problem.operation.list?.name || "任务状态"}</Text>
          <Text className="todo-muted">{problem.problem?.message || "其他设备已修改这项内容，本机修改仍然保留。"}</Text>
          <View className="todo-inline">
            <Button
              className="todo-link"
              onClick={() =>
                void run(async () => {
                  const answer = await Taro.showModal({
                    title: "使用云端内容？",
                    content: "这项内容在本机的待同步修改将被移除。",
                    confirmText: "使用云端",
                  });
                  if (answer.confirm) {
                    todo.store.current!.discardEntity(problem.operation.entityID);
                    todo.redraw();
                    void todo.sync();
                  }
                })
              }
            >
              使用云端
            </Button>
            {problem.operation.kind.startsWith("task.") && (
              <Button
                className="todo-link"
                onClick={() =>
                  void run(() => {
                    const task =
                      todo.snapshot.tasks.find((item) => item.id === problem.operation.entityID) ??
                      todo.store.current!.serverSnapshot.tasks.find((item) => item.id === problem.operation.entityID);
                    const original = task ? taskDraft(task) : problem.operation.task;
                    if (!original) throw new Error("无法恢复内容，请先保留云端版本");
                    const copy = { ...original, id: newID(), parentID: "", repeatKind: "none" as const, repeatDay: 0 };
                    // Save the copy first; a storage error cannot discard the user's branch.
                    todo.enqueue({ kind: "task.save", entityID: copy.id, baseVersion: 0, task: copy });
                    todo.store.current!.discardEntity(problem.operation.entityID);
                    todo.redraw();
                    void todo.sync();
                  })
                }
              >
                保留为新待办
              </Button>
            )}
          </View>
        </View>
      )}
      <ScrollView scrollY className="todo-main">
        {tab === "tasks" && (
          <>
            <View className="todo-search">
              <Text>⌕</Text>
              <Input
                className="todo-search-input"
                placeholder="搜索待办和备注"
                value={query}
                onInput={(event) => setQuery(event.detail.value)}
              />
              {query && (
                <Button className="todo-link" onClick={() => setQuery("")}>
                  清除
                </Button>
              )}
            </View>
            {(history || listID) && (
              <Button
                className="todo-link todo-back"
                onClick={() => {
                  setHistory(false);
                  setListID("");
                  setPeriod("all");
                }}
              >
                ‹ 全部待办
              </Button>
            )}
            {!history && !listID && (
              <View className="todo-filters">
                {(
                  [
                    ["today", "今天"],
                    ["week", "近七天"],
                    ["all", "全部"],
                  ] as const
                ).map(([value, label]) => (
                  <Button
                    key={value}
                    className={`todo-chip ${period === value ? "is-selected" : ""}`}
                    onClick={() => setPeriod(value)}
                  >
                    {label}
                  </Button>
                ))}
                <Button
                  className={`todo-chip ${important ? "is-selected" : ""}`}
                  onClick={() => setImportant(!important)}
                >
                  重要
                </Button>
              </View>
            )}
            {(query || history) && (
              <Text className="todo-muted todo-search-state">
                {searchStatus}
                {searchError ? ` · ${searchError}` : ""}
              </Text>
            )}
            {!history && overdue.length > 0 && (
              <>
                <View className="todo-section">
                  <Text className="todo-overdue">已逾期</Text>
                  <Text>{overdue.length}</Text>
                </View>
                <View className="todo-task-list">{overdue.slice(0, limit).map(renderTask)}</View>
              </>
            )}
            {!history && rest.length > 0 && (
              <>
                <View className="todo-section">
                  <Text>{activeHeading}</Text>
                  <Text>{rest.length}</Text>
                </View>
                <View className="todo-task-list">
                  {rest.slice(0, Math.max(0, limit - overdue.length)).map(renderTask)}
                </View>
              </>
            )}
            {(history || completed.length > 0) && (
              <>
                <Button className="todo-done-toggle" onClick={() => setShowDone(!showDone)}>
                  {history || showDone ? "⌄" : "›"} 已完成 · {completed.length}
                </Button>
                {(history || showDone) && (
                  <View className="todo-task-list">{completed.slice(0, limit).map(renderTask)}</View>
                )}
              </>
            )}
            {visible.length === 0 && (
              <View className="todo-empty">
                <View className="todo-mark">✓</View>
                <Text className="todo-heading">{emptyTitle}</Text>
                <Text className="todo-muted">{query ? "换个关键词试试" : emptyHint[period]}</Text>
                {!query && !history && (
                  <Button className="todo-link" onClick={addTask}>
                    ＋ 记下一件事
                  </Button>
                )}
              </View>
            )}
            {visible.length > limit && (
              <Button className="todo-link" onClick={() => setLimit(limit + 60)}>
                再显示 60 项
              </Button>
            )}
            {remote && remote.length < remoteTotal && (
              <Button
                className="todo-link"
                loading={searching}
                disabled={searching}
                onClick={() => {
                  setLimit(limit + 50);
                  setPage(page + 1);
                }}
              >
                加载更多记录
              </Button>
            )}
          </>
        )}
        {tab === "lists" && (
          <>
            <View className="todo-card todo-list-card" onClick={() => enterList("inbox")}>
              <Text className="todo-heading">收件箱</Text>
              <Text className="todo-muted">
                {allTasks.filter((task) => task.listID === "inbox" && !task.parentID && !task.completedAt).length}{" "}
                件待办 ›
              </Text>
            </View>
            {lists.map((list, index) => (
              <View className="todo-card" key={list.id}>
                <View className="todo-list-card" onClick={() => enterList(list.id)}>
                  <Text className="todo-heading">{list.name}</Text>
                  <Text className="todo-muted">
                    {allTasks.filter((task) => task.listID === list.id && !task.parentID && !task.completedAt).length}{" "}
                    件待办 ›
                  </Text>
                </View>
                <View className="todo-inline">
                  <Button className="todo-link" onClick={() => void run(() => manageList(list))}>
                    改名
                  </Button>
                  <Button
                    className="todo-link"
                    disabled={index === 0}
                    onClick={() =>
                      void run(() =>
                        todo.enqueue({
                          kind: "list.save",
                          entityID: list.id,
                          baseVersion: list.version,
                          list: { id: list.id, name: list.name, position: lists[index - 1].position - 1 },
                        }),
                      )
                    }
                  >
                    上移
                  </Button>
                  <Button className="todo-link todo-danger-text" onClick={() => void run(() => deleteList(list))}>
                    删除
                  </Button>
                </View>
              </View>
            ))}
            <Button className="todo-primary" onClick={() => void run(() => manageList())}>
              ＋ 新建清单
            </Button>
          </>
        )}
        {tab === "mine" && (
          <>
            <View className="todo-card">
              <Text className="todo-heading">省下心力，专注当下</Text>
              <Text className="todo-muted">用一张清单，安放日常的小事。</Text>
              <View
                className="todo-field"
                onClick={() => {
                  setHistory(true);
                  setQuery("");
                  setListID("");
                  setTab("tasks");
                }}
              >
                <Text>已完成记录</Text>
                <Text>›</Text>
              </View>
              <View className="todo-field" onClick={() => setFeedback(true)}>
                <Text>反馈与建议</Text>
                <Text>›</Text>
              </View>
            </View>
            <View className="todo-card">
              <Text className="todo-heading">导出我的清单</Text>
              <Picker
                mode="selector"
                range={["全部清单", "收件箱", ...lists.map((list) => list.name)]}
                onChange={(event) =>
                  setExportList(["", "inbox", ...lists.map((list) => list.id)][Number(event.detail.value)])
                }
              >
                <View className="todo-field">
                  <Text>清单</Text>
                  <Text>{exportListName} ›</Text>
                </View>
              </Picker>
              <Picker
                mode="selector"
                range={["全部任务", "未完成", "已完成"]}
                value={exportRange}
                onChange={(event) => setExportRange(Number(event.detail.value))}
              >
                <View className="todo-field">
                  <Text>范围</Text>
                  <Text>{["全部任务", "未完成", "已完成"][exportRange]} ›</Text>
                </View>
              </Picker>
              <View className="todo-inline">
                <Button
                  className="todo-link"
                  disabled={exporting || !todo.online}
                  onClick={() => void run(() => exportData("text"))}
                >
                  复制为文本
                </Button>
                <Button
                  className="todo-link"
                  disabled={exporting || !todo.online}
                  onClick={() => void run(() => exportData("csv"))}
                >
                  导出 CSV
                </Button>
              </View>
            </View>
            <View className="todo-card">
              <View className="todo-field">
                <Text>同步状态</Text>
                <Text>{syncStatus}</Text>
              </View>
              {todo.error && <Text className="todo-error">{todo.error}</Text>}
              <Button className="todo-link" onClick={() => void todo.connect()}>
                重新连接
              </Button>
              <Text className="todo-muted">修改先保存在本机，联网后自动同步。</Text>
            </View>
          </>
        )}
        <View className="todo-spacer" />
      </ScrollView>
      {undo && (
        <View className="todo-undo">
          <Text>{undo.kind === "task.restore" ? "任务已删除" : "已完成一件事"}</Text>
          <Button
            className="todo-link"
            onClick={() =>
              void run(() => {
                const latest = findTask(undo.task);
                todo.enqueue({ kind: undo.kind, entityID: latest.id, baseVersion: latest.version, completed: false });
                setUndo(undefined);
              })
            }
          >
            撤销
          </Button>
          <Button className="todo-link" ariaLabel="关闭撤销提示" onClick={() => setUndo(undefined)}>
            ×
          </Button>
        </View>
      )}
      {tab === "tasks" && !history && (
        <Button className="todo-add" ariaLabel="新增待办" onClick={addTask}>
          ＋
        </Button>
      )}
      <View className="todo-nav">
        {(
          [
            ["tasks", "✓", "待办"],
            ["lists", "☰", "清单"],
            ["mine", "○", "我的"],
          ] as const
        ).map(([value, icon, label]) => (
          <Button
            key={value}
            className={`todo-nav-item ${tab === value ? "is-active" : ""}`}
            onClick={() => {
              setTab(value);
              if (value === "tasks") setHistory(false);
            }}
          >
            <Text className="todo-nav-icon">{icon}</Text>
            <Text>{label}</Text>
          </Button>
        ))}
      </View>
    </View>
  );
}
