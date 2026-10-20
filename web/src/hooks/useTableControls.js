import { useCallback, useMemo, useState } from 'react';

/**
 * Column sorting + column visibility for the list tables.
 *
 * Before this, comparing quotas or hunting a tripped channel meant scanning
 * every row by eye. Sorting is per-column (numeric or locale-aware string) and
 * the visible-column set is remembered in localStorage so a chosen layout
 * survives reloads.
 *
 * @param {object}   options
 * @param {Array}    options.columns  [{ key, label, sortable?, defaultVisible?, sortValue?(row) }]
 * @param {string}   [options.storageKey]  localStorage key for column visibility.
 * @param {object}   [options.initialSort] { key, dir: 'asc' | 'desc' }
 */
export default function useTableControls({ columns, storageKey, initialSort = null }) {
  const defaultVisibility = useMemo(
    () => Object.fromEntries(columns.map((c) => [c.key, c.defaultVisible !== false])),
    [columns],
  );

  const [visible, setVisible] = useState(() => {
    if (!storageKey || typeof window === 'undefined') return defaultVisibility;
    try {
      const raw = window.localStorage.getItem(storageKey);
      if (!raw) return defaultVisibility;
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== 'object') return defaultVisibility;
      // Merge so columns added in a later release are not hidden by stale state.
      return { ...defaultVisibility, ...parsed };
    } catch {
      return defaultVisibility;
    }
  });

  const [sort, setSort] = useState(initialSort);

  const persist = useCallback((next) => {
    setVisible(next);
    if (!storageKey || typeof window === 'undefined') return;
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(next));
    } catch {
      /* storage unavailable (private mode) — visibility simply is not remembered */
    }
  }, [storageKey]);

  const toggleColumn = useCallback((key) => {
    persist({ ...visible, [key]: visible[key] === false });
  }, [persist, visible]);

  const resetColumns = useCallback(() => persist(defaultVisibility), [persist, defaultVisibility]);

  const toggleSort = useCallback((key) => {
    setSort((prev) => {
      if (!prev || prev.key !== key) return { key, dir: 'asc' };
      return { key, dir: prev.dir === 'asc' ? 'desc' : 'asc' };
    });
  }, []);

  const isVisible = useCallback((key) => visible[key] !== false, [visible]);

  const sortRows = useCallback((rows) => {
    if (!sort?.key) return rows;
    const column = columns.find((c) => c.key === sort.key);
    if (!column) return rows;
    const read = column.sortValue || ((row) => row[column.key]);
    const dir = sort.dir === 'desc' ? -1 : 1;
    return [...rows].sort((a, b) => {
      const va = read(a);
      const vb = read(b);
      if (va === null || va === undefined || va === '') return vb === null || vb === undefined || vb === '' ? 0 : 1;
      if (vb === null || vb === undefined || vb === '') return -1;
      if (typeof va === 'number' && typeof vb === 'number') return (va - vb) * dir;
      return String(va).localeCompare(String(vb), 'zh-Hans-CN', { numeric: true, sensitivity: 'base' }) * dir;
    });
  }, [columns, sort]);

  return { sort, setSort, toggleSort, sortRows, visible, isVisible, toggleColumn, resetColumns, columns };
}
