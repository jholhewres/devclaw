import { clsx, type ClassValue } from 'clsx';
import { cx } from '@/utils/cx';

/** Merge Tailwind classes with conflict resolution (uses extended tailwind-merge) */
export function cn(...inputs: ClassValue[]) {
  return cx(clsx(inputs));
}

/** Re-export cx utilities for component style objects */
export { cx, sortCx } from '@/utils/cx';

/** Translator function type for timeAgo */
type TFn = (key: string, opts?: Record<string, unknown>) => string;

/** Format a timestamp into a human-readable relative time */
export function timeAgo(
  date: string | Date | null | undefined,
  t?: TFn,
): string {
  if (!date) return '';
  const d = typeof date === 'string' ? new Date(date) : date;
  if (isNaN(d.getTime())) return '';
  const now = new Date();
  const diff = now.getTime() - d.getTime();
  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (t) {
    if (seconds < 60) return t('common.timeAgo.justNow');
    if (minutes < 60) return t('common.timeAgo.minutesAgo', { count: minutes });
    if (hours < 24) return t('common.timeAgo.hoursAgo', { count: hours });
    if (days < 7) return t('common.timeAgo.daysAgo', { count: days });
    return d.toLocaleDateString();
  }

  if (seconds < 60) return 'just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (hours < 24) return `${hours}h ago`;
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString();
}

/** Truncate a string with ellipsis */
export function truncate(str: string | null | undefined, max: number): string {
  if (!str) return '';
  if (str.length <= max) return str;
  return `${str.slice(0, max - 3)}...`;
}

/** Format token count (e.g., 125300 -> "125.3k") */
export function formatTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}k`;
  return `${(n / 1_000_000).toFixed(2)}M`;
}
