import React, { useState, useEffect } from 'react';
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
  FileSpreadsheet
} from 'lucide-react';

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
}) {
  const [copiedId, setCopiedId] = useState('');
  const [autoRefreshInterval, setAutoRefreshInterval] = useState(0); // 0 (off), 5, 10, 30
  const [statusFilter, setStatusFilter] = useState('all'); // 'all' | 'success' | 'error'

  // Handle auto-refresh interval
  useEffect(() => {
    if (!autoRefreshInterval || autoRefreshInterval <= 0) return;
    const timer = setInterval(() => {
      fetchLogs();
    }, autoRefreshInterval * 1000);
    return () => clearInterval(timer);
  }, [autoRefreshInterval, fetchLogs]);

  const copyWithFeedback = (text, type = 'Trace ID') => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedId(text);
    setTimeout(() => setCopiedId(''), 2000);
    if (showToast) showToast(`${type} 已复制到剪贴板`, 'success');
  };

  // Export filtered logs to CSV
  const handleExportCSV = () => {
    if (!filteredLogs || filteredLogs.length === 0) {
      if (showToast) showToast('当前没有可导出的日志记录', 'warning');
      return;
    }

    const headers = [
      'ID',
      '请求时间',
      'Trace ID',
      '会话 ID',
      '对话 ID',
      '请求模型',
      '命中服务商',
      '租户/密钥',
      '提示词Token',
      '补全Token',
      '总Token',
      '扣减费用(元)',
      '缓存命中Token',
      '耗时(ms)',
      '首字时延(ms)',
      'HTTP状态码',
      '异常说明'
    ];

    const rows = filteredLogs.map((l) => [
      l.id,
      `"${l.created_at || ''}"`,
      `"${l.trace_id || ''}"`,
      `"${l.session_id || ''}"`,
      `"${l.chat_id || ''}"`,
      `"${l.model || ''}"`,
      `"${l.channel || ''}"`,
      `"${l.tenant_id || ''}"`,
      l.prompt_tokens || 0,
      l.completion_tokens || 0,
      l.total_tokens || 0,
      (l.cost || 0).toFixed(4),
      l.cached_tokens || 0,
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

    if (showToast) showToast(`成功导出 ${filteredLogs.length} 条审计日志`, 'success');
  };

  // Apply in-memory status filter
  const displayedLogs = filteredLogs.filter((log) => {
    const code = log.status_code || 200;
    if (statusFilter === 'success') {
      return code >= 200 && code < 300 && !log.error_message;
    }
    if (statusFilter === 'error') {
      return code >= 400 || !!log.error_message;
    }
    return true;
  });

  return (
    <div className="space-y-6">
      {/* Header and Controls Card */}
      <div className="bg-white dark:bg-[#111726] p-6 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs space-y-4">
        <div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-2xl bg-sky-50 dark:bg-sky-950/60 text-sky-600 dark:text-sky-400 flex items-center justify-center border border-sky-200/60 dark:border-sky-800/60 shadow-xs">
              <History className="w-5 h-5" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h3 className="font-bold text-slate-900 dark:text-slate-100 text-sm">
                  对话审计与请求日志
                </h3>
                {autoRefreshInterval > 0 && (
                  <span className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-ping"></span>
                    <span>{autoRefreshInterval}s 轮询中</span>
                  </span>
                )}
              </div>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                全链路 Trace 追溯、Token 消耗计量与企业安全审计留存
              </p>
            </div>
          </div>

          {/* Actions & Search */}
          <div className="flex flex-wrap items-center gap-2.5 w-full lg:w-auto">
            {/* Active Chat/Session/Trace Filter Tag */}
            {sessionFilter && (
              <div className="flex items-center space-x-2 px-3 py-1.5 rounded-xl bg-purple-50 dark:bg-purple-950/60 border border-purple-200 dark:border-purple-800 text-xs text-purple-700 dark:text-purple-300 animate-in fade-in">
                <span className="text-[11px] text-slate-400">已锁定链路:</span>
                <span className="font-mono font-bold max-w-[140px] truncate">{sessionFilter}</span>
                <button
                  onClick={() => {
                    setSessionFilter('');
                    fetchLogs({ sessionFilter: '' });
                  }}
                  className="hover:text-rose-500 ml-1 font-bold transition cursor-pointer"
                  title="清除筛选"
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
                placeholder="筛选 Trace / 会话 / 模型 / 渠道..."
                className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-8 pr-3 py-2 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
              />
            </div>

            {/* Auto-Refresh Dropdown */}
            <div className="relative">
              <select
                value={autoRefreshInterval}
                onChange={(e) => setAutoRefreshInterval(Number(e.target.value))}
                className="bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 rounded-xl px-2.5 py-2 text-xs font-semibold focus:outline-none cursor-pointer transition shadow-2xs"
                title="选择自动刷新频率"
              >
                <option value={0}>暂停自动刷新</option>
                <option value={5}>⚡ 每 5 秒刷新</option>
                <option value={10}>⏱️ 每 10 秒刷新</option>
                <option value={30}>⏲️ 每 30 秒刷新</option>
              </select>
            </div>

            {/* Manual Refresh Button */}
            <button
              onClick={() => fetchLogs()}
              disabled={logLoading}
              className="px-3.5 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition border border-slate-200 dark:border-slate-700 cursor-pointer shadow-2xs"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${logLoading ? 'animate-spin' : ''}`} />
              <span>刷新</span>
            </button>

            {/* Export CSV Button */}
            <button
              onClick={handleExportCSV}
              className="px-3 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition border border-slate-200 dark:border-slate-700 cursor-pointer shadow-2xs"
              title="导出当前筛选结果为 CSV 审计报表"
            >
              <FileSpreadsheet className="w-3.5 h-3.5 text-emerald-600" />
              <span>导出报表</span>
            </button>

            {/* Clear All Logs Button */}
            <button
              onClick={handleClearLogs}
              className="px-3 py-2 bg-rose-50 dark:bg-rose-950/40 hover:bg-rose-100 dark:hover:bg-rose-900/60 text-rose-600 dark:text-rose-300 border border-rose-200 dark:border-rose-900/60 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer"
              title="清空全部审计调用日志"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>清空</span>
            </button>
          </div>
        </div>

        {/* Filter Bar: Time Range & Status Filter */}
        <div className="flex flex-wrap items-center justify-between pt-3 border-t border-slate-100 dark:border-slate-800/80 gap-3 text-xs">
          {/* Time Range */}
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex items-center space-x-1.5 text-slate-500 dark:text-slate-400 font-medium">
              <Clock className="w-3.5 h-3.5 text-slate-400" />
              <span>时间跨度:</span>
            </div>
            <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-xl border border-slate-200/80 dark:border-slate-800">
              {[
                { id: 'all', label: '全部' },
                { id: '1h', label: '近 1 小时' },
                { id: 'today', label: '今天' },
                { id: '7d', label: '近 7 天' },
                { id: 'custom', label: '自定义' },
              ].map((item) => (
                <button
                  key={item.id}
                  onClick={() => {
                    setTimeRange(item.id);
                    if (item.id !== 'custom') {
                      fetchLogs({ timeRange: item.id });
                    }
                  }}
                  className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                    timeRange === item.id
                      ? 'bg-white dark:bg-slate-800 text-indigo-600 dark:text-indigo-400 shadow-2xs font-semibold'
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
                  className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2 py-1 text-xs text-slate-700 dark:text-slate-200"
                  title="开始时间"
                />
                <span className="text-slate-400">至</span>
                <input
                  type="datetime-local"
                  value={customEndTime}
                  onChange={(e) => setCustomEndTime(e.target.value)}
                  className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2 py-1 text-xs text-slate-700 dark:text-slate-200"
                  title="结束时间"
                />
                <button
                  onClick={() => fetchLogs({ timeRange: 'custom' })}
                  className="px-2.5 py-1 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-medium transition cursor-pointer"
                >
                  查询
                </button>
              </div>
            )}
          </div>

          {/* Status Filter Tabs */}
          <div className="flex items-center space-x-1.5">
            <span className="text-slate-500 dark:text-slate-400 font-medium">调用状态:</span>
            <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-xl border border-slate-200/80 dark:border-slate-800">
              <button
                onClick={() => setStatusFilter('all')}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'all'
                    ? 'bg-white dark:bg-slate-800 text-indigo-600 dark:text-indigo-400 shadow-2xs font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                }`}
              >
                全部
              </button>
              <button
                onClick={() => setStatusFilter('success')}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'success'
                    ? 'bg-white dark:bg-slate-800 text-emerald-600 dark:text-emerald-400 shadow-2xs font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                }`}
              >
                仅成功 (2xx)
              </button>
              <button
                onClick={() => setStatusFilter('error')}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'error'
                    ? 'bg-white dark:bg-slate-800 text-rose-600 dark:text-rose-400 shadow-2xs font-semibold'
                    : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                }`}
              >
                仅异常 (4xx/5xx)
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Batch Action Bar */}
      {selectedLogIds.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
            <span>已选中 {selectedLogIds.length} 条调用日志</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDeleteLogs}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>批量删除</span>
            </button>
            <button
              onClick={() => setSelectedLogIds([])}
              className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
            >
              取消选择
            </button>
          </div>
        </div>
      )}

      {/* Logs Table Card */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs flex flex-col">
        <div className="overflow-auto max-h-[calc(100vh-380px)]">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="text-slate-500 dark:text-slate-400 text-xs uppercase">
                <th className="py-2.5 px-3 w-10 text-center sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">
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
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">请求时间</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">Trace ID / 对话 ID</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">请求模型 (Model)</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">命中渠道 (Provider)</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">租户 / API 密钥</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">Token (输入/输出/总)</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">扣费 / 缓存命中</th>
                <th className="py-2.5 px-4 font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">耗时 / TTFT</th>
                <th className="py-2.5 px-4 text-right font-semibold sticky top-0 z-10 bg-slate-50 dark:bg-slate-900 border-b border-slate-200 dark:border-slate-800">状态与详情</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-xs">
              {displayedLogs.map((log) => {
                const isError = (log.status_code && log.status_code >= 400) || !!log.error_message;
                return (
                  <tr
                    key={log.id}
                    onClick={() => setActiveLogDetail(log)}
                    className="even:bg-slate-50/50 dark:even:bg-slate-900/40 hover:bg-indigo-50/40 dark:hover:bg-indigo-950/30 transition cursor-pointer group"
                  >
                    <td className="py-2.5 px-3 text-center" onClick={(e) => e.stopPropagation()}>
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
                    <td className="py-2.5 px-4 text-slate-500 dark:text-slate-400 font-sans whitespace-nowrap" title={log.created_at}>
                      {log.created_at ? formatLogTime(log.created_at) : '刚刚'}
                    </td>
                    <td className="py-2.5 px-4 font-mono text-xs">
                      <div className="flex items-center space-x-1.5">
                        <span
                          className="text-purple-600 dark:text-purple-400 truncate max-w-[130px] font-semibold"
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
                              className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-600 transition"
                              title="复制 Trace ID"
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
                                if (showToast) showToast(`已筛选 Trace: ${log.trace_id}`, 'info');
                              }}
                              className="p-1 hover:bg-purple-50 dark:hover:bg-purple-900/50 rounded text-purple-600 dark:text-purple-400 transition"
                              title="按 Trace ID 快速过滤"
                            >
                              <Search className="w-3 h-3" />
                            </button>
                          </>
                        )}
                      </div>
                      {log.session_id && (
                        <div className="flex items-center space-x-1 mt-0.5">
                          <span
                            className="text-[10px] text-slate-400 truncate max-w-[120px]"
                            title={`会话: ${log.session_id}`}
                          >
                            会话: {log.session_id}
                          </span>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              copyWithFeedback(log.session_id, '会话 ID');
                            }}
                            className="text-slate-400 hover:text-slate-600"
                            title="复制会话 ID"
                          >
                            <Copy className="w-2.5 h-2.5" />
                          </button>
                        </div>
                      )}
                      {log.chat_id && (
                        <span
                          className="block text-[10px] text-sky-500 dark:text-sky-400 truncate max-w-[130px]"
                          title={`对话: ${log.chat_id}`}
                        >
                          对话: {log.chat_id}
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 px-4 font-semibold text-slate-900 dark:text-slate-100 font-mono">
                      <span
                        className="px-2 py-0.5 bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 rounded-md border border-indigo-100 dark:border-indigo-800/60 max-w-[150px] inline-block truncate align-middle"
                        title={log.model}
                      >
                        {log.model || '-'}
                      </span>
                    </td>
                    <td className="py-2.5 px-4 text-slate-700 dark:text-slate-300 font-sans">
                      {log.channel ? (
                        <span
                          className="px-2 py-0.5 bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 rounded border border-slate-200 dark:border-slate-700 font-mono text-[11px] truncate max-w-[120px] inline-block align-middle"
                          title={log.channel}
                        >
                          {log.channel}
                        </span>
                      ) : (
                        <span className="text-slate-400 font-mono">-</span>
                      )}
                    </td>
                    <td className="py-2.5 px-4 font-mono text-xs text-slate-600 dark:text-slate-300">
                      <div className="truncate max-w-[120px]" title={log.tenant_id}>
                        {log.tenant_id || '-'}
                      </div>
                    </td>
                    <td className="py-2.5 px-4 font-mono text-xs">
                      <span className="text-slate-600 dark:text-slate-400">
                        {log.prompt_tokens ?? 0}
                      </span>
                      <span className="text-slate-400 mx-1">/</span>
                      <span className="text-slate-600 dark:text-slate-400">
                        {log.completion_tokens ?? 0}
                      </span>
                      <span className="text-slate-400 mx-1">/</span>
                      <span className="font-bold text-slate-900 dark:text-slate-100">
                        {log.total_tokens ?? 0}
                      </span>
                    </td>
                    <td className="py-2.5 px-4 text-slate-700 dark:text-slate-300 font-sans">
                      <div className="flex items-center space-x-1.5">
                        <span className="font-bold text-slate-900 dark:text-slate-100 font-mono">
                          ¥{(log.cost || 0).toFixed(4)}
                        </span>
                        {log.is_off_peak && (
                          <span
                            className="px-1.5 py-0.2 rounded text-[10px] font-bold bg-indigo-50 text-indigo-700 border border-indigo-200"
                            title={`分时计费优惠 ${(log.off_peak_discount || 0.5) * 100}%`}
                          >
                            🌙 闲时 {Math.round((log.off_peak_discount || 0.5) * 10)}折
                          </span>
                        )}
                      </div>
                      {log.cached_tokens > 0 ? (
                        <span className="block text-[10px] text-emerald-600 dark:text-emerald-400 font-semibold mt-0">
                          ⚡ 缓存: {log.cached_tokens} (省 90%)
                        </span>
                      ) : (
                        <span className="block text-[10px] text-slate-400 mt-0">无缓存命中</span>
                      )}
                    </td>
                    <td className="py-2.5 px-4 text-slate-700 dark:text-slate-300 font-sans">
                      <span
                        className={`font-bold ${
                          (log.duration_ms || 0) < 500
                            ? 'text-emerald-600 dark:text-emerald-400'
                            : (log.duration_ms || 0) < 2000
                            ? 'text-amber-600 dark:text-amber-400'
                            : 'text-rose-600 dark:text-rose-400'
                        }`}
                      >
                        {formatDuration(log.duration_ms ?? 0)}
                      </span>
                      {log.ttft_ms > 0 && (
                        <span className="block text-[11px] text-amber-600 dark:text-amber-400">
                          TTFT: {formatDuration(log.ttft_ms)}
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 px-4 text-right font-sans space-x-2">
                      <span
                        className={`px-2 py-0.5 rounded-full text-xs font-semibold ${
                          !isError
                            ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                            : (log.status_code || 200) < 500
                            ? 'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800'
                            : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800'
                        }`}
                      >
                        {log.status_code || 200}
                      </span>
                      <span className="text-[11px] text-indigo-600 dark:text-indigo-400 group-hover:underline">
                        详情 →
                      </span>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteSingleLog(log.id);
                        }}
                        className="p-1 text-slate-400 hover:text-rose-600 transition rounded-lg hover:bg-rose-50 dark:hover:bg-rose-950/50 inline-flex items-center align-middle cursor-pointer"
                        title="删除此记录"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </td>
                  </tr>
                );
              })}
              {displayedLogs.length === 0 && (
                <tr>
                  <td colSpan="10" className="py-12 text-center text-slate-400 font-sans">
                    <div className="flex flex-col items-center justify-center space-y-2">
                      <History className="w-8 h-8 text-slate-300 dark:text-slate-600 stroke-1" />
                      <span>暂无匹配的审计调用记录</span>
                      {(logFilter || sessionFilter || statusFilter !== 'all') && (
                        <button
                          onClick={() => {
                            setLogFilter('');
                            setSessionFilter('');
                            setStatusFilter('all');
                            fetchLogs({ sessionFilter: '' });
                          }}
                          className="text-xs text-indigo-600 hover:underline font-semibold cursor-pointer"
                        >
                          重置所有筛选条件
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
        <div className="flex items-center justify-between p-4 border-t border-slate-200/60 dark:border-slate-800/60">
          <span className="text-xs text-slate-500 dark:text-slate-400">
            第 {Math.floor(logOffset / logPageSize) + 1} 页 · 本页展示 {displayedLogs.length} 条
            {displayedLogs.length < logsLength ? ` (筛选后 / 累计 ${logsLength})` : ''}
          </span>
          <div className="flex items-center space-x-2">
            <button
              type="button"
              onClick={() => onLogPageChange(logOffset - logPageSize)}
              disabled={logLoading || logOffset <= 0}
              className="px-3 py-1.5 text-xs rounded-xl border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              上一页
            </button>
            <button
              type="button"
              onClick={() => onLogPageChange(logOffset + logPageSize)}
              disabled={logLoading || logsLength < logPageSize}
              className="px-3 py-1.5 text-xs rounded-xl border border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              下一页
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
