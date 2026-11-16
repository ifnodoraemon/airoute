import { useCallback, useEffect, useRef, useState } from 'react';
import { extractErrorMessage } from '../api/client';

export default function useInfraHealth(adminFetch, showToast) {
  const [state, setState] = useState({ components: [], loading: true, error: '', lastUpdated: null });
  const generation = useRef(0);
  const controller = useRef(null);
  const load = useCallback(async () => {
    const current = ++generation.current;
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    setState(value => ({ ...value, loading: true, error: '' }));
    try {
      const res = await adminFetch('/api/v1/admin/system/middlewares', { signal: abort.signal });
      const body = await res.json().catch(() => null);
      if (!res.ok || body?.code !== 0 || !Array.isArray(body.data)) {
        throw new Error(extractErrorMessage(body, `HTTP ${res.status}`));
      }
      if (generation.current === current) {
        setState({ components: body.data, loading: false, error: '', lastUpdated: Date.now() });
      }
    } catch (err) {
      if (abort.signal.aborted || generation.current !== current) return;
      const message = err?.message || String(err);
      setState(value => ({ ...value, loading: false, error: message }));
      showToast?.(message, 'error');
    }
  }, [adminFetch, showToast]);

  const cancel = useCallback(() => {
    generation.current++;
    controller.current?.abort();
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 30000);
    return () => {
      clearInterval(interval);
      cancel();
    };
  }, [load, cancel]);
  return { ...state, load };
}
