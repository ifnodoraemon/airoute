import { useEffect, useCallback } from 'react';
import { getConsoleRoute } from '../lib/consoleRoute';
import { allowedTabsFor } from '../lib/navGroups';

export default function useConsoleNavigation({
  adminToken, adminUser, applyRoute, authTab, currentTab, restoringSession, setAuthTab, setCurrentTab,
  viewMode
}) {
  const getRouteFromURL = useCallback(() => getConsoleRoute(window.location, {
    pending: restoringSession, user: adminUser, token: adminToken,
  }), [restoringSession, adminUser, adminToken]);

  // Synchronize browser address bar URL hash with active view and tab
  useEffect(() => {
    if (restoringSession) return;
    let targetHash = '';
    if (viewMode === 'landing') {
      if (window.location.hash && window.location.hash !== '#/' && window.location.hash !== '#/landing') {
        const curHash = window.location.hash.replace(/^#\/?/, '').trim();
        if (!['features', 'multimodal', 'docs', 'pricing'].includes(curHash)) {
          targetHash = '#/';
        }
      }
    } else if (viewMode === 'auth') {
      targetHash = authTab === 'register' ? '#/register' : '#/login';
    } else if (viewMode === 'status') {
      targetHash = '#/status';
    } else if (viewMode === 'console') {
      targetHash = `#/${currentTab}`;
    }

    if (viewMode === 'status' || viewMode === 'landing' || viewMode === 'auth') {
      if (window.location.hash !== targetHash || window.location.search) {
        // Only the hash identifies the public page; the query string carries the
        // audit-log/channel filters (logs.*, channels.*). Rebuilding the URL from
        // the pathname dropped them, so a filtered view could not be shared or
        // reloaded after stopping by the portal.
        const url = new URL(window.location.href);
        url.hash = targetHash || '';
        window.history.replaceState(null, '', url);
      }
    } else if (targetHash && window.location.hash !== targetHash) {
      window.history.replaceState(null, '', targetHash);
    }
  }, [viewMode, currentTab, authTab, setCurrentTab, setAuthTab, restoringSession]);

  // Browser Back / Forward. Defined at component scope rather than inside the
  // effect so it can be memoised: a hook may not be called from within a callback.
  const handleLocationChange = useCallback(() => {
    const routeInfo = getRouteFromURL();
    applyRoute(routeInfo);
    setCurrentTab(prev => prev !== routeInfo.tab ? routeInfo.tab : prev);
  }, [getRouteFromURL, applyRoute, setCurrentTab]);

  // Listen to browser Back / Forward buttons (hashchange and popstate)
  useEffect(() => {
    window.addEventListener('hashchange', handleLocationChange);
    window.addEventListener('popstate', handleLocationChange);
    return () => {
      window.removeEventListener('hashchange', handleLocationChange);
      window.removeEventListener('popstate', handleLocationChange);
    };
  }, [handleLocationChange]);

  // Keep the active tab inside what the signed-in role may open.
  //
  // The allowed set comes from lib/navGroups.js — the same list the sidebar renders
  // and lib/consoleRoute.js resolves deep links against — so a tab can no longer be
  // advertised in the navigation while a hand-written guard forbids it. That drift
  // is exactly what hid the self-service usage page from regular users: the guard's
  // private list omitted `dashboard`, so "Overview" bounced them to the wallet.
  //
  // An unknown/forbidden tab falls back to the role's FIRST allowed tab (the same
  // rule the route resolver uses) instead of a hard-coded page.
  useEffect(() => {
    if (restoringSession || !adminUser) return;
    const allowedTabs = allowedTabsFor(adminUser.role === 'admin');
    if (!allowedTabs.includes(currentTab)) {
      setCurrentTab(allowedTabs[0]);
    }
  }, [adminUser, currentTab, setCurrentTab, restoringSession]);
  return {

  };
}
