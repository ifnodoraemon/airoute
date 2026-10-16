import React, { useState, useEffect, useMemo } from 'react';
import {
  History,
  Search,
  RefreshCw,
  Trash2,
  Clock,
  Copy,
  Check,
  Download,
  CheckCircle2,
  AlertCircle,
  FileSpreadsheet,
  Filter,
  X,
  RotateCcw,
  Sparkles,
  Cpu,
  Layers
} from 'lucide-react';
import { copyToClipboard } from '../config';

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
  logPageSize = 50,
  onLogPageChange,
  setActiveLogDetail,
  showToast,
  handleDeleteSingleLog,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [copiedId, setCopiedId] = useState('');
  const [autoRefreshInterval, setAutoRefreshInterval] = useState(0); // 0 (off), 5, 10, 30

  // Multi-condition combined filter states
  const [modelFilter, setModelFilter] = useState('all');
  const [channelFilter, setChannelFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all'); // 'all' | 'success' | 'error'
  const [cacheFilter, setCacheFilter] = useState('all'); // 'all' | 'cached' | 'uncached'
  const [latencyFilter, setLatencyFilter] = useState('all'); // 'all' | 'fast' | 'slow'

  // Handle auto-refresh interval
  useEffect(() => {
    if (!autoRefreshInterval || autoRefreshInterval <= 0) return;
    const timer = setInterval(() => {
      fetchLogs();
    }, autoRefreshInterval * 1000);
    return () => clearInterval(timer);
  }, [autoRefreshInterval, fetchLogs]);

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
    Boolean(sessionFilter);

  const handleResetFilters = () => {
    setModelFilter('all');
    setChannelFilter('all');
    setStatusFilter('all');
    setCacheFilter('all');
    setLatencyFilter('all');
    setLogFilter('');
    if (sessionFilter) {
      setSessionFilter('');
      fetchLogs({ sessionFilter: '' });
    }
  };

  // Export filtered logs to CSV
  const handleExportCSV = () => {
    if (!displayedLogs || displayedLogs.length === 0) {
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

    const rows = displayedLogs.map((l) => [
      l.id,
      `"${l.created_at || ''}"`,
      `"${l.trace_id || ''}"`,
      `"${l.session_id || ''}"`,
      `"${l.chat_id || ''}"`,
      `"${l.model || ''}"`,
      `"${l.channel || ''}"`,
      `"${l.tenant_id || ''}"`,
      getUncachedPrompt(l),
      l.completion_tokens || 0,
      l.cached_tokens || 0,
      getLogTPS(l),
      (l.cost || 0).toFixed(4),
      l.duration_ms || 0,
      l.ttft_ms || 0,
      l.status_code || 200,
      `"${(l.error_message || '').replace(/"/g, '""')}"`
    ]);

    const csvContent = '\uFEFF' + [headers.join(','), ...rows.map((r) => r.join(','))].join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `airoute-audit-logs-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);

    if (showToast) showToast(isZh ? `成功导出 ${displayedLogs.length} 条审计日志` : `Successfully exported ${displayedLogs.length} audit logs`, 'success');
  };

  return (
    <div className="space-y-8">
      {/* Header and Controls (Directly on canvas) */}
      <div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-white">
              {isZh ? '对话审计与请求日志' : 'Audit Logs & Traces'}
            </h1>
            {autoRefreshInterval > 0 && (
              <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[11px] font-mono font-medium bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200/60 dark:border-emerald-800/60">
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

        {/* Actions & Search */}
        <div className="flex flex-wrap items-center gap-2.5 w-full lg:w-auto">
          {/* Active Chat/Session/Trace Filter Tag */}
          {sessionFilter && (
            <div className="flex items-center space-x-2 px-3 py-1.5 rounded-lg bg-indigo-50/80 dark:bg-indigo-950/40 border border-indigo-200/80 dark:border-indigo-800 text-xs text-indigo-700 dark:text-indigo-300 animate-in fade-in">
              <span className="text-[11px] text-slate-400">{isZh ? '已锁定链路:' : 'Locked Trace:'}</span>
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
          <div className="relative flex-1 sm:w-64">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={logFilter}
              onChange={(e) => setLogFilter(e.target.value)}
              placeholder={isZh ? '筛选 Trace / 会话 / 模型 / 渠道...' : 'Filter trace, session, model, channel...'}
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg pl-9 pr-3 py-1.5 text-xs text-slate-800 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:border-indigo-500 transition shadow-sm"
            />
          </div>

          {/* Auto-Refresh Dropdown */}
          <div className="relative">
            <select
              value={autoRefreshInterval}
              onChange={(e) => setAutoRefreshInterval(Number(e.target.value))}
              className="bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200 border border-slate-200/80 dark:border-slate-800 rounded-lg px-2.5 py-1.5 text-xs font-medium focus:outline-none cursor-pointer transition shadow-sm"
              title={isZh ? '选择自动刷新频率' : 'Select refresh interval'}
            >
              <option value={0}>{isZh ? '暂停自动刷新' : 'Pause Auto-Refresh'}</option>
              <option value={5}>{isZh ? '每 5 秒刷新' : 'Every 5s'}</option>
              <option value={10}>{isZh ? '每 10 秒刷新' : 'Every 10s'}</option>
              <option value={30}>{isZh ? '每 30 秒刷新' : 'Every 30s'}</option>
            </select>
          </div>

          {/* Manual Refresh Button */}
          <button
            onClick={() => fetchLogs()}
            disabled={logLoading}
            className="px-3.5 py-1.5 bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200 rounded-lg text-xs font-medium flex items-center space-x-1.5 transition border border-slate-200/80 dark:border-slate-800 cursor-pointer shadow-sm disabled:opacity-50"
          >
            <RefreshCw className={`w-3.5 h-3.5 text-slate-400 ${logLoading ? 'animate-spin' : ''}`} />
            <span>{isZh ? '刷新' : 'Refresh'}</span>
          </button>

          {/* Export CSV Button */}
          <button
            onClick={handleExportCSV}
            className="px-3 py-1.5 bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-200 rounded-lg text-xs font-medium flex items-center space-x-1.5 transition border border-slate-200/80 dark:border-slate-800 cursor-pointer shadow-sm"
            title={isZh ? '导出当前筛选结果为 CSV 审计报表' : 'Export current filtered results as CSV'}
          >
            <FileSpreadsheet className="w-3.5 h-3.5 text-slate-400" />
            <span>{isZh ? '导出报表' : 'Export CSV'}</span>
          </button>

          {/* Clear All Logs Button */}
          <button
            onClick={handleClearLogs}
            className="px-3 py-1.5 bg-white dark:bg-[#0F1420] hover:bg-rose-50 dark:hover:bg-rose-950/40 text-rose-600 dark:text-rose-400 border border-slate-200/80 dark:border-slate-800 hover:border-rose-200 dark:hover:border-rose-900/60 rounded-lg text-xs font-medium flex items-center space-x-1.5 transition cursor-pointer shadow-sm"
            title={isZh ? '清空全部审计调用日志' : 'Clear all audit logs'}
          >
            <Trash2 className="w-3.5 h-3.5" />
            <span>{isZh ? '清空' : 'Clear'}</span>
          </button>
        </div>
      </div>

      {/* Multi-Condition Combined Filter Bar */}
      <div className="p-4 bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-2xl shadow-xs space-y-3">
        {/* Row 1: Time Range and Matching Count */}
        <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-slate-500 dark:text-slate-400 font-medium flex items-center space-x-1">
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
                <span className="text-slate-400">{isZh ? '至' : 'to'}</span>
                <input
                  type="datetime-local"
                  value={customEndTime}
                  onChange={(e) => setCustomEndTime(e.target.value)}
                  className="bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-2 py-1 text-xs text-slate-700 dark:text-slate-200 shadow-xs"
                  title={isZh ? '结束时间' : 'End time'}
                />
                <button
                  onClick={() => fetchLogs({ timeRange: 'custom' })}
                  className="px-2.5 py-1 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg font-medium transition cursor-pointer shadow-xs"
                >
                  {isZh ? '查询' : 'Query'}
                </button>
              </div>
            )}
          </div>

          <div className="text-xs text-slate-500 dark:text-slate-400 font-mono">
            {isZh
              ? `匹配到 ${displayedLogs.length} / 共 ${filteredLogs.length} 条记录`
              : `Showing ${displayedLogs.length} of ${filteredLogs.length} logs`}
          </div>
        </div>

        {/* Row 2: Combined Multi-Condition Dropdown Selectors */}
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-2.5 text-xs pt-1 border-t border-slate-100 dark:border-slate-800/60">
          {/* 1. Model Selector */}
          <div>
            <label className="block text-[11px] text-slate-500 dark:text-slate-400 mb-1 font-medium">
              {isZh ? '请求模型' : 'Model'}
            </label>
            <select
              value={modelFilter}
              onChange={(e) => setModelFilter(e.target.value)}
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-300 shadow-2xs focus:outline-none focus:border-indigo-500 cursor-pointer font-mono"
            >
              <option value="all">{isZh ? '全部模型' : 'All Models'}</option>
              {availableModels.map(m => (
                <option key={m} value={m}>{m}</option>
              ))}
            </select>
          </div>

          {/* 2. Channel Selector */}
          <div>
            <label className="block text-[11px] text-slate-500 dark:text-slate-400 mb-1 font-medium">
              {isZh ? '命中渠道' : 'Channel'}
            </label>
            <select
              value={channelFilter}
              onChange={(e) => setChannelFilter(e.target.value)}
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-300 shadow-2xs focus:outline-none focus:border-indigo-500 cursor-pointer font-mono"
            >
              <option value="all">{isZh ? '全部服务商渠道' : 'All Channels'}</option>
              {availableChannels.map(ch => (
                <option key={ch} value={ch}>{ch}</option>
              ))}
            </select>
          </div>

          {/* 3. HTTP Status */}
          <div>
            <label className="block text-[11px] text-slate-500 dark:text-slate-400 mb-1 font-medium">
              {isZh ? '响应状态' : 'HTTP Status'}
            </label>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-300 shadow-2xs focus:outline-none focus:border-indigo-500 cursor-pointer"
            >
              <option value="all">{isZh ? '全部状态 (2xx/4xx/5xx)' : 'All Statuses'}</option>
              <option value="success">{isZh ? '● 仅成功 (2xx)' : 'Success Only'}</option>
              <option value="error">{isZh ? '▲ 仅异常 (≥400)' : 'Errors Only'}</option>
            </select>
          </div>

          {/* 4. Cache Hit */}
          <div>
            <label className="block text-[11px] text-slate-500 dark:text-slate-400 mb-1 font-medium">
              {isZh ? '前缀缓存' : 'Prompt Cache'}
            </label>
            <select
              value={cacheFilter}
              onChange={(e) => setCacheFilter(e.target.value)}
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-300 shadow-2xs focus:outline-none focus:border-indigo-500 cursor-pointer"
            >
              <option value="all">{isZh ? '全部缓存状态' : 'All Cache'}</option>
              <option value="cached">{isZh ? '★ 命中前缀缓存 (>0)' : 'Cache Hit'}</option>
              <option value="uncached">{isZh ? '○ 未命中缓存 (=0)' : 'Cache Miss'}</option>
            </select>
          </div>

          {/* 5. Latency Filter */}
          <div>
            <label className="block text-[11px] text-slate-500 dark:text-slate-400 mb-1 font-medium">
              {isZh ? '耗时区间' : 'Latency Range'}
            </label>
            <select
              value={latencyFilter}
              onChange={(e) => setLatencyFilter(e.target.value)}
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-2.5 py-1.5 text-xs text-slate-700 dark:text-slate-300 shadow-2xs focus:outline-none focus:border-indigo-500 cursor-pointer"
            >
              <option value="all">{isZh ? '全部耗时区间' : 'All Latencies'}</option>
              <option value="fast">{isZh ? '⚡ 极速请求 (<500ms)' : 'Fast (<500ms)'}</option>
              <option value="slow">{isZh ? '🐢 慢请求 (≥2000ms)' : 'Slow (≥2s)'}</option>
            </select>
          </div>
        </div>

        {/* Row 3: Active Filters Chips Bar (Shown when any filter is active) */}
        {hasActiveFilters && (
          <div className="flex flex-wrap items-center justify-between gap-2 pt-2 border-t border-slate-100 dark:border-slate-800/60 animate-in fade-in">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[11px] text-slate-400 font-medium flex items-center space-x-1 mr-1">
                <Filter className="w-3 h-3 text-indigo-500" />
                <span>{isZh ? '已生效条件:' : 'Active Filters:'}</span>
              </span>

              {modelFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 border border-purple-200/70 dark:border-purple-800 text-[11px] font-mono">
                  <span>{isZh ? '模型:' : 'Model:'} {modelFilter}</span>
                  <button onClick={() => setModelFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {channelFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200/70 dark:border-indigo-800 text-[11px]">
                  <span>{isZh ? '渠道:' : 'Channel:'} {channelFilter}</span>
                  <button onClick={() => setChannelFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {statusFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 text-[11px]">
                  <span>{statusFilter === 'success' ? (isZh ? '状态: 成功' : 'Status: Success') : (isZh ? '状态: 异常' : 'Status: Error')}</span>
                  <button onClick={() => setStatusFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {cacheFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border border-amber-200/70 dark:border-amber-800 text-[11px]">
                  <span>{cacheFilter === 'cached' ? (isZh ? '命中缓存' : 'Cached') : (isZh ? '未命中' : 'Uncached')}</span>
                  <button onClick={() => setCacheFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {latencyFilter !== 'all' && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-sky-50 dark:bg-sky-950/40 text-sky-700 dark:text-sky-300 border border-sky-200/70 dark:border-sky-800 text-[11px]">
                  <span>{latencyFilter === 'fast' ? '<500ms' : '≥2s'}</span>
                  <button onClick={() => setLatencyFilter('all')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {Boolean(logFilter) && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 text-[11px]">
                  <span>"{logFilter}"</span>
                  <button onClick={() => setLogFilter('')} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}

              {Boolean(sessionFilter) && (
                <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border border-indigo-200 text-[11px] font-mono">
                  <span>Trace/会话: {sessionFilter}</span>
                  <button onClick={() => { setSessionFilter(''); fetchLogs({ sessionFilter: '' }); }} className="hover:text-rose-500 cursor-pointer">
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              )}
            </div>

            <button
              onClick={handleResetFilters}
              className="text-xs text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 font-medium inline-flex items-center space-x-1 cursor-pointer transition"
            >
              <RotateCcw className="w-3 h-3" />
              <span>{isZh ? '重置所有筛选' : 'Reset All'}</span>
            </button>
          </div>
        )}
      </div>

      {/* Batch Action Bar */}
      {selectedLogIds.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50/70 dark:bg-indigo-950/30 border border-indigo-200/80 dark:border-indigo-800/60 px-5 py-3 rounded-xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
            <span>{isZh ? `已选中 ${selectedLogIds.length} 条调用日志` : `Selected ${selectedLogIds.length} log entries`}</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDeleteLogs}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-medium rounded-lg shadow-sm transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>{isZh ? '批量删除' : 'Batch Delete'}</span>
            </button>
            <button
              onClick={() => setSelectedLogIds([])}
              className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-lg hover:bg-slate-50 transition cursor-pointer"
            >
              {isZh ? '取消选择' : 'Deselect All'}
            </button>
          </div>
        </div>
      )}

      {/* Logs Table Card */}
      <div className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-2xl overflow-hidden shadow-xs flex flex-col">
        <div className="overflow-auto max-h-[calc(100vh-380px)]">
          <table className="w-full text-left border-collapse min-w-[1080px]">
            <thead className="sticky top-0 z-20 shadow-2xs">
              <tr className="text-slate-500 dark:text-slate-400 text-xs uppercase font-medium tracking-wide">
                <th className="py-3 px-3 w-10 text-center sticky top-0 z-20 bg-slate-50 dark:bg-[#0F1420] border-b border-slate-200 dark:border-slate-800">
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
                    {/* 1. Checkbox */}
                    <td className="py-3 px-3 text-center" onClick={(e) => e.stopPropagation()}>
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
                            className="text-[10px] text-slate-400 truncate max-w-[110px]"
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
                          className="block text-[10px] text-slate-400 truncate max-w-[120px]"
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
                        <div className="text-[10px] text-slate-400 font-mono mt-0.5 truncate max-w-[140px]">
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
                          <span className="text-[10px] text-slate-400 font-normal">tok/s</span>
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
                            className="px-1.5 py-0.2 rounded text-[10px] font-medium bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300"
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
                      {log.ttft_ms > 0 && (
                        <div className="text-[10px] text-slate-400 font-mono">
                          TTFT: {formatDuration(log.ttft_ms)}
                        </div>
                      )}
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
                      <span className="text-xs text-slate-500 dark:text-slate-400 group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition font-medium">
                        {isZh ? '详情 →' : 'Detail →'}
                      </span>
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
                    </td>
                  </tr>
                );
              })}
              {displayedLogs.length === 0 && (
                <tr>
                  <td colSpan="11" className="py-12 text-center text-slate-400 font-sans">
                    <div className="flex flex-col items-center justify-center space-y-2">
                      <History className="w-8 h-8 text-slate-300 dark:text-slate-600 stroke-1" />
                      <span>{isZh ? '暂无匹配的审计调用记录' : 'No matching audit logs found'}</span>
                      {hasActiveFilters && (
                        <button
                          onClick={handleResetFilters}
                          className="text-xs text-indigo-600 hover:underline font-medium cursor-pointer"
                        >
                          {isZh ? '重置所有联合筛选条件' : 'Reset all combined filters'}
                        </button>
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
              ? `第 ${Math.floor(logOffset / logPageSize) + 1} 页 · 本页展示 ${displayedLogs.length} 条`
              : `Page ${Math.floor(logOffset / logPageSize) + 1} · Showing ${displayedLogs.length}`}
            {displayedLogs.length < logsLength ? (isZh ? ` (筛选后 / 累计 ${logsLength})` : ` (filtered / total ${logsLength})`) : ''}
          </span>
          <div className="flex items-center space-x-2">
            <button
              type="button"
              onClick={() => onLogPageChange(logOffset - logPageSize)}
              disabled={logLoading || logOffset <= 0}
              className="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed transition cursor-pointer shadow-sm"
            >
              {isZh ? '上一页' : 'Previous'}
            </button>
            <button
              type="button"
              onClick={() => onLogPageChange(logOffset + logPageSize)}
              disabled={logLoading || logsLength < logPageSize}
              className="px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed transition cursor-pointer shadow-sm"
            >
              {isZh ? '下一页' : 'Next'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
