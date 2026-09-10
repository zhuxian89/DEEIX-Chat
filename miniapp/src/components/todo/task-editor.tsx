import { useState } from "react";
import { Button, Input, Picker, ScrollView, Switch, Text, Textarea, View } from "@tarojs/components";
import type { TaskDraft, TodoList, TodoTask } from "../../product/todo/types";
import { beijingDate } from "../../product/todo/dates";

const repeats = ["不重复", "每天", "周一至周五", "每周", "每月"];
const repeatKinds: TaskDraft["repeatKind"][] = ["none", "daily", "weekdays", "weekly", "monthly"];
const weekdays = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"];
type Props = {
  initial: TaskDraft;
  lists: TodoList[];
  childrenTasks: TodoTask[];
  onSave: (draft: TaskDraft) => void;
  onClose: () => void;
  onDelete?: () => void;
  onChild: (title: string) => void;
  onToggleChild: (task: TodoTask) => void;
  onDeleteChild: (task: TodoTask) => void;
};

export function TaskEditor(props: Props) {
  const [draft, setDraft] = useState(props.initial);
  const [error, setError] = useState("");
  const [childTitle, setChildTitle] = useState("");
  const change = <K extends keyof TaskDraft>(key: K, value: TaskDraft[K]) =>
    setDraft((current) => ({ ...current, [key]: value }));
  const lists = [
    { id: "inbox", name: "收件箱" },
    ...props.lists.filter((list) => list.id !== "inbox" && !list.deletedAt),
  ];
  const listIndex = Math.max(
    0,
    lists.findIndex((list) => list.id === draft.listID),
  );
  const save = () => {
    const title = draft.title.trim();
    if (!title) {
      setError("给这件事起个名字");
      return;
    }
    if (draft.repeatKind !== "none" && !draft.dueDate) {
      setError("重复任务需要先选择日期");
      return;
    }
    try {
      props.onSave({ ...draft, title });
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "保存失败");
    }
  };
  const dateShortcut = (offset: number) => {
    change("dueDate", beijingDate(offset));
  };
  const changeRepeat = (kind: TaskDraft["repeatKind"]) => {
    setDraft((current) => {
      let repeatDay = 0;
      if (kind === "monthly") repeatDay = Number(current.dueDate.slice(8)) || 1;
      else if (kind === "weekly") repeatDay = new Date(`${current.dueDate || "2026-01-05"}T12:00:00`).getDay();
      return { ...current, repeatKind: kind, repeatDay };
    });
  };
  const addChild = () => {
    const title = childTitle.trim();
    if (!title) return;
    try {
      props.onChild(title);
      setChildTitle("");
    } catch {
      setError("请先保存任务，再添加步骤");
    }
  };
  return (
    <View className="todo-editor">
      <View className="todo-toolbar">
        <Button className="todo-link" onClick={props.onClose}>
          取消
        </Button>
        <Text className="todo-heading">任务详情</Text>
        <Button className="todo-link" onClick={save}>
          保存
        </Button>
      </View>
      <ScrollView scrollY className="todo-editor-scroll">
        <View className="todo-card">
          <Text className="todo-label">要做什么</Text>
          <Input
            className="todo-input todo-title-input"
            value={draft.title}
            maxlength={200}
            placeholder="记下一件事"
            onInput={(event) => change("title", event.detail.value)}
          />
          <Text className="todo-label">备注</Text>
          <Textarea
            className="todo-notes"
            value={draft.notes}
            maxlength={10000}
            placeholder="补充细节，让开始更轻松"
            onInput={(event) => change("notes", event.detail.value)}
          />
          <View className="todo-field">
            <Text>重要任务</Text>
            <Switch
              color="#23765c"
              checked={draft.important}
              onChange={(event) => change("important", event.detail.value)}
            />
          </View>
          {!draft.parentID && (
            <Picker
              mode="selector"
              range={lists.map((list) => list.name)}
              value={listIndex}
              onChange={(event) => change("listID", lists[Number(event.detail.value)].id)}
            >
              <View className="todo-field">
                <Text>所属清单</Text>
                <Text>{lists[listIndex].name} ›</Text>
              </View>
            </Picker>
          )}
          <Picker mode="date" value={draft.dueDate} onChange={(event) => change("dueDate", event.detail.value)}>
            <View className="todo-field">
              <Text>日期</Text>
              <Text>{draft.dueDate || "不设日期"} ›</Text>
            </View>
          </Picker>
          <View className="todo-inline">
            <Button className="todo-chip" onClick={() => dateShortcut(0)}>
              今天
            </Button>
            <Button className="todo-chip" onClick={() => dateShortcut(1)}>
              明天
            </Button>
            <Button
              className="todo-chip"
              onClick={() =>
                setDraft((current) => ({ ...current, dueDate: "", dueTime: "", repeatKind: "none", repeatDay: 0 }))
              }
            >
              清除日期
            </Button>
          </View>
          {draft.dueDate && (
            <>
              <Picker
                mode="time"
                value={draft.dueTime || "09:00"}
                onChange={(event) => change("dueTime", event.detail.value)}
              >
                <View className="todo-field">
                  <Text>时间</Text>
                  <Text>{draft.dueTime || "全天"} ›</Text>
                </View>
              </Picker>
              {draft.dueTime && (
                <Button className="todo-link" onClick={() => change("dueTime", "")}>
                  改为全天任务
                </Button>
              )}
              <Text className="todo-muted">日期和时间按北京时间保存</Text>
            </>
          )}
          {!draft.parentID && (
            <Picker
              mode="selector"
              range={repeats}
              value={repeatKinds.indexOf(draft.repeatKind)}
              onChange={(event) => changeRepeat(repeatKinds[Number(event.detail.value)])}
            >
              <View className="todo-field">
                <Text>重复</Text>
                <Text>{repeats[repeatKinds.indexOf(draft.repeatKind)]} ›</Text>
              </View>
            </Picker>
          )}
          {draft.repeatKind === "weekly" && (
            <Picker
              mode="selector"
              range={weekdays}
              value={draft.repeatDay}
              onChange={(event) => change("repeatDay", Number(event.detail.value))}
            >
              <View className="todo-field">
                <Text>每周几</Text>
                <Text>{weekdays[draft.repeatDay]} ›</Text>
              </View>
            </Picker>
          )}
          {draft.repeatKind === "monthly" && (
            <Picker
              mode="selector"
              range={Array.from({ length: 31 }, (_, index) => `${index + 1} 日`)}
              value={draft.repeatDay - 1}
              onChange={(event) => change("repeatDay", Number(event.detail.value) + 1)}
            >
              <View className="todo-field">
                <Text>每月几号</Text>
                <Text>{draft.repeatDay} 日 ›</Text>
              </View>
            </Picker>
          )}
        </View>
        {!draft.parentID && (
          <View className="todo-card">
            <Text className="todo-heading">拆成小步骤</Text>
            {props.childrenTasks.map((task) => (
              <View className="todo-field" key={task.id}>
                <Button
                  className={`todo-check ${task.completedAt ? "is-done" : ""}`}
                  ariaLabel={task.completedAt ? "取消完成步骤" : "完成步骤"}
                  onClick={() => props.onToggleChild(task)}
                >
                  {task.completedAt ? "✓" : ""}
                </Button>
                <Text className={task.completedAt ? "todo-strike" : ""}>{task.title}</Text>
                <Button className="todo-link" ariaLabel="删除步骤" onClick={() => props.onDeleteChild(task)}>
                  ×
                </Button>
              </View>
            ))}
            <View className="todo-inline">
              <Input
                className="todo-input"
                value={childTitle}
                maxlength={200}
                placeholder="添加一个步骤"
                onInput={(event) => setChildTitle(event.detail.value)}
              />
              <Button className="todo-link" onClick={addChild}>
                添加
              </Button>
            </View>
          </View>
        )}
        {error && <Text className="todo-error">{error}</Text>}
        {props.onDelete && (
          <Button className="todo-danger" onClick={props.onDelete}>
            删除任务
          </Button>
        )}
        <View className="todo-spacer" />
      </ScrollView>
    </View>
  );
}
