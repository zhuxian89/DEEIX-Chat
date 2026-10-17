"use client";

import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

type NotificationConfig = {
  enabled: boolean;
  templateId: string;
  titleField: string;
  timeField: string;
  state: string;
};

function parseConfig(value: string): NotificationConfig | null {
  try {
    const c: unknown = JSON.parse(value);
    if (!c || typeof c !== "object" || !("enabled" in c) || typeof c.enabled !== "boolean") return null;
    for (const key of ["templateId", "titleField", "timeField", "state"]) {
      if (!(key in c) || typeof (c as Record<string, unknown>)[key] !== "string") return null;
    }
    return c as NotificationConfig;
  } catch {
    return null;
  }
}

export function CompanionNotificationField({ value, onChange }: {
  value: string;
  onChange: (value: string) => void;
}) {
  const config = parseConfig(value);
  if (!config) {
    return (
      <label className="grid gap-1.5 text-sm">
        <span>提醒配置 JSON（当前内容无法结构化，请修正后保存）</span>
        <Textarea rows={8} value={value} onChange={(event) => onChange(event.target.value)} />
      </label>
    );
  }
  const update = (patch: Partial<NotificationConfig>) => onChange(JSON.stringify({ ...config, ...patch }));
  return (
    <div className="space-y-4">
      <label className="flex min-h-10 items-center justify-between gap-4 text-sm">
        <span>启用微信提醒</span>
        <Switch checked={config.enabled} onCheckedChange={(enabled) => update({ enabled })} />
      </label>
      <p className="text-xs text-muted-foreground">关闭后仍保留应用内提醒；微信通知需要用户授权。保存后生效。</p>
      <label className="grid gap-1.5 text-sm">
        <span>日程提醒模板 ID</span>
        <Input value={config.templateId} maxLength={128} onChange={(event) => update({ templateId: event.target.value })} />
      </label>
      <div className="grid grid-cols-2 gap-3">
        <label className="grid gap-1.5 text-sm">
          <span>日程标题字段</span>
          <Input value={config.titleField} placeholder="thing5" onChange={(event) => update({ titleField: event.target.value })} />
        </label>
        <label className="grid gap-1.5 text-sm">
          <span>提醒时间字段</span>
          <Input value={config.timeField} placeholder="time30" onChange={(event) => update({ timeField: event.target.value })} />
        </label>
      </div>
      <p className="text-xs text-muted-foreground">字段填写 thing5、time30 这样的名称，不包含花括号和 .DATA。更换模板后，用户需重新授权。</p>
      <label className="grid gap-1.5 text-sm">
        <span>通知打开的小程序版本</span>
        <Select value={config.state} onValueChange={(state) => update({ state })}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="formal">正式版</SelectItem>
            <SelectItem value="trial">体验版</SelectItem>
            <SelectItem value="developer">开发版</SelectItem>
          </SelectContent>
        </Select>
      </label>
    </div>
  );
}
