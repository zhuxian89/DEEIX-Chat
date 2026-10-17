// Use UTC getters after a fixed offset so device timezone and Intl support do not matter.
export function formatMessageTime(createdAt?: string): string {
  if (!createdAt?.trim()) return "";
  const timestamp = Date.parse(createdAt);
  if (!Number.isFinite(timestamp)) return "";
  const beijing = new Date(timestamp + 8 * 60 * 60 * 1000);
  if (!Number.isFinite(beijing.getTime())) return "";
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${beijing.getUTCFullYear()}-${pad(beijing.getUTCMonth() + 1)}-${pad(beijing.getUTCDate())} ${pad(beijing.getUTCHours())}:${pad(beijing.getUTCMinutes())}`;
}
