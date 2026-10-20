import { useState, useEffect, useMemo, useRef } from 'react';
import {
  History,
  Search,
  RefreshCw,
  Trash2,
  Clock,
  Copy,
  Check,
  FileSpreadsheet,
  Filter,
  FilterX,
  X,
  RotateCcw,
  SlidersHorizontal,
  MoreHorizontal,
  ChevronDown
} from 'lucide-react';
import { copyToClipboard, toCSV } from '../config';

// Same-day rows show time only; older days get a compact date prefix.
const formatLogTime = (iso) => {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const now = new Date();
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  const hm = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  return sameDay
    ? hm
    : `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${hm}`;
};

// Durations past one second read better in seconds.
const formatDuration = (ms) => (ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${ms} ms`);

// Real decoding / generation speed (tokens per second)
const getLogTPS = (log) => {
  if (!log.completion_tokens || log.completion_tokens <= 0) return '-';
  const durationMs = log.duration_ms || 0;
  const ttftMs = log.ttft_ms || 0;
  const genDurationMs = (durationMs > ttftMs && ttftMs > 0) ? (durationMs - ttftMs) : durationMs;
  if (genDurationMs <= 0) return '-';
  const tps = (log.completion_tokens * 1000.0) / genDurationMs;
  return tps.toFixed(1);
};

// Prompt tokens excluding cached prefix tokens
const getUncachedPrompt = (log) => {
  const prompt = log.prompt_tokens || 0;
  const cached = log.cached_tokens || 0;
  return prompt >= cached ? prompt - cached : prompt;
};

export default function LogsView({
  sessionFilter,
  setSessionFilter,
  logFilter,
  setLogFilter,
  logFilters,
  setLogFilters,
  fetchLogs,
  logLoading,
  handleClearLogs,
  timeRange,
  setTimeRange,
  customStartTime,
  setCustomStartTime,
  customEndTime,
  setCustomEndTime,
  selectedLogIds,
  setSelectedLogIds,
  handleBatchDeleteLogs,
  filteredLogs = [],
  logsLength = 0,
  logOffset = 0,
  logTotal = 0,
  onExportAll,
  logPageSize = 50,
  onLogPageChange,
  setActiveLogDetail,
  showToast,
  handleDeleteSingleLog,
  isAdmin = false,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [copiedId, setCopiedId] = useState('');
  const [autoRefreshInterval, setAutoRefreshInterval] = useState(10); // 0 (off), 5, 10, 30
  const [showMoreFilters, setShowMoreFilters] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);

  // The combined facet filters are owned by App.jsx: it is the component that
  // builds the server-side query (and persists them in the URL under `logs.*`).
  // Keeping a second useUrlFilters instance here meant App never saw the values,
  // so model/channel/status/cache/latency only trimmed the already-loaded page.
  const filters = logFilters || {
    model: 'all',
    channel: 'all',
    status: 'all',
    cache: 'all',
    latency: 'all',
  };
  const setFilters = setLogFilters || (() => {});
  const resetUrlFilters = () => setFilters({
    model: 'all',
    channel: 'all',
    status: 'all',
    cache: 'all',
    latency: 'all',
  });
  const {
    model: modelFilter,
    channel: channelFilter,
    status: statusFilter, // 'all' | 'success' | 'error'
    cache: cacheFilter, // 'all' | 'cached' | 'uncached'
    latency: latencyFilter, // 'all' | 'fast' | 'slow'
  } = filters;

  const setModelFilter = (value) => setFilters({ model: value });
  const setChannelFilter = (value) => setFilters({ channel: value });
  const setStatusFilter = (value) => setFilters({ status: value });
  const setCacheFilter = (value) => setFilters({ cache: value });
  const setLatencyFilter = (value) => setFilters({ latency: value });

  // The keyword search is owned by App.jsx (which does the filtering and keeps
  // `logs.q` in the URL), so there is nothing to mirror here.

  // Handle auto-refresh interval.
  //
  // fetchLogs is a plain function defined by App, so its identity changes on
  // every App render (the 8s data poll re-renders constantly). Depending on it
  // here restarted the timer before it could ever fire, which silently disabled
  // auto-refresh. Keep it in a ref and depend only on the chosen interval.
  const fetchLogsRef = useRef(fetchLogs);
  useEffect(() => {
    fetchLogsRef.current = fetchLogs;
  }, [fetchLogs]);
  useEffect(() => {
    if (!autoRefreshInterval || autoRefreshInterval <= 0) return undefined;
    const timer = setInterval(() => {
      fetchLogsRef.current();
    }, autoRefreshInterval * 1000);
    return () => clearInterval(timer);
  }, [autoRefreshInterval]);

  // Close the overflow menu on Escape (a backdrop handles outside clicks).
  useEffect(() => {
    if (!menuOpen) return undefined;
    const onKeyDown = (e) => {
      if (e.key === 'Escape') setMenuOpen(false);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [menuOpen]);

  const copyWithFeedback = async (text, type = 'Trace ID') => {
    if (!text) return;
    await copyToClipboard(text);
    setCopiedId(text);
    setTimeout(() => setCopiedId(''), 2000);
    if (showToast) showToast(isZh ? `${type} 已复制到剪贴板` : `${type} copied to clipboard`, 'success');
  };

  // Extract distinct models and channels from filteredLogs for multi-condition dropdowns
  const availableModels = useMemo(() => {
    const set = new Set();
    (filteredLogs || []).forEach(l => { if (l.model) set.add(l.model); });
    return Array.from(set).sort();
  }, [filteredLogs]);

  const availableChannels = useMemo(() => {
    const set = new Set();
    (filteredLogs || []).forEach(l => { if (l.channel) set.add(l.channel); });
    return Array.from(set).sort();
  }, [filteredLogs]);

  // Multi-condition combined filtering across model, channel, status, cache, and latency
  const displayedLogs = useMemo(() => {
    return (filteredLogs || []).filter((log) => {
      const code = log.status_code || 200;

      // 1. Status Filter
      if (statusFilter === 'success' && (code >= 400 || log.error_message)) return false;
      if (statusFilter === 'error' && code < 400 && !log.error_message) return false;

      // 2. Model Filter
      if (modelFilter !== 'all' && log.model !== modelFilter) return false;

      // 3. Channel Filter
      if (channelFilter !== 'all' && log.channel !== channelFilter) return false;

      // 4. Cache Filter
      if (cacheFilter === 'cached' && (log.cached_tokens || 0) <= 0) return false;
      if (cacheFilter === 'uncached' && (log.cached_tokens || 0) > 0) return false;

      // 5. Latency Filter
      if (latencyFilter === 'fast' && (log.duration_ms || 0) >= 500) return false;
      if (latencyFilter === 'slow' && (log.duration_ms || 0) < 2000) return false;

      return true;
    });
  }, [filteredLogs, statusFilter, modelFilter, channelFilter, cacheFilter, latencyFilter]);

  const hasActiveFilters = modelFilter !== 'all' ||
    channelFilter !== 'all' ||
    statusFilter !== 'all' ||
    cacheFilter !== 'all' ||
    latencyFilter !== 'all' ||
    Boolean(logFilter) ||
    Boolean(sessionFilter) ||
    timeRange !== 'all';

  // Count of filters that live in the collapsible panel (shown on its toggle, so
  // a collapsed panel never hides the fact that rows are being filtered out).
  const activeFilterCount = [
    modelFilter !== 'all',
    channelFilter !== 'all',
    statusFilter !== 'all',
    cacheFilter !== 'all',
    latencyFilter !== 'all',
    timeRange !== 'all',
  ].filter(Boolean).length;

  // Every active condition, including the keyword and the parent-owned trace lock.
  const totalActiveFilterCount = activeFilterCount + (logFilter ? 1 : 0) + (sessionFilter ? 1 : 0);

  const timeRangeLabels = {
    '1h': isZh ? '近 1 小时' : 'Last 1h',
    today: isZh ? '今天' : 'Today',
    '7d': isZh ? '近 7 天' : 'Last 7d',
    custom: isZh ? '自定义区间' : 'Custom range',
  };

  const handleResetFilters = () => {
    resetUrlFilters();
    setLogFilter('');
    const needsServerRefetch = Boolean(sessionFilter) || timeRange !== 'all';
    if (sessionFilter) setSessionFilter('');
    if (timeRange !== 'all') setTimeRange('all');
    if (needsServerRefetch) fetchLogs({ timeRange: 'all', sessionFilter: '' });
  };

  // Export the WHOLE filtered result set to CSV.
  //
  // Exporting `displayedLogs` only wrote the rows of the current page, so a user
  // who filtered down to "all errors this week" silently received 50 rows. We now
  // ask App.jsx to page through the server-side filter and hand back every match.
  const handleExportCSV = async () => {
    let exportRows = displayedLogs;
    if (onExportAll) {
      try {
        exportRows = await onExportAll();
      } catch (e) {
        if (showToast) showToast((isZh ? '导出失败: ' : 'Export failed: ') + e.message, 'warning');
        return;
      }
    }
    if (!exportRows || exportRows.length === 0) {
      if (showToast) showToast(isZh ? '当前没有可导出的日志记录' : 'No logs available for export', 'warning');
      return;
    }

    const headers = isZh ? [
      'ID',
      '请求时间',
      'Trace ID',
      '会话 ID',
      '对话 ID',
      '请求模型',
      '命中渠道',
      '租户/密钥',
      '未缓存输入Token',
      '输出Token',
      '缓存Token',
      '生成速率(TPS)',
      '扣减费用(元)',
      '耗时(ms)',
      '首字时延(ms)',
      'HTTP状态码',
      '异常说明'
    ] : [
      'ID',
      'Request Time',
      'Trace ID',
      'Session ID',
      'Chat ID',
      'Model',
      'Channel Provider',
      'Tenant/Key',
      'Uncached Prompt Tokens',
      'Completion Tokens',
      'Cached Tokens',
      'Tokens Per Second (TPS)',
      'Cost (CNY)',
      'Duration (ms)',
      'TTFT (ms)',
      'HTTP Status',
      'Error Message'
    ];

    const rows = exportRows.map((l) => [
      l.id,
      l.created_at || '',
      l.trace_id || '',
      l.session_id || '',
      l.chat_id || '',
      l.model || '',
      l.channel || '',
      l.tenant_id || '',
      getUncachedPrompt(l),
      l.completion_tokens || 0,
      l.cached_tokens || 0,
      getLogTPS(l),
      (l.cost || 0).toFixed(4),
      l.duration_ms || 0,
      l.ttft_ms || 0,
      l.status_code || 200,
      l.error_message || ''
    ]);

    const csvContent = toCSV(headers, rows);
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `airoute-audit-logs-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);

    if (showToast) showToast(isZh ? `成功导出 ${exportRows.length} 条审计日志（全部匹配记录）` : `Exported ${exportRows.length} matching audit logs`, 'success');
  };

  return (
    <div className="space-y-4">
      {/* Header and Controls (Directly on canvas) */}
      <div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-white">
              {isZh ? '对话审计与请求日志' : 'Audit Logs & Traces'}
            </h1>
            {autoRefreshInterval > 0 && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-xs font-mono font-medium bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200/60 dark:border-emerald-800/60">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                <span>{autoRefreshInterval}s {isZh ? '轮询中' : 'polling'}</span>
              </span>
            )}
          </div>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">
            {isZh
              ? '全链路 Trace 追溯、Token 消耗计量与企业安全审计留存'
              : 'End-to-end trace auditing, token metering, and enterprise compliance logs'}
          </p>
        </div>

        {/* Primary controls stay visible: keyword search + refresh. Export, clear
            logs and the heavier filter selects live in the panel / overflow menu. */}
        <div className="flex flex-wrap items-center gap-2 w-full lg:w-auto">
          {/* Active Chat/Session/Trace Filter Tag */}
          {sessionFilter && (
            <div className="flex items-center space-x-2 px-3 py-1.5 rounded-lg bg-indigo-50/80 dark:bg-indigo-950/40 border border-indigo-200/80 dark:border-indigo-800 text-xs text-indigo-700 dark:text-indigo-300 animate-in fade-in">
              <span className="text-xs text-slate-500 dark:text-slate-400">{isZh ? '已锁定链路:' : 'Locked Trace:'}</span>
              <span className="font-mono font-semibold max-w-[140px] truncate">{sessionFilter}</span>
              <button
                onClick={() => {
                  setSessionFilter('');
                  fetchLogs({ sessionFilter: '' });
                }}
                className="hover:text-rose-500 ml-1 font-semibold transition cursor-pointer"
                title={isZh ? '清除筛选' : 'Clear filter'}
              >
                ✕
              </button>
            </div>
          )}

          {/* Keyword Search Input */}
          <div className="relative flex-1 min-w-[180px] sm:w-64">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={logFilter}
              onChange={(e) => setLogFilter(e.target.value)}
              placeholder={isZh ? '筛选 Trace / 会话 / 模型 / 渠道...' : 'Filter trace, session, model, channel...'}
              className="ui-field ui-field-sm pl-9"
            />
          </div>

          {/* Manual Refresh Button */}
          <button
            onClick={() => fetchLogs()}
            disabled={logLoading}
            className="ui-btn ui-btn-secondary ui-btn-sm shrink-0"
            title={isZh ? '立即刷新日志' : 'Refresh logs now'}
          >
            <RefreshCw className={`w-3.5 h-3.5 text-slate-400 ${logLoading ? 'animate-spin' : ''}`} />
            <span>{isZh ? '刷新' : 'Refresh'}</span>
          </button>

          {/* Collapsible panel for the heavier filter controls */}
          <button
            onClick={() => setShowMoreFilters((v) => !v)}
            aria-expanded={showMoreFilters}
            className={`ui-btn ui-btn-secondary ui-btn-sm shrink-0 ${
              showMoreFilters || activeFilterCount > 0
                ? 'border-indigo-200 dark:border-indigo-800 text-indigo-700 dark:text-indigo-300'
                : ''
            }`}
            title={isZh ? '时间跨度、模型、渠道、状态、缓存与耗时筛选' : 'Time range, model, channel, status, cache and latency filters'}
          >
            <SlidersHorizontal className="w-3.5 h-3.5 text-slate-400" />
            <span>{isZh ? '更多筛选' : 'More filters'}</span>
            {activeFilterCount > 0 && (
              <span className="rounded-full bg-indigo-100 dark:bg-indigo-950/70 px-1.5 text-xs font-semibold text-indigo-700 dark:text-indigo-300">
                {activeFilterCount}
              </span>
            )}
            <ChevronDown className={`w-3.5 h-3.5 text-slate-400 transition-transform ${showMoreFilters ? 'rotate-180' : ''}`} />
          </button>

          {/* Overflow menu: auto-refresh rate, CSV export, and the destructive clear */}
          <div className="relative shrink-0">
            <button
              onClick={() => setMenuOpen((v) => !v)}
              aria-haspopup="true"
              aria-expanded={menuOpen}
              className="ui-icon-btn border border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#0F1420] shadow-sm"
              title={isZh ? '更多操作' : 'More actions'}
            >
              <MoreHorizontal className="w-4 h-4" />
            </button>

            {menuOpen && (
              <>
                <div className="fixed inset-0 z-20" onClick={() => setMenuOpen(false)} />
                <div className="ui-menu min-w-[230px]">
                  <div className="px-2.5 py-1.5">
                    <label htmlFor="logs-auto-refresh" className="block mb-1 text-xs font-medium text-slate-500 dark:text-slate-400">
                      {isZh ? '自动刷新频率' : 'Auto-refresh'}
                    </label>
                    <select
                      id="logs-auto-refresh"
                      value={autoRefreshInterval}
                      onChange={(e) => setAutoRefreshInterval(Number(e.target.value))}
                      className="ui-field ui-field-sm cursor-pointer"
                      title={isZh ? '选择自动刷新频率' : 'Select refresh interval'}
                    >
                      <option value={0}>{isZh ? '暂停自动刷新' : 'Pause Auto-Refresh'}</option>
                      <option value={5}>{isZh ? '每 5 秒刷新' : 'Every 5s'}</option>
                      <option value={10}>{isZh ? '每 10 秒刷新' : 'Every 10s'}</option>
                      <option value={30}>{isZh ? '每 30 秒刷新' : 'Every 30s'}</option>
                    </select>
                  </div>

                  <div className="my-1 border-t border-slate-100 dark:border-slate-800" />

                  <button
                    onClick={() => {
                      setMenuOpen(false);
                      handleExportCSV();
                    }}
                    className="ui-menu-item"
                    title={isZh ? '导出当前筛选结果为 CSV 审计报表' : 'Export current filtered results as CSV'}
                  >
                    <FileSpreadsheet className="w-3.5 h-3.5 text-slate-400" />
                    <span>{isZh ? '导出 CSV 报表' : 'Export CSV'}</span>
                  </button>

                  <div className="my-1 border-t border-slate-100 dark:border-slate-800" />

                  {/* Destructive action, visually separated from the rest. Only
                      super-admins may clear the audit trail. */}
                  {isAdmin && (
                    <button
                      onClick={() => {
                        setMenuOpen(false);
                        handleClearLogs();
                      }}
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs font-medium text-rose-600 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-950/40 transition cursor-pointer"
                      title={isZh ? '清空全部审计调用日志（需确认）' : 'Clear all audit logs (ask for confirmation)'}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      <span>{isZh ? '清空全部日志' : 'Clear all logs'}</span>
                    </button>
                  )}
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      {/* Collapsible filter panel: time range plus the five combined selects */}
      {showMoreFilters && (
        <div className="p-4 bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-2xl shadow-xs space-y-3 animate-in fade-in">
          {/* Row 1: Time Range and Matching Count */}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="ui-meta font-medium flex items-center space-x-1">
                <Clock className="w-3.5 h-3.5 text-slate-400" />
                <span>{isZh ? '时间跨度:' : 'Time Range:'}</span>
              </span>
              <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-lg border border-slate-200/80 dark:border-slate-800">
                {[
                  { id: 'all', label: isZh ? '全部' : 'All' },
                  { id: '1h', label: isZh ? '近 1 小时' : '1h' },
                  { id: 'today', label: isZh ? '今天' : 'Today' },
                  { id: '7d', label: isZh ? '近 7 天' : '7d' },
                  { id: 'custom', label: isZh ? '自定义' : 'Custom' },
                ].map((item) => (
                  <button
                    key={item.id}
                    onClick={() => {
                      setTimeRange(item.id);
                      if (item.id !== 'custom') {
                        fetchLogs({ timeRange: item.id });
                      }
                    }}
                    className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                      timeRange === item.id
                        ? 'bg-white dark:bg-slate-800 text-slate-900 dark:text-white shadow-xs font-semibold'
                        : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                    }`}
                  >
                    {item.label}
                  </button>
                ))}
              </div>

              {/* Custom Time Picker */}
              {timeRange === 'custom' && (
                <div className="flex items-center space-x-1.5 animate-in fade-in">
                  <input
                    type="datetime-local"
                    value={customStartTime}
                    onChange={(e) => setCustomStartTime(e.target.value)}
                    className="bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-700 dark:text-slate-200 shadow-xs"
                    title={isZh ? '开始时间' : 'Start time'}
                  />
                  <span className="ui-meta">{isZh ? '至' : 'to'}</span>
                  <input
                    type="datetime-local"
                    value={customEndTime}
                    onChange={(e) => setCustomEndTime(e.target.value)}
                    className="bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-700 dark:text-slate-200 shadow-xs"
                    title={isZh ? '结束时间' : 'End time'}
                  />
                  <button
                    onClick={() => fetchLogs({ timeRange: 'custom' })}
                    className="ui-btn ui-btn-primary ui-btn-sm"
                  >
                    {isZh ? '查询' : 'Query'}
                  </button>
                </div>
              )}
            </div>

            <div className="ui-meta font-mono">
              {isZh
                ? `本页 ${displayedLogs.length} 条${logTotal > 0 ? ` · 全库匹配 ${logTotal} 条` : ''}（搜索与筛选作用于全部记录）`
                : `${displayedLogs.length} on this page${logTotal > 0 ? ` · ${logTotal} matching in total` : ''} (search/filter cover all records)`}
            </div>
          </div>

          {/* Row 2: Combined Multi-Condition Dropdown Selectors */}
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-2.5 pt-3 border-t border-slate-100 dark:border-slate-800/60">
            {/* 1. Model Selector */}
            <div>
              <label className="block mb-1 text-xs font-medium text-slate-500 dark:text-slate-400">
                {isZh ? '请求模型' : 'Model'}
              </label>
              <select
                value={modelFilter}
                onChange={(e) => setModelFilter(e.target.value)}
                className="ui-field ui-field-sm cursor-pointer font-mono"
              >
                <option value="all">{isZh ? '全部模型' : 'All Models'}</option>
                {availableModels.map(m => (
                  <option key={m} value={m}>{m}</option>
                ))}
              </select>
            </div>

            {/* 2. Channel Selector */}
            <div>
              <label className="block mb-1 text-xs font-medium text-slate-500 dark:text-slate-400">
                {isZh ? '命中渠道' : 'Channel'}
              </label>
              <select
                value={channelFilter}
                onChange={(e) => setChannelFilter(e.target.value)}
                className="ui-field ui-field-sm cursor-pointer font-mono"
              >
                <option value="all">{isZh ? '全部服务商渠道' : 'All Channels'}</option>
                {availableChannels.map(ch => (
                  <option key={ch} value={ch}>{ch}</option>
                ))}
              </select>
            </div>

            {/* 3. HTTP Status */}
            <div>
              <label className="block mb-1 text-xs font-medium text-slate-500 dark:text-slate-400">
                {isZh ? '响应状态' : 'HTTP Status'}
              </label>
              <select
                value={statusFilter}
                onChange={(e) => setStatusFilter(e.target.value)}
                className="ui-field ui-field-sm cursor-pointer"
              >
                <option value="all">{isZh ? '全部状态 (2xx/4xx/5xx)' : 'All Statuses'}</option>
                <option value="success">{isZh ? '● 仅成功 (2xx)' : 'Success Only'}</option>
                <option value="error">{isZh ? '▲ 仅异常 (≥400)' : 'Errors Only'}</option>
              </select>
            </div>

            {/* 4. Cache Hit */}
            <div>
              <label className="block mb-1 text-xs font-medium text-slate-500 dark:text-slate-400">
                {isZh ? '前缀缓存' : 'Prompt Cache'}
              </label>
              <select
                value={cacheFilter}
                onChange={(e) => setCacheFilter(e.target.value)}
                className="ui-field ui-field-sm cursor-pointer"
              >
                <option value="all">{isZh ? '全部缓存状态' : 'All Cache'}</option>
                <option value="cached">{isZh ? '★ 命中前缀缓存 (>0)' : 'Cache Hit'}</option>
                <option value="uncached">{isZh ? '○ 未命中缓存 (=0)' : 'Cache Miss'}</option>
              </select>
            </div>

            {/* 5. Latency Filter */}
            <div>
              <label className="block mb-1 text-xs font-medium text-slate-500 dark:text-slate-400">
                {isZh ? '耗时区间' : 'Latency Range'}
              </label>
              <select
                value={latencyFilter}
                onChange={(e) => setLatencyFilter(e.target.value)}
                className="ui-field ui-field-sm cursor-pointer"
              >
                <option value="all">{isZh ? '全部耗时区间' : 'All Latencies'}</option>
                <option value="fast">{isZh ? '⚡ 极速请求 (≤1s)' : 'Fast (≤1s)'}</option>
                <option value="slow">{isZh ? '🐢 慢请求 (>1s)' : 'Slow (>1s)'}</option>
              </select>
            </div>
          </div>
        </div>
      )}

      {/* Active filter summary: always rendered when filtering, so a collapsed
          panel can never silently hide rows, with a one-click clear. */}
      {hasActiveFilters && (
        <div className="flex flex-wrap items-center justify-between gap-2 animate-in fade-in">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="ui-meta font-medium flex items-center space-x-1 mr-1">
              <Filter className="w-3.5 h-3.5 text-indigo-500" />
              <span>{isZh ? `已生效条件 (${totalActiveFilterCount}):` : `Active filters (${totalActiveFilterCount}):`}</span>
            </span>

            {modelFilter !== 'all' && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 border border-purple-200/70 dark:border-purple-800 text-xs font-mono">
                <span>{isZh ? '模型:' : 'Model:'} {modelFilter}</span>
                <button onClick={() => setModelFilter('all')} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除模型筛选' : 'Clear model filter'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {channelFilter !== 'all' && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200/70 dark:border-indigo-800 text-xs">
                <span>{isZh ? '渠道:' : 'Channel:'} {channelFilter}</span>
                <button onClick={() => setChannelFilter('all')} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除渠道筛选' : 'Clear channel filter'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {statusFilter !== 'all' && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs">
                <span>{statusFilter === 'success' ? (isZh ? '状态: 成功' : 'Status: Success') : (isZh ? '状态: 异常' : 'Status: Error')}</span>
                <button onClick={() => setStatusFilter('all')} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除状态筛选' : 'Clear status filter'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {cacheFilter !== 'all' && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border border-amber-200/70 dark:border-amber-800 text-xs">
                <span>{cacheFilter === 'cached' ? (isZh ? '命中缓存' : 'Cached') : (isZh ? '未命中' : 'Uncached')}</span>
                <button onClick={() => setCacheFilter('all')} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除缓存筛选' : 'Clear cache filter'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {latencyFilter !== 'all' && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-sky-50 dark:bg-sky-950/40 text-sky-700 dark:text-sky-300 border border-sky-200/70 dark:border-sky-800 text-xs">
                <span>{latencyFilter === 'fast' ? '<500ms' : '≥2s'}</span>
                <button onClick={() => setLatencyFilter('all')} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除耗时筛选' : 'Clear latency filter'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {timeRange !== 'all' && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs">
                <span>{isZh ? '时间:' : 'Time:'} {timeRangeLabels[timeRange] || timeRange}</span>
                <button
                  onClick={() => {
                    setTimeRange('all');
                    fetchLogs({ timeRange: 'all' });
                  }}
                  className="hover:text-rose-500 cursor-pointer"
                  title={isZh ? '清除时间筛选' : 'Clear time filter'}
                >
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {Boolean(logFilter) && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs">
                <span>"{logFilter}"</span>
                <button onClick={() => setLogFilter('')} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除关键词' : 'Clear keyword'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}

            {Boolean(sessionFilter) && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 text-xs font-mono">
                <span>{isZh ? 'Trace/会话:' : 'Trace:'} {sessionFilter}</span>
                <button onClick={() => { setSessionFilter(''); fetchLogs({ sessionFilter: '' }); }} className="hover:text-rose-500 cursor-pointer" title={isZh ? '清除链路筛选' : 'Clear trace filter'}>
                  <X className="w-3 h-3" />
                </button>
              </span>
            )}
          </div>

          <button
            onClick={handleResetFilters}
            className="ui-btn ui-btn-ghost ui-btn-sm shrink-0"
          >
            <RotateCcw className="w-3.5 h-3.5" />
            <span>{isZh ? '清除筛选' : 'Clear filters'}</span>
          </button>
        </div>
      )}

      {/* Batch Action Bar (admin-only destructive actions) */}
      {isAdmin && selectedLogIds.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50/70 dark:bg-indigo-950/30 border border-indigo-200/80 dark:border-indigo-800/60 px-5 py-3 rounded-xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
            <span>{isZh ? `已选中 ${selectedLogIds.length} 条调用日志` : `Selected ${selectedLogIds.length} log entries`}</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDeleteLogs}
              className="ui-btn ui-btn-danger ui-btn-sm"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>{isZh ? '批量删除' : 'Batch Delete'}</span>
            </button>
            <button
              onClick={() => setSelectedLogIds([])}
              className="ui-btn ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消选择' : 'Deselect All'}
            </button>
          </div>
        </div>
      )}

      {/* Logs Table Card */}
      <div className="ui-card overflow-hidden flex flex-col">
        <div className="overflow-auto max-h-[calc(100vh-320px)]">
          <table className="w-full text-left border-collapse min-w-[1080px]">
            <thead className="sticky top-0 z-20 shadow-2xs">
              <tr className="text-slate-500 dark:text-slate-400 text-xs uppercase font-medium tracking-wide">
                <th className="py-3 px-3 w-10 text-center sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">
                  {isAdmin && (
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                      checked={displayedLogs.length > 0 && selectedLogIds.length === displayedLogs.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedLogIds(displayedLogs.map((l) => l.id));
                        } else {
                          setSelectedLogIds([]);
                        }
                      }}
                    />
                  )}
                </th>
                <th className="py-3 px-3 font-semibold sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">{isZh ? '时间' : 'Time'}</th>
                <th className="py-3 px-3 font-semibold sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">{isZh ? '链路 / Trace' : 'Trace / Session'}</th>
                <th className="py-3 px-3 font-semibold sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">{isZh ? '模型 / 渠道' : 'Model / Provider'}</th>
                <th className="py-3 px-2 font-semibold text-right w-20 min-w-[65px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800" title={isZh ? '未缓存输入 Prompt Token' : 'Uncached prompt tokens'}>{isZh ? '输入' : 'Input'}</th>
                <th className="py-3 px-2 font-semibold text-right w-20 min-w-[65px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800" title={isZh ? '模型输出 Completion Token' : 'Completion tokens'}>{isZh ? '输出' : 'Output'}</th>
                <th className="py-3 px-2 font-semibold text-right w-18 min-w-[60px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800" title={isZh ? '前缀缓存命中 Token' : 'Cached prefix tokens'}>{isZh ? '缓存' : 'Cached'}</th>
                <th className="py-3 px-2 font-semibold text-right w-20 min-w-[65px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800" title={isZh ? '生成速率 (Tokens/秒)' : 'Tokens per second'}>{isZh ? 'TPS' : 'TPS'}</th>
                <th className="py-3 px-2 font-semibold text-right w-20 min-w-[65px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">{isZh ? '费用' : 'Cost'}</th>
                <th className="py-3 px-2 font-semibold text-right w-24 min-w-[80px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">{isZh ? '耗时 / TTFT' : 'Duration / TTFT'}</th>
                <th className="py-3 px-2.5 text-right font-semibold w-24 min-w-[85px] sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">{isZh ? '状态 / 操作' : 'Status / Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 text-xs">
              {displayedLogs.map((log) => {
                const isError = (log.status_code && log.status_code >= 400) || !!log.error_message;
                const uncachedPrompt = getUncachedPrompt(log);
                const tps = getLogTPS(log);

                return (
                  <tr
                    key={log.id}
                    onClick={() => setActiveLogDetail(log)}
                    className="hover:bg-slate-50/70 dark:hover:bg-slate-800/40 transition cursor-pointer group"
                  >
                    {/* 1. Checkbox (selection drives admin batch deletion) */}
                    <td className="py-3 px-3 text-center" onClick={(e) => e.stopPropagation()}>
                      {isAdmin && (
                        <input
                          type="checkbox"
                          className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                          checked={selectedLogIds.includes(log.id)}
                          onChange={(e) => {
                            e.stopPropagation();
                            if (e.target.checked) {
                              setSelectedLogIds((prev) => [...prev, log.id]);
                            } else {
                              setSelectedLogIds((prev) => prev.filter((x) => x !== log.id));
                            }
                          }}
                        />
                      )}
                    </td>

                    {/* 2. Request Time */}
                    <td className="py-3 px-3.5 text-slate-500 dark:text-slate-400 font-mono tabular-nums whitespace-nowrap" title={log.created_at}>
                      {log.created_at ? formatLogTime(log.created_at) : (isZh ? '刚刚' : 'Just now')}
                    </td>

                    {/* 3. Trace / Session */}
                    <td className="py-3 px-3.5 font-mono tabular-nums text-xs">
                      <div className="flex items-center space-x-1.5">
                        <span
                          className="text-slate-700 dark:text-slate-300 truncate max-w-[120px] font-medium"
                          title={log.trace_id}
                        >
                          {log.trace_id ? log.trace_id : <span className="text-slate-400 font-sans">-</span>}
                        </span>
                        {log.trace_id && (
                          <>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                copyWithFeedback(log.trace_id, 'Trace ID');
                              }}
                              className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md text-slate-400 hover:text-slate-600 transition"
                              title={isZh ? "复制 Trace ID" : "Copy Trace ID"}
                            >
                              {copiedId === log.trace_id ? (
                                <Check className="w-3 h-3 text-emerald-500" />
                              ) : (
                                <Copy className="w-3 h-3" />
                              )}
                            </button>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                setSessionFilter(log.trace_id);
                                fetchLogs({ sessionFilter: log.trace_id });
                                if (showToast) showToast(isZh ? `已筛选 Trace: ${log.trace_id}` : `Filtered Trace: ${log.trace_id}`, 'info');
                              }}
                              className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-md text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400 transition"
                              title={isZh ? "按 Trace ID 快速过滤" : "Quick filter by Trace ID"}
                            >
                              <Search className="w-3 h-3" />
                            </button>
                          </>
                        )}
                      </div>
                      {log.session_id && (
                        <div className="flex items-center space-x-1 mt-0.5">
                          <span
                            className="text-xs text-slate-500 dark:text-slate-400 truncate max-w-[110px]"
                            title={isZh ? `会话: ${log.session_id}` : `Session: ${log.session_id}`}
                          >
                            {isZh ? '会话:' : 'Session:'} {log.session_id}
                          </span>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              copyWithFeedback(log.session_id, isZh ? '会话 ID' : 'Session ID');
                            }}
                            className="text-slate-400 hover:text-slate-600"
                            title={isZh ? "复制会话 ID" : "Copy Session ID"}
                          >
                            <Copy className="w-2.5 h-2.5" />
                          </button>
                        </div>
                      )}
                      {log.chat_id && (
                        <span
                          className="block text-xs text-slate-500 dark:text-slate-400 truncate max-w-[120px]"
                          title={isZh ? `对话: ${log.chat_id}` : `Chat: ${log.chat_id}`}
                        >
                          {isZh ? '对话:' : 'Chat:'} {log.chat_id}
                        </span>
                      )}
                    </td>

                    {/* 4. Model / Provider */}
                    <td className="py-3 px-3.5 text-xs">
                      <span
                        className="px-2 py-0.5 bg-slate-100 dark:bg-slate-800 text-slate-800 dark:text-slate-200 rounded-md max-w-[140px] inline-block truncate align-middle font-mono font-medium"
                        title={log.model}
                      >
                        {log.model || '-'}
                      </span>
                      {log.channel && (
                        <div className="text-xs text-slate-500 dark:text-slate-400 font-mono mt-0.5 truncate max-w-[140px]">
                          {isZh ? '渠道:' : 'Provider:'} {log.channel}
                        </div>
                      )}
                    </td>

                    {/* 5. Uncached Prompt */}
                    <td className="py-3 px-2 text-right font-mono tabular-nums text-slate-700 dark:text-slate-300 font-medium whitespace-nowrap">
                      {uncachedPrompt.toLocaleString()}
                    </td>

                    {/* 6. Completion */}
                    <td className="py-3 px-2 text-right font-mono tabular-nums text-slate-700 dark:text-slate-300 font-medium whitespace-nowrap">
                      {(log.completion_tokens || 0).toLocaleString()}
                    </td>

                    {/* 7. Cached */}
                    <td className="py-3 px-2 text-right font-mono tabular-nums whitespace-nowrap">
                      {(log.cached_tokens || 0) > 0 ? (
                        <span className="text-amber-600 dark:text-amber-400 font-medium">
                          {(log.cached_tokens || 0).toLocaleString()}
                        </span>
                      ) : (
                        <span className="text-slate-400">-</span>
                      )}
                    </td>

                    {/* 8. TPS */}
                    <td className="py-3 px-2 text-right font-mono tabular-nums whitespace-nowrap">
                      {tps !== '-' ? (
                        <div className="flex items-center justify-end space-x-0.5 font-semibold">
                          <span className="text-indigo-600 dark:text-indigo-400">{tps}</span>
                          <span className="text-xs text-slate-500 dark:text-slate-400 font-normal">tok/s</span>
                        </div>
                      ) : (
                        <span className="text-slate-400">-</span>
                      )}
                    </td>

                    {/* 9. Cost */}
                    <td className="py-3 px-2 text-right font-mono tabular-nums whitespace-nowrap">
                      <div className="flex items-center justify-end space-x-1">
                        <span className="font-semibold text-slate-900 dark:text-white">
                          ¥{(log.cost || 0).toFixed(4)}
                        </span>
                        {log.is_off_peak && (
                          <span
                            className="px-1.5 py-0.2 rounded text-xs font-medium bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300"
                            title={`Off-peak ${(log.off_peak_discount || 0.5) * 100}%`}
                          >
                            {isZh ? `闲` : `Off`}
                          </span>
                        )}
                      </div>
                    </td>

                    {/* 10. Duration / TTFT */}
                    <td className="py-3 px-2 text-right font-mono tabular-nums whitespace-nowrap">
                      <div
                        className={`font-semibold ${
                          (log.duration_ms || 0) < 500
                            ? 'text-emerald-600 dark:text-emerald-400'
                            : (log.duration_ms || 0) < 2000
                            ? 'text-amber-600 dark:text-amber-400'
                            : 'text-rose-600 dark:text-rose-400'
                        }`}
                      >
                        {formatDuration(log.duration_ms ?? 0)}
                      </div>
                      <div className="text-xs font-mono text-slate-500 dark:text-slate-400">
                        TTFT: {(log.ttft_ms || 0) > 0 ? formatDuration(log.ttft_ms) : '-'}
                      </div>
                    </td>

                    {/* 11. Status & Actions */}
                    <td className="py-3 px-2.5 text-right space-x-1.5 whitespace-nowrap">
                      <span
                        className={`px-2 py-0.5 rounded-full text-xs font-mono font-medium ${
                          !isError
                            ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                            : (log.status_code || 200) < 500
                            ? 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800'
                            : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800'
                        }`}
                      >
                        {log.status_code || 200}
                      </span>
                      {log.error_message && (
                        <span
                          className="inline-block align-middle max-w-[150px] truncate text-xs text-amber-600 dark:text-amber-400"
                          title={log.error_message}
                        >
                          {log.error_message}
                        </span>
                      )}
                      <span className="text-xs text-slate-500 dark:text-slate-400 group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition font-medium">
                        {isZh ? '详情 →' : 'Detail →'}
                      </span>
                      {isAdmin && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteSingleLog(log.id);
                          }}
                          className="p-1 text-slate-400 hover:text-rose-600 transition rounded-md hover:bg-rose-50 dark:hover:bg-rose-950/50 inline-flex items-center align-middle cursor-pointer"
                          title={isZh ? '删除此记录' : 'Delete log'}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
              {displayedLogs.length === 0 && (
                <tr>
                  <td colSpan="11" className="py-12 text-center font-sans">
                    <div className="flex flex-col items-center justify-center space-y-2">
                      {hasActiveFilters ? (
                        <>
                          <FilterX className="w-8 h-8 text-slate-300 dark:text-slate-600 stroke-1" />
                          <span className="ui-meta font-medium">
                            {isZh ? '没有日志匹配当前筛选条件' : 'No logs match the current filters'}
                          </span>
                          <span className="ui-meta">
                            {isZh ? '试着放宽条件，或清除筛选后重新查看全部日志。' : 'Loosen the conditions, or clear the filters to see every log again.'}
                          </span>
                          <button
                            onClick={handleResetFilters}
                            className="ui-btn ui-btn-secondary ui-btn-sm"
                          >
                            <RotateCcw className="w-3.5 h-3.5" />
                            <span>{isZh ? '清除筛选' : 'Clear filters'}</span>
                          </button>
                        </>
                      ) : (
                        <>
                          <History className="w-8 h-8 text-slate-300 dark:text-slate-600 stroke-1" />
                          <span className="ui-meta font-medium">
                            {isZh ? '暂无审计调用日志' : 'No audit logs yet'}
                          </span>
                          <span className="ui-meta">
                            {isZh ? '发起一次对话请求后，这里会出现对应的链路记录。' : 'Trace records will appear here after the first chat request.'}
                          </span>
                          <button
                            onClick={() => fetchLogs()}
                            disabled={logLoading}
                            className="ui-btn ui-btn-secondary ui-btn-sm"
                          >
                            <RefreshCw className={`w-3.5 h-3.5 text-slate-400 ${logLoading ? 'animate-spin' : ''}`} />
                            <span>{isZh ? '刷新' : 'Refresh'}</span>
                          </button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Server-side pagination */}
        <div className="flex items-center justify-between p-4 border-t border-slate-100 dark:border-slate-800/80">
          <span className="text-xs text-slate-500 dark:text-slate-400 font-mono tabular-nums">
            {isZh
              ? `第 ${Math.floor(logOffset / logPageSize) + 1} 页 · 本页 ${displayedLogs.length} 条${logTotal > 0 ? ` · 筛选结果共 ${logTotal} 条` : ''}`
              : `Page ${Math.floor(logOffset / logPageSize) + 1} · ${displayedLogs.length} shown${logTotal > 0 ? ` · ${logTotal} matching` : ''}`}
            {displayedLogs.length < logsLength ? (isZh ? ` (筛选后 / 累计 ${logsLength})` : ` (filtered / total ${logsLength})`) : ''}
          </span>
          <div className="flex items-center space-x-2">
            <button
              type="button"
              onClick={() => onLogPageChange(logOffset - logPageSize)}
              disabled={logLoading || logOffset <= 0}
              className="ui-btn ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '上一页' : 'Previous'}
            </button>
            <button
              type="button"
              onClick={() => onLogPageChange(logOffset + logPageSize)}
              disabled={logLoading || (logTotal > 0 ? logOffset + logPageSize >= logTotal : logsLength < logPageSize)}
              className="ui-btn ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '下一页' : 'Next'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
