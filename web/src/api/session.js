// Session credential storage for the console.
//
// The session token itself lives in an HttpOnly cookie; only a "logged in"
// marker and non-sensitive profile hints are kept in localStorage, so an XSS
// cannot read a long-lived credential out of web storage.
//
// Extracted from App.jsx so the request layer and the views share one
// definition of what is stored and what is deliberately not stored.

// SESSION_MARKER is a "logged in" flag, never a credential. The request layer
// must not send it as a bearer token: the server would validate it (and reject
// it) before falling back to the HttpOnly cookie.
export const SESSION_MARKER = '__cookie_session__';

// Legacy keys held the raw session token before the session moved into an
// HttpOnly cookie. They are still read (so an existing session survives the
// upgrade) but never written.
const LEGACY_TOKEN_KEYS = ['airoute_token', 'airoute_admin_token'];
const USER_KEYS = ['airoute_user', 'airoute_admin_user'];

export function getStoredToken() {
  if (typeof window === 'undefined') return '';
  return localStorage.getItem('airoute_token') || localStorage.getItem('airoute_admin_token') || '';
}

export function getStoredUser() {
  if (typeof window === 'undefined') return null;
  const raw = localStorage.getItem('airoute_user') || localStorage.getItem('airoute_admin_user');
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function setStoredAuth(_token, user) {
  if (typeof window === 'undefined') return;
  // Only profile hints are persisted locally; the legacy token keys are dropped
  // on login/logout so a long-lived credential cannot be read back out.
  for (const key of LEGACY_TOKEN_KEYS) localStorage.removeItem(key);
  if (user) {
    const userStr = typeof user === 'string' ? user : JSON.stringify(user);
    localStorage.setItem('airoute_user', userStr);
  }
}

export function clearStoredAuth() {
  if (typeof window === 'undefined') return;
  for (const key of [...LEGACY_TOKEN_KEYS, ...USER_KEYS]) localStorage.removeItem(key);
}

// readCSRFCookie reads the double-submit CSRF token the gateway sets alongside
// the session cookie.
export function readCSRFCookie() {
  if (typeof document === 'undefined') return '';
  const match = document.cookie.match(/(?:^|;\s*)airoute_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : '';
}
