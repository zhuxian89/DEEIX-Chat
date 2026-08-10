"use client";

import { Check, Pencil, Plus, Power, RefreshCw } from "lucide-react";
import * as React from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  createAdminWeChatRule,
  createAdminWeChatTemplate,
  getAdminWeChatSummary,
  getPublicBuildInfo,
  listAdminWeChatActions,
  listAdminWeChatIssuances,
  listAdminWeChatLogs,
  listAdminWeChatRules,
  listAdminWeChatTemplates,
  setAdminWeChatRuleEnabled,
  setAdminWeChatTemplateEnabled,
  updateAdminWeChatRule,
  updateAdminWeChatTemplate,
  type WeChatActionOption,
  type WeChatBuildInfo,
  type WeChatInvocationLog,
  type WeChatIssuanceRecord,
  type WeChatKeywordRule,
  type WeChatReplyTemplate,
  type WeChatSummary,
} from "@/features/admin/api";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

type RuleDraft = { id?: number; keyword: string; action: string; templateId: string; enabled: boolean };
type TemplateDraft = { id?: number; name: string; content: string; enabled: boolean };

const emptyRule: RuleDraft = { keyword: "", action: "issue_registration_code", templateId: "", enabled: true };
const emptyTemplate: TemplateDraft = { name: "", content: "", enabled: true };

function formatDate(value: string | null | undefined) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function actionLabel(actions: WeChatActionOption[], key: string) {
  return actions.find((item) => item.key === key)?.label ?? key;
}

function statusBadge(enabled: boolean) {
  return <Badge variant={enabled ? "default" : "outline"}>{enabled ? "启用" : "停用"}</Badge>;
}

export function AdminWeChatPage() {
  const [token, setToken] = React.useState("");
  const [summary, setSummary] = React.useState<WeChatSummary | null>(null);
  const [buildInfo, setBuildInfo] = React.useState<WeChatBuildInfo | null>(null);
  const [actions, setActions] = React.useState<WeChatActionOption[]>([]);
  const [rules, setRules] = React.useState<WeChatKeywordRule[]>([]);
  const [templates, setTemplates] = React.useState<WeChatReplyTemplate[]>([]);
  const [issuances, setIssuances] = React.useState<WeChatIssuanceRecord[]>([]);
  const [issuanceTotal, setIssuanceTotal] = React.useState(0);
  const [issuancePage, setIssuancePage] = React.useState(1);
  const [logs, setLogs] = React.useState<WeChatInvocationLog[]>([]);
  const [logTotal, setLogTotal] = React.useState(0);
  const [logPage, setLogPage] = React.useState(1);
  const [query, setQuery] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const [refreshing, setRefreshing] = React.useState(false);
  const [ruleDraft, setRuleDraft] = React.useState<RuleDraft | null>(null);
  const [templateDraft, setTemplateDraft] = React.useState<TemplateDraft | null>(null);

  const loadBase = React.useCallback(async (accessToken: string) => {
    const [summaryResult, actionResult, ruleResult, templateResult, buildResult] = await Promise.all([
      getAdminWeChatSummary(accessToken),
      listAdminWeChatActions(accessToken),
      listAdminWeChatRules(accessToken),
      listAdminWeChatTemplates(accessToken),
      getPublicBuildInfo().catch(() => null),
    ]);
    setSummary(summaryResult);
    setActions(actionResult);
    setRules(ruleResult.results ?? []);
    setTemplates(templateResult.results ?? []);
    setBuildInfo(buildResult);
  }, []);

  const loadIssuances = React.useCallback(async (accessToken: string, page: number, q: string) => {
    const result = await listAdminWeChatIssuances(accessToken, page, 25, q);
    setIssuances(result.results ?? []);
    setIssuanceTotal(result.total ?? 0);
  }, []);

  const loadLogs = React.useCallback(async (accessToken: string, page: number, q: string) => {
    const result = await listAdminWeChatLogs(accessToken, page, 25, { q });
    setLogs(result.results ?? []);
    setLogTotal(result.total ?? 0);
  }, []);

  const refresh = React.useCallback(async (accessToken: string) => {
    setRefreshing(true);
    try {
      await Promise.all([loadBase(accessToken), loadIssuances(accessToken, issuancePage, query), loadLogs(accessToken, logPage, query)]);
    } finally {
      setRefreshing(false);
    }
  }, [issuancePage, logPage, loadBase, loadIssuances, loadLogs, query]);

  React.useEffect(() => {
    void resolveAccessToken().then(async (accessToken) => {
      if (!accessToken) return;
      setToken(accessToken);
      setLoading(true);
      try {
        await refresh(accessToken);
      } catch {
        toast.error("微信公众号管理数据加载失败");
      } finally {
        setLoading(false);
      }
    });
  }, [refresh]);

  const reload = async () => {
    if (!token) return;
    try {
      await refresh(token);
      toast.success("已刷新");
    } catch {
      toast.error("刷新失败");
    }
  };

  const saveRule = async () => {
    if (!token || !ruleDraft) return;
    const templateId = Number(ruleDraft.templateId);
    if (!ruleDraft.keyword.trim() || !ruleDraft.action || !templateId) {
      toast.error("请填写关键词、功能和回复模板");
      return;
    }
    try {
      const payload = { keyword: ruleDraft.keyword.trim(), action: ruleDraft.action, templateId, enabled: ruleDraft.enabled };
      if (ruleDraft.id) await updateAdminWeChatRule(token, ruleDraft.id, payload);
      else await createAdminWeChatRule(token, payload);
      setRuleDraft(null);
      await loadBase(token);
      toast.success("关键词规则已保存");
    } catch {
      toast.error("关键词规则保存失败");
    }
  };

  const toggleRule = async (item: WeChatKeywordRule) => {
    if (!token) return;
    try {
      await setAdminWeChatRuleEnabled(token, item.id, !item.enabled);
      await loadBase(token);
    } catch {
      toast.error("关键词状态更新失败");
    }
  };

  const saveTemplate = async () => {
    if (!token || !templateDraft) return;
    if (!templateDraft.name.trim() || !templateDraft.content.trim()) {
      toast.error("请填写模板名称和内容");
      return;
    }
    try {
      const payload = { name: templateDraft.name.trim(), responseType: "text", content: templateDraft.content, enabled: templateDraft.enabled };
      if (templateDraft.id) await updateAdminWeChatTemplate(token, templateDraft.id, payload);
      else await createAdminWeChatTemplate(token, payload);
      setTemplateDraft(null);
      await loadBase(token);
      toast.success("回复模板已保存");
    } catch {
      toast.error("回复模板保存失败");
    }
  };

  const toggleTemplate = async (item: WeChatReplyTemplate) => {
    if (!token) return;
    try {
      await setAdminWeChatTemplateEnabled(token, item.id, !item.enabled);
      await loadBase(token);
    } catch {
      toast.error("模板状态更新失败");
    }
  };

  const search = async () => {
    if (!token) return;
    setIssuancePage(1);
    setLogPage(1);
    try {
      await Promise.all([loadIssuances(token, 1, query), loadLogs(token, 1, query)]);
    } catch {
      toast.error("查询失败");
    }
  };

  const changeIssuancePage = async (page: number) => {
    if (!token || page < 1 || page > Math.max(1, Math.ceil(issuanceTotal / 25))) return;
    setIssuancePage(page);
    await loadIssuances(token, page, query);
  };

  const changeLogPage = async (page: number) => {
    if (!token || page < 1 || page > Math.max(1, Math.ceil(logTotal / 25))) return;
    setLogPage(page);
    await loadLogs(token, page, query);
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-2xl font-semibold">微信公众号管理</h2>
          <p className="mt-1 text-sm text-muted-foreground">管理关键词、功能、回复模板和注册码发放情况。</p>
        </div>
        <Button variant="outline" size="icon" title="刷新" onClick={() => void reload()} disabled={refreshing || loading}>
          <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
        </Button>
      </div>

      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList className="w-full justify-start overflow-x-auto rounded-md">
          <TabsTrigger value="overview">概览</TabsTrigger>
          <TabsTrigger value="rules">关键词</TabsTrigger>
          <TabsTrigger value="templates">模板</TabsTrigger>
          <TabsTrigger value="bindings">绑定关系</TabsTrigger>
          <TabsTrigger value="issuances">注册码发放记录</TabsTrigger>
          <TabsTrigger value="logs">通用处理日志</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-3">
            <Stat label="已发放注册码" value={summary?.issuanceCount ?? 0} />
            <Stat label="成功处理" value={summary?.successCount ?? 0} />
            <Stat label="失败处理" value={summary?.failureCount ?? 0} />
          </div>
          <div className="rounded-md border p-4 text-sm">
            <div className="font-medium">当前部署版本</div>
            {buildInfo ? (
              <dl className="mt-3 grid gap-2 sm:grid-cols-[110px_1fr]">
                <dt className="text-muted-foreground">版本</dt><dd>{buildInfo.version}</dd>
                <dt className="text-muted-foreground">Commit</dt><dd className="break-all font-mono text-xs">{buildInfo.commit}</dd>
                <dt className="text-muted-foreground">构建时间</dt><dd>{buildInfo.buildTime}</dd>
              </dl>
            ) : <p className="mt-2 text-muted-foreground">版本信息暂时不可用。</p>}
          </div>
        </TabsContent>

        <TabsContent value="rules">
          <SectionHeader title="关键词规则" action={<Button onClick={() => setRuleDraft(emptyRule)}><Plus className="mr-2 size-4" />新增关键词</Button>} />
          <RuleTable items={rules} actions={actions} onEdit={setRuleDraft} onToggle={(item) => void toggleRule(item)} />
        </TabsContent>

        <TabsContent value="templates">
          <SectionHeader title="回复模板" action={<Button onClick={() => setTemplateDraft(emptyTemplate)}><Plus className="mr-2 size-4" />新增模板</Button>} />
          <TemplateTable items={templates} rules={rules} onEdit={setTemplateDraft} onToggle={(item) => void toggleTemplate(item)} />
        </TabsContent>

        <TabsContent value="bindings">
          <SectionHeader title="绑定关系" description="关键词规则是唯一关系来源；这里用于总览和跳转编辑。" />
          <BindingTable items={rules} actions={actions} onEditRule={setRuleDraft} onEditTemplate={(id) => {
            const item = templates.find((template) => template.id === id);
            if (item) setTemplateDraft({ id: item.id, name: item.name, content: item.content, enabled: item.enabled });
          }} />
        </TabsContent>

        <TabsContent value="issuances" className="space-y-3">
          <SearchBar value={query} onChange={setQuery} onSearch={() => void search()} />
          <IssuanceTable items={issuances} />
          <Pager page={issuancePage} total={issuanceTotal} onChange={(page) => void changeIssuancePage(page)} />
        </TabsContent>

        <TabsContent value="logs" className="space-y-3">
          <SearchBar value={query} onChange={setQuery} onSearch={() => void search()} />
          <LogTable items={logs} actions={actions} />
          <Pager page={logPage} total={logTotal} onChange={(page) => void changeLogPage(page)} />
        </TabsContent>
      </Tabs>

      <Dialog open={ruleDraft !== null} onOpenChange={(open) => { if (!open) setRuleDraft(null); }}>
        <DialogContent>
          <DialogHeader><DialogTitle>{ruleDraft?.id ? "编辑关键词规则" : "新增关键词规则"}</DialogTitle><DialogDescription>一次完成关键词、功能和回复模板绑定。</DialogDescription></DialogHeader>
          {ruleDraft ? <div className="space-y-4">
            <label className="grid gap-1 text-sm"><span>关键词</span><Input value={ruleDraft.keyword} onChange={(event) => setRuleDraft({ ...ruleDraft, keyword: event.target.value })} placeholder="例如 13003" /></label>
            <label className="grid gap-1 text-sm"><span>功能</span><select className="h-8 rounded-md border border-input/40 bg-transparent px-3 text-xs" value={ruleDraft.action} onChange={(event) => setRuleDraft({ ...ruleDraft, action: event.target.value })}>{actions.map((item) => <option key={item.key} value={item.key}>{item.label}</option>)}</select></label>
            <label className="grid gap-1 text-sm"><span>回复模板</span><select className="h-8 rounded-md border border-input/40 bg-transparent px-3 text-xs" value={ruleDraft.templateId} onChange={(event) => setRuleDraft({ ...ruleDraft, templateId: event.target.value })}><option value="">选择模板</option>{templates.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
          </div> : null}
          <DialogFooter><Button variant="outline" onClick={() => setRuleDraft(null)}>取消</Button><Button onClick={() => void saveRule()}>保存</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={templateDraft !== null} onOpenChange={(open) => { if (!open) setTemplateDraft(null); }}>
        <DialogContent>
          <DialogHeader><DialogTitle>{templateDraft?.id ? "编辑回复模板" : "新增回复模板"}</DialogTitle><DialogDescription>首期支持文本、表情、URL 和 action 声明的占位符。</DialogDescription></DialogHeader>
          {templateDraft ? <div className="space-y-4">
            <label className="grid gap-1 text-sm"><span>模板名称</span><Input value={templateDraft.name} onChange={(event) => setTemplateDraft({ ...templateDraft, name: event.target.value })} /></label>
            <label className="grid gap-1 text-sm"><span>模板内容</span><Textarea rows={6} value={templateDraft.content} onChange={(event) => setTemplateDraft({ ...templateDraft, content: event.target.value })} placeholder="你的专属注册码：{{CODE}}" /></label>
          </div> : null}
          <DialogFooter><Button variant="outline" onClick={() => setTemplateDraft(null)}>取消</Button><Button onClick={() => void saveTemplate()}>保存</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return <div className="rounded-md border p-4"><div className="text-sm text-muted-foreground">{label}</div><div className="mt-2 text-2xl font-semibold tabular-nums">{value}</div></div>;
}

function SectionHeader({ title, description, action }: { title: string; description?: string; action?: React.ReactNode }) {
  return <div className="mb-3 flex flex-wrap items-start justify-between gap-3"><div><h3 className="text-lg font-semibold">{title}</h3>{description ? <p className="mt-1 text-sm text-muted-foreground">{description}</p> : null}</div>{action}</div>;
}

function RuleTable({ items, actions, onEdit, onToggle }: { items: WeChatKeywordRule[]; actions: WeChatActionOption[]; onEdit: (draft: RuleDraft) => void; onToggle: (item: WeChatKeywordRule) => void }) {
  return <div className="overflow-x-auto rounded-md border"><Table><TableHeader><TableRow><TableHead>关键词</TableHead><TableHead>功能</TableHead><TableHead>模板</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{items.length === 0 ? <TableRow><TableCell colSpan={5}>暂无关键词规则。</TableCell></TableRow> : items.map((item) => <TableRow key={item.id}><TableCell className="font-mono">{item.keyword}</TableCell><TableCell>{actionLabel(actions, item.action)}</TableCell><TableCell>{item.templateName || "-"}</TableCell><TableCell>{statusBadge(item.enabled)}</TableCell><TableCell className="text-right"><div className="flex justify-end gap-1"><Button size="icon" variant="ghost" title="编辑" onClick={() => onEdit({ id: item.id, keyword: item.keyword, action: item.action, templateId: String(item.templateId), enabled: item.enabled })}><Pencil className="size-4" /></Button><Button size="icon" variant="ghost" title={item.enabled ? "停用" : "启用"} onClick={() => onToggle(item)}><Power className="size-4" /></Button></div></TableCell></TableRow>)}</TableBody></Table></div>;
}

function TemplateTable({ items, rules, onEdit, onToggle }: { items: WeChatReplyTemplate[]; rules: WeChatKeywordRule[]; onEdit: (draft: TemplateDraft) => void; onToggle: (item: WeChatReplyTemplate) => void }) {
  return <div className="overflow-x-auto rounded-md border"><Table><TableHeader><TableRow><TableHead>模板</TableHead><TableHead>内容</TableHead><TableHead>绑定关键词</TableHead><TableHead>状态</TableHead><TableHead className="text-right">操作</TableHead></TableRow></TableHeader><TableBody>{items.length === 0 ? <TableRow><TableCell colSpan={5}>暂无回复模板。</TableCell></TableRow> : items.map((item) => { const bound = rules.filter((rule) => rule.templateId === item.id); return <TableRow key={item.id}><TableCell>{item.name}</TableCell><TableCell className="max-w-sm whitespace-pre-wrap text-sm text-muted-foreground">{item.content}</TableCell><TableCell>{bound.length ? bound.map((rule) => rule.keyword).join("、") : "-"}</TableCell><TableCell>{statusBadge(item.enabled)}</TableCell><TableCell className="text-right"><div className="flex justify-end gap-1"><Button size="icon" variant="ghost" title="编辑" onClick={() => onEdit({ id: item.id, name: item.name, content: item.content, enabled: item.enabled })}><Pencil className="size-4" /></Button><Button size="icon" variant="ghost" title={item.enabled ? "停用" : "启用"} onClick={() => onToggle(item)}><Power className="size-4" /></Button></div></TableCell></TableRow>; })}</TableBody></Table></div>;
}

function BindingTable({ items, actions, onEditRule, onEditTemplate }: { items: WeChatKeywordRule[]; actions: WeChatActionOption[]; onEditRule: (draft: RuleDraft) => void; onEditTemplate: (id: number) => void }) {
  return <div className="overflow-x-auto rounded-md border"><Table><TableHeader><TableRow><TableHead>关键词</TableHead><TableHead>功能</TableHead><TableHead>回复模板</TableHead><TableHead>状态</TableHead><TableHead className="text-right">跳转</TableHead></TableRow></TableHeader><TableBody>{items.length === 0 ? <TableRow><TableCell colSpan={5}>暂无绑定关系。</TableCell></TableRow> : items.map((item) => <TableRow key={item.id}><TableCell className="font-mono">{item.keyword}</TableCell><TableCell>{actionLabel(actions, item.action)}</TableCell><TableCell>{item.templateName || "-"}</TableCell><TableCell>{statusBadge(item.enabled)}</TableCell><TableCell className="text-right"><div className="flex justify-end gap-1"><Button size="icon" variant="ghost" title="编辑关键词绑定" onClick={() => onEditRule({ id: item.id, keyword: item.keyword, action: item.action, templateId: String(item.templateId), enabled: item.enabled })}><Pencil className="size-4" /></Button><Button size="icon" variant="ghost" title="编辑模板" onClick={() => onEditTemplate(item.templateId)}><Check className="size-4" /></Button></div></TableCell></TableRow>)}</TableBody></Table></div>;
}

function SearchBar({ value, onChange, onSearch }: { value: string; onChange: (value: string) => void; onSearch: () => void }) {
  return <div className="flex max-w-xl gap-2"><Input value={value} onChange={(event) => onChange(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") onSearch(); }} placeholder="搜索 OpenID、关键词或注册码" /><Button variant="outline" onClick={onSearch}>查询</Button></div>;
}

function IssuanceTable({ items }: { items: WeChatIssuanceRecord[] }) {
  return <div className="overflow-x-auto rounded-md border"><Table><TableHeader><TableRow><TableHead>OpenID</TableHead><TableHead>注册码</TableHead><TableHead>状态</TableHead><TableHead>注册用户 ID</TableHead><TableHead>发放时间</TableHead></TableRow></TableHeader><TableBody>{items.length === 0 ? <TableRow><TableCell colSpan={5}>暂无发放记录。</TableCell></TableRow> : items.map((item) => <TableRow key={item.id}><TableCell className="max-w-xs break-all font-mono text-xs">{item.openID}</TableCell><TableCell className="font-mono">{item.code}</TableCell><TableCell>{item.status}</TableCell><TableCell>{item.usedByUserId || "-"}</TableCell><TableCell>{formatDate(item.createdAt)}</TableCell></TableRow>)}</TableBody></Table></div>;
}

function LogTable({ items, actions }: { items: WeChatInvocationLog[]; actions: WeChatActionOption[] }) {
  return <div className="overflow-x-auto rounded-md border"><Table><TableHeader><TableRow><TableHead>时间</TableHead><TableHead>OpenID</TableHead><TableHead>关键词</TableHead><TableHead>功能</TableHead><TableHead>结果</TableHead><TableHead>错误摘要</TableHead></TableRow></TableHeader><TableBody>{items.length === 0 ? <TableRow><TableCell colSpan={6}>暂无处理日志。</TableCell></TableRow> : items.map((item) => <TableRow key={item.id}><TableCell>{formatDate(item.createdAt)}</TableCell><TableCell className="max-w-xs break-all font-mono text-xs">{item.openID || "-"}</TableCell><TableCell className="font-mono">{item.keyword}</TableCell><TableCell>{actionLabel(actions, item.action)}</TableCell><TableCell>{item.result}</TableCell><TableCell className="max-w-xs text-sm text-muted-foreground">{item.errorMessage || "-"}</TableCell></TableRow>)}</TableBody></Table></div>;
}

function Pager({ page, total, onChange }: { page: number; total: number; onChange: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / 25));
  return <div className="flex items-center justify-end gap-2 text-sm text-muted-foreground"><span>第 {page} / {pages} 页</span><Button size="sm" variant="outline" disabled={page <= 1} onClick={() => onChange(page - 1)}>上一页</Button><Button size="sm" variant="outline" disabled={page >= pages} onClick={() => onChange(page + 1)}>下一页</Button></div>;
}
