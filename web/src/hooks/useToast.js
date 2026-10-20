// The toast: one transient message with an auto-hide timer.
//
// Extracted from App.jsx. The important property is that showToast keeps a stable
// identity: the request layer registers it in an effect, and a new function on
// every render would re-register the API client on every render.

import { useCallback, useEffect, useRef, useState } from 'react';

const HIDE_AFTER_MS = 3200;

export function useToast() {
  const [toast, setToast] = useState({ show: false, message: '', type: 'info' });
  const timerRef = useRef(null);

  // Stable identity, deliberately: see the note above.
  const showToast = useCallback((message, type = 'info') => {
    if (timerRef.current) clearTimeout(timerRef.current);
    setToast({ show: true, message, type });
    timerRef.current = setTimeout(() => {
      setToast((prev) => ({ ...prev, show: false }));
    }, HIDE_AFTER_MS);
  }, []);

  const dismissToast = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    setToast((prev) => ({ ...prev, show: false }));
  }, []);

  // A pending timer after unmount would set state on a dead component.
  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current);
  }, []);

  return { toast, showToast, dismissToast };
}
