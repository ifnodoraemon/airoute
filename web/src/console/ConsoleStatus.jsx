import { useConsoleContext } from './ConsoleContext';
import { I18nContext } from '../i18n';
import Toast from '../components/Toast';
import { Suspense } from 'react';
import { LoginModal, ServiceStatus, ViewLoader } from './consoleComponents';

export default function ConsoleStatus() {
  const {
  adminToken, adminUser, batchTesting, channelLatencies, channels, dismissToast, fetchData, fetchLogs,
  handleBatchPing, isZh, lang, modelRoutes, resetWorkspaceState, setAuthTab, setLang, setShowLoginModal,
  setViewMode, showLoginModal, showToast, signIn, t, toast, toggleLang
} = useConsoleContext();
  return (
    <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
      <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white">
        <Toast toast={toast} onClose={dismissToast} />
        <Suspense fallback={<ViewLoader lang={lang} />}>
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
            showToast={showToast}
            onBatchProbe={handleBatchPing}
            batchTesting={batchTesting}
          />
          <LoginModal
            isOpen={showLoginModal}
            onClose={() => setShowLoginModal(false)}
            lang={lang}
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
