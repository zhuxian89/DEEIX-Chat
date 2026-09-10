export function beijingDate(offset = 0, now = Date.now()): string {
  const date = new Date(now + 8 * 60 * 60 * 1000);
  date.setUTCDate(date.getUTCDate() + offset);
  return date.toISOString().slice(0, 10);
}

export function isOverdue(task: { dueDate: string; dueTime: string; completedAt?: string; deletedAt?: string }, now = Date.now()): boolean {
  if (!task.dueDate || task.completedAt || task.deletedAt) return false;
  const date = new Date(now + 8 * 60 * 60 * 1000).toISOString();
  const today = date.slice(0, 10);
  return task.dueDate < today || (task.dueDate === today && !!task.dueTime && task.dueTime < date.slice(11, 16));
}
