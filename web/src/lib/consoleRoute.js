import { getStoredToken, getStoredUser } from '../api/session';
import { allowedTabsFor } from './navGroups';

const CONSOLE_TABS = ['dashboard', 'models', 'pricing', 'channels', 'reconciliation',
  'keys', 'wallet', 'logs', 'skills', 'mcp', 'users', 'playground', 'docs'];
// What a regular user may open comes from lib/navGroups.js — the same source the
// sidebar and the console's role guard read, so the three can no longer disagree.
const USER_TABS = allowedTabsFor(false);
// A forbidden/unknown deep link falls back to the role's first allowed tab.
const USER_FALLBACK_TAB = USER_TABS[0];

// A cookie cannot be inspected by JavaScript. Preserve console deep links until
// /auth/me answers; local profile hints are never proof of a valid session.
export function getConsoleRoute(location, {
  pending = true, user = getStoredUser(), token = getStoredToken(),
} = {}) {
  const hash = location.hash.replace(/^#\/?/, '').trim().toLowerCase();
  const route = hash || (new URLSearchParams(location.search).get('tab') || '').toLowerCase();
  const defaultTab = 'dashboard';
  const result = { viewMode: 'landing', tab: defaultTab, authTab: 'login' };
  if (['status', 'service-status'].includes(route)) return { ...result, viewMode: 'status' };
  if (['auth', 'login', 'register'].includes(route)) {
    return { ...result, viewMode: 'auth', authTab: route === 'register' ? 'register' : 'login' };
  }
  if (route === 'console' || CONSOLE_TABS.includes(route)) {
    const tab = route === 'console' ? defaultTab : route;
    return {
      ...result,
      viewMode: pending || token || user ? 'console' : 'auth',
      tab: !pending && user?.role !== 'admin' && !USER_TABS.includes(tab) ? USER_FALLBACK_TAB : tab,
    };
  }
  return result;
}
