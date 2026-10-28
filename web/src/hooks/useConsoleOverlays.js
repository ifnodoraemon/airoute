// The console's overlays: the command palette, the onboarding tour, the topology
// map, the AI copilot, and the account/login dialogs.
//
// They are plain booleans, so this hook earns its place by naming the group and by
// giving the render tree one object to read instead of a dozen independent flags.
// It deliberately does NOT enforce mutual exclusion: nothing did before, and adding
// that here would change which dialogs can be open together.

import { useCallback, useState } from 'react';

export function useConsoleOverlays() {
  const [showCommandPalette, setShowCommandPalette] = useState(false);
  const [showOnboardingModal, setShowOnboardingModal] = useState(false);
  const [showTopologyModal, setShowTopologyModal] = useState(false);
  const [showAiCopilot, setShowAiCopilot] = useState(false);
  const [copilotInitialQuery, setCopilotInitialQuery] = useState('');
  const [showLoginModal, setShowLoginModal] = useState(false);
  const [showAccountModal, setShowAccountModal] = useState(false);
  const [accountModalTab, setAccountModalTab] = useState('profile');

  // Every overlay closes on logout, so the next session does not inherit a dialog
  // opened by the previous one.
  const reset = useCallback(() => {
    setShowCommandPalette(false);
    setShowOnboardingModal(false);
    setShowTopologyModal(false);
    setShowAiCopilot(false);
    setCopilotInitialQuery('');
    setShowLoginModal(false);
    setShowAccountModal(false);
    setAccountModalTab('profile');
  }, []);

  return {
    showCommandPalette,
    setShowCommandPalette,
    showOnboardingModal,
    setShowOnboardingModal,
    showTopologyModal,
    setShowTopologyModal,
    showAiCopilot,
    setShowAiCopilot,
    copilotInitialQuery,
    setCopilotInitialQuery,
    showLoginModal,
    setShowLoginModal,
    showAccountModal,
    setShowAccountModal,
    accountModalTab,
    setAccountModalTab,
    reset,
  };
}
