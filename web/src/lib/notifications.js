import { apiFetch, extractErrorMessage } from '../api/client';

export function notificationRequestID() {
  return globalThis.crypto?.randomUUID?.() || `notice_${Date.now()}_${Math.random().toString(36).slice(2)}`;
}

export async function notificationJSON(url, options, fetcher = apiFetch) {
  const res = await fetcher(url, options);
  const json = await res?.json().catch(() => null);
  if (!res?.ok || !json || json.code !== 0) {
    throw new Error(extractErrorMessage(json, '请求失败，请稍后重试 / Request failed, please retry'));
  }
  return json.data;
}

export function notificationDate(value, lang, dateOnly = false) {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) return '—';
  const options = { year: 'numeric', month: 'short', day: 'numeric' };
  if (!dateOnly) Object.assign(options, { hour: '2-digit', minute: '2-digit' });
  return date.toLocaleString(lang === 'zh' ? 'zh-CN' : 'en-US', options);
}

export function notificationLevel(level, lang) {
  const zh = lang === 'zh';
  return {
    info: { label: zh ? '普通' : 'Info', className: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-950/50 dark:text-indigo-300' },
    warning: { label: zh ? '重要' : 'Important', className: 'bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300' },
    critical: { label: zh ? '紧急' : 'Urgent', className: 'bg-rose-50 text-rose-700 dark:bg-rose-950/50 dark:text-rose-300' },
  }[level] || { label: zh ? '普通' : 'Info', className: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-950/50 dark:text-indigo-300' };
}
