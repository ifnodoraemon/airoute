import { useConsoleSelections } from './useConsoleSelections';
import { useState } from 'react';
import useNotificationCounts from './useNotificationCounts';
import { useConsoleChrome } from './useConsoleChrome';

export default function useConsoleWorkspaceUi({
  adminUser, initialRoute, restoringSession, viewMode
}) {
  const {
    activeQuickKey, setActiveQuickKey,
    activeLogDetail, setActiveLogDetail,
    traceSnapshotId, setTraceSnapshotId,
    curlExportCmd, setCurlExportCmd,
    playgroundScenario, setPlaygroundScenario,
    reset: resetSelections,
  } = useConsoleSelections();
  const [dashboardSelectedKey, setDashboardSelectedKey] = useState('');
  const [showAnnouncementModal, setShowAnnouncementModal] = useState(false);
  const [editingAnnouncementId, setEditingAnnouncementId] = useState(null);
  // Bumped after an announcement is published so the banner refreshes at once
  // instead of waiting for its 60s poll.
  const [announcementRefreshKey, setAnnouncementRefreshKey] = useState(0);
  const [showNotificationCompose, setShowNotificationCompose] = useState(false);
  const [notificationKind, setNotificationKind] = useState('announcement');
  const [notificationReadKey, setNotificationReadKey] = useState(0);
  const notificationUserKey = String(adminUser?.id || adminUser?.username || '');
  const notificationCounts = useNotificationCounts({
    enabled: viewMode === 'console' && !restoringSession && !!adminUser,
    userKey: notificationUserKey,
    refreshKey: `${announcementRefreshKey}:${notificationReadKey}`,
  });
  const {
    sidebarCollapsed,
    mobileMenuOpen, setMobileMenuOpen,
    currentTab, setCurrentTab,
    playModality, setPlayModality,
    closeMobileMenu,
    collapseSidebar,
    expandSidebar,
  } = useConsoleChrome({ initialTab: initialRoute.tab });
  return {
    activeLogDetail, activeQuickKey, announcementRefreshKey, closeMobileMenu, collapseSidebar, curlExportCmd,
    currentTab, dashboardSelectedKey, editingAnnouncementId, expandSidebar, mobileMenuOpen, notificationCounts,
    notificationKind, notificationUserKey, playModality, playgroundScenario, resetSelections,
    setActiveLogDetail, setActiveQuickKey, setAnnouncementRefreshKey, setCurlExportCmd, setCurrentTab,
    setDashboardSelectedKey, setEditingAnnouncementId, setMobileMenuOpen, setNotificationKind,
    setNotificationReadKey, setPlayModality, setPlaygroundScenario, setShowAnnouncementModal,
    setShowNotificationCompose, setTraceSnapshotId, showAnnouncementModal, showNotificationCompose,
    sidebarCollapsed, traceSnapshotId
  };
}
