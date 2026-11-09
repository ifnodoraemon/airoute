import { useCallback, useEffect, useRef, useState } from 'react';
import { apiFetch } from '../api/client';
import { notificationJSON } from '../lib/notifications';

export default function useNotificationFeed({ kind, refreshKey, onChanged, fetcher = apiFetch }) {
  const [data, setData] = useState(null);
  const [page, setPage] = useState(1);
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [busy, setBusy] = useState(false);
  const inFlight = useRef(false);
  const requestRef = useRef(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => { mounted.current = false; };
  }, []);

  useEffect(() => { setData(null); }, [kind, page, unreadOnly]);

  useEffect(() => {
    let disposed = false;
    const load = async () => {
      requestRef.current?.abort();
      const request = new AbortController();
      requestRef.current = request;
      setLoading(true);
      setError('');
      try {
        const params = new URLSearchParams({ kind, page: String(page), page_size: '20', unread: String(unreadOnly) });
        const result = await notificationJSON(`/api/v1/user/notifications?${params}`, { signal: request.signal }, fetcher);
        if (!Array.isArray(result?.items)) throw new Error('消息数据无效 / Invalid message data');
        if (!disposed && !request.signal.aborted) {
          if (page > 1 && result.items.length === 0) setPage(Math.max(1, Math.min(page - 1, Math.ceil(result.total / 20))));
          else setData(result);
        }
      } catch (e) {
        if (!disposed && !request.signal.aborted) setError(e.message);
      } finally {
        if (!disposed && !request.signal.aborted) setLoading(false);
      }
    };
    const poll = () => { if (!document.hidden && !inFlight.current) load(); };
    load();
    const timer = setInterval(poll, 60000);
    window.addEventListener('focus', poll);
    return () => {
      disposed = true;
      requestRef.current?.abort();
      clearInterval(timer);
      window.removeEventListener('focus', poll);
    };
  }, [kind, page, unreadOnly, refreshKey, reloadKey, fetcher]);

  const reload = useCallback(() => setReloadKey(value => value + 1), []);
  const mutate = async (url, body, update) => {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError('');
    try {
      await notificationJSON(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }, fetcher);
      if (!mounted.current) return;
      requestRef.current?.abort();
      setLoading(false);
      setData(update);
      onChanged?.();
      return true;
    } catch (e) {
      if (mounted.current) setError(e.message);
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(false);
    }
  };

  const markRead = (item) => {
    if (item.read_at) return;
    return mutate(`/api/v1/user/notifications/${encodeURIComponent(item.id)}/read`, {}, old => {
      const current = old?.items.find(row => row.id === item.id);
      if (!current || current.read_at) return old;
      const countKey = kind === 'announcement' ? 'announcement_unread_count' : 'message_unread_count';
      return {
        ...old,
        unread_count: Math.max(0, old.unread_count - 1),
        [countKey]: Math.max(0, old[countKey] - 1),
        items: old.items.map(row => row.id === item.id ? { ...row, read_at: new Date().toISOString() } : row),
      };
    });
  };

  const markAllRead = async () => {
    if (!data?.latest_sequence) return;
    const saved = await mutate('/api/v1/user/notifications/read-all', { through_sequence: data.latest_sequence, kind }, old => {
      if (!old) return old;
      const countKey = kind === 'announcement' ? 'announcement_unread_count' : 'message_unread_count';
      return {
        ...old, [countKey]: 0, unread_count: Math.max(0, old.unread_count - old[countKey]),
        items: old.items.map(row => ({ ...row, read_at: row.read_at || new Date().toISOString() })),
      };
    });
    if (saved && mounted.current) reload();
  };

  return {
    data, page, setPage, unreadOnly, setUnreadOnly, loading, error, busy,
    reload, markRead, markAllRead,
  };
}
