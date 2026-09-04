export function periodUsagePercent(usedUSD: number, creditUSD: number): number {
  if (!Number.isFinite(usedUSD) || !Number.isFinite(creditUSD) || creditUSD <= 0) {
    return 0;
  }
  return Math.min(100, Math.max(0, Math.round(usedUSD / creditUSD * 100)));
}

export function formatUSD(value: number): string {
  return `$${(Number.isFinite(value) ? Math.max(0, value) : 0).toFixed(2)}`;
}

export function formatAccountDate(value: string | null | undefined): string {
  if (!value?.trim()) {
    return "暂无";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "暂无";
  }
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}/${month}/${day}`;
}
