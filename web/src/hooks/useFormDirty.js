import { useEffect, useMemo, useRef } from 'react';

/** JSON stringify with sorted keys, so key order never fakes a change. */
export function stableStringify(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value ?? null);
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;
  return `{${Object.keys(value)
    .sort()
    .map((k) => `${JSON.stringify(k)}:${stableStringify(value[k])}`)
    .join(',')}}`;
}

/**
 * Track whether a form's draft differs from the values it was opened with.
 *
 * Used to warn before throwing away typed input when a modal is dismissed via
 * the backdrop, the close button or Escape.
 *
 * @param {boolean} isOpen  Whether the owning modal is currently open.
 * @param {any} value       The draft object (compared by value, key order agnostic).
 * @returns {boolean} true when the user has unsaved edits.
 */
export default function useFormDirty(isOpen, value) {
  const baselineRef = useRef(null);
  const snapshot = useMemo(() => stableStringify(value ?? {}), [value]);

  useEffect(() => {
    if (!isOpen) {
      baselineRef.current = null;
      return;
    }
    if (baselineRef.current === null) baselineRef.current = snapshot;
  }, [isOpen, snapshot]);

  return isOpen && baselineRef.current !== null && baselineRef.current !== snapshot;
}
