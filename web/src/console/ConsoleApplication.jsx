import ConsoleLanding from './ConsoleLanding';
import ConsoleStatus from './ConsoleStatus';
import { useConsoleContext } from './ConsoleContext';
import { QuickStartModal, LogDetailModal, CurlExportModal, AuthPage, ViewLoader } from './consoleComponents';

import ConsoleSidebar from './ConsoleSidebar';
import ConsoleHeader from './ConsoleHeader';
import ConsoleContent from './ConsoleContent';
import ConsoleDialogs from './ConsoleDialogs';
import { Suspense } from 'react';
import { ShieldAlert, AlertCircle, Lock } from 'lucide-react';
import AnnouncementBanner from '../components/AnnouncementBanner';
import Toast from '../components/Toast';
import { apiFetch } from '../api/client';

import { I18nContext } from '../i18n';
import { inferClientModality } from '../lib/modelModality';

export default function ConsoleApplication() {
  const {
    activeLogDetail, activeQuickKey, adminUser, announcementRefreshKey, authTab, closeMobileMenu, copyToClipboard, curlExportCmd, dismissToast, fetchData,
    fetchLogs, handleDeleteSingleLog, isZh, lang, mobileMenuOpen, modelRoutes, models,
    resetWorkspaceState, restoringSession, setAccountModalTab, setActiveLogDetail, setActiveQuickKey,
    setCurlExportCmd, setCurrentTab, setLang, setNotificationKind, setPlayModality, setPlayModel,
    setPlaygroundScenario, setSessionFilter, setShowAccountModal, setTraceSnapshotId,
    setViewMode, showToast, signIn, t, toast, toggleLang, viewMode
  } = useConsoleContext();
  if (restoringSession && viewMode === 'console') return <ViewLoader lang={lang} />;

  if (viewMode === 'landing') {
    return (
      <ConsoleLanding  />
    );
  }

  if (viewMode === 'auth') {
    return (
      <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
        <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white">
          <Toast toast={toast} onClose={dismissToast} />
          <Suspense fallback={<ViewLoader lang={lang} />}>
            <AuthPage
              initialTab={authTab}
              lang={lang}
              toggleLang={toggleLang}
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
      <ConsoleStatus  />
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
        />
      )}

      {/* Log Detail Inspector Modal */}
      {activeLogDetail && (
        <LogDetailModal
          log={activeLogDetail}
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
          onNavigate={(targetTab, model) => {
            if (targetTab === 'playground' && model) {
              setPlaygroundScenario(null);
              setPlayModel(model);
              setPlayModality(modelRoutes.find(route => route.model === model)?.modality || inferClientModality(model));
            }
            setCurrentTab(targetTab);
            setViewMode('console');
            setActiveLogDetail(null);
          }}
          lang={lang}
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
      <ConsoleSidebar  />

      {/* Main Container */}
      <main className="flex-1 flex flex-col min-w-0 overflow-y-auto">
        {/* Top Header */}
        <ConsoleHeader  />

        {/* Global System Announcement Banner */}
        <AnnouncementBanner lang={lang} adminFetch={apiFetch} refreshKey={announcementRefreshKey} onOpen={() => {
          setNotificationKind('announcement');
          setCurrentTab('notifications');
        }} />

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

          {/* A crashed view must not take the shell with it: the sidebar, header and
              the module switcher stay usable and the retry button re-renders just
              this region. Keyed by tab, so moving to another module always starts
              from a clean boundary. */}
          <ConsoleContent   />
        </div>
      </main>

      <ConsoleDialogs  />
    </div>
    </I18nContext.Provider>
  );

}
