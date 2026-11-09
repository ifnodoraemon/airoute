import { useConsoleContext } from './ConsoleContext';
import { AccountManageModal, AiCopilot, AnnouncementModal, ChannelModal, CommandPalette, KeyModal, NotificationComposeModal, OnboardingModal, TraceSnapshotModal, VisualTopologyModal } from './consoleComponents';
import { Suspense } from 'react';
import { apiFetch } from '../api/client';
import { inferClientModality } from '../lib/modelModality';

export default function ConsoleDialogs() {
  const {
  accountModalTab, adminUser, applyPreset, channels, copilotInitialQuery, copyToClipboard, createdKey,
  editingAnnouncementId, editingChannelId, groupMultipliers, handleCreateChannel, handleCreateKey,
  handleLogout, handleOpenCreateKey, handleProbeChannel, isZh, keys, lang, logs, modelRoutes, models,
  newChannel, newKey, openCreateChannel, prices, pricingGroups, probeAlert, probing, resetOffset,
  setAnnouncementRefreshKey, setCopilotInitialQuery, setCurrentTab, setLogFilters, setNewChannel, setNewKey,
  setPlayModality, setPlayModel, setShowAccountModal, setShowAiCopilot, setShowAnnouncementModal,
  setShowChannelModal, setShowCommandPalette, setShowKeyModal, setShowNotificationCompose,
  setShowOnboardingModal, setShowTopologyModal, setTraceSnapshotId, setViewMode, showAccountModal,
  showAiCopilot, showAnnouncementModal, showChannelModal, showCommandPalette, showKeyModal,
  showNotificationCompose, showOnboardingModal, showToast, showTopologyModal, stats, submittingChannel,
  submittingKey, t, thinkingCapabilities, toggleLang, toggleProtocol, traceSnapshotId
} = useConsoleContext();
  return (
    <Suspense fallback={null}>
      {/* Modal: New Provider with Smart Auto-Probe */}
      <ChannelModal
        resolvedCapabilities={channels.find(channel => channel.id === editingChannelId && channel.type === newChannel.type)?.thinking_capabilities || {}}
        upstreamModels={channels.find(channel => channel.id === editingChannelId)?.upstream_models || []}
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
        adminFetch={apiFetch}
        showToast={showToast}
        lang={lang}
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
          announcementId={editingAnnouncementId}
          onClose={() => setShowAnnouncementModal(false)}
          adminFetch={apiFetch}
          showToast={showToast}
          lang={lang}
          onPublished={() => setAnnouncementRefreshKey(key => key + 1)}
        />
      )}

      {showNotificationCompose && adminUser?.role === 'admin' && (
        <NotificationComposeModal
          onClose={() => setShowNotificationCompose(false)}
          onSent={() => setAnnouncementRefreshKey(value => value + 1)}
          showToast={showToast}
          lang={lang}
        />
      )}

      {/* Global Command Palette (Cmd+K) */}
      <CommandPalette
        isOpen={showCommandPalette}
        onClose={() => setShowCommandPalette(false)}
        setCurrentTab={setCurrentTab}
        setViewMode={setViewMode}
        models={models}
        onSelectModel={(model) => {
          setPlayModel(model);
          setPlayModality(modelRoutes.find(route => route.model === model)?.modality || inferClientModality(model));
        }}
        adminUser={adminUser}
        onOpenNewKey={handleOpenCreateKey}
        onOpenNewChannel={openCreateChannel}
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
        thinkingCapabilities={thinkingCapabilities}
        onCreateKey={handleOpenCreateKey}
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

        showToast={showToast}
        adminFetch={apiFetch}
        initialQuery={copilotInitialQuery}
        onClearInitialQuery={() => setCopilotInitialQuery('')}
      />
    </Suspense>
  );
}
