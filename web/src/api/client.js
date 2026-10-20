// Single HTTP client for Control Plane APIs.
//
// This previously existed only as a pair of closures inside App.jsx that
// captured eight pieces of component state, so every view that needed an
// authenticated call either had to receive those closures through props or call
// fetch() itself and re-implement authentication, CSRF and 401 handling.
//
// The client holds no React state. The shell registers a token getter and an
// unauthorized handler once, and both are consulted at call time — which also
// removes the stale-closure hazard the old render-scoped closures had.
//
// Deliberately free of React imports so it can be unit tested directly.

import { SESSION_MARKER, readCSRFCookie } from './session';

let tokenProvider = () => '';
let unauthorizedHandler = () => {};

/**
 * Register the session hooks the client uses on every request.
 *
 * Both values are functions and are read at call time, never captured:
 *   token()                  -> current bearer token, or '' for cookie sessions
 *   unauthorized({response, token}) -> called on every 401, before the caller
 *                                       sees the response
 */
export function configureApiClient({ token, unauthorized } = {}) {
  if (typeof token === 'function') tokenProvider = token;
  if (typeof unauthorized === 'function') unauthorizedHandler = unauthorized;
}

/** Restore the initial state. Intended for tests. */
export function resetApiClient() {
  tokenProvider = () => '';
  unauthorizedHandler = () => {};
}

/**
 * Authenticated fetch. Adds the bearer token (unless the session is cookie
 * based), the CSRF header, and same-origin credentials, and reports 401s to the
 * registered handler. Returns the raw Response so callers that stream or read
 * headers keep working.
 */
export async function apiFetch(url, options = {}) {
  const { token, headers: optionHeaders, ...rest } = options;
  const headers = { ...(optionHeaders || {}) };
  const effectiveToken = token || tokenProvider();

  // SESSION_MARKER only means "logged in"; sending it as a bearer token would
  // make the server validate it (and reject it) before falling back to the
  // HttpOnly cookie that holds the real credential.
  if (effectiveToken && effectiveToken !== SESSION_MARKER) {
    headers['Authorization'] = `Bearer ${effectiveToken}`;
  }
  const csrf = readCSRFCookie();
  if (csrf) {
    headers['X-CSRF-Token'] = csrf;
  }

  const res = await fetch(url, { ...rest, headers, credentials: 'same-origin' });
  if (res.status === 401) {
    unauthorizedHandler({ response: res, token: effectiveToken });
  }
  return res;
}

/**
 * Authenticated JSON request that also treats application-level failures as
 * errors. Mutating flows must never report success for a rejected request, so a
 * non-zero envelope code throws just like a non-2xx status does.
 */
export async function apiJSON(url, options = {}) {
  const res = await apiFetch(url, options);
  let data = null;
  try {
    data = await res.json();
  } catch {
    data = null;
  }
  if (!res.ok) {
    throw new Error((data && (data.error || data.message)) || `HTTP ${res.status}`);
  }
  if (data && typeof data.code === 'number' && data.code !== 0) {
    throw new Error(data.error || data.message || `Request failed with code ${data.code}`);
  }
  return data;
}
