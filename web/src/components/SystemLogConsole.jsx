import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import {
  Terminal,
  Play,
  Pause,
  Trash2,
  Copy,
  Check,
  Search,
  RefreshCw,
  AlertTriangle,
  Info,
  Flame,
  X
} from 'lucide-react';
import { apiFetch } from '../api/client';
import { copyToClipboard } from '../config';

export default function SystemLogConsole({
  adminFetch,
  initialTraceId = '',
  onClose,
  showToast,
  lang = 'zh',
}) {
  const isZh = lang === 'zh';
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [isPaused, setIsPaused] = useState(false);
  const [searchQuery, setSearchQuery] = useState(initialTraceId || '');
  const [selectedLevel, setSelectedLevel] = useState('ALL'); // ALL, ERROR, WARN, INFO, DEBUG
  const [selectedCategory, setSelectedCategory] = useState('all'); // all, breaker, prober, sync, router, auth, billing, system
  const [selectedLog, setSelectedLog] = useState(null);
  const [copiedId, setCopiedId] = useState(false);
  const [connected, setConnected] = useState(false);
  const [reconnectAttempt, setReconnectAttempt] = useState(0);

  const consoleContainerRef = useRef(null);
  const sseRef = useRef(null);
  const retryRef = useRef(null);
  const pollRef = useRef(null);
  // Only auto-scroll while the operator is already at the bottom; scrolling on
  // every record used to drag the view away mid-inspection.
  const followBottomRef = useRef(true);

  // Fetch initial history
  const fetchRecentLogs = useCallback(async () => {
    try {
      setLoading(true);
      const fetcher = adminFetch || apiFetch;
      const res = await fetcher('/api/v1/admin/system/logs?limit=300');
      if (res && res.ok) {
        const json = await res.json();
        if (json && json.code === 0 && Array.isArray(json.data)) {
          // Logs are returned newest first; reverse for terminal bottom-scroll
          setLogs(json.data.slice().reverse());
        }
      }
    } catch (e) {
      // Quietly ignore
    } finally {
      setLoading(false);
    }
  }, [adminFetch]);

  // Connect SSE for live streaming, with bounded reconnect backoff and a polling
  // fallback. The native EventSource retry does not cover a revoked token or a
  // proxy that closed the stream, and the old code merely greyed out the dot.
  useEffect(() => {
    fetchRecentLogs();

    let active = true;

    const stopPolling = () => {
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
    };
    const startPolling = () => {
      if (pollRef.current) return;
      pollRef.current = setInterval(() => {
        if (active) fetchRecentLogs();
      }, 5000);
    };

    let attempt = 0;
    const connect = () => {
      if (!active) return;
      // The session lives in an HttpOnly cookie. EventSource cannot set an
      // Authorization header, but withCredentials sends the cookie, which the
      // admin auth middleware accepts for GET requests. The old `?token=` query
      // parameter came from a localStorage credential that is no longer written
      // (and a credential in a URL leaks into proxy/access logs).
      let source;
      try {
        source = new EventSource('/api/v1/admin/system/logs/stream', { withCredentials: true });
      } catch {
        startPolling();
        return;
      }
      sseRef.current = source;

      source.onopen = () => {
        if (!active) return;
        attempt = 0;
        setReconnectAttempt(0);
        setConnected(true);
        stopPolling();
      };

      source.addEventListener('log', (e) => {
        if (!active) return;
        try {
          const entry = JSON.parse(e.data);
          setLogs((prev) => {
            if (prev.some((item) => item.id === entry.id)) return prev;
            const next = [...prev, entry];
            return next.length > 2000 ? next.slice(-2000) : next;
          });
        } catch {}
      });

      source.onerror = () => {
        if (!active) return;
        setConnected(false);
        // Take over reconnection: close first so the browser's own retry does
        // not race our scheduled attempt and open a second stream.
        try {
          source.close();
        } catch {}
        if (sseRef.current === source) sseRef.current = null;
        attempt += 1;
        setReconnectAttempt(attempt);
        startPolling();
        const delay = Math.min(30000, 2000 * 2 ** Math.min(attempt - 1, 4));
        if (retryRef.current) clearTimeout(retryRef.current);
        retryRef.current = setTimeout(connect, delay);
      };
    };

    connect();

    return () => {
      active = false;
      stopPolling();
      if (retryRef.current) {
        clearTimeout(retryRef.current);
        retryRef.current = null;
      }
      if (sseRef.current) {
        sseRef.current.close();
        sseRef.current = null;
      }
    };
  }, [fetchRecentLogs]);

  const handleConsoleScroll = () => {
    const container = consoleContainerRef.current;
    if (!container) return;
    followBottomRef.current = container.scrollHeight - container.scrollTop - container.clientHeight < 48;
  };

  // Follow new lines only while the operator is already at the bottom.
  useEffect(() => {
    if (isPaused || !followBottomRef.current) return;
    const container = consoleContainerRef.current;
    if (container) container.scrollTop = container.scrollHeight;
  }, [logs, isPaused]);

  // Filter logs
  const filteredLogs = useMemo(() => {
    return logs.filter((l) => {
      if (selectedLevel !== 'ALL' && l.level !== selectedLevel) {
        return false;
      }
      if (selectedCategory !== 'all' && l.category !== selectedCategory) {
        return false;
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase().trim();
        const msgMatch = (l.message || '').toLowerCase().includes(q);
        const traceMatch = (l.trace_id || '').toLowerCase().includes(q);
        const nodeMatch = (l.node_id || '').toLowerCase().includes(q);
        const catMatch = (l.category || '').toLowerCase().includes(q);
        let attrMatch = false;
        if (l.attrs) {
          attrMatch = Object.entries(l.attrs).some(
            ([k, v]) =>
              k.toLowerCase().includes(q) ||
              String(v).toLowerCase().includes(q)
          );
        }
        return msgMatch || traceMatch || nodeMatch || catMatch || attrMatch;
      }
      return true;
    });
  }, [logs, selectedLevel, selectedCategory, searchQuery]);

  // Rendering all 2000 buffered lines re-laid out the whole list on every record.
  // Show the newest window; filters, copy and the counters still use the full set.
  const VISIBLE_LOG_LIMIT = 300;
  const visibleLogs = useMemo(() => filteredLogs.slice(-VISIBLE_LOG_LIMIT), [filteredLogs]);

  const handleClear = () => {
    setLogs([]);
    setSelectedLog(null);
  };

  const handleCopyLogs = async () => {
    const text = filteredLogs
      .map(
        (l) =>
          `[${new Date(l.timestamp).toISOString()}] [${l.level}] [${l.category || 'system'}] ${l.message} ${
            l.attrs ? JSON.stringify(l.attrs) : ''
          }`
      )
      .join('\n');
    await copyToClipboard(text);
    setCopiedId(true);
    setTimeout(() => setCopiedId(false), 2000);
    showToast?.(isZh ? '当前筛选日志已复制' : 'Logs copied to clipboard', 'success');
  };

  const getLevelStyle = (level) => {
    switch (level) {
      case 'ERROR':
        return 'text-rose-400 bg-rose-950/40 border-rose-800/60';
      case 'WARN':
        return 'text-amber-400 bg-amber-950/40 border-amber-800/60';
      case 'INFO':
        return 'text-emerald-400 bg-emerald-950/40 border-emerald-800/60';
      case 'DEBUG':
        return 'text-sky-400 bg-sky-950/40 border-sky-800/60';
      default:
        return 'text-slate-400 bg-slate-800/40 border-slate-700/60';
    }
  };

  const getCategoryBadge = (cat) => {
    switch (cat) {
      case 'breaker':
        return { label: isZh ? '熔断/断路器' : 'Breaker', color: 'text-rose-400 bg-rose-950/60 border-rose-800/50' };
      case 'prober':
        return { label: isZh ? '渠道探活' : 'Prober', color: 'text-amber-400 bg-amber-950/60 border-amber-800/50' };
      case 'sync':
        return { label: isZh ? '集群同步' : 'Sync', color: 'text-[#91A7FF] bg-[#242D48] border-[#355CFF]/30' };
      case 'router':
        return { label: isZh ? '路由调度' : 'Router', color: 'text-cyan-400 bg-cyan-950/60 border-cyan-800/50' };
      case 'auth':
        return { label: isZh ? '鉴权配额' : 'Auth', color: 'text-blue-400 bg-blue-950/60 border-blue-800/50' };
      case 'billing':
        return { label: isZh ? '计费扣费' : 'Billing', color: 'text-emerald-400 bg-emerald-950/60 border-emerald-800/50' };
      default:
        return { label: isZh ? '系统内核' : 'System', color: 'text-slate-400 bg-slate-800/60 border-slate-700/50' };
    }
  };

  return (
    <div className="flex flex-col h-[750px] max-h-[85vh] bg-[#111214] text-slate-200 rounded-2xl border border-[#2C3038] shadow-2xl overflow-hidden font-mono text-xs">
      {/* Top Toolbar */}
      <div className="bg-[#191B1F] px-4 py-3 border-b border-[#2C3038] flex flex-wrap items-center justify-between gap-3 shrink-0">
        <div className="flex items-center space-x-3">
          <div className="flex items-center space-x-2">
            <div className="p-1.5 rounded-lg bg-[#355CFF]/15 text-[#91A7FF] border border-[#355CFF]/30">
              <Terminal className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <span className="font-semibold text-[#F1F2F4] font-sans text-sm">
                  {isZh ? '网关系统运行日志 (Live Tail)' : 'Gateway System Runtime Console'}
                </span>
                <span
                  className={`inline-flex items-center space-x-1 px-2 py-0.5 rounded-full text-[10px] font-medium border ${
                    connected
                      ? 'bg-emerald-950/60 text-emerald-400 border-emerald-800/50'
                      : 'bg-amber-950/60 text-amber-400 border-amber-800/50'
                  }`}
                >
                  <span
                    className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'}`}
                  ></span>
                  <span>
                    {connected
                      ? isZh
                        ? '实时已连接'
                        : 'LIVE SSE'
                      : reconnectAttempt > 0
                      ? isZh
                        ? `重连中 #${reconnectAttempt}`
                        : `RECONNECTING #${reconnectAttempt}`
                      : isZh
                      ? '连接中'
                      : 'CONNECTING'}
                  </span>
                </span>
              </div>
              <span className="text-[11px] text-[#A1A7B0] font-sans block mt-0.5">
                {isZh
                  ? '支持内核事件、探活状态、断路器跳闸与集群同步秒级排障'
                  : 'Live engine telemetry, health prober, circuit breakers & cluster sync'}
              </span>
            </div>
          </div>
        </div>

        {/* Controls */}
        <div className="flex items-center space-x-2">
          {/* Pause / Resume Button */}
          <button
            onClick={() => setIsPaused(!isPaused)}
            className={`px-2.5 py-1.5 rounded-lg font-sans text-xs font-medium flex items-center space-x-1.5 border transition cursor-pointer ${
              isPaused
                ? 'bg-amber-500/20 text-amber-300 border-amber-500/40 hover:bg-amber-500/30'
                : 'bg-slate-800 text-slate-300 border-slate-700 hover:bg-slate-700'
            }`}
            title={isPaused ? (isZh ? '恢复实时自动滚屏' : 'Resume auto-scroll') : (isZh ? '暂停滚屏便于分析排查' : 'Pause auto-scroll')}
          >
            {isPaused ? <Play className="w-3.5 h-3.5 text-amber-400" /> : <Pause className="w-3.5 h-3.5 text-slate-400" />}
            <span>{isPaused ? (isZh ? '已暂停 (点此恢复)' : 'Paused (Resume)') : (isZh ? '暂停滚屏' : 'Pause')}</span>
          </button>

          {/* Refresh History */}
          <button
            onClick={fetchRecentLogs}
            disabled={loading}
            className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 transition cursor-pointer"
            title={isZh ? '重新拉取最近历史日志' : 'Reload history logs'}
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>

          {/* Copy Logs */}
          <button
            onClick={handleCopyLogs}
            className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 transition cursor-pointer"
            title={isZh ? '复制当前筛选日志' : 'Copy filtered logs'}
          >
            {copiedId ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
          </button>

          {/* Clear View */}
          <button
            onClick={handleClear}
            className="p-1.5 rounded-lg bg-slate-800 hover:bg-rose-900/40 text-slate-300 hover:text-rose-300 border border-slate-700 hover:border-rose-800 transition cursor-pointer"
            title={isZh ? '清空控制台屏幕' : 'Clear console screen'}
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>

          {onClose && (
            <button
              onClick={onClose}
              className="p-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-400 hover:text-white border border-slate-700 transition cursor-pointer ml-1"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Filter Chips Toolbar */}
      <div className="bg-[#191B1F]/90 px-4 py-2.5 border-b border-[#2C3038] flex flex-wrap items-center justify-between gap-2.5 shrink-0">
        <div className="flex flex-wrap items-center gap-1.5">
          {/* Level Pills */}
          <div className="flex items-center bg-[#111214] p-0.5 rounded-lg border border-[#2C3038] text-[11px]">
            {['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG'].map((lvl) => (
              <button
                key={lvl}
                onClick={() => setSelectedLevel(lvl)}
                className={`px-2 py-0.5 rounded-md font-sans transition cursor-pointer ${
                  selectedLevel === lvl
                    ? 'bg-[#355CFF] text-white font-semibold shadow-xs'
                    : 'text-[#A1A7B0] hover:text-[#F1F2F4]'
                }`}
              >
                {lvl}
              </button>
            ))}
          </div>

          {/* Troubleshooting Category Quick Chips */}
          <div className="flex flex-wrap items-center gap-1">
            {[
              { id: 'all', label: isZh ? '全部' : 'All' },
              { id: 'breaker', label: isZh ? '熔断/断路器' : 'Breaker' },
              { id: 'prober', label: isZh ? '探活探测' : 'Prober' },
              { id: 'sync', label: isZh ? '集群同步' : 'Sync' },
              { id: 'router', label: isZh ? '路由调度' : 'Router' },
              { id: 'billing', label: isZh ? '计费扣费' : 'Billing' },
            ].map((cat) => (
              <button
                key={cat.id}
                onClick={() => setSelectedCategory(cat.id)}
                className={`px-2 py-0.5 rounded-md text-[11px] font-sans border transition cursor-pointer ${
                  selectedCategory === cat.id
                    ? 'bg-[#242D48] text-[#91A7FF] border-[#355CFF]/60 font-medium'
                    : 'bg-[#111214] text-[#A1A7B0] border-[#2C3038] hover:border-[#355CFF]/30 hover:text-[#F1F2F4]'
                }`}
              >
                {cat.label}
              </button>
            ))}
          </div>
        </div>

        {/* Live Search */}
        <div className="relative min-w-[200px] sm:min-w-[260px]">
          <Search className="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={isZh ? '搜索日志内容、Trace ID、渠道...' : 'Search logs, trace_id, channel...'}
            className="w-full bg-slate-900/90 text-slate-200 placeholder-slate-500 text-xs pl-8 pr-7 py-1 rounded-lg border border-slate-800 focus:border-[#355CFF] focus:outline-none"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
            >
              <X className="w-3 h-3" />
            </button>
          )}
        </div>
      </div>

      {/* Main Console View Area */}
      <div className="flex-1 flex overflow-hidden">
        {/* Terminal Log Stream Table */}
        <div
          ref={consoleContainerRef}
          onScroll={handleConsoleScroll}
          className="flex-1 overflow-y-auto p-4 space-y-1 divide-y divide-slate-800/30 font-mono text-[11px] leading-relaxed select-text"
        >
          {filteredLogs.length > VISIBLE_LOG_LIMIT && (
            <div className="sticky top-0 z-10 -mt-1 mb-1 bg-[#0e131d]/95 px-2 py-1 text-[10px] font-sans text-[#A1A7B0] border-b border-slate-800/60">
              {isZh
                ? `仅渲染最近 ${VISIBLE_LOG_LIMIT} 条（筛选命中 ${filteredLogs.length} 条）`
                : `Rendering the latest ${VISIBLE_LOG_LIMIT} of ${filteredLogs.length} matches`}
            </div>
          )}
          {loading && logs.length === 0 ? (
            <div className="h-full flex items-center justify-center text-slate-500 space-x-2">
              <RefreshCw className="w-4 h-4 animate-spin text-[#355CFF] dark:text-[#91A7FF]" />
              <span>{isZh ? '正在拉取网关系统日志...' : 'Connecting to gateway system logs...'}</span>
            </div>
          ) : filteredLogs.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-slate-500 space-y-1 p-8 text-center">
              <Check className="w-6 h-6 text-emerald-500 mb-1" />
              <span className="font-sans font-medium text-slate-400">
                {isZh ? '当前筛选条件下暂无日志' : 'No logs match current filter criteria'}
              </span>
              <span className="text-xs text-slate-600 font-sans">
                {isZh ? '若发生上游报错或熔断跳闸将在此实时输出' : 'Errors and breaker triggers will stream here live'}
              </span>
            </div>
          ) : (
            visibleLogs.map((log) => {
              const catBadge = getCategoryBadge(log.category);
              const isSelected = selectedLog?.id === log.id;
              return (
                <div
                  key={log.id}
                  onClick={() => setSelectedLog(isSelected ? null : log)}
                  className={`pt-1 pb-1 px-2 rounded hover:bg-slate-800/60 transition cursor-pointer flex items-start gap-2.5 ${
                    isSelected ? 'bg-[#242D48] border border-[#355CFF]/40' : ''
                  }`}
                >
                  {/* Timestamp */}
                  <span className="text-slate-500 shrink-0 tabular-nums">
                    {new Date(log.timestamp).toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 3 })}
                  </span>

                  {/* Level */}
                  <span
                    className={`px-1.5 py-0.2 rounded text-[10px] font-bold border shrink-0 ${getLevelStyle(
                      log.level
                    )}`}
                  >
                    {log.level}
                  </span>

                  {/* Category */}
                  <span
                    className={`px-1.5 py-0.2 rounded text-[10px] border shrink-0 ${catBadge.color}`}
                  >
                    {catBadge.label}
                  </span>

                  {/* Trace ID Tag if present */}
                  {log.trace_id && (
                    <span className="text-cyan-400 bg-cyan-950/40 border border-cyan-800/50 px-1 rounded text-[10px] shrink-0 font-mono">
                      #{log.trace_id.slice(-8)}
                    </span>
                  )}

                  {/* Message */}
                  <span
                    className={`flex-1 break-all ${
                      log.level === 'ERROR'
                        ? 'text-rose-300 font-semibold'
                        : log.level === 'WARN'
                        ? 'text-amber-200'
                        : 'text-slate-300'
                    }`}
                  >
                    {log.message}
                  </span>

                  {/* Attrs Preview Chip */}
                  {log.attrs && Object.keys(log.attrs).length > 0 && (
                    <span className="text-[10px] text-slate-500 bg-slate-900 px-1.5 py-0.5 rounded border border-slate-800 shrink-0">
                      {Object.keys(log.attrs).length} attrs
                    </span>
                  )}
                </div>
              );
            })
          )}
        </div>

        {/* Selected Log Detail Inspector Drawer */}
        {selectedLog && (
          <div className="w-80 md:w-96 border-l border-slate-800 bg-[#0e131d] p-4 overflow-y-auto space-y-4 shrink-0 animate-in slide-in-from-right duration-150">
            <div className="flex items-center justify-between border-b border-slate-800 pb-2">
              <span className="font-semibold text-slate-200 font-sans text-xs flex items-center gap-1.5">
                <Info className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
                <span>{isZh ? '排障日志详情分析' : 'Troubleshooting Log Detail'}</span>
              </span>
              <button
                onClick={() => setSelectedLog(null)}
                className="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* Quick Metrics */}
            <div className="space-y-2 text-xs">
              <div className="flex items-center justify-between py-1 border-b border-slate-800/60">
                <span className="text-slate-500">{isZh ? '级别' : 'Level'}:</span>
                <span className={`px-2 py-0.5 rounded font-bold border text-[11px] ${getLevelStyle(selectedLog.level)}`}>
                  {selectedLog.level}
                </span>
              </div>
              <div className="flex items-center justify-between py-1 border-b border-slate-800/60">
                <span className="text-slate-500">{isZh ? '时间戳' : 'Time'}:</span>
                <span className="text-slate-300 font-mono text-[11px]">
                  {new Date(selectedLog.timestamp).toLocaleString()}
                </span>
              </div>
              <div className="flex items-center justify-between py-1 border-b border-slate-800/60">
                <span className="text-slate-500">{isZh ? '节点' : 'Node'}:</span>
                <span className="text-[#91A7FF] font-mono text-[11px]">{selectedLog.node_id || 'airoute-gateway'}</span>
              </div>
              {selectedLog.trace_id && (
                <div className="flex items-center justify-between py-1 border-b border-slate-800/60">
                  <span className="text-slate-500">Trace ID:</span>
                  <span className="text-cyan-400 font-mono text-[11px] select-all">{selectedLog.trace_id}</span>
                </div>
              )}
            </div>

            {/* Message */}
            <div className="space-y-1">
              <span className="text-[11px] text-slate-400 font-semibold">{isZh ? '日志消息' : 'Message'}:</span>
              <div className="p-2.5 rounded-lg bg-slate-900 border border-slate-800 text-slate-200 break-all select-all">
                {selectedLog.message}
              </div>
            </div>

            {/* Attrs JSON */}
            {selectedLog.attrs && Object.keys(selectedLog.attrs).length > 0 && (
              <div className="space-y-1">
                <span className="text-[11px] text-slate-400 font-semibold">{isZh ? '结构化元数据' : 'Structured Attributes'}:</span>
                <pre className="p-2.5 rounded-lg bg-slate-900 border border-slate-800 text-emerald-400 text-[10px] overflow-x-auto select-all leading-relaxed">
                  {JSON.stringify(selectedLog.attrs, null, 2)}
                </pre>
              </div>
            )}

            {/* Troubleshooting Action Tip */}
            {selectedLog.category === 'breaker' && (
              <div className="p-3 rounded-lg bg-rose-950/40 border border-rose-800/60 text-rose-300 text-[11px] space-y-1">
                <div className="font-semibold flex items-center gap-1">
                  <Flame className="w-3.5 h-3.5 text-rose-400" />
                  <span>{isZh ? '熔断故障排查建议' : 'Breaker Troubleshooting'}</span>
                </div>
                <p className="text-rose-400/90 leading-normal font-sans">
                  {isZh
                    ? '该渠道因连续多次请求超时或 5xx 错误已被断路器熔断隔离。流量正在转移至备用渠道。建议检查该上游提供商状态或临时切换备用权重。'
                    : 'Channel tripped due to upstream consecutive errors or timeouts. Traffic is failing over to healthy providers.'}
                </p>
              </div>
            )}

            {selectedLog.category === 'prober' && (
              <div className="p-3 rounded-lg bg-amber-950/40 border border-amber-800/60 text-amber-300 text-[11px] space-y-1">
                <div className="font-semibold flex items-center gap-1">
                  <AlertTriangle className="w-3.5 h-3.5 text-amber-400" />
                  <span>{isZh ? '渠道探活告警建议' : 'Prober Diagnostic'}</span>
                </div>
                <p className="text-amber-400/90 leading-normal font-sans">
                  {isZh
                    ? '网关探活程序检测到上游提供商响应异常或延迟超过阈值，请检查上游 Base URL 网络连通性。'
                    : 'Upstream probe detected connectivity failures or latency spikes. Verify provider base URL.'}
                </p>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Bottom Status Bar */}
      <div className="bg-[#111622] px-4 py-2 border-t border-slate-800 flex items-center justify-between text-[11px] text-slate-400 shrink-0">
        <div className="flex items-center space-x-3">
          <span>{isZh ? `已加载: ${filteredLogs.length} 条` : `Showing: ${filteredLogs.length} entries`}</span>
          {filteredLogs.length > visibleLogs.length && (
            <>
              <span>·</span>
              <span>{isZh ? `渲染: 最近 ${visibleLogs.length} 条` : `Rendered: latest ${visibleLogs.length}`}</span>
            </>
          )}
          <span>·</span>
          <span>{isZh ? `内存缓冲池: ${logs.length} / 2000` : `Ring Buffer: ${logs.length}/2000`}</span>
        </div>
        <div className="flex items-center space-x-2">
          <span className={connected ? 'text-emerald-400' : 'text-amber-400'}>
            {connected
              ? isZh
                ? '实时流正常'
                : 'Live stream OK'
              : reconnectAttempt > 0
              ? isZh
                ? `实时流断开，重连中 (#${reconnectAttempt})`
                : `Stream lost, reconnecting (#${reconnectAttempt})`
              : isZh
              ? '正在连接实时流…'
              : 'Connecting live stream…'}
          </span>
          {searchQuery && (
            <span className="text-[#91A7FF]">
              {isZh ? `过滤词: "${searchQuery}"` : `Filtered by "${searchQuery}"`}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
