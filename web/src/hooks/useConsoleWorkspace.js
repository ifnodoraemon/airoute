import { useUiPreferences } from './useUiPreferences';

import { translations } from '../i18n';
import { useToast } from './useToast';

import useConsoleIdentity from './useConsoleIdentity';
import useConsoleSessionEffects from './useConsoleSessionEffects';
import useConsoleApiSession from './useConsoleApiSession';
import useConsoleWorkspaceReset from './useConsoleWorkspaceReset';
import useConsoleLogout from './useConsoleLogout';
import useConsoleWorkspaceUi from './useConsoleWorkspaceUi';
import useConsoleNavigation from './useConsoleNavigation';
import useConsoleWorkspaceData from './useConsoleWorkspaceData';
import useConsoleLogPolling from './useConsoleLogPolling';

export default function useConsoleWorkspace() {
  const preferences = useUiPreferences({ translations });
  const toasts = useToast();
  const identity = useConsoleIdentity({ ...preferences, ...toasts });
  // These callbacks run after hook initialization; retaining that order keeps
  // session restoration and logout tied to the current workspace.
  useConsoleSessionEffects({
    ...identity, ...preferences, ...toasts,
    fetchData: (...args) => workspaceData.fetchData(...args),
    fetchLogs: (...args) => workspaceData.fetchLogs(...args),
    setCurrentTab: (...args) => workspaceUi.setCurrentTab(...args),
  });
  useConsoleApiSession(identity);
  const reset = useConsoleWorkspaceReset({
    resetOverlays: identity.resetOverlays,
    getData: () => workspaceData,
    getUi: () => workspaceUi,
  });
  const logout = useConsoleLogout({ ...identity, ...preferences, ...toasts, ...reset });
  const workspaceUi = useConsoleWorkspaceUi(identity);
  useConsoleNavigation({ ...identity, ...workspaceUi });
  const workspaceData = useConsoleWorkspaceData({ ...identity, ...preferences, ...toasts });
  const paging = useConsoleLogPolling({ ...identity, ...workspaceUi, ...workspaceData });
  return { ...preferences, ...toasts, ...identity, ...reset, ...logout, ...workspaceUi, ...workspaceData, ...paging };
}
