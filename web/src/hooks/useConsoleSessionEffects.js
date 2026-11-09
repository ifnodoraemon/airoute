import { useEffect, useCallback } from 'react';
import { useSessionRestore } from './useSessionRestore';
import { setPublicGatewayUrl } from '../config';

export default function useConsoleSessionEffects({
  endSession, fetchData, fetchLogs, finishSessionRestore, getSessionGeneration, isZh, restoreSession,
  setCurrentTab, setShowCommandPalette, setShowOnboardingModal, showToast, signIn
}) {
  useEffect(() => {
    const handleGlobalKeyDown = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setShowCommandPalette(prev => !prev);
      }
    };
    window.addEventListener('keydown', handleGlobalKeyDown);
    return () => window.removeEventListener('keydown', handleGlobalKeyDown);
  }, [setShowCommandPalette, setShowOnboardingModal]);

  // The session handlers are read at call time, so a late answer from the gateway
  // is judged against the view mode the user is on now rather than the one
  // captured when this component mounted.
  const handleSessionRestored = useCallback((user) => {
    restoreSession(user);
  }, [restoreSession]);

  // A rejected restore on first load moves the visitor out of the console without
  // an expiry notice: they never had a session to lose.
  const handleSessionExpired = useCallback(() => {
    endSession({ notify: false });
  }, [endSession]);

  // Restore the browser session from the HttpOnly cookie instead of trusting
  // a long-lived token in localStorage.
  useSessionRestore({
    onRestored: handleSessionRestored,
    onExpired: handleSessionExpired,
    onSettled: finishSessionRestore,
    getSessionGeneration,
  });

  // Query gateway public status to resolve canonical external gateway URL
  useEffect(() => {
    fetch('/api/v1/public/status')
      .then(res => res.json())
      .then(data => {
        if (data && data.data && data.data.public_url) {
          setPublicGatewayUrl(data.data.public_url);
        }
      })
      .catch(() => {});
  }, []);

  // The callback sets the session cookie; restore it through any replica.
  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const params = new URLSearchParams(window.location.search);
    const completed = params.get('oauth_complete');
    const oauthError = params.get('oauth_error');
    if (!completed && !oauthError) return undefined;

    const stripParams = () => {
      const url = new URL(window.location.href);
      url.searchParams.delete('oauth_complete');
      url.searchParams.delete('oauth_error');
      const search = url.searchParams.toString();
      window.history.replaceState({}, '', url.pathname + (search ? `?${search}` : '') + url.hash);
    };

    if (oauthError) {
      stripParams();
      showToast(oauthError, 'error');
      return undefined;
    }

    let cancelled = false;
    (async () => {
      try {
        const res = await fetch('/api/v1/auth/me', { credentials: 'same-origin' });
        const data = await res.json().catch(() => ({}));
        if (!res.ok || data.code !== 0) {
          throw new Error(data.error || (isZh ? 'OAuth 登录失败' : 'OAuth sign-in failed'));
        }
        if (cancelled) return;
        const user = data.data?.user || data.data;
        signIn(user);
        setCurrentTab(user?.role === 'admin' ? 'dashboard' : 'wallet');
        showToast(
          isZh ? `欢迎回来，${user?.username || ''}！` : `Welcome back, ${user?.username || ''}!`,
          'success'
        );
        fetchData();
        fetchLogs();
      } catch (e) {
        if (!cancelled) {
          showToast(
            (isZh ? 'OAuth 登录失败: ' : 'OAuth sign-in failed: ') + e.message,
            'error'
          );
        }
      } finally {
        stripParams();
      }
    })();

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return {

  };
}
