// Restores the browser session from the HttpOnly cookie on mount.
//
// Extracted from App.jsx, where it was one half of a two-purpose effect whose
// dependency array was empty while the body read `viewMode`. That is the classic
// stale-closure shape: the value was captured at mount, so a session that failed
// to restore after the user had already navigated was judged against the view
// mode from before the navigation.
//
// The handlers are held in refs and refreshed on every render, so the effect can
// legitimately run exactly once while always calling the current handler.

import { useEffect, useRef } from 'react';
import { getStoredToken } from '../api/session';

/**
 * @param {object}   options
 * @param {Function} options.onRestored called with the restored user profile
 * @param {Function} options.onExpired  called when the session is not usable
 */
export function useSessionRestore({ onRestored, onExpired }) {
  const restoredRef = useRef(onRestored);
  const expiredRef = useRef(onExpired);

  useEffect(() => {
    restoredRef.current = onRestored;
  }, [onRestored]);

  useEffect(() => {
    expiredRef.current = onExpired;
  }, [onExpired]);

  // Runs once: the indirections above are what keep it current.
  useEffect(() => {
    const savedToken = getStoredToken();
    const headers = savedToken ? { Authorization: `Bearer ${savedToken}` } : {};

    fetch('/api/v1/auth/me', { headers, credentials: 'same-origin' })
      .then((res) => (res.ok ? res.json() : { code: 401 }))
      .then((data) => {
        if (data.code === 0 && data.data) {
          restoredRef.current(data.data.user || data.data);
        } else {
          expiredRef.current();
        }
      })
      .catch(() => {});
  }, []);
}
