import { useEffect, useRef } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, Columns3, RotateCcw } from 'lucide-react';

/**
 * Sortable table header cell. Non-sortable columns render as plain text.
 * `aria-sort` is set so the state is announced by screen readers.
 */
export function SortableTh({ column, sort, onSort, className = '' }) {
  const active = sort?.key === column.key;
  const ariaSort = active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none';

  if (column.sortable === false) {
    return (
      <th scope="col" className={`ui-th ${className}`}>
        {column.label}
      </th>
    );
  }

  return (
    <th scope="col" className={`ui-th ${className}`} aria-sort={ariaSort}>
      <button
        type="button"
        onClick={() => onSort(column.key)}
        className={`inline-flex items-center gap-1 transition cursor-pointer hover:text-slate-800 dark:hover:text-slate-100 ${
          active ? 'text-slate-800 dark:text-slate-100' : ''
        }`}
      >
        <span>{column.label}</span>
        {active ? (
          sort.dir === 'asc' ? (
            <ArrowUp className="h-3 w-3 shrink-0" />
          ) : (
            <ArrowDown className="h-3 w-3 shrink-0" />
          )
        ) : (
          <ArrowUpDown className="h-3 w-3 shrink-0 opacity-40" />
        )}
      </button>
    </th>
  );
}

/** Dropdown that toggles column visibility, with the choice remembered locally. */
export function ColumnToggle({ controls, lang = 'zh' }) {
  const { columns, isVisible, toggleColumn, resetColumns } = controls;
  const ref = useRef(null);
  const isZh = lang === 'zh';
  const hiddenCount = columns.filter((c) => !isVisible(c.key)).length;

  useEffect(() => {
    const onDocMouseDown = (e) => {
      if (ref.current && !ref.current.contains(e.target)) ref.current.open = false;
    };
    document.addEventListener('mousedown', onDocMouseDown);
    return () => document.removeEventListener('mousedown', onDocMouseDown);
  }, []);

  return (
    <details ref={ref} className="relative">
      <summary className="ui-btn-secondary ui-btn-sm list-none [&::-webkit-details-marker]:hidden">
        <Columns3 className="h-3.5 w-3.5" />
        <span>
          {isZh ? '列' : 'Columns'}
          {hiddenCount > 0 ? ` ${columns.length - hiddenCount}/${columns.length}` : ''}
        </span>
      </summary>
      <div className="ui-menu">
        <div className="flex items-center justify-between px-2.5 py-1.5">
          <span className="ui-section-title">{isZh ? '显示列' : 'Visible columns'}</span>
          <button
            type="button"
            onClick={resetColumns}
            className="inline-flex items-center gap-1 text-xs text-indigo-600 dark:text-indigo-400 hover:underline cursor-pointer"
          >
            <RotateCcw className="h-3 w-3" />
            <span>{isZh ? '重置' : 'Reset'}</span>
          </button>
        </div>
        <div className="max-h-64 overflow-y-auto">
          {columns.map((col) => (
            <label key={col.key} className="ui-menu-item">
              <input
                type="checkbox"
                checked={isVisible(col.key)}
                onChange={() => toggleColumn(col.key)}
                className="h-3.5 w-3.5 accent-indigo-600 cursor-pointer"
              />
              <span>{col.label}</span>
            </label>
          ))}
        </div>
      </div>
    </details>
  );
}
