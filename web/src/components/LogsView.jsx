import React from 'react';
import { History, Search, RefreshCw, Trash2, Clock, Copy } from 'lucide-react';

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
  filteredLogs,
  setActiveLogDetail,
  showToast,
  handleDeleteSingleLog,
}) {
  return (
    <div className="space-y-6">
      <div className="bg-white dark:bg-[#111726] p-6 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs space-y-4">
        <div className="flex flex-col lg:flex-row justify-between items-start lg:items-center gap-4">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-2xl bg-sky-50 dark:bg-sky-950/60 text-sky-600 dark:text-sky-400 flex items-center justify-center border border-sky-200/60 dark:border-sky-800/60">
              <History className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-bold text-slate-900 dark:text-slate-100 text-sm">
                对话审计与请求日志
              </h3>
            </div>
          </div>

          {/* Actions & Search */}
          <div className="flex flex-wrap items-center gap-3 w-full lg:w-auto">
            {/* Active Chat/Session/Trace Filter Tag */}
            {sessionFilter && (
              <div className="flex items-center space-x-2 px-3 py-1.5 rounded-xl bg-purple-50 dark:bg-purple-950/60 border border-purple-200 dark:border-purple-800 text-xs text-purple-700 dark:text-purple-300 animate-in fade-in">
                <span className="text-[11px] text-slate-400">已锁定链路/对话:</span>
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

            <div className="relative flex-1 sm:w-64">
              <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
              <input
                type="text"
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
                placeholder="筛选 Trace ID / 对话 ID / 模型 / 渠道..."
                className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-8 pr-3 py-2 text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
              />
            </div>

            <button
              onClick={() => fetchLogs()}
              disabled={logLoading}
              className="px-3.5 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition border border-slate-200 dark:border-slate-700 cursor-pointer"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${logLoading ? 'animate-spin' : ''}`} />
              <span>刷新</span>
            </button>

            <button
              onClick={handleClearLogs}
              className="px-3.5 py-2 bg-rose-50 dark:bg-rose-950/40 hover:bg-rose-100 dark:hover:bg-rose-900/60 text-rose-600 dark:text-rose-300 border border-rose-200 dark:border-rose-900/60 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer"
              title="清空全部审计调用日志"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>清空日志</span>
            </button>
          </div>
        </div>

        {/* Time Range Filter Bar */}
        <div className="flex flex-wrap items-center justify-between pt-3 border-t border-slate-100 dark:border-slate-800/80 gap-3 text-xs">
          <div className="flex items-center space-x-2">
            <Clock className="w-3.5 h-3.5 text-slate-400" />
            <span className="text-slate-500 dark:text-slate-400 font-medium">时间段筛选:</span>
            <div className="inline-flex bg-slate-100 dark:bg-slate-900 p-0.5 rounded-xl border border-slate-200/80 dark:border-slate-800">
              {[
                { id: 'all', label: '全部时间' },
                { id: '1h', label: '最近 1 小时' },
                { id: 'today', label: '今天' },
                { id: '7d', label: '最近 7 天' },
                { id: 'custom', label: '自定义时间' },
              ].map((item) => (
                <button
                  key={item.id}
                  onClick={() => {
                    setTimeRange(item.id);
                    if (item.id !== 'custom') {
                      fetchLogs({ timeRange: item.id });
                    }
                  }}
                  className={`px-3 py-1 rounded-lg text-xs font-medium transition cursor-pointer ${
                    timeRange === item.id
                      ? 'bg-white dark:bg-slate-800 text-indigo-600 dark:text-indigo-400 shadow-2xs font-semibold'
                      : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-200'
                  }`}
                >
                  {item.label}
                </button>
              ))}
            </div>
          </div>

          {/* Custom time picker inputs */}
          {timeRange === 'custom' && (
            <div className="flex items-center space-x-2 animate-in fade-in">
              <input
                type="datetime-local"
                value={customStartTime}
                onChange={(e) => setCustomStartTime(e.target.value)}
                className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1 text-xs text-slate-700 dark:text-slate-200"
                title="开始时间"
              />
              <span className="text-slate-400">至</span>
              <input
                type="datetime-local"
                value={customEndTime}
                onChange={(e) => setCustomEndTime(e.target.value)}
                className="bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1 text-xs text-slate-700 dark:text-slate-200"
                title="结束时间"
              />
              <button
                onClick={() => fetchLogs({ timeRange: 'custom' })}
                className="px-3 py-1 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-medium transition cursor-pointer"
              >
                查询
              </button>
            </div>
          )}
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

      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
        <table className="w-full text-left border-collapse">
          <thead>
            <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/80 dark:bg-slate-900/80">
              <th className="py-3.5 px-4 w-10 text-center">
                <input
                  type="checkbox"
                  className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                  checked={filteredLogs.length > 0 && selectedLogIds.length === filteredLogs.length}
                  onChange={(e) => {
                    if (e.target.checked) {
                      setSelectedLogIds(filteredLogs.map((l) => l.id));
                    } else {
                      setSelectedLogIds([]);
                    }
                  }}
                />
              </th>
              <th className="py-3.5 px-6 font-semibold">请求时间</th>
              <th className="py-3.5 px-6 font-semibold">Trace ID / 对话 ID</th>
              <th className="py-3.5 px-6 font-semibold">请求模型 (Model)</th>
              <th className="py-3.5 px-6 font-semibold">命中渠道 (Provider)</th>
              <th className="py-3.5 px-6 font-semibold">租户 / API 密钥</th>
              <th className="py-3.5 px-6 font-semibold">Token (输入/输出/总)</th>
              <th className="py-3.5 px-6 font-semibold">扣费 / 缓存命中</th>
              <th className="py-3.5 px-6 font-semibold">耗时 / TTFT</th>
              <th className="py-3.5 px-6 text-right font-semibold">状态与详情</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm font-mono text-xs">
            {filteredLogs.map((log) => (
              <tr
                key={log.id}
                onClick={() => setActiveLogDetail(log)}
                className="hover:bg-indigo-50/40 dark:hover:bg-indigo-950/30 transition cursor-pointer group"
              >
                <td className="py-4 px-4 text-center" onClick={(e) => e.stopPropagation()}>
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
                <td className="py-4 px-6 text-slate-500 dark:text-slate-400 font-sans whitespace-nowrap">
                  {log.created_at ? new Date(log.created_at).toLocaleTimeString() : '刚刚'}
                </td>
                <td className="py-4 px-6 font-mono text-xs">
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
                            navigator.clipboard.writeText(log.trace_id);
                            showToast('Trace ID 已复制', 'success');
                          }}
                          className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded text-slate-400 hover:text-slate-600 transition"
                          title="复制 Trace ID"
                        >
                          <Copy className="w-3 h-3" />
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            setSessionFilter(log.trace_id);
                            fetchLogs({ sessionFilter: log.trace_id });
                            showToast(`已筛选: ${log.trace_id}`, 'info');
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
                    <span
                      className="block text-[10px] text-slate-400 truncate max-w-[130px]"
                      title={`会话: ${log.session_id}`}
                    >
                      会话: {log.session_id}
                    </span>
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
                <td className="py-4 px-6 font-semibold text-slate-900 dark:text-slate-100 font-mono">
                  <span className="px-2 py-0.5 bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 rounded-md border border-indigo-100 dark:border-indigo-800/60">
                    {log.model || '-'}
                  </span>
                </td>
                <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                  {log.channel ? (
                    <span className="px-2 py-0.5 bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 rounded-md border border-emerald-100 dark:border-emerald-800/60">
                      {log.channel}
                    </span>
                  ) : (
                    <span className="text-slate-400">直通/多源</span>
                  )}
                </td>
                <td className="py-4 px-6 text-slate-600 dark:text-slate-300 font-sans">
                  <span className="font-semibold text-slate-800 dark:text-slate-200">
                    {log.tenant_id || 'anonymous'}
                  </span>
                  {log.api_key && (
                    <span className="block text-[10px] text-slate-400 font-mono mt-0.5 truncate max-w-[120px]">
                      {log.api_key}
                    </span>
                  )}
                </td>
                <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                  {log.total_tokens > 0 ? (
                    <span>
                      {log.prompt_tokens} + {log.completion_tokens} ={' '}
                      <strong className="text-indigo-600 dark:text-indigo-400">{log.total_tokens}</strong>
                    </span>
                  ) : (
                    <span className="text-slate-400">-</span>
                  )}
                </td>
                <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
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
                    <span className="block text-[10px] text-emerald-600 dark:text-emerald-400 font-semibold mt-0.5">
                      ⚡ 缓存: {log.cached_tokens} (省 90%)
                    </span>
                  ) : (
                    <span className="block text-[10px] text-slate-400 mt-0.5">无缓存命中</span>
                  )}
                </td>
                <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-sans">
                  <span className="font-bold text-slate-900 dark:text-slate-100">{log.duration_ms} ms</span>
                  {log.ttft_ms > 0 && (
                    <span className="block text-[11px] text-amber-600 dark:text-amber-400">
                      TTFT: {log.ttft_ms} ms
                    </span>
                  )}
                </td>
                <td className="py-4 px-6 text-right font-sans space-x-2">
                  <span
                    className={`px-2 py-0.5 rounded-full text-xs font-semibold ${
                      log.status_code === 200
                        ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                        : log.status_code === 429
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
            ))}
            {filteredLogs.length === 0 && (
              <tr>
                <td colSpan="10" className="py-12 text-center text-slate-400 font-sans">
                  暂无匹配的审计调用记录
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
