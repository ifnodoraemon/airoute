import { getStoredToken, getStoredUser } from '../api/session';

const CONSOLE_TABS = ['dashboard', 'models', 'pricing', 'channels', 'reconciliation',
  'keys', 'wallet', 'logs', 'skills', 'mcp', 'users', 'playground', 'docs'];
const USER_TABS = ['wallet', 'keys', 'playground', 'pricing', 'logs', 'docs'];

// A cookie cannot be inspected by JavaScript. Preserve console deep links until
// /auth/me answers; local profile hints are never proof of a valid session.
export function getConsoleRoute(location, {
  pending = true, user = getStoredUser(), token = getStoredToken(),
} = {}) {
  const hash = location.hash.replace(/^#\/?/, '').trim().toLowerCase();
  const route = hash || (new URLSearchParams(location.search).get('tab') || '').toLowerCase();
  const defaultTab = user?.role === 'admin' ? 'dashboard' : 'wallet';
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
      tab: !pending && user?.role !== 'admin' && !USER_TABS.includes(tab) ? 'wallet' : tab,
    };
  }
  return result;
}
