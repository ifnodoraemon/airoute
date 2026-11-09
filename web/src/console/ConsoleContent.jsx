import ConsoleDashboard from './ConsoleDashboard';
import { useConsoleContext } from './ConsoleContext';
import { ChannelsView, DocsView, KeysView, LogsView, McpIntegrationView, ModelRoutesManager, MyUsageView, NotificationCenterView, PlaygroundView, PricingManager, SkillHubView, SupplierReconciliationView, UserManagementView, ViewLoader, WalletManagementView } from './consoleComponents';
import ErrorBoundary from '../components/ErrorBoundary';
import { Suspense } from 'react';
import { apiFetch } from '../api/client';
import { openProcurement } from '../lib/supplierQuotes';
import { LOG_PAGE_SIZE } from '../hooks/useLogsData';

export default function ConsoleContent() {
  const {
  adminUser, announcementRefreshKey, batchTesting, channelLatencies, channels, copiedKey, copyToClipboard,
  currentTab, customEndTime, customStartTime, fetchAllLogsForExport, fetchData,
  fetchLogs, filteredLogs, groupMultipliers, handleBatchDeleteChannels,
  handleBatchDeleteKeys, handleBatchDeleteLogs, handleBatchPing, handleBatchStatusChannels,
  handleBatchStatusKeys, handleClearLogs, handleDeleteChannel, handleDeleteKey,
  handleDeleteSingleLog, handleEditChannel, handleLogPageChange, handleOpenCreateKey, handleTestChannel,
  handleToggleKeyStatus, handleUpdateKeyGroup, keys, lang, logError, logFacets, logFilter, logFilters,
  logLoading, logOffset, logTotal, logs, modelRoutes, models, notificationCounts, notificationKind,
  notificationUserKey, openCreateChannel, patchUser, playModality, playModel, playgroundScenario, prices,
  pricingGroups, resetOffset, selectedChannelIds, selectedKeyIds, selectedLogIds, sessionFilter,
  setActiveLogDetail, setActiveQuickKey, setAnnouncementRefreshKey, setCurrentTab, setCustomEndTime,
  setCustomStartTime, setDashboardSelectedKey, setEditingAnnouncementId, setLogFilter, setLogFilters,
  setNotificationKind, setNotificationReadKey, setPlayModality, setPlayModel, setPlaygroundScenario,
  setSelectedChannelIds, setSelectedKeyIds, setSelectedLogIds, setSessionFilter, setShowAnnouncementModal,
  setShowNotificationCompose, setTimeRange, showToast, testingId, thinkingCapabilities, timeRange
} = useConsoleContext();
  return (
    <ErrorBoundary key={currentTab} lang={lang}>
      <Suspense fallback={<ViewLoader lang={lang} />}>
      {/* 1a. SELF-SERVICE DASHBOARD (regular users) */}
      {currentTab === 'notifications' && (
        <NotificationCenterView
          key={notificationUserKey}
          kind={notificationKind}
          onKindChange={setNotificationKind}
          counts={notificationCounts}
          isAdmin={adminUser?.role === 'admin'}
          onPublish={() => { setEditingAnnouncementId(null); setShowAnnouncementModal(true); }}
          onEdit={id => { setEditingAnnouncementId(id); setShowAnnouncementModal(true); }}
          onAnnouncementChanged={() => setAnnouncementRefreshKey(value => value + 1)}
          onSend={() => setShowNotificationCompose(true)}
          refreshKey={announcementRefreshKey}
          onChanged={() => setNotificationReadKey(value => value + 1)}
          lang={lang}
        />
      )}

      {currentTab === 'dashboard' && adminUser?.role !== 'admin' && (
        <MyUsageView adminFetch={apiFetch} showToast={showToast} lang={lang} />
      )}

      {/* 1. ENTERPRISE BUSINESS & TELEMETRY DASHBOARD */}
      {currentTab === 'dashboard' && adminUser?.role === 'admin' && (
        <ConsoleDashboard  />
      )}

      {/* 1.5. MODEL ROUTES TAB */}
      {currentTab === 'models' && (
        <ModelRoutesManager
          isAdmin={adminUser?.role === 'admin'}
          adminFetch={apiFetch}
          showToast={showToast}
          lang={lang}
          setCurrentTab={setCurrentTab}
        />
      )}

      {/* 1.6. MODEL PRICING & RATES TAB */}
      {currentTab === 'pricing' && (
        <PricingManager
          adminFetch={apiFetch}
          showToast={showToast}
          isAdmin={adminUser?.role === 'admin'}
          lang={lang}
          modelRoutes={modelRoutes}
          channels={channels}
          setCurrentTab={setCurrentTab}
          groupMultipliers={groupMultipliers}
          onUpdateGroupMultipliers={fetchData}
        />
      )}

      {/* 2. CHANNELS / PROVIDERS TAB */}
      {currentTab === 'channels' && (
        <ChannelsView
          openCreateChannel={openCreateChannel}
          channels={channels}
          batchTesting={batchTesting}
          handleBatchPing={handleBatchPing}
          selectedChannelIds={selectedChannelIds}
          setSelectedChannelIds={setSelectedChannelIds}
          handleBatchStatusChannels={handleBatchStatusChannels}
          handleBatchDeleteChannels={handleBatchDeleteChannels}
          channelLatencies={channelLatencies}
          copyToClipboard={copyToClipboard}
          handleTestChannel={handleTestChannel}
          testingId={testingId}
          handleEditChannel={handleEditChannel}
          handleDeleteChannel={handleDeleteChannel}
          onNavigateToReconciliation={() => setCurrentTab('reconciliation')}
          onNavigateToProcurement={(channel = '') => openProcurement(setCurrentTab, channel)}
          lang={lang}
        />
      )}

      {/* 2.5. SUPPLIER RECONCILIATION TAB */}
      {currentTab === 'reconciliation' && (
        <SupplierReconciliationView
          adminFetch={apiFetch}
          showToast={showToast}
          onNavigateToLogs={(channelName) => {
            setLogFilter(channelName);
            setCurrentTab('logs');
          }}
          lang={lang}
        />
      )}

      {/* 3. API KEYS TAB */}
      {currentTab === 'keys' && (
        <KeysView
          keys={keys}
          models={models}
          showToast={showToast}
          handleOpenCreateKey={handleOpenCreateKey}
          selectedKeyIds={selectedKeyIds}
          setSelectedKeyIds={setSelectedKeyIds}
          handleBatchStatusKeys={handleBatchStatusKeys}
          handleBatchDeleteKeys={handleBatchDeleteKeys}
          copyToClipboard={copyToClipboard}
          copiedKey={copiedKey}
          handleToggleKeyStatus={handleToggleKeyStatus}
          setActiveQuickKey={setActiveQuickKey}
          handleDeleteKey={handleDeleteKey}
          handleUpdateKeyGroup={handleUpdateKeyGroup}
          adminUser={adminUser}
          pricingGroups={pricingGroups}
          groupMultipliers={groupMultipliers}
          onNavigateToPricing={() => setCurrentTab('pricing')}
          onNavigateToDashboard={(keyId) => {
            if (adminUser?.role !== 'admin') {
              setLogFilters({ q: '', key_id: String(keyId), session: '', correlation_type: 'session_id', range: 'all', start: '', end: '', model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'all' });
              resetOffset();
              setCurrentTab('logs');
            } else {
              // The dashboard drills down by row id: the key ID (`ak_…`)
              // shown in the list is a display identifier, not a filter.
              setDashboardSelectedKey(keyId != null ? String(keyId) : '');
              setCurrentTab('dashboard');
            }
          }}
          lang={lang}
        />
      )}

      {/* 4. AUDIT LOGS TAB */}
      {currentTab === 'logs' && (
        <LogsView
          sessionFilter={sessionFilter}
          setSessionFilter={setSessionFilter}
          logFilter={logFilter}
          setLogFilter={setLogFilter}
          logFilters={logFilters}
          logFacets={logFacets}
          setLogFilters={setLogFilters}
          fetchLogs={fetchLogs}
          logLoading={logLoading}
          logError={logError}
          handleClearLogs={handleClearLogs}
          timeRange={timeRange}
          setTimeRange={setTimeRange}
          customStartTime={customStartTime}
          setCustomStartTime={setCustomStartTime}
          customEndTime={customEndTime}
          setCustomEndTime={setCustomEndTime}
          selectedLogIds={selectedLogIds}
          setSelectedLogIds={setSelectedLogIds}
          handleBatchDeleteLogs={handleBatchDeleteLogs}
          filteredLogs={filteredLogs}
          logTotal={logTotal}
          onExportAll={fetchAllLogsForExport}
          logsLength={logs.length}
          logOffset={logOffset}
          logPageSize={LOG_PAGE_SIZE}
          onLogPageChange={handleLogPageChange}
          setActiveLogDetail={setActiveLogDetail}
          showToast={showToast}
          handleDeleteSingleLog={handleDeleteSingleLog}
          isAdmin={adminUser?.role === 'admin'}
          lang={lang}
        />
      )}

      {/* 5. SKILL HUB TAB */}
      {currentTab === 'skills' && (
        <SkillHubView
          adminFetch={apiFetch}
          onTrial={(scenario) => { setPlaygroundScenario(scenario); setPlayModel(''); setPlayModality('chat'); setCurrentTab('playground'); }}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          showToast={showToast}
          lang={lang}

        />
      )}

      {/* 6. MCP HUB TAB */}
      {currentTab === 'mcp' && (
        <McpIntegrationView
          adminFetch={apiFetch}
          onCopy={(txt) => {
            copyToClipboard(txt);
          }}
          showToast={showToast}
          lang={lang}

        />
      )}

      {/* 6. USER MANAGEMENT TAB */}
      {currentTab === 'users' && (
        <UserManagementView
          adminUser={adminUser}
          onUserCreated={fetchData}
          adminFetch={apiFetch}
          showToast={showToast}
          lang={lang}
        />
      )}

      {/* 7. WALLET MANAGEMENT TAB */}
      {currentTab === 'wallet' && (
        <WalletManagementView
          adminUser={adminUser}
          adminFetch={apiFetch}
          showToast={showToast}
          onUserUpdated={(updatedUser) => {
            if (updatedUser) patchUser(updatedUser);
          }}
          onBalanceUpdate={(newBal) => {
            if (newBal !== undefined) patchUser({ balance: newBal });
          }}
          lang={lang}
        />
      )}

      {/* 5. MULTIMODAL PLAYGROUND TAB */}
      {currentTab === 'playground' && (
        <PlaygroundView
          thinkingCapabilities={thinkingCapabilities}
          onCreateKey={() => { setCurrentTab('keys'); handleOpenCreateKey(); }}
          keys={keys}
          models={models}
          modelRoutes={modelRoutes}
          prices={prices}
          showToast={showToast}
          initialModel={playModel}
          initialModality={playModality}
          initialScenario={playgroundScenario}
          lang={lang}

        />
      )}

      {/* 6. DOCS TAB */}
      {currentTab === 'docs' && (
        <DocsView showToast={showToast} lang={lang}  />
      )}
      </Suspense>
    </ErrorBoundary>
  );
}
