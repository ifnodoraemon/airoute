import { useState, useEffect, useCallback, useRef, lazy, Suspense } from 'react';
import {
  Zap,
  Server,
  Building2,
  Gauge,
  Calculator,
  ReceiptText,
  Key,
  Terminal,
  ShieldAlert,

  AlertCircle,
  Cpu,
  Sparkles,
  BookOpen,
  History,
  Search,
  BarChart3,
  Globe,
  LogOut,
  User,
  Lock,
  ShieldCheck,
  DollarSign,
  Coins,
  Wallet,
  Sun,
  Moon,
  Menu,
  PanelLeftClose,
  PanelLeft,
  Megaphone
} from 'lucide-react';
import AnnouncementBanner from './components/AnnouncementBanner';
import Toast from './components/Toast';
import { apiFetch } from './api/client';
import { useApiSession } from './hooks/useApiSession';
import { useSessionRestore } from './hooks/useSessionRestore';
import { useConsoleData } from './hooks/useConsoleData';
import { useLogsData, LOG_PAGE_SIZE } from './hooks/useLogsData';
import { useChannelActions } from './hooks/useChannelActions';
import { useKeyActions } from './hooks/useKeyActions';
import { getConsoleRoute } from './lib/consoleRoute';

// Lazy loaded views and heavy modals for code splitting and fast initial loading.
const QuickStartModal = lazy(() => import('./components/QuickStartModal'));
const LogDetailModal = lazy(() => import('./components/LogDetailModal'));
const CurlExportModal = lazy(() => import('./components/CurlExportModal'));
const ChannelModal = lazy(() => import('./components/ChannelModal'));
const KeyModal = lazy(() => import('./components/KeyModal'));
const LandingPage = lazy(() => import('./components/LandingPage'));
const AuthPage = lazy(() => import('./components/AuthPage'));
const LoginModal = lazy(() => import('./components/LoginModal'));
const CommandPalette = lazy(() => import('./components/CommandPalette'));
const AccountManageModal = lazy(() => import('./components/AccountManageModal'));
const ModelRoutesManager = lazy(() => import('./components/ModelRoutesManager'));
const TraceSnapshotModal = lazy(() => import('./components/TraceSnapshotModal'));

const ServiceStatus = lazy(() => import('./components/ServiceStatus'));
const PricingManager = lazy(() => import('./components/PricingManager'));
const SkillHubView = lazy(() => import('./components/SkillHubView'));
const McpIntegrationView = lazy(() => import('./components/McpIntegrationView'));
const UserManagementView = lazy(() => import('./components/UserManagementView'));
const OrganizationsView = lazy(() => import('./components/OrganizationsView'));
const BudgetsView = lazy(() => import('./components/BudgetsView'));
const CostAnalyticsView = lazy(() => import('./components/CostAnalyticsView'));
const WalletLedgerView = lazy(() => import('./components/WalletLedgerView'));
const WalletManagementView = lazy(() => import('./components/WalletManagementView'));
const ChannelsView = lazy(() => import('./components/ChannelsView'));
const SupplierReconciliationView = lazy(() => import('./components/SupplierReconciliationView'));
const KeysView = lazy(() => import('./components/KeysView'));
const LogsView = lazy(() => import('./components/LogsView'));
const DashboardView = lazy(() => import('./components/DashboardView'));
const MyUsageView = lazy(() => import('./components/MyUsageView'));
const PlaygroundView = lazy(() => import('./components/PlaygroundView'));
const DocsView = lazy(() => import('./components/DocsView'));
const OnboardingModal = lazy(() => import('./components/OnboardingModal'));
const VisualTopologyModal = lazy(() => import('./components/VisualTopologyModal'));
const AnnouncementModal = lazy(() => import('./components/AnnouncementModal'));
const AiCopilot = lazy(() => import('./components/AiCopilot'));

function ViewLoader() {
  return (
    <div className="flex flex-col items-center justify-center p-16 text-[#656B75] dark:text-[#A1A7B0]">
      <div className="w-7 h-7 border-2 border-[#355CFF] border-t-transparent rounded-full animate-spin mb-3"></div>
      <span className="text-xs font-medium tracking-wide">Loading module...</span>
    </div>
  );
}
import { translations, I18nContext } from './i18n';
import { setPublicGatewayUrl } from './config';
import { inferClientModality } from './lib/modelModality';
import useUrlFilters from './hooks/useUrlFilters';
import { useUiPreferences } from './hooks/useUiPreferences';
import { useToast } from './hooks/useToast';
import { useConsoleOverlays } from './hooks/useConsoleOverlays';
import { useConsoleSelections } from './hooks/useConsoleSelections';
import { useConsoleChrome } from './hooks/useConsoleChrome';
import { useConsoleSession } from './hooks/useConsoleSession';

export default function App() {
  // Language, theme and the toast each carry their own logic (persistence, the
  // document class, an auto-hide timer), so they live in their own hooks.
  const { lang, setLang, theme, toggleLang, toggleTheme, t, isZh } =
    useUiPreferences({ translations });
  const { toast, showToast, dismissToast } = useToast();

  const initialRoute = getConsoleRoute(window.location);

  // View mode: 'landing' (公共门户首页), 'console' (工作台), 'status' (对外状态页), 'auth' (认证页)
  // The session state machine. App supplies the UI reaction to each transition
  // (the login dialog and the expiry notice) rather than interleaving it here.
  const {
    viewMode,
    adminToken,
    adminUser,
    authTab,
    restoringSession,
    getSessionGeneration,
    finishSessionRestore,
    setViewMode,
    setAuthTab,
    signIn,
    restoreSession,
    patchUser,
    endSession,
    signOutLocal,
    applyRoute,
  } = useConsoleSession({
    initialRoute,
    onEntered: () => setShowLoginModal(false),
    onEnded: ({ notify }) => {
      setShowLoginModal(true);
      if (notify) {
        showToast(isZh ? '登录凭证已失效，请重新登录' : 'Session expired, please login again', 'warning');
      }
    },
  });

  // Account & Authentication state (Unified token & user based on role)
  const {
    showCommandPalette, setShowCommandPalette,
    showOnboardingModal, setShowOnboardingModal,
    showTopologyModal, setShowTopologyModal,
    showAiCopilot, setShowAiCopilot,
    copilotInitialQuery, setCopilotInitialQuery,
    showLoginModal, setShowLoginModal,
    showAccountModal, setShowAccountModal,
    accountModalTab, setAccountModalTab,
    reset: resetOverlays,
  } = useConsoleOverlays();


  // Global Command Palette shortcut (Cmd+K / Ctrl+K).
  //
  // Escape is deliberately not handled here: open overlays register themselves in
  // the escape layer stack (src/lib/escapeLayer.js) and only the topmost one acts,
  // so one Escape closes the palette without also dismissing a form modal opened
  // underneath it (which raised an unsaved-changes prompt).
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

  // Tear the session down when the gateway rejects the credential. This policy
  // stays in the shell because it is UI behaviour; the request layer only reports
  // that a 401 happened.
  const handleUnauthorized = useCallback(({ token: usedToken }) => {
    // Only a console session holding a credential is torn down here; the landing
    // and auth pages also make unauthenticated calls.
    if (viewMode !== 'console' || !usedToken) return;
    endSession({ notify: true });
  }, [viewMode, endSession]);

  // Bind the shared request layer (src/api/client.js) to this session. It used to
  // be a pair of closures defined in this component (adminFetch/adminJSON) that
  // every view had to receive through props; the props below are kept so existing
  // views retain their contract.
  useApiSession({ token: adminToken, onUnauthorized: handleUnauthorized, getSessionGeneration });

  // Clear every piece of tenant-scoped data so a following session (or React
  // DevTools) cannot observe the previous account's channels, keys or logs.
  const resetWorkspaceState = () => {
    // The console data hook owns its own slices, including stats, prices and
    // pricingGroups, which this function used to leave behind.
    resetConsoleData();
    resetLogsData();
    resetChannelActions();
    resetKeyActions();
    resetSelections();
    resetOverlays();
    // The batch selections and the channel form belong to the channel/key hooks,
    // which do not clear them on logout: they are only reset here.
    setSelectedLogIds([]);
    setSelectedKeyIds([]);
    setSelectedChannelIds([]);
    setEditingChannelId(null);
    setShowChannelModal(false);
    setShowKeyModal(false);
  };

  const handleLogout = async () => {
    try {
      await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'same-origin' });
    } catch (e) {
      // Even if the call fails, drop all local state below.
    }
    signOutLocal();
    resetWorkspaceState();
    showToast(isZh ? '已安全退出账号' : 'Successfully logged out', 'info');
  };


  const {
    activeQuickKey, setActiveQuickKey,
    activeLogDetail, setActiveLogDetail,
    traceSnapshotId, setTraceSnapshotId,
    curlExportCmd, setCurlExportCmd,
    reset: resetSelections,
  } = useConsoleSelections();
  const [dashboardSelectedKey, setDashboardSelectedKey] = useState('');
  const [showAnnouncementModal, setShowAnnouncementModal] = useState(false);
  // Bumped after an announcement is published so the banner refreshes at once
  // instead of waiting for its 60s poll.
  const [announcementRefreshKey, setAnnouncementRefreshKey] = useState(0);
  const {
    sidebarCollapsed,
    mobileMenuOpen, setMobileMenuOpen,
    currentTab, setCurrentTab,
    playModality, setPlayModality,
    closeMobileMenu,
    collapseSidebar,
    expandSidebar,
  } = useConsoleChrome({ initialTab: initialRoute.tab });

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

  // Automatically ensure regular user stays within permitted tabs.
  //
  // 'models' is intentionally absent: the model-routes view is built entirely on
  // admin-only endpoints (/admin/models/routes, /admin/channels), so a regular
  // user landing there saw a page that could only fail with permission errors.
  // 'pricing' stays (GET /admin/pricing is available to any authenticated key
  // owner) but its write actions are hidden for non-admins.
  useEffect(() => {
    if (!restoringSession && adminUser && adminUser.role !== 'admin') {
      const allowedTabs = ['wallet', 'keys', 'playground', 'pricing', 'logs', 'docs'];
      if (!allowedTabs.includes(currentTab)) {
        setCurrentTab('wallet');
      }
    }
  }, [adminUser, currentTab, setCurrentTab, restoringSession]);

  // The console's tenant-scoped data and its loader live in one hook. The loader
  // is aliased to fetchData so the twenty-odd existing call sites — including the
  // onRefresh / fetchData props the views receive — keep working unchanged.
  const {
    stats,
    channels,
    keys,
    models,
    modelRoutes,
    prices,
    pricingGroups,
    groupMultipliers,
    playModel,
    setPlayModel,
    refresh: fetchData,
    reset: resetConsoleData,
  } = useConsoleData({ adminUser });
  // Log filters are mirrored into the URL query string (`logs.*`) so a filtered
  // audit view survives switching modules and can be shared as a link.
  // Every audit-log facet lives here (single source of truth) so fetchLogs can
  // send it to the server. Previously the facets lived in LogsView's own
  // useUrlFilters instance, so App never saw them and filtered only the loaded
  // page in memory — silently hiding matches on other pages.
  const [logFilters, setLogFilters] = useUrlFilters('logs', {
    q: '',
    range: 'all', // 'all' | '1h' | 'today' | '7d' | 'custom'
    start: '',
    end: '',
    session: '',
    correlation_type: 'session_id',
    key_id: '',
    model: 'all',
    channel: 'all',
    status: 'all',
    cache: 'all',
    latency: 'all',
    // Page cursor is URL state as well: refreshing or sharing a link should land
    // on the same page of the audit trail.
    page: '1',
  });
  const logPageNumber = Math.max(1, parseInt(logFilters.page, 10) || 1);
  const logPageSize = LOG_PAGE_SIZE;
  const logFilter = logFilters.q;
  const setLogFilter = (value) => setLogFilters({ q: value });
  const timeRange = logFilters.range;
  const setTimeRange = (value) => setLogFilters({ range: value });
  const customStartTime = logFilters.start;
  const setCustomStartTime = (value) => setLogFilters({ start: value });
  const customEndTime = logFilters.end;
  const setCustomEndTime = (value) => setLogFilters({ end: value });
  const sessionFilter = logFilters.session;
  const setSessionFilter = (value, type = 'session_id') => setLogFilters({ session: value, correlation_type: type });
  // The audit-log data and its loaders live in one hook. The filters stay here:
  // they are URL-bound and rendered by the log toolbar. The hook keeps its own
  // mirror of them, so the ref that used to live at this point is gone.
  const {
    logs,
    recentLogs,
    fetchRecentLogs,
    logTotal,
    logOffset,
    logLoading,
    logError,
    logFacets,
    fetchLogs,
    fetchAllLogsForExport,
    goToOffset,
    resetOffset,
    selectedLogIds,
    setSelectedLogIds,
    handleBatchDeleteLogs,
    handleClearLogs,
    handleDeleteSingleLog,
    reset: resetLogsData,
  } = useLogsData({ filters: logFilters, showToast, isZh });

  // The channel and key domains own their own form state, selections and actions.
  // The hooks take only what they do not own: the toast, the language, the loaded
  // lists and the console refresh.
  const channelActions = useChannelActions({
    channels,
    showToast,
    isZh,
    refresh: fetchData,
  });
  const keyActions = useKeyActions({
    keys,
    adminUser,
    showToast,
    isZh,
    refresh: fetchData,
  });

  // The console JSX below is unchanged: it reads the same names it always did, now
  // sourced from the domain hooks.
  const {
    showChannelModal, editingChannelId, newChannel, submittingChannel,
    probing, probeAlert, testingId, batchTesting, channelLatencies, selectedChannelIds,
    setNewChannel, setShowChannelModal, setEditingChannelId, setSelectedChannelIds, setProbeAlert,
    handleProbeChannel, applyPreset, toggleProtocol,
    handleBatchPing, handleBatchTest,
    handleCreateChannel, handleEditChannel, handleDeleteChannel, handleTestChannel,
    handleBatchDeleteChannels, handleBatchStatusChannels,
    reset: resetChannelActions,
  } = channelActions;

  const {
    showKeyModal, newKey, submittingKey, selectedKeyIds, copiedKey, createdKey,
    setNewKey, setShowKeyModal, setSelectedKeyIds,
    handleOpenCreateKey, handleCreateKey, handleUpdateKeyGroup,
    handleDeleteKey, handleToggleKeyStatus,
    handleBatchDeleteKeys, handleBatchStatusKeys, copyToClipboard,
    reset: resetKeyActions,
  } = keyActions;


  // Fetch EVERY row matching the active log filters (used by the CSV export).
  // Bounded so a pathological filter cannot pull an unbounded result set.

  useEffect(() => {
    if (restoringSession || !adminToken || viewMode !== 'console') return;
    fetchData();
    fetchRecentLogs();
    const interval = setInterval(() => {
      fetchData();
      // The dashboard shows raw recent streaming logs, completely isolated from
      // any search or facet filters applied on the logs tab.
      if (currentTab === 'dashboard') {
        fetchRecentLogs();
      }
    }, 8000);
    return () => clearInterval(interval);
  }, [currentTab, adminToken, viewMode, fetchData, fetchRecentLogs, restoringSession]);

  // One owner for filter-driven loads on the logs tab. Debouncing also prevents
  // a click and several effects from issuing competing copies of the same query.
  //
  // The page cursor is URL state (`logs.page`). A filter change resets to page 1,
  // paging itself is handled by handleLogPageChange (which fetches directly), and
  // returning to the tab keeps the page the link points at — so the effect stays
  // silent when only the cursor moved.
  const logFilterSignatureRef = useRef('');
  const logsTabActiveRef = useRef(false);
  useEffect(() => {
    if (currentTab !== 'logs' || viewMode !== 'console') {
      logsTabActiveRef.current = false;
      return undefined;
    }
    if (restoringSession || !adminToken) return undefined;

    const signature = JSON.stringify([
      logFilters.q, logFilters.range, logFilters.start, logFilters.end,
      logFilters.session, logFilters.correlation_type, logFilters.key_id,
      logFilters.model, logFilters.channel, logFilters.status, logFilters.cache, logFilters.latency,
    ]);
    const enteringTab = !logsTabActiveRef.current;
    logsTabActiveRef.current = true;
    const firstRun = logFilterSignatureRef.current === '';
    const signatureChanged = firstRun || logFilterSignatureRef.current !== signature;
    logFilterSignatureRef.current = signature;
    if (!enteringTab && !signatureChanged) return undefined;

    let targetPage;
    if (firstRun) {
      // Deep link / restored URL: honour the page it names.
      targetPage = logPageNumber;
    } else if (signatureChanged) {
      // A different result set makes the old offset meaningless.
      targetPage = 1;
    } else {
      // Returning to the tab: keep the page currently in the URL.
      targetPage = logPageNumber;
    }
    const targetOffset = (targetPage - 1) * logPageSize;
    const timer = setTimeout(() => {
      if (signatureChanged && logPageNumber !== 1) setLogFilters({ page: '1' });
      goToOffset(targetOffset);
      fetchLogs({ offset: targetOffset });
    }, 350);
    return () => clearTimeout(timer);
  }, [logFilters, currentTab, viewMode, fetchLogs, goToOffset, restoringSession, adminToken, logPageNumber, logPageSize, setLogFilters]);

  // Server-side pagination for audit logs (offset-based, page size above).
  const handleLogPageChange = (newOffset) => {
    if (newOffset < 0) return;
    const nextPage = Math.floor(newOffset / logPageSize) + 1;
    goToOffset(newOffset);
    fetchLogs({ offset: newOffset });
    if (String(nextPage) !== logFilters.page) setLogFilters({ page: String(nextPage) });
  };

  // Batch Log Operations



  // Batch Key Operations


  // Batch Channel Operations

  // `overrideIds` lets a single row act on itself (ChannelsView passes
  // [ch.id]); without it the batch handler only looked at the checkbox
  // selection, so a single-row toggle either did nothing or changed a
  // different, unrelated set of channels.

  // Handle Key Modal Opening. Credentials are always generated server-side
  // with crypto/rand; the browser never invents an authentication secret.

  // Handle Key Creation


  // The server already applied keyword/facet filters, so the page is exactly the
  // result set. Re-filtering here would be wrong (it would hide rows the server
  // matched on fields the client does not check, e.g. error_message).
  const filteredLogs = logs;

  if (restoringSession && viewMode === 'console') return <ViewLoader />;

  if (viewMode === 'landing') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white">
          <Toast toast={toast} onClose={dismissToast} />
          <Suspense fallback={<ViewLoader />}>
            <LandingPage
              isLoggedIn={!!adminToken || !!adminUser}
              adminUser={adminUser}
              lang={lang}
              setLang={setLang}
              toggleLang={toggleLang}
              t={t}
              onOpenLogin={() => {
                setAuthTab('login');
                setViewMode('auth');
              }}
              onEnterConsole={() => {
                if (adminToken) {
                  setViewMode('console');
                } else {
                  setAuthTab('login');
                  setViewMode('auth');
                }
              }}
              onViewDocs={() => {
                const el = document.getElementById('docs');
                if (el) {
                  const headerOffset = 70;
                  const elementPosition = el.getBoundingClientRect().top;
                  const offsetPosition = elementPosition + window.pageYOffset - headerOffset;
                  window.scrollTo({
                    top: offsetPosition,
                    behavior: 'smooth'
                  });
                }
              }}
              onViewStatus={() => setViewMode('status')}
              stats={stats}
              channelsCount={channels.length}
              modelsCount={models.length}
            />
            <LoginModal
              isOpen={showLoginModal}
              onClose={() => setShowLoginModal(false)}
              lang={lang}
              t={t}
              onLoginSuccess={(token, user) => {
                resetWorkspaceState();
                signIn(user, { token, tab: user.role === 'admin' ? 'dashboard' : 'wallet' });
                showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
                fetchData(token);
                fetchLogs({}, token);
              }}
            />
          </Suspense>
        </div>
      </I18nContext.Provider>
    );
  }

  if (viewMode === 'auth') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white">
          <Toast toast={toast} onClose={dismissToast} />
          <Suspense fallback={<ViewLoader />}>
            <AuthPage
              initialTab={authTab}
              lang={lang}
              setLang={setLang}
              toggleLang={toggleLang}
              t={t}
              onLoginSuccess={(token, user) => {
                // Every login path must start from a clean workspace: without this
                // a session begun here inherited the previous account's keys, logs,
                // stats, selections and log filters.
                resetWorkspaceState();
                signIn(user, { token, tab: user.role === 'admin' ? 'dashboard' : 'wallet' });
                showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
                fetchData(token);
                fetchLogs({}, token);
              }}
              onBackHome={() => setViewMode('landing')}
            />
          </Suspense>
        </div>
      </I18nContext.Provider>
    );
  }

  if (viewMode === 'status') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white">
          <Toast toast={toast} onClose={dismissToast} />
          <Suspense fallback={<ViewLoader />}>
            <ServiceStatus
              isStandalone={true}
              onBackHome={() => setViewMode('landing')}
              onEnterConsole={() => {
                if (adminToken) {
                  setViewMode('console');
                } else {
                  setAuthTab('login');
                  setViewMode('auth');
                }
              }}
              onOpenLogin={() => {
                setAuthTab('login');
                setViewMode('auth');
              }}
              isLoggedIn={!!adminToken || !!adminUser}
              lang={lang}
              setLang={setLang}
              t={t}
              modelRoutes={modelRoutes}
              channels={channels}
              onRefresh={fetchData}
              probeLatencies={channelLatencies}
              adminFetch={apiFetch}
              showToast={showToast}
              onBatchProbe={handleBatchPing}
              batchTesting={batchTesting}
            />
            <LoginModal
              isOpen={showLoginModal}
              onClose={() => setShowLoginModal(false)}
              lang={lang}
              t={t}
              onLoginSuccess={(token, user) => {
                resetWorkspaceState();
                signIn(user, { token, tab: user.role === 'admin' ? 'dashboard' : 'wallet' });
                showToast(isZh ? `欢迎回来，${user.username}！` : `Welcome back, ${user.username}!`, 'success');
                fetchData(token);
                fetchLogs({}, token);
              }}
            />
          </Suspense>
        </div>
      </I18nContext.Provider>
    );
  }

  return (
    <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
      <div className="flex h-screen overflow-hidden bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white transition-colors duration-150">
      {/* Toast Notification */}
      <Toast toast={toast} onClose={dismissToast} />

      {/* Quick Start Modal */}
      {activeQuickKey && (
        <QuickStartModal
          apiKey={activeQuickKey}
          models={models}
          onClose={() => setActiveQuickKey(null)}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* Log Detail Inspector Modal */}
      {activeLogDetail && (
        <LogDetailModal
          log={activeLogDetail}
          prices={prices}
          onClose={() => setActiveLogDetail(null)}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          onFilterBySession={(sid) => {
            // Only update the filter state: the logs-tab effect owns the fetch
            // and resets the page cursor. Fetching here as well issued the same
            // query twice on every click.
            setSessionFilter(sid, 'trace_id');
            setCurrentTab('logs');
            showToast(isZh ? `已按 Trace ID: ${sid} 筛选` : `Filtered by Trace ID: ${sid}`, 'info');
          }}
          onOpenTrace={adminUser?.role === 'admin' ? ((value, field = 'trace_id') => setTraceSnapshotId({ value, field })) : undefined}
          onDeleteLog={adminUser?.role === 'admin' ? handleDeleteSingleLog : undefined}
          onNavigate={(targetTab) => {
            setCurrentTab(targetTab);
            setViewMode('console');
            setActiveLogDetail(null);
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* cURL Export Modal */}
      {curlExportCmd && (
        <CurlExportModal
          curlCmd={curlExportCmd}
          onClose={() => setCurlExportCmd('')}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          lang={lang}
          t={t}
        />
      )}

      {/* Mobile Drawer Backdrop */}
      {mobileMenuOpen && (
        <div
          onClick={closeMobileMenu}
          className="fixed inset-0 bg-slate-900/60 z-30 md:hidden backdrop-blur-sm animate-in fade-in"
        />
      )}

      {/* Sidebar */}
      <aside
        className={`bg-white dark:bg-[#191B1F] border-r border-[#E4E6E9] dark:border-[#2C3038] flex flex-col shrink-0 transition-all duration-200 z-30 ${
          sidebarCollapsed ? 'w-16' : 'w-56'
        } ${
          mobileMenuOpen ? 'fixed inset-y-0 left-0 w-56 shadow-2xl flex' : 'hidden md:flex'
        }`}
      >
        <div className="h-16 px-4 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
          <div
            onClick={() => setViewMode('landing')}
            className={`flex items-center ${sidebarCollapsed ? 'justify-center w-full' : 'space-x-2.5'} cursor-pointer group transition hover:opacity-90`}
            title={lang === 'zh' ? '点击返回门户首页' : 'Return to product portal'}
          >
            <div className="w-8 h-8 rounded-lg bg-[#355CFF] flex items-center justify-center text-white shrink-0 shadow-xs">
              <Zap className="w-4 h-4 fill-white text-white" />
            </div>
            {!sidebarCollapsed && (
              <div className="min-w-0">
                <h1 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] tracking-tight leading-none">
                  Airoute
                </h1>
                <span className="text-[10px] text-[#656B75] dark:text-[#A1A7B0] font-mono tracking-tight block mt-1">
                  AI Gateway
                </span>
              </div>
            )}
          </div>

          {!sidebarCollapsed && (
            <button
              onClick={collapseSidebar}
              className="p-1 rounded-md text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition-colors cursor-pointer"
              title={isZh ? '收起侧边栏' : 'Collapse sidebar'}
            >
              <PanelLeftClose className="w-4 h-4" />
            </button>
          )}
        </div>

        {sidebarCollapsed && (
          <div className="p-2 border-b border-[#E4E6E9] dark:border-[#2C3038] flex justify-center">
            <button
              onClick={expandSidebar}
              className="p-1.5 rounded-md text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition-colors cursor-pointer"
              title={isZh ? '展开侧边栏' : 'Expand sidebar'}
            >
              <PanelLeft className="w-4 h-4" />
            </button>
          </div>
        )}

        {/* Sidebar Nav */}
        {(() => {
          const navBtnClass = (tab) => `w-full flex items-center ${sidebarCollapsed ? 'justify-center px-2' : 'justify-between px-2.5'} py-2 rounded-lg text-xs font-medium transition-colors duration-150 cursor-pointer ${
            currentTab === tab
              ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] font-semibold'
              : 'text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
          }`;
          const navIconColor = (tab) => (currentTab === tab ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]');

          return (
            <nav className="flex-1 p-2 space-y-4 overflow-y-auto">
              {adminUser?.role === 'admin' ? (
                /* Admin Grouped Enterprise Navigation */
                <div className="space-y-4">
                  {/* Group 1: 监控与分析 */}
                  <div>
                    {!sidebarCollapsed && (
                      <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                        {isZh ? '监控与分析' : 'Monitoring'}
                      </span>
                    )}
                    <div className="space-y-0.5">
                      <button
                        onClick={() => setCurrentTab('dashboard')}
                        title={t.navDashboard}
                        className={navBtnClass('dashboard')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <BarChart3 className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('dashboard')}`} />
                          {!sidebarCollapsed && <span>{t.navDashboard}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('logs')}
                        title={t.navLogs}
                        className={navBtnClass('logs')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <History className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('logs')}`} />
                          {!sidebarCollapsed && <span>{t.navLogs}</span>}
                        </div>
                        {!sidebarCollapsed && logs.length > 0 && (
                          <span className={`text-xs font-mono ${currentTab === 'logs' ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                            {logs.length}
                          </span>
                        )}
                      </button>
                    </div>
                  </div>

                  {/* Group 2: 模型网关与调度 */}
                  <div>
                    {!sidebarCollapsed && (
                      <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                        {isZh ? '模型网关与调度' : 'Gateway'}
                      </span>
                    )}
                    <div className="space-y-0.5">
                      <button
                        onClick={() => setCurrentTab('models')}
                        title={t.navModels}
                        className={navBtnClass('models')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Cpu className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('models')}`} />
                          {!sidebarCollapsed && <span>{t.navModels}</span>}
                        </div>
                        {!sidebarCollapsed && models.length > 0 && (
                          <span className={`text-xs font-mono ${currentTab === 'models' ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                            {models.length}
                          </span>
                        )}
                      </button>

                      <button
                        onClick={() => setCurrentTab('channels')}
                        title={t.navChannels}
                        className={navBtnClass('channels')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Server className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('channels')}`} />
                          {!sidebarCollapsed && <span>{t.navChannels}</span>}
                        </div>
                        {!sidebarCollapsed && channels.length > 0 && (
                          <span className={`text-xs font-mono ${currentTab === 'channels' ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                            {channels.length}
                          </span>
                        )}
                      </button>

                      <button
                        onClick={() => setCurrentTab('reconciliation')}
                        title={t.navReconciliation}
                        className={navBtnClass('reconciliation')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Coins className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('reconciliation')}`} />
                          {!sidebarCollapsed && <span>{t.navReconciliation}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('pricing')}
                        title={t.navPricing}
                        className={navBtnClass('pricing')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <DollarSign className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('pricing')}`} />
                          {!sidebarCollapsed && <span>{t.navPricing}</span>}
                        </div>
                      </button>
                    </div>
                  </div>

                  {/* Group 3: 访问控制与资产 */}
                  <div>
                    {!sidebarCollapsed && (
                      <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                        {isZh ? '安全与资产' : 'Security'}
                      </span>
                    )}
                    <div className="space-y-0.5">
                      <button
                        onClick={() => setCurrentTab('keys')}
                        title={t.navKeys}
                        className={navBtnClass('keys')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Key className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('keys')}`} />
                          {!sidebarCollapsed && <span>{t.navKeys}</span>}
                        </div>
                        {!sidebarCollapsed && keys.length > 0 && (
                          <span className={`text-xs font-mono ${currentTab === 'keys' ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                            {keys.length}
                          </span>
                        )}
                      </button>

                      <button
                        onClick={() => setCurrentTab('users')}
                        title={t.navUsers || (isZh ? '用户管理' : 'User Management')}
                        className={navBtnClass('users')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <ShieldCheck className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('users')}`} />
                          {!sidebarCollapsed && <span>{t.navUsers || (isZh ? '用户管理' : 'Users')}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('orgs')}
                        title={isZh ? '组织与成本中心' : 'Organizations'}
                        className={navBtnClass('orgs')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Building2 className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('orgs')}`} />
                          {!sidebarCollapsed && <span>{isZh ? '组织与成本中心' : 'Organizations'}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('budgets')}
                        title={isZh ? '预算与告警' : 'Budgets & Alerts'}
                        className={navBtnClass('budgets')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Gauge className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('budgets')}`} />
                          {!sidebarCollapsed && <span>{isZh ? '预算与告警' : 'Budgets & Alerts'}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('costs')}
                        title={isZh ? '成本与毛利' : 'Cost & Margin'}
                        className={navBtnClass('costs')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Calculator className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('costs')}`} />
                          {!sidebarCollapsed && <span>{isZh ? '成本与毛利' : 'Cost & Margin'}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('ledger')}
                        title={isZh ? '钱包资金流水' : 'Wallet Ledger'}
                        className={navBtnClass('ledger')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <ReceiptText className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('ledger')}`} />
                          {!sidebarCollapsed && <span>{isZh ? '钱包资金流水' : 'Wallet Ledger'}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('wallet')}
                        title={isZh ? '卡密与充值' : 'Wallet & Credits'}
                        className={navBtnClass('wallet')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Wallet className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('wallet')}`} />
                          {!sidebarCollapsed && <span>{isZh ? '卡密与充值' : 'Wallet & Credits'}</span>}
                        </div>
                      </button>
                    </div>
                  </div>

                  {/* Group 4: 演练与协议生态 */}
                  <div>
                    {!sidebarCollapsed && (
                      <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                        {isZh ? '演练与生态' : 'Ecosystem'}
                      </span>
                    )}
                    <div className="space-y-0.5">
                      <button
                        onClick={() => setCurrentTab('playground')}
                        title={t.navPlayground}
                        className={navBtnClass('playground')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Terminal className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('playground')}`} />
                          {!sidebarCollapsed && <span>{t.navPlayground}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('skills')}
                        title={t.navSkills || 'Skill Hub'}
                        className={navBtnClass('skills')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Sparkles className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('skills')}`} />
                          {!sidebarCollapsed && <span>{t.navSkills || 'Skill Hub'}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('mcp')}
                        title={t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub')}
                        className={navBtnClass('mcp')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Cpu className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('mcp')}`} />
                          {!sidebarCollapsed && <span>{t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub')}</span>}
                        </div>
                      </button>
                    </div>
                  </div>
                </div>
              ) : (
                /* Regular User Navigation */
                <div className="space-y-4">
                  <div>
                    {!sidebarCollapsed && (
                      <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                        {isZh ? '资产与凭据' : 'Assets'}
                      </span>
                    )}
                    <div className="space-y-0.5">
                      <button
                        onClick={() => setCurrentTab('dashboard')}
                        title={t.navDashboard}
                        className={navBtnClass('dashboard')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <BarChart3 className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('dashboard')}`} />
                          {!sidebarCollapsed && <span>{t.navDashboard}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('wallet')}
                        title={t.navWallet || (isZh ? '我的钱包' : 'My Wallet')}
                        className={navBtnClass('wallet')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Wallet className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('wallet')}`} />
                          {!sidebarCollapsed && <span>{t.navWallet || (isZh ? '我的钱包' : 'My Wallet')}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('keys')}
                        title={t.navKeys}
                        className={navBtnClass('keys')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Key className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('keys')}`} />
                          {!sidebarCollapsed && <span>{t.navKeys}</span>}
                        </div>
                        {!sidebarCollapsed && keys.length > 0 && (
                          <span className={`text-xs font-mono ${currentTab === 'keys' ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}>
                            {keys.length}
                          </span>
                        )}
                      </button>
                    </div>
                  </div>

                  <div>
                    {!sidebarCollapsed && (
                      <span className="px-2.5 text-[10px] font-semibold tracking-wider text-[#656B75] dark:text-[#A1A7B0] uppercase block mb-1">
                        {isZh ? '演练与使用' : 'Usage'}
                      </span>
                    )}
                    <div className="space-y-0.5">
                      <button
                        onClick={() => setCurrentTab('playground')}
                        title={t.navPlayground}
                        className={navBtnClass('playground')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <Terminal className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('playground')}`} />
                          {!sidebarCollapsed && <span>{t.navPlayground}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('pricing')}
                        title={t.navPricing}
                        className={navBtnClass('pricing')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <DollarSign className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('pricing')}`} />
                          {!sidebarCollapsed && <span>{t.navPricing}</span>}
                        </div>
                      </button>

                      <button
                        onClick={() => setCurrentTab('logs')}
                        title={t.navLogs}
                        className={navBtnClass('logs')}
                      >
                        <div className="flex items-center space-x-2.5">
                          <History className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('logs')}`} />
                          {!sidebarCollapsed && <span>{t.navLogs}</span>}
                        </div>
                      </button>
                    </div>
                  </div>
                </div>
              )}

              <div className="pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
                <button
                  onClick={() => setCurrentTab('docs')}
                  className={navBtnClass('docs')}
                  title={isZh ? '查阅开发文档' : 'Documentation'}
                >
                  <div className="flex items-center space-x-2.5">
                    <BookOpen className={`w-4 h-4 shrink-0 transition-colors duration-150 ${navIconColor('docs')}`} />
                    {!sidebarCollapsed && <span>{t.navViewDocs || (isZh ? '开发文档' : 'Docs')}</span>}
                  </div>
                </button>
              </div>
            </nav>
          );
        })()}

        {/* Sidebar bottom status */}
        <div className="p-3 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs">
          <button
            type="button"
            onClick={() => setViewMode('status')}
            className={`w-full flex items-center ${sidebarCollapsed ? 'justify-center' : 'space-x-2'} text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition-colors cursor-pointer`}
            title={isZh ? '查看公开服务状态' : 'Public Status'}
          >
            <span className="w-2 h-2 rounded-full bg-emerald-500 shrink-0"></span>
            {!sidebarCollapsed && <span className="text-[11px] font-mono">{t.statusOperationalBadge}</span>}
          </button>
        </div>
      </aside>

      {/* Main Container */}
      <main className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        {/* Top Header */}
        <header className="h-16 bg-[#F7F7F5]/90 dark:bg-[#111214]/90 backdrop-blur-md border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between px-4 sm:px-6 lg:px-8 xl:px-10 sticky top-0 z-20">
          <div className="flex items-center space-x-3">
            {/* Mobile Hamburger Toggle */}
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="md:hidden p-2 rounded-lg text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] transition-colors cursor-pointer"
              title={isZh ? '打开导航栏' : 'Toggle navigation menu'}
            >
              <Menu className="w-5 h-5" />
            </button>

            {/* Clean Breadcrumb */}
            <div className="flex items-center space-x-2 text-xs">
              <span className="text-[#656B75] dark:text-[#A1A7B0] font-normal">
                {adminUser?.role === 'admin'
                  ? ['dashboard', 'logs'].includes(currentTab)
                    ? (isZh ? '监控与分析' : 'Monitoring')
                    : ['models', 'channels', 'reconciliation', 'pricing'].includes(currentTab)
                    ? (isZh ? '模型网关与调度' : 'Gateway')
                    : ['keys', 'users', 'wallet'].includes(currentTab)
                    ? (isZh ? '安全与资产' : 'Security')
                    : (isZh ? '演练与生态' : 'Ecosystem')
                  : ['wallet', 'keys'].includes(currentTab)
                  ? (isZh ? '资产与凭据' : 'Assets')
                  : (isZh ? '演练与使用' : 'Usage')}
              </span>
              <span className="text-[#E4E6E9] dark:text-[#2C3038]">/</span>
              <span className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {currentTab === 'dashboard' && t.navDashboard}
                {currentTab === 'models' && t.navModels}
                {currentTab === 'pricing' && t.navPricing}
                {currentTab === 'channels' && t.navChannels}
                {currentTab === 'reconciliation' && (t.navReconciliation || (isZh ? '供应商对账' : 'Reconciliation'))}
                {currentTab === 'keys' && t.navKeys}
                {currentTab === 'wallet' && (adminUser?.role === 'admin' ? (isZh ? '卡密与充值' : 'Wallet & Credits') : (t.navWallet || (isZh ? '我的钱包' : 'My Wallet')))}
                {currentTab === 'logs' && t.navLogs}
                {currentTab === 'skills' && (t.navSkills || 'Skill Hub')}
                {currentTab === 'mcp' && (t.navMcp || (isZh ? 'MCP 广场' : 'MCP Hub'))}
                {currentTab === 'users' && (t.navUsers || (isZh ? '用户管理' : 'User Management'))}
                {currentTab === 'orgs' && (isZh ? '组织与成本中心' : 'Organizations')}
                {currentTab === 'budgets' && (isZh ? '预算与告警' : 'Budgets & Alerts')}
                {currentTab === 'ledger' && (isZh ? '钱包资金流水' : 'Wallet Ledger')}
                {currentTab === 'costs' && (isZh ? '成本与毛利' : 'Cost & Margin')}
                {currentTab === 'playground' && t.navPlayground}
                {currentTab === 'docs' && t.navDocs}
              </span>
            </div>
          </div>

          <div className="flex items-center space-x-2.5">
            {/* Command Palette Trigger */}
            <button
              onClick={() => setShowCommandPalette(true)}
              className="hidden sm:flex items-center space-x-2 px-3 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition-colors cursor-pointer shadow-2xs"
              title={isZh ? '全局快捷检索与指令面板 (⌘K)' : 'Command Palette (⌘K)'}
            >
              <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
              <span>{isZh ? '快速搜索...' : 'Search...'}</span>
              <kbd className="text-[10px] bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] rounded px-1.5 py-0.5 font-mono text-[#656B75] dark:text-[#A1A7B0]">
                ⌘K
              </kbd>
            </button>

            {/* Theme Toggle Button */}
            <button
              onClick={toggleTheme}
              className="p-2 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] transition-colors border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs cursor-pointer"
              title={theme === 'dark' ? (isZh ? '切换为浅色模式' : 'Switch to Light Mode') : (isZh ? '切换为深色模式' : 'Switch to Dark Mode')}
            >
              {theme === 'dark' ? <Sun className="w-3.5 h-3.5 text-amber-400" /> : <Moon className="w-3.5 h-3.5 text-[#656B75]" />}
            </button>

            {/* Language Switcher Pill */}
            <button
              onClick={toggleLang}
              className="text-xs px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] font-medium flex items-center space-x-1.5 transition-colors border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs cursor-pointer"
              title={lang === 'zh' ? 'Switch to English' : '切换为简体中文'}
            >
              <Globe className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
              <span>{t.langToggle}</span>
            </button>

            {/* Admin Announcement Trigger */}
            {adminUser?.role === 'admin' && (
              <button
                onClick={() => setShowAnnouncementModal(true)}
                className="hidden sm:flex items-center space-x-1.5 text-xs text-[#202124] dark:text-[#F1F2F4] font-medium px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs transition-colors cursor-pointer"
                title={isZh ? '发布或编辑系统级全员公告' : 'Publish or edit system announcement'}
              >
                <Megaphone className="w-3.5 h-3.5 text-[#355CFF]" />
                <span>{isZh ? '发布公告' : 'Announcement'}</span>
              </button>
            )}

            {/* Account info & actions */}
            <div className="flex items-center space-x-2 pl-2 border-l border-[#E4E6E9] dark:border-[#2C3038]">
              <button
                onClick={() => { setAccountModalTab('profile'); setShowAccountModal(true); }}
                className="flex items-center space-x-1.5 text-xs text-[#202124] dark:text-[#F1F2F4] font-medium px-2.5 py-1.5 rounded-lg bg-white dark:bg-[#191B1F] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xs transition-colors cursor-pointer"
                title={isZh ? '账号设置' : 'Account Settings'}
              >
                <User className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
                <span>{adminUser?.username || 'admin'}</span>
              </button>

              <button
                onClick={handleLogout}
                className="p-1.5 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-950/40 text-[#656B75] hover:text-rose-600 dark:hover:text-rose-400 transition-colors cursor-pointer"
                title={isZh ? '退出登录' : 'Logout'}
              >
                <LogOut className="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </header>

        {/* Global System Announcement Banner */}
        <AnnouncementBanner lang={lang} adminFetch={apiFetch} refreshKey={announcementRefreshKey} />

        <div className="px-4 sm:px-6 lg:px-8 xl:px-10 py-7 max-w-[1536px] w-full mx-auto space-y-7">
          {/* Default Password Security Warning Banner for Admins */}
          {adminUser && adminUser.role === 'admin' && adminUser.is_default_password && (
            <div className="bg-rose-50/80 dark:bg-rose-950/30 border border-rose-200/80 dark:border-rose-900/40 rounded-2xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-sm animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-xl bg-rose-100 dark:bg-rose-900/50 text-rose-700 dark:text-rose-300 flex items-center justify-center shrink-0">
                  <ShieldAlert className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-semibold text-rose-900 dark:text-rose-200">
                    {isZh ? '安全风险预警：当前管理员账号使用默认初始密码' : 'Security Alert: Admin account is using default initial password'}
                  </h4>
                  <p className="text-xs text-rose-700 dark:text-rose-300/80 mt-0.5">
                    {isZh ? '为保障网关控制台与算力资产安全，强烈建议您立即修改初始密码，避免未授权访问风险。' : 'To protect gateway security and compute assets, please change default password immediately.'}
                  </p>
                </div>
              </div>
              <button
                onClick={() => { setAccountModalTab('password'); setShowAccountModal(true); }}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-700 text-white rounded-lg text-xs font-medium transition shrink-0 cursor-pointer flex items-center space-x-1.5"
              >
                <Lock className="w-3.5 h-3.5" />
                <span>{isZh ? '立即修改密码' : 'Change Password'}</span>
              </button>
            </div>
          )}

          {/* Low Balance Warning Banner for Regular Users */}
          {adminUser && adminUser.role !== 'admin' && Number(adminUser.balance || 0) < 5 && (
            <div className="bg-amber-50/80 dark:bg-amber-950/30 border border-amber-200/80 dark:border-amber-900/40 rounded-2xl p-5 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 shadow-sm animate-in fade-in">
              <div className="flex items-center space-x-3.5">
                <div className="w-10 h-10 rounded-xl bg-amber-100 dark:bg-amber-900/50 text-amber-700 dark:text-amber-300 flex items-center justify-center shrink-0">
                  <AlertCircle className="w-5 h-5" />
                </div>
                <div>
                  <h4 className="text-sm font-semibold text-amber-900 dark:text-amber-200">
                    {Number(adminUser.balance || 0) <= 0 ? (isZh ? '钱包额度已耗尽 (余额不足)' : 'Wallet Balance Depleted') : (isZh ? '钱包余额偏低预警' : 'Low Wallet Balance Alert')}
                  </h4>
                  <p className="text-xs text-amber-700 dark:text-amber-300/80 mt-0.5">
                    {isZh 
                      ? `当前可用额度为 ¥${Number(adminUser.balance || 0).toFixed(4)}。为避免生产 API 接口调用中断，请及时充值或兑换卡密。`
                      : `Current balance is ¥${Number(adminUser.balance || 0).toFixed(4)}. Please top up or redeem cards to prevent API interruption.`}
                  </p>
                </div>
              </div>
              <div className="flex items-center space-x-2 shrink-0">
                <button
                  onClick={() => setCurrentTab('wallet')}
                  className="ui-btn-primary cursor-pointer"
                >
                  {isZh ? '前往充值 / 兑换卡密 →' : 'Top up / Redeem Card →'}
                </button>
              </div>
            </div>
          )}

          <Suspense fallback={<ViewLoader />}>
            {/* 1a. SELF-SERVICE DASHBOARD (regular users) */}
            {currentTab === 'dashboard' && adminUser?.role !== 'admin' && (
              <MyUsageView adminFetch={apiFetch} showToast={showToast} lang={lang} />
            )}

            {/* 1. ENTERPRISE BUSINESS & TELEMETRY DASHBOARD */}
            {currentTab === 'dashboard' && adminUser?.role === 'admin' && (
              <DashboardView
                stats={stats}
                onNavigateToErrors={() => {
                  setLogFilters({ q: '', range: 'all', start: '', end: '', session: '', model: 'all', channel: 'all', status: 'error', cache: 'all', latency: 'all' });
                  resetOffset();
                  setCurrentTab('logs');
                }}
                channels={channels}
                logs={recentLogs.length > 0 ? recentLogs : logs}
                channelLatencies={channelLatencies}
                testingId={testingId}
                batchTesting={batchTesting}
                handleBatchTest={handleBatchTest}
                handleTestChannel={handleTestChannel}
                fetchData={fetchData}
                fetchLogs={fetchRecentLogs}
                showToast={showToast}
                setCurrentTab={setCurrentTab}
                setShowChannelModal={setShowChannelModal}
                setActiveLogDetail={setActiveLogDetail}
                adminUser={adminUser}
                adminFetch={apiFetch}
                keys={keys}
                selectedKey={dashboardSelectedKey}
                onSelectKey={setDashboardSelectedKey}
                onOpenTopology={() => setShowTopologyModal(true)}
                lang={lang}
                t={t}
              />
            )}

            {/* 1.5. MODEL ROUTES TAB */}
            {currentTab === 'models' && (
              <ModelRoutesManager
                isAdmin={adminUser?.role === 'admin'}
                adminFetch={apiFetch}
                showToast={showToast}
                lang={lang}
                t={t}
                setCurrentTab={setCurrentTab}
                onNavigateToPlayground={(m, modality) => {
                  setPlayModel(m);
                  if (modality && modality !== 'chat') {
                    setPlayModality(modality);
                  } else {
                    setPlayModality('chat');
                  }
                  setCurrentTab('playground');
                }}
              />
            )}

            {/* 1.6. MODEL PRICING & RATES TAB */}
            {currentTab === 'pricing' && (
              <PricingManager
                adminFetch={apiFetch}
                showToast={showToast}
                stats={stats}
                isAdmin={adminUser?.role === 'admin'}
                lang={lang}
                t={t}
                modelRoutes={modelRoutes}
                setCurrentTab={setCurrentTab}
                groupMultipliers={groupMultipliers}
                onUpdateGroupMultipliers={fetchData}
              />
            )}

            {/* 2. CHANNELS / PROVIDERS TAB */}
            {currentTab === 'channels' && (
              <ChannelsView
                channels={channels}
                batchTesting={batchTesting}
                handleBatchPing={handleBatchPing}
                setEditingChannelId={setEditingChannelId}
                setNewChannel={setNewChannel}
                setProbeAlert={setProbeAlert}
                setShowChannelModal={setShowChannelModal}
                selectedChannelIds={selectedChannelIds}
                setSelectedChannelIds={setSelectedChannelIds}
                handleBatchStatusChannels={handleBatchStatusChannels}
                handleBatchDeleteChannels={handleBatchDeleteChannels}
                channelLatencies={channelLatencies}
                copyToClipboard={copyToClipboard}
                handleTestChannel={handleTestChannel}
                testingId={testingId}
                handleEditChannel={handleEditChannel}
                handleDeleteChannel={handleDeleteChannel}
                onNavigateToReconciliation={() => setCurrentTab('reconciliation')}
                lang={lang}
                t={t}
              />
            )}

            {/* 2.5. SUPPLIER RECONCILIATION TAB */}
            {currentTab === 'reconciliation' && (
              <SupplierReconciliationView
                adminFetch={apiFetch}
                showToast={showToast}
                onNavigateToLogs={(channelName) => {
                  setLogFilter(channelName);
                  setCurrentTab('logs');
                }}
                lang={lang}
                t={t}
              />
            )}

            {/* 3. API KEYS TAB */}
            {currentTab === 'keys' && (
              <KeysView
                keys={keys}
                models={models}
                showToast={showToast}
                handleOpenCreateKey={handleOpenCreateKey}
                selectedKeyIds={selectedKeyIds}
                setSelectedKeyIds={setSelectedKeyIds}
                handleBatchStatusKeys={handleBatchStatusKeys}
                handleBatchDeleteKeys={handleBatchDeleteKeys}
                copyToClipboard={copyToClipboard}
                copiedKey={copiedKey}
                handleToggleKeyStatus={handleToggleKeyStatus}
                setActiveQuickKey={setActiveQuickKey}
                handleDeleteKey={handleDeleteKey}
                handleUpdateKeyGroup={handleUpdateKeyGroup}
                adminUser={adminUser}
                pricingGroups={pricingGroups}
                groupMultipliers={groupMultipliers}
                onNavigateToPricing={() => setCurrentTab('pricing')}
                onNavigateToDashboard={(keyStr, keyId) => {
                  if (adminUser?.role !== 'admin') {
                    setLogFilters({ q: '', key_id: String(keyId), session: '', correlation_type: 'session_id', range: 'all', start: '', end: '', model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'all' });
                    resetOffset();
                    setCurrentTab('logs');
                  } else {
                    // The dashboard drills down by row id: the list value is a
                    // display mask and cannot identify a key server-side.
                    setDashboardSelectedKey(keyId != null ? String(keyId) : '');
                    setCurrentTab('dashboard');
                  }
                }}
                lang={lang}
                t={t}
              />
            )}

            {/* 4. AUDIT LOGS TAB */}
            {currentTab === 'logs' && (
              <LogsView
                sessionFilter={sessionFilter}
                setSessionFilter={setSessionFilter}
                logFilter={logFilter}
                setLogFilter={setLogFilter}
                logFilters={logFilters}
                logFacets={logFacets}
                setLogFilters={setLogFilters}
                fetchLogs={fetchLogs}
                logLoading={logLoading}
                logError={logError}
                handleClearLogs={handleClearLogs}
                timeRange={timeRange}
                setTimeRange={setTimeRange}
                customStartTime={customStartTime}
                setCustomStartTime={setCustomStartTime}
                customEndTime={customEndTime}
                setCustomEndTime={setCustomEndTime}
                selectedLogIds={selectedLogIds}
                setSelectedLogIds={setSelectedLogIds}
                handleBatchDeleteLogs={handleBatchDeleteLogs}
                filteredLogs={filteredLogs}
                logTotal={logTotal}
                onExportAll={fetchAllLogsForExport}
                logsLength={logs.length}
                logOffset={logOffset}
                logPageSize={LOG_PAGE_SIZE}
                onLogPageChange={handleLogPageChange}
                setActiveLogDetail={setActiveLogDetail}
                showToast={showToast}
                handleDeleteSingleLog={handleDeleteSingleLog}
                isAdmin={adminUser?.role === 'admin'}
                lang={lang}
                t={t}
              />
            )}

            {/* 5. SKILL HUB TAB */}
            {currentTab === 'skills' && (
              <SkillHubView
                adminFetch={apiFetch}
                onCopy={(txt) => {
                  copyToClipboard(txt);
                }}
                showToast={showToast}
                lang={lang}
                t={t}
              />
            )}

            {/* 6. MCP HUB TAB */}
            {currentTab === 'mcp' && (
              <McpIntegrationView
                adminFetch={apiFetch}
                adminToken={adminToken}
                keys={keys}
                onCopy={(txt) => {
                  copyToClipboard(txt);
                }}
                showToast={showToast}
                lang={lang}
                t={t}
              />
            )}

            {/* 6. USER MANAGEMENT TAB */}
            {currentTab === 'users' && (
              <UserManagementView
                adminUser={adminUser}
                adminToken={adminToken}
                adminFetch={apiFetch}
                showToast={showToast}
                lang={lang}
                t={t}
              />
            )}

            {/* 7. ORGANIZATION TREE */}
            {currentTab === 'orgs' && (
              <OrganizationsView
                adminFetch={apiFetch}
                showToast={showToast}
                lang={lang}
              />
            )}

            {/* 7. BUDGETS & ALERTS */}
            {currentTab === 'budgets' && (
              <BudgetsView adminFetch={apiFetch} showToast={showToast} lang={lang} />
            )}

            {/* 7. COST & MARGIN */}
            {currentTab === 'costs' && (
              <CostAnalyticsView adminFetch={apiFetch} showToast={showToast} lang={lang} />
            )}

            {/* 7. WALLET LEDGER */}
            {currentTab === 'ledger' && (
              <WalletLedgerView adminFetch={apiFetch} showToast={showToast} lang={lang} />
            )}

            {/* 7. WALLET MANAGEMENT TAB */}
            {currentTab === 'wallet' && (
              <WalletManagementView
                adminUser={adminUser}
                adminToken={adminToken}
                adminFetch={apiFetch}
                showToast={showToast}
                onUserUpdated={(updatedUser) => {
                  if (updatedUser) patchUser(updatedUser);
                }}
                onBalanceUpdate={(newBal) => {
                  if (newBal !== undefined) patchUser({ balance: newBal });
                }}
                lang={lang}
                t={t}
              />
            )}

            {/* 5. MULTIMODAL PLAYGROUND TAB */}
            {currentTab === 'playground' && (
              <PlaygroundView
                keys={keys}
                models={models}
                modelRoutes={modelRoutes}
                prices={prices}
                adminFetch={apiFetch}
                showToast={showToast}
                initialModel={playModel}
                initialModality={playModality}
                lang={lang}
                t={t}
              />
            )}

            {/* 6. DOCS TAB */}
            {currentTab === 'docs' && (
              <DocsView showToast={showToast} lang={lang} t={t} />
            )}
          </Suspense>
        </div>
      </main>

      <Suspense fallback={null}>
        {/* Modal: New Provider with Smart Auto-Probe */}
        <ChannelModal
          isOpen={showChannelModal}
          onClose={() => setShowChannelModal(false)}
          editingChannelId={editingChannelId}
          newChannel={newChannel}
          setNewChannel={setNewChannel}
          probeAlert={probeAlert}
          probing={probing}
          applyPreset={applyPreset}
          handleProbeChannel={handleProbeChannel}
          toggleProtocol={toggleProtocol}
          onSubmit={handleCreateChannel}
          submitting={submittingChannel}
          lang={lang}
          t={t}
        />

        {/* Modal: New API Key */}
        <KeyModal
          isOpen={showKeyModal}
          onClose={() => setShowKeyModal(false)}
          newKey={newKey}
          setNewKey={setNewKey}
          pricingGroups={pricingGroups}
          groupMultipliers={groupMultipliers}
          adminUser={adminUser}
          onSubmit={handleCreateKey}
          submitting={submittingKey}
          createdKey={createdKey}
          onCopy={(txt) => copyToClipboard(txt)}
          lang={lang}
          t={t}
        />

        {/* Account & Security Modal */}
        <AccountManageModal
          isOpen={showAccountModal}
          initialTab={accountModalTab}
          onClose={() => setShowAccountModal(false)}
          adminUser={adminUser}
          adminToken={adminToken}
          adminFetch={apiFetch}
          showToast={showToast}
          lang={lang}
          t={t}
          onPasswordChanged={() => {
            showToast(isZh ? '密码修改成功，请牢记新密码' : 'Password changed successfully', 'success');
          }}
        />

        {/* The login dialog is rendered by the landing and status views only: the
            session-expiry transition switches to the landing view before it asks
            for credentials, so a copy here could never render and had already
            drifted (it was missing the workspace teardown). */}

        {/* 3-Step Guided Developer Onboarding Modal */}
        <OnboardingModal
          isOpen={showOnboardingModal}
          onClose={() => setShowOnboardingModal(false)}
          keys={keys}
          models={models}
          onNavigateToPlayground={(m) => {
            setPlayModel(m);
            setCurrentTab('playground');
          }}
          onCreateKey={() => { setShowOnboardingModal(false); setCurrentTab('keys'); handleOpenCreateKey(); }}
          showToast={showToast}
          lang={lang}
          t={t}
        />

        {/* Visual Routing Topology & Failover Modal */}
        <VisualTopologyModal
          isOpen={showTopologyModal && adminUser?.role === 'admin'}
          onClose={() => setShowTopologyModal(false)}
          adminFetch={apiFetch}
          onViewLogs={(model, trace = '') => {
            setLogFilters({ q: trace, range: 'all', start: '', end: '', session: '', model, channel: 'all', status: 'all', cache: 'all', latency: 'all' });
            resetOffset();
            setCurrentTab('logs');
          }}
          onManageRoutes={() => setCurrentTab('models')}
          lang={lang}
        />

        {/* Redacted request/response snapshots for a trace id */}
        {traceSnapshotId && (
          <TraceSnapshotModal
            isOpen={!!traceSnapshotId}
            traceId={traceSnapshotId?.value || traceSnapshotId}
            initialQueryField={traceSnapshotId?.field || 'trace_id'}
            onClose={() => setTraceSnapshotId('')}
            lang={lang}
            adminFetch={apiFetch}
          />
        )}

        {/* System Announcement Publish Modal */}
        {showAnnouncementModal && adminUser?.role === 'admin' && (
          <AnnouncementModal
            isOpen={showAnnouncementModal}
            onClose={() => setShowAnnouncementModal(false)}
            adminFetch={apiFetch}
            showToast={showToast}
            lang={lang}
            onPublished={() => setAnnouncementRefreshKey(key => key + 1)}
          />
        )}

        {/* Global Command Palette (Cmd+K) */}
        <CommandPalette
          isOpen={showCommandPalette}
          onClose={() => setShowCommandPalette(false)}
          currentTab={currentTab}
          setCurrentTab={setCurrentTab}
          setViewMode={setViewMode}
          models={models}
          channels={channels}
          onSelectModel={(model) => {
            setPlayModel(model);
            setPlayModality(modelRoutes.find(route => route.model === model)?.modality || inferClientModality(model));
          }}
          keys={keys}
          adminUser={adminUser}
          onOpenNewKey={handleOpenCreateKey}
          onOpenNewChannel={() => {
            setEditingChannelId(null);
            setNewChannel({
              name: '',
              type: 'gpustack',
              base_url: '',
              api_key: '',
              priority: 1,
              weight: 10,
              timeout_seconds: 60,
              models_str: '',
              mapping_rules: [],
              protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'rerank', 'images'],
            });
            setProbeAlert(null);
            setShowChannelModal(true);
          }}
          toggleLang={toggleLang}
          lang={lang}
          onLogout={handleLogout}
          showToast={showToast}
          onOpenCopilotWithQuery={(q) => {
            setCopilotInitialQuery(q || '');
            setShowAiCopilot(true);
          }}
        />

        {/* Airoute AI Copilot (Cmd+J / Floating Omnibar) */}
        <AiCopilot
          isOpen={showAiCopilot}
          onClose={() => setShowAiCopilot(false)}
          onToggle={() => setShowAiCopilot(prev => !prev)}
          setCurrentTab={setCurrentTab}
          setViewMode={setViewMode}
          models={models}
          channels={channels}
          keys={keys}
          logs={logs}
          modelRoutes={modelRoutes}
          prices={prices}
          stats={stats}
          lang={lang}
          t={t}
          showToast={showToast}
          adminUser={adminUser}
          adminFetch={apiFetch}
          initialQuery={copilotInitialQuery}
          onClearInitialQuery={() => setCopilotInitialQuery('')}
        />
      </Suspense>
    </div>
    </I18nContext.Provider>
  );
}
