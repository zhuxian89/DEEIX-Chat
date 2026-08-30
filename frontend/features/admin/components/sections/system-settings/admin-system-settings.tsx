"use client";

import {
  ChevronLeft,
  ChevronRight,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Trash2,
} from "lucide-react";
import * as React from "react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import {
  deleteAdminSetting,
  listAdminAvailableSettings,
  listAdminSettings,
  patchAdminSettings,
} from "@/features/admin/api";
import type { SettingItem, SettingsGrouped } from "@/shared/api/settings.types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const PAGE_SIZE = 20;

type SettingRow = SettingItem & { namespace: string };

type SettingDraft = {
  mode: "restore" | "edit";
  namespace: string;
  key: string;
  value: string;
  valueType: SettingItem["valueType"];
  sensitive: boolean;
  configured: boolean;
};

function flattenSettings(groups: SettingsGrouped): SettingRow[] {
  return Object.entries(groups)
    .flatMap(([namespace, items]) => items.map((item) => ({ ...item, namespace })))
    .sort((left, right) =>
      `${left.namespace}:${left.key}`.localeCompare(`${right.namespace}:${right.key}`),
    );
}

export function AdminSystemSettingsPage() {
  const [rows, setRows] = React.useState<SettingRow[]>([]);
  const [availableRows, setAvailableRows] = React.useState<SettingRow[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [query, setQuery] = React.useState("");
  const [namespace, setNamespace] = React.useState("all");
  const [page, setPage] = React.useState(1);
  const [draft, setDraft] = React.useState<SettingDraft | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<SettingRow | null>(null);

  const load = React.useCallback(async () => {
    const accessToken = await resolveAccessToken();
    if (!accessToken) return;
    setLoading(true);
    try {
      const [settings, available] = await Promise.all([
        listAdminSettings(accessToken),
        listAdminAvailableSettings(accessToken),
      ]);
      setRows(flattenSettings(settings));
      setAvailableRows(flattenSettings(available));
    } catch {
      toast.error("参数加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  const namespaces = React.useMemo(
    () => Array.from(new Set(rows.map((item) => item.namespace))).sort(),
    [rows],
  );
  const filteredRows = React.useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return rows.filter((item) => {
      if (namespace !== "all" && item.namespace !== namespace) return false;
      if (!normalizedQuery) return true;
      return [item.namespace, item.key, item.description, item.sensitive ? "sensitive" : ""]
        .join(" ")
        .toLowerCase()
        .includes(normalizedQuery);
    });
  }, [namespace, query, rows]);
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / PAGE_SIZE));
  const pageRows = filteredRows.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);

  React.useEffect(() => {
    setPage(1);
  }, [namespace, query]);

  React.useEffect(() => {
    if (page > pageCount) setPage(pageCount);
  }, [page, pageCount]);

  const openEdit = (item: SettingRow) => {
    setDraft({
      mode: "edit",
      namespace: item.namespace,
      key: item.key,
      value: item.sensitive ? "" : item.value,
      valueType: item.valueType || "string",
      sensitive: item.sensitive,
      configured: item.configured,
    });
  };

  const openRestore = () => {
    const first = availableRows[0];
    if (!first) return;
    setDraft({
      mode: "restore",
      namespace: first.namespace,
      key: first.key,
      value: first.value,
      valueType: first.valueType || "string",
      sensitive: first.sensitive,
      configured: false,
    });
  };

  const selectRestoreDefinition = (value: string) => {
    const selected = availableRows.find((item) => `${item.namespace}:${item.key}` === value);
    if (!selected) return;
    setDraft((current) =>
      current?.mode === "restore"
        ? {
            ...current,
            namespace: selected.namespace,
            key: selected.key,
            value: selected.value,
            valueType: selected.valueType || "string",
            sensitive: selected.sensitive,
            configured: false,
          }
        : current,
    );
  };

  const save = async () => {
    if (!draft) return;
    const nextNamespace = draft.namespace.trim();
    const nextKey = draft.key.trim();
    if (!nextNamespace || !nextKey) {
      toast.error("请填写命名空间和配置键");
      return;
    }
    if (draft.sensitive && draft.mode === "edit" && !draft.value.trim()) {
      toast.error("请输入新的敏感值");
      return;
    }

    const accessToken = await resolveAccessToken();
    if (!accessToken) return;
    setSaving(true);
    try {
      await patchAdminSettings(accessToken, {
        items: [{
          namespace: nextNamespace,
          key: nextKey,
          value: draft.value,
          clear: draft.mode === "restore" && draft.sensitive && !draft.value.trim(),
        }],
      });
      setDraft(null);
      await load();
      toast.success(draft.mode === "restore" ? "参数已恢复" : "参数已更新");
    } catch {
      toast.error(draft.mode === "restore" ? "参数恢复失败" : "参数更新失败");
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!deleteTarget) return;
    const accessToken = await resolveAccessToken();
    if (!accessToken) return;
    setSaving(true);
    try {
      await deleteAdminSetting(accessToken, deleteTarget.namespace, deleteTarget.key);
      setDeleteTarget(null);
      await load();
      toast.success("参数已删除");
    } catch {
      toast.error("参数删除失败");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold tracking-normal">参数配置</h2>
          <p className="mt-1 text-sm text-muted-foreground">系统公共参数</p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            title="刷新"
            onClick={() => void load()}
            disabled={loading}
          >
            <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
          </Button>
          <Button onClick={openRestore} disabled={availableRows.length === 0}>
            <Plus className="mr-2 size-4" />
            恢复参数
          </Button>
        </div>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="relative min-w-0 flex-1 sm:max-w-md">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="pl-9"
            placeholder="搜索命名空间、配置键或说明"
          />
        </div>
        <Select value={namespace} onValueChange={setNamespace}>
          <SelectTrigger className="w-full sm:w-48">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部命名空间</SelectItem>
            {namespaces.map((item) => (
              <SelectItem key={item} value={item}>
                {item}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="overflow-x-auto rounded-md border">
        <Table className="min-w-[860px] table-fixed">
          <TableHeader>
            <TableRow>
              <TableHead className="w-32">命名空间</TableHead>
              <TableHead className="w-56">配置键</TableHead>
              <TableHead className="w-52">当前值</TableHead>
              <TableHead>说明</TableHead>
              <TableHead className="w-24">类型</TableHead>
              <TableHead className="w-24 text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  正在加载
                </TableCell>
              </TableRow>
            ) : pageRows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                  暂无匹配参数
                </TableCell>
              </TableRow>
            ) : (
              pageRows.map((item) => (
                <TableRow key={`${item.namespace}:${item.key}`}>
                  <TableCell>
                    <Badge variant="outline">{item.namespace}</Badge>
                  </TableCell>
                  <TableCell className="truncate font-mono text-xs" title={item.key}>
                    {item.key}
                  </TableCell>
                  <TableCell className="truncate font-mono text-xs" title={item.sensitive ? undefined : item.value}>
                    {item.sensitive ? (
                      <span className="flex items-center gap-2">
                        <span>{item.configured ? "••••••••" : "-"}</span>
                        <Badge variant={item.configured ? "secondary" : "outline"}>
                          {item.configured ? "已配置" : "未配置"}
                        </Badge>
                      </span>
                    ) : (
                      item.value || "-"
                    )}
                  </TableCell>
                  <TableCell className="truncate text-sm text-muted-foreground" title={item.description}>
                    {item.description || "-"}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{item.valueType || "string"}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button size="icon" variant="ghost" title="编辑" onClick={() => openEdit(item)}>
                        <Pencil className="size-4" />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        title="删除"
                        className="text-destructive hover:text-destructive"
                        onClick={() => setDeleteTarget(item)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex min-h-9 items-center justify-between gap-3 text-sm text-muted-foreground">
        <span>共 {filteredRows.length} 条</span>
        <div className="flex items-center gap-2">
          <Button
            size="icon"
            variant="outline"
            title="上一页"
            disabled={page <= 1}
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <span className="min-w-20 text-center tabular-nums">
            {page} / {pageCount}
          </span>
          <Button
            size="icon"
            variant="outline"
            title="下一页"
            disabled={page >= pageCount}
            onClick={() => setPage((current) => Math.min(pageCount, current + 1))}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>

      <SettingDialog
        draft={draft}
        availableRows={availableRows}
        saving={saving}
        onChange={setDraft}
        onSelectRestoreDefinition={selectRestoreDefinition}
        onSave={() => void save()}
      />

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !saving && !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除参数</AlertDialogTitle>
            <AlertDialogDescription>
              确认删除 {deleteTarget ? `${deleteTarget.namespace}:${deleteTarget.key}` : "该参数"}？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>取消</AlertDialogCancel>
            <AlertDialogAction disabled={saving} onClick={(event) => { event.preventDefault(); void remove(); }}>
              {saving ? "删除中" : "删除"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function SettingDialog({
  draft,
  availableRows,
  saving,
  onChange,
  onSelectRestoreDefinition,
  onSave,
}: {
  draft: SettingDraft | null;
  availableRows: SettingRow[];
  saving: boolean;
  onChange: (draft: SettingDraft | null) => void;
  onSelectRestoreDefinition: (value: string) => void;
  onSave: () => void;
}) {
  const update = (patch: Partial<SettingDraft>) => {
    if (draft) onChange({ ...draft, ...patch });
  };

  return (
    <Dialog open={draft !== null} onOpenChange={(open) => !saving && !open && onChange(null)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{draft?.mode === "restore" ? "恢复参数" : "编辑参数"}</DialogTitle>
          <DialogDescription>
            {draft?.mode === "restore" ? "恢复已删除的内置参数" : `${draft?.namespace}:${draft?.key}`}
          </DialogDescription>
        </DialogHeader>
        {draft ? (
          <div className="space-y-4">
            {draft.mode === "restore" ? (
              <label className="grid gap-1.5 text-sm">
                <span>内置参数</span>
                <Select
                  value={`${draft.namespace}:${draft.key}`}
                  onValueChange={onSelectRestoreDefinition}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {availableRows.map((item) => (
                      <SelectItem key={`${item.namespace}:${item.key}`} value={`${item.namespace}:${item.key}`}>
                        {item.namespace}:{item.key}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </label>
            ) : null}
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="grid gap-1.5 text-sm">
                <span>命名空间</span>
                <Input
                  value={draft.namespace}
                  disabled
                  onChange={(event) => update({ namespace: event.target.value })}
                />
              </label>
              <label className="grid gap-1.5 text-sm">
                <span>配置键</span>
                <Input
                  value={draft.key}
                  disabled
                  onChange={(event) => update({ key: event.target.value })}
                />
              </label>
            </div>
            <SettingValueField draft={draft} onChange={(value) => update({ value })} />
          </div>
        ) : null}
        <DialogFooter>
          <Button variant="outline" disabled={saving} onClick={() => onChange(null)}>
            取消
          </Button>
          <Button disabled={saving} onClick={onSave}>
            {saving ? "保存中" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SettingValueField({ draft, onChange }: { draft: SettingDraft; onChange: (value: string) => void }) {
  if (draft.valueType === "bool") {
    return (
      <label className="flex min-h-10 items-center justify-between gap-4 text-sm">
        <span>参数值</span>
        <Switch checked={draft.value === "true"} onCheckedChange={(checked) => onChange(String(checked))} />
      </label>
    );
  }

  if (draft.valueType === "json") {
    return (
      <label className="grid gap-1.5 text-sm">
        <span>参数值</span>
        <Textarea rows={8} className="font-mono text-xs" value={draft.value} onChange={(event) => onChange(event.target.value)} />
      </label>
    );
  }

  return (
    <label className="grid gap-1.5 text-sm">
      <span>{draft.sensitive && draft.configured ? "新参数值" : "参数值"}</span>
      <Input
        type={draft.sensitive ? "password" : draft.valueType === "int" ? "number" : "text"}
        value={draft.value}
        autoComplete="off"
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}
