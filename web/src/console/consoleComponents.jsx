import { lazy } from 'react';

import { translations } from '../i18n';

export const QuickStartModal = lazy(() => import('../components/QuickStartModal'));

export const LogDetailModal = lazy(() => import('../components/LogDetailModal'));

export const CurlExportModal = lazy(() => import('../components/CurlExportModal'));

export const ChannelModal = lazy(() => import('../components/ChannelModal'));

export const KeyModal = lazy(() => import('../components/KeyModal'));

export const LandingPage = lazy(() => import('../components/LandingPage'));

export const AuthPage = lazy(() => import('../components/AuthPage'));

export const LoginModal = lazy(() => import('../components/LoginModal'));

export const CommandPalette = lazy(() => import('../components/CommandPalette'));

export const AccountManageModal = lazy(() => import('../components/AccountManageModal'));

export const ModelRoutesManager = lazy(() => import('../components/ModelRoutesManager'));

export const TraceSnapshotModal = lazy(() => import('../components/TraceSnapshotModal'));

export const ServiceStatus = lazy(() => import('../components/ServiceStatus'));

export const PricingManager = lazy(() => import('../components/PricingManager'));

export const SkillHubView = lazy(() => import('../components/SkillHubView'));

export const McpIntegrationView = lazy(() => import('../components/McpIntegrationView'));

export const UserManagementView = lazy(() => import('../components/UserManagementView'));

export const WalletManagementView = lazy(() => import('../components/WalletManagementView'));

export const ChannelsView = lazy(() => import('../components/ChannelsView'));

export const SupplierReconciliationView = lazy(() => import('../components/SupplierReconciliationView'));

export const KeysView = lazy(() => import('../components/KeysView'));

export const LogsView = lazy(() => import('../components/LogsView'));

export const DashboardView = lazy(() => import('../components/DashboardView'));

export const MyUsageView = lazy(() => import('../components/MyUsageView'));

export const PlaygroundView = lazy(() => import('../components/PlaygroundView'));

export const DocsView = lazy(() => import('../components/DocsView'));

export const OnboardingModal = lazy(() => import('../components/OnboardingModal'));

export const VisualTopologyModal = lazy(() => import('../components/VisualTopologyModal'));

export const AnnouncementModal = lazy(() => import('../components/AnnouncementModal'));

export const NotificationCenterView = lazy(() => import('../components/NotificationCenterView'));

export const NotificationComposeModal = lazy(() => import('../components/NotificationComposeModal'));

export const AiCopilot = lazy(() => import('../components/AiCopilot'));

export function ViewLoader({ lang = 'zh' }) {
  const t = translations[lang] || translations.zh;
  return (
    <div className="flex flex-col items-center justify-center p-16 text-[#656B75] dark:text-[#A1A7B0]">
      <div className="w-7 h-7 border-2 border-[#355CFF] border-t-transparent rounded-full animate-spin mb-3"></div>
      <span className="text-xs font-medium tracking-wide">{t.loadingModule}</span>
    </div>
  );
}
