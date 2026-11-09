import { getConsoleRoute } from '../lib/consoleRoute';
import { useConsoleSession } from './useConsoleSession';

import { useConsoleOverlays } from './useConsoleOverlays';

export default function useConsoleIdentity({
  isZh, showToast
}) {
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
  return {
    accountModalTab, adminToken, adminUser, applyRoute, authTab, copilotInitialQuery, endSession,
    finishSessionRestore, getSessionGeneration, initialRoute, patchUser, resetOverlays, restoreSession,
    restoringSession, setAccountModalTab, setAuthTab, setCopilotInitialQuery, setShowAccountModal,
    setShowAiCopilot, setShowCommandPalette, setShowLoginModal, setShowOnboardingModal, setShowTopologyModal,
    setViewMode, showAccountModal, showAiCopilot, showCommandPalette, showLoginModal, showOnboardingModal,
    showTopologyModal, signIn, signOutLocal, viewMode
  };
}
