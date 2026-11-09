import { useConsoleContext } from './ConsoleContext';
import { I18nContext } from '../i18n';
import Toast from '../components/Toast';
import { Suspense } from 'react';
import { LandingPage, LoginModal, ViewLoader } from './consoleComponents';

export default function ConsoleLanding() {
  const {
  adminToken, adminUser, dismissToast, fetchData, fetchLogs, isZh, lang, resetWorkspaceState, setAuthTab,
  setLang, setShowLoginModal, setViewMode, showLoginModal, showToast, signIn, t, toast, toggleLang
} = useConsoleContext();
  return (
    <I18nContext.Provider value={{ lang, setLang, toggleLang, t }}>
      <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] font-sans selection:bg-[#355CFF] selection:text-white">
        <Toast toast={toast} onClose={dismissToast} />
        <Suspense fallback={<ViewLoader lang={lang} />}>
          <LandingPage
            isLoggedIn={!!adminToken || !!adminUser}
            lang={lang}
            toggleLang={toggleLang}
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
