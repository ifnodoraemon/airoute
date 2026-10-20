// The console shell: the sidebar and mobile menu, the active view tab, and the
// playground modality that another view can pre-select when it navigates there.
//
// Grouped because they are the state the shell owns on behalf of the views around
// it. They are plain values with no persistence or timers, so this hook is
// deliberately thin: its value is that the shell's state has a name and a home,
// not that it computes anything.

import { useCallback, useState } from 'react';

export function useConsoleChrome({ initialTab = 'dashboard' } = {}) {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [currentTab, setCurrentTab] = useState(initialTab);
  // Set by the channel and route views so "open in playground" lands on the right
  // modality instead of the default.
  const [playModality, setPlayModality] = useState('chat');

  const openMobileMenu = useCallback(() => setMobileMenuOpen(true), []);
  const closeMobileMenu = useCallback(() => setMobileMenuOpen(false), []);
  const collapseSidebar = useCallback(() => setSidebarCollapsed(true), []);
  const expandSidebar = useCallback(() => setSidebarCollapsed(false), []);

  // Selecting a tab closes the mobile menu: it is an overlay, and leaving it open
  // hides the view the user just asked for.
  const selectTab = useCallback((tab) => {
    setCurrentTab(tab);
    setMobileMenuOpen(false);
  }, []);

  return {
    sidebarCollapsed,
    setSidebarCollapsed,
    mobileMenuOpen,
    setMobileMenuOpen,
    currentTab,
    setCurrentTab,
    playModality,
    setPlayModality,
    openMobileMenu,
    closeMobileMenu,
    collapseSidebar,
    expandSidebar,
    selectTab,
  };
}
