// The console's session: who is signed in, which view is showing, and the named
// transitions between those states.
//
// Extracted from App.jsx, where the same "the session is gone" teardown appeared
// three times with slightly different side effects, and the credential was cleared
// by four different callers. Naming the transitions is the point: `endSession`,
// `restoreSession` and `signIn` each mean one thing, and the UI reaction to each is
// passed in rather than interleaved.
//
// The token state is a marker, never a credential: the real session lives in an
// HttpOnly cookie (see api/session.js).

import { useCallback, useRef, useState } from 'react';
import { SESSION_MARKER, clearStoredAuth, getStoredToken, getStoredUser, setStoredAuth } from '../api/session';

export function useConsoleSession({ initialRoute, onEntered, onEnded }) {
  const [viewMode, setViewMode] = useState(() => initialRoute.viewMode);
  // Legacy key first: a session created before the cookie migration still works.
  const [adminToken, setAdminToken] = useState(() => getStoredToken());
  const [adminUser, setAdminUser] = useState(() => getStoredUser());
  const [authTab, setAuthTab] = useState(() => initialRoute.authTab);
  const [restoringSession, setRestoringSession] = useState(true);
  const sessionGeneration = useRef(0);
  const getSessionGeneration = useCallback(() => sessionGeneration.current, []);
  const finishSessionRestore = useCallback(() => setRestoringSession(false), []);

  // The transitions below are passed to effects and to the request layer, so they
  // read the view mode at call time rather than the value captured when their
  // closure was created.
  const viewModeRef = useRef(viewMode);
  viewModeRef.current = viewMode;

  const isAdmin = adminUser?.role === 'admin';
  const isAuthenticated = !!adminUser;

  /**
   * The credential to send as a bearer token, or '' when there is none.
   *
   * The stored marker means "signed in via the HttpOnly cookie"; sending it would
   * make the gateway validate the literal string and reject the request before it
   * ever falls back to the cookie. Only a credential obtained from a sign-in
   * endpoint is a real token.
   */
  const credentialToken = adminToken === SESSION_MARKER ? '' : (adminToken || '');

  const clearCredential = useCallback(() => {
    sessionGeneration.current += 1;
    setRestoringSession(false);
    setAdminToken('');
    setAdminUser(null);
    clearStoredAuth();
  }, []);

  /**
   * A signed-in session became available: a login form, an OAuth grant, or a cookie
   * restore. The console is entered unless the route is still resolving an auth page
   * the user asked for.
   *
   * `token` is the credential the sign-in endpoint returned, when there is one. A
   * cookie restore has none, so it records a marker instead — deliberately not a
   * credential, because the request layer must not send it as a bearer token.
   * Either way the value is non-empty, which is what gates the data polling loop and
   * the isLoggedIn props.
   */
  const signIn = useCallback((user, { token, tab, closeAuth = true } = {}) => {
    sessionGeneration.current += 1;
    setRestoringSession(false);
    setStoredAuth('', user);
    setAdminToken(token || SESSION_MARKER);
    setAdminUser(user);
    setViewMode('console');
    if (tab) setAuthTab(tab);
    if (closeAuth) onEntered?.();
  }, [onEntered]);

  /**
   * The HttpOnly cookie session was still valid on load. Unlike signIn this does
   * not force the console: a visitor reading the landing page stays there, and only
   * the auth page is skipped, because a valid session makes it pointless.
   */
  const restoreSession = useCallback((user) => {
    setRestoringSession(false);
    setStoredAuth('', user);
    setAdminToken(SESSION_MARKER);
    setAdminUser(user);
    setViewMode((prev) => (prev === 'auth' ? 'console' : prev));
  }, []);

  /**
   * The session is no longer usable: the restore failed, the gateway answered 401,
   * or a profile fetch was rejected. Leaves the console rather than showing a shell
   * whose every panel will fail.
   *
   * `notify` distinguishes the two cases the UI cares about: an expired session
   * tells the user why they were moved, a rejected restore on first load does not.
   */
  const endSession = useCallback(({ notify = false } = {}) => {
    clearCredential();
    if (viewModeRef.current === 'console') {
      setViewMode('landing');
      onEnded?.({ notify });
    }
  }, [clearCredential, onEnded]);

  /** The user chose to leave. The workspace teardown is the caller's. */
  const signOutLocal = useCallback(() => {
    clearCredential();
    setViewMode('landing');
  }, [clearCredential]);

  /**
   * Replaces the signed-in profile with a freshly fetched one, persisting the
   * non-sensitive parts. Does not touch the credential: a profile refresh must not
   * sign anyone in or out.
   */
  const refreshUser = useCallback((user) => {
    if (!user) return;
    setAdminUser((previous) => {
      if (!previous || previous.username !== user.username) return previous;
      setStoredAuth('', user);
      return user;
    });
  }, []);

  /**
   * Merges a partial update (a new balance, a renamed account) into the signed-in
   * profile and persists it. Written as a functional update so two patches in the
   * same tick do not overwrite each other.
   */
  const patchUser = useCallback((patch) => {
    setAdminUser((prev) => {
      const merged = prev ? { ...prev, ...patch } : prev;
      if (merged) setStoredAuth('', merged);
      return merged;
    });
  }, []);

  /** Applies a route resolved from the address bar (hash or popstate). */
  const applyRoute = useCallback((routeInfo) => {
    setViewMode((prev) => (prev !== routeInfo.viewMode ? routeInfo.viewMode : prev));
    setAuthTab((prev) => (prev !== routeInfo.authTab ? routeInfo.authTab : prev));
  }, []);

  const openAuth = useCallback((tab = 'login') => {
    setAuthTab(tab);
    setViewMode('auth');
  }, []);

  return {
    viewMode,
    adminToken,
    adminUser,
    authTab,
    isAdmin,
    isAuthenticated,
    credentialToken,
    restoringSession,
    getSessionGeneration,
    finishSessionRestore,
    setViewMode,
    setAuthTab,
    signIn,
    restoreSession,
    refreshUser,
    patchUser,
    endSession,
    signOutLocal,
    applyRoute,
    openAuth,
  };
}
