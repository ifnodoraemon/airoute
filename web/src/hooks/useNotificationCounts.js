import { useEffect, useState } from 'react';
import { apiFetch } from '../api/client';
import { notificationJSON } from '../lib/notifications';

export default function useNotificationCounts({ enabled, userKey, refreshKey = 0, fetcher = apiFetch }) {
  const [result, setResult] = useState({ userKey: null, counts: null });
  useEffect(() => {
    const setCounts = counts => setResult({ userKey, counts });
    setResult(old => old.userKey === userKey && enabled ? old : { userKey, counts: null });
    if (!enabled) return undefined;
    let controller;
    let disposed = false;
    const refresh = async () => {
      controller?.abort();
      const request = new AbortController();
      controller = request;
      try {
        const data = await notificationJSON('/api/v1/user/notifications/unread-count', { signal: request.signal }, fetcher);
        if (!disposed && !request.signal.aborted) {
          setCounts(typeof data?.unread_count === 'number' ? data : null);
        }
      } catch {
        if (!disposed && !request.signal.aborted) setCounts(null);
      }
    };
    const poll = () => { if (!document.hidden) refresh(); };
    refresh();
    const timer = setInterval(poll, 60000);
    window.addEventListener('focus', poll);
    return () => {
      disposed = true;
      controller?.abort();
      clearInterval(timer);
      window.removeEventListener('focus', poll);
    };
  }, [enabled, userKey, refreshKey, fetcher]);
  return enabled && result.userKey === userKey ? result.counts : null;
}
