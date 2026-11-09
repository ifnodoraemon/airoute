import { useConsoleContext } from './ConsoleContext';
import { DashboardView } from './consoleComponents';
import { apiFetch } from '../api/client';

export default function ConsoleDashboard() {
  const {
  adminUser, batchTesting, channelLatencies, channels, dashboardSelectedKey, fetchData, fetchRecentLogs,
  handleBatchTest, handleTestChannel, keys, lang, logs, recentLogs, resetOffset, setActiveLogDetail,
  setCurrentTab, setDashboardSelectedKey, setLogFilters, setShowChannelModal, setShowTopologyModal, showToast,
  stats, testingId
} = useConsoleContext();
  return (
    <DashboardView
      stats={stats}
      onNavigateToErrors={() => {
        setLogFilters({ q: '', range: 'all', start: '', end: '', session: '', model: 'all', channel: 'all', status: 'error', cache: 'all', latency: 'all' });
        resetOffset();
        setCurrentTab('logs');
      }}
      channels={channels}
      logs={recentLogs.length > 0 ? recentLogs : logs}
      channelLatencies={channelLatencies}
      testingId={testingId}
      batchTesting={batchTesting}
      handleBatchTest={handleBatchTest}
      handleTestChannel={handleTestChannel}
      fetchData={fetchData}
      fetchLogs={fetchRecentLogs}
      showToast={showToast}
      setCurrentTab={setCurrentTab}
      setShowChannelModal={setShowChannelModal}
      setActiveLogDetail={setActiveLogDetail}
      adminUser={adminUser}
      adminFetch={apiFetch}
      keys={keys}
      selectedKey={dashboardSelectedKey}
      onSelectKey={setDashboardSelectedKey}
      onOpenTopology={() => setShowTopologyModal(true)}
      lang={lang}

    />
  );
}
