import buildDashboardAttention from '../lib/dashboardAttention';
import useDashboardActions from '../hooks/useDashboardActions';
import useDashboardSummary from '../hooks/useDashboardSummary';
import useDashboardTraffic from '../hooks/useDashboardTraffic';
import useDashboardData from '../hooks/useDashboardData';
import DashboardHeader from './DashboardHeader';
import DashboardAttention from './DashboardAttention';
import DashboardLiveMetrics from './DashboardLiveMetrics';
import DashboardKpis from './DashboardKpis';
import DashboardTrend from './DashboardTrend';
import DashboardWorkloads from './DashboardWorkloads';
import DashboardUpstreams from './DashboardUpstreams';
import DashboardTraffic from './DashboardTraffic';

// The live endpoint is in-memory and cheap; 2s keeps the badge honest during a
// stream without hammering the gateway.
const LIVE_POLL_MS = 2000;
// The chart and the audit stream are aggregates, so they move far slower than
// the speedometers and are refreshed on their own, slower cadence.
const DASHBOARD_REFRESH_MS = 15000;

export default function DashboardView({
  stats: overviewStats = {},
  channels = [],
  logs: overviewLogs = [],
  keys = [],
  channelLatencies = {},
  testingId,
  batchTesting,
  handleBatchTest,
  handleTestChannel,
  fetchData,
  fetchLogs,
  showToast,
  setCurrentTab,
  onNavigateToErrors,
  setShowChannelModal,
  setActiveLogDetail,
  adminUser,
  onOpenTopology,
  adminFetch,
  selectedKey: propSelectedKey,
  onSelectKey,
  lang = 'zh',

}) {
  const isZh = lang === 'zh';
  const {
    chartMetric, dismissedAttentions, fetchDashboard, handleDismissAttention, handleSelectKeyDrilldown,
    hoveredBucket, keyStats, lastRefreshed, liveStats, recentLogs, recentLogsLoading, refreshing, selectedKey,
    setChartMetric, setHoveredBucket, setLastRefreshed, setRefreshing, setTimeframe, statsKey, timeframe,
    timeseriesData
  } = useDashboardData({
    DASHBOARD_REFRESH_MS, LIVE_POLL_MS, adminFetch, onSelectKey, propSelectedKey
  });

  const {
    activeKeyObj, activeRequests, activeStreams, currentRPM, currentTPM, errorLogs, liveDetail, livePhase, logs,
    maxMetricVal, peakRPM, peakTPM, slaAvailability, stats, timeBuckets, totalErrorRequests, totalRequests
  } = useDashboardTraffic({
    chartMetric, isZh, keyStats, keys, liveStats, overviewLogs, overviewStats, recentLogs, selectedKey,
    statsKey, timeframe, timeseriesData
  });

  // 2b. ACTIONABLE SIGNALS for the "needs attention" strip.
  // Derived only from data already held in props — no extra requests.
  const {
    attention, chartSvgData, errorBudget, errorCountByCode, fmtPct, modalityStats, percentiles,
    providerSlaStats, tokenMetrics, topModels
  } = useDashboardSummary({
    channelLatencies, channels, chartMetric, errorLogs, isZh, logs, maxMetricVal, slaAvailability, stats,
    timeBuckets, timeframe, timeseriesData, totalErrorRequests, totalRequests
  });

  const {
    copiedReport, handleExportExecutiveReport, handleRefresh
  } = useDashboardActions({
    channels, fetchDashboard, fetchData, fetchLogs, fmtPct, isZh, logs, percentiles, refreshing,
    setLastRefreshed, setRefreshing, showToast, slaAvailability, stats, tokenMetrics, totalRequests
  });

  // Only signals we can actually derive from the props; `logs` is one page, so
  // the failure count is always phrased as "of the last N logged requests".
  const {
    attentionItems, attentionTones
  } = buildDashboardAttention({
    adminUser, attention, channels, errorCountByCode, errorLogs, isZh, onNavigateToErrors, setCurrentTab,
    setShowChannelModal
  });

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* 1. Page Header (Directly on canvas, no box-in-box card) */}
      <DashboardHeader {...{
        adminUser, copiedReport, handleExportExecutiveReport, handleRefresh, isZh, lastRefreshed, onOpenTopology,
        refreshing, setShowChannelModal
      }} />

      {/* 2. NEEDS ATTENTION — what needs action, shown before the totals */}
      <DashboardAttention {...{
        attention, attentionItems, attentionTones, channels, dismissedAttentions, handleDismissAttention, isZh
      }} />

      {/* 2.5 REAL-TIME TELEMETRY COCKPIT & KEY DRILLDOWN */}
      <DashboardLiveMetrics {...{
        activeKeyObj, activeRequests, activeStreams, adminUser, attention, channels, currentRPM, currentTPM,
        handleSelectKeyDrilldown, isZh, keys, liveDetail, livePhase, peakRPM, peakTPM, selectedKey, slaAvailability,
        stats
      }} />

      {/* 3. CORE TELEMETRY KPIS (4 CARDS) */}
      <DashboardKpis {...{
        currentRPM, currentTPM, fmtPct, isZh, percentiles, slaAvailability, stats, tokenMetrics
      }} />

      {/* 4. INTERACTIVE MULTI-METRIC TIME-SERIES VISUALIZER */}
      <DashboardTrend {...{
        chartMetric, chartSvgData, errorLogs, fmtPct, hoveredBucket, isZh, maxMetricVal, percentiles,
        setChartMetric, setHoveredBucket, setTimeframe, slaAvailability, timeBuckets, timeframe, totalErrorRequests,
        totalRequests
      }} />

      {/* 5. WORKLOAD DISTRIBUTION & TELEMETRY PANELS (FOUR QUADRANTS) */}
      <DashboardWorkloads {...{
        errorBudget, isZh, modalityStats, onOpenTopology, providerSlaStats, setCurrentTab, slaAvailability,
        topModels
      }} />

      {/* 6. UPSTREAM PROVIDERS HEALTH & CIRCUIT BREAKER MATRIX */}
      <DashboardUpstreams {...{
        adminUser, batchTesting, channelLatencies, channels, handleBatchTest, handleTestChannel, isZh,
        setCurrentTab, setShowChannelModal, testingId
      }} />

      {/* 7. RECENT TRAFFIC AUDIT STREAM */}
      <DashboardTraffic {...{
        isZh, logs, recentLogs, recentLogsLoading, selectedKey, setActiveLogDetail, setCurrentTab
      }} />
    </div>
  );
}
