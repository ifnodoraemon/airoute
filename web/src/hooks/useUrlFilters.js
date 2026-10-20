import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Keep a view's filter object in the URL query string.
 *
 * Two problems this solves:
 *  - filters used to live only in component state, so switching modules and
 *    coming back silently reset them;
 *  - there was no way to share "the exact list I am looking at" as a link.
 *
 * Values are stored under `<prefix>.<key>` (e.g. `channels.status=breaker_open`)
 * so several views can keep their own filters in one URL without colliding.
 * Defaults are omitted from the URL to keep shared links short, and every
 * write uses `replaceState` so typing in a search box does not spam history.
 *
 * All values are kept as strings — the same shape the views already use for
 * select/toggle state — which keeps callers free of coercion bugs.
 *
 * @param {string} prefix  Namespace, e.g. 'channels'.
 * @param {Record<string, string>} defaults  Initial value for each filter key.
 * @returns {[Record<string,string>, (patch: Record<string,string>) => void, () => void]}
 */
export default function useUrlFilters(prefix, defaults) {
  const defaultsRef = useRef(defaults);
  defaultsRef.current = defaults;

  const read = useCallback(() => {
    const base = defaultsRef.current;
    const next = { ...base };
    if (typeof window === 'undefined') return next;
    const params = new URLSearchParams(window.location.search);
    for (const key of Object.keys(base)) {
      const raw = params.get(`${prefix}.${key}`);
      if (raw !== null && raw !== '') next[key] = raw;
    }
    return next;
  }, [prefix]);

  const [filters, setFiltersState] = useState(read);

  const write = useCallback((next) => {
    if (typeof window === 'undefined') return;
    const base = defaultsRef.current;
    const url = new URL(window.location.href);
    for (const key of Object.keys(base)) {
      const param = `${prefix}.${key}`;
      const value = next[key];
      if (value === undefined || value === null || value === '' || value === base[key]) {
        url.searchParams.delete(param);
      } else {
        url.searchParams.set(param, String(value));
      }
    }
    window.history.replaceState(window.history.state, '', url);
  }, [prefix]);

  const setFilters = useCallback((patch) => {
    setFiltersState((prev) => (typeof patch === 'function' ? patch(prev) : { ...prev, ...patch }));
  }, []);

  const resetFilters = useCallback(() => {
    setFiltersState({ ...defaultsRef.current });
  }, []);

  // Mirror the current filters into the query string. `replaceState` keeps the
  // browser history clean while typing, and the write is idempotent so the
  // popstate listener below can safely feed values back in.
  useEffect(() => {
    write(filters);
  }, [filters, write]);

  // Back/forward navigation should restore whatever the URL says.
  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const onPop = () => setFiltersState(read());
    window.addEventListener('popstate', onPop);
    window.addEventListener('hashchange', onPop);
    return () => {
      window.removeEventListener('popstate', onPop);
      window.removeEventListener('hashchange', onPop);
    };
  }, [read]);

  return [filters, setFilters, resetFilters];
}
