import { RefreshCw } from 'lucide-react';

export default function DashboardTraffic({
  isZh, logs, recentLogs, recentLogsLoading, selectedKey, setActiveLogDetail, setCurrentTab
}) {
  return (
    <div className="ui-card overflow-hidden">
      <div className="p-6 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
              {isZh ? '最近调用流水' : 'Recent Inbound Audit Stream'}
            </h3>
            <span className="text-[11px] font-mono px-1.5 py-0.5 rounded bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]">
              TOP 10
            </span>
          </div>
          <p className="ui-meta mt-0.5">
            {selectedKey
              ? (isZh ? '所选密钥最近调用，不受筛选影响' : 'Latest calls for selected key, ignoring filters')
              : (isZh ? '最近调用流水，按时间倒序' : 'Latest requests, newest first')}
          </p>
        </div>
        <button
          onClick={() => setCurrentTab('logs')}
          className="text-xs font-medium text-[#355CFF] dark:text-[#91A7FF] hover:underline transition-colors cursor-pointer"
        >
          {isZh ? '查看全部调用日志 →' : 'All Audit Logs →'}
        </button>
      </div>

      {(() => {
        const displayLogs = recentLogs.length > 0 ? recentLogs : (logs || []).slice(0, 10);
        if (recentLogsLoading && recentLogs.length === 0) {
          return (
            <div className="p-8 text-center ui-meta flex items-center justify-center gap-2">
              <RefreshCw className="w-4 h-4 animate-spin text-[#656B75] dark:text-[#A1A7B0]" />
              <span>{isZh ? '正在加载最新流水...' : 'Loading recent stream...'}</span>
            </div>
          );
        }
        if (displayLogs.length === 0) {
          return (
            <div className="p-8 text-center ui-meta">
              {isZh ? '暂无请求记录' : 'No request logs recorded yet'}
            </div>
          );
        }
        return (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr>
                  <th className="ui-th">{isZh ? '时间' : 'Timestamp'}</th>
                  <th className="ui-th">{isZh ? '调用方' : 'Client'}</th>
                  <th className="ui-th">{isZh ? '模型' : 'Model'}</th>
                  <th className="ui-th">{isZh ? '渠道' : 'Channel'}</th>
                  <th className="ui-th">{isZh ? '状态' : 'Status'}</th>
                  <th className="ui-th text-right">{isZh ? '首字 / 耗时' : 'TTFT / Latency'}</th>
                  <th className="ui-th text-right">{isZh ? 'Token 消耗' : 'Tokens'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60">
                {displayLogs.map((log) => (
                  <tr
                    key={log.id}
                    onClick={() => setActiveLogDetail && setActiveLogDetail(log)}
                    className="ui-tr-hover cursor-pointer"
                  >
                    <td className="ui-td font-mono text-[#656B75] dark:text-[#A1A7B0] text-xs">
                      {new Date(log.created_at).toLocaleTimeString()}
                    </td>
                    <td className="ui-td font-medium text-[#202124] dark:text-[#F1F2F4]">
                      {log.tenant_id || (log.api_key ? (log.api_key.length > 12 ? `${log.api_key.slice(0, 8)}...` : log.api_key) : 'anonymous')}
                    </td>
                    <td className="ui-td font-mono font-medium text-[#202124] dark:text-[#F1F2F4]">
                      {log.model}
                    </td>
                    <td className="ui-td text-[#656B75] dark:text-[#A1A7B0] font-mono text-xs">
                      {log.channel || log.channel_name || (isZh ? '默认通道' : 'Default')}
                    </td>
                    <td className="ui-td">
                      <span
                        className={`inline-flex items-center px-2 py-0.5 rounded font-mono text-[11px] font-medium ${
                          log.status_code >= 200 && log.status_code < 300
                            ? 'bg-emerald-500/10 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                            : 'bg-rose-500/10 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                        }`}
                      >
                        {log.status_code}
                      </span>
                    </td>
                    <td className="ui-td font-mono tabular-nums text-right text-[#202124] dark:text-[#F1F2F4]">
                      {log.ttft_ms > 0 ? `${log.ttft_ms}ms` : '—'} / {log.duration_ms ? `${log.duration_ms}ms` : '—'}
                    </td>
                    <td className="ui-td font-mono tabular-nums text-right text-[#202124] dark:text-[#F1F2F4]">
                      {(log.total_tokens || 0).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        );
      })()}
    </div>
  );
}
