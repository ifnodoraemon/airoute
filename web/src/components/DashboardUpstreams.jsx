import { Activity, RefreshCw, Plus, Play } from 'lucide-react';

export default function DashboardUpstreams({
  adminUser, batchTesting, channelLatencies, channels, handleBatchTest, handleTestChannel, isZh,
  setCurrentTab, setShowChannelModal, testingId
}) {
  return (
    <div className="ui-card overflow-hidden">
      <div className="p-6 border-b border-[#E4E6E9] dark:border-[#2C3038] flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div>
          <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '上游渠道健康度与测速' : 'Upstream Provider Health & Latency'}
          </h3>
          <p className="ui-meta mt-0.5">
            {isZh ? '通道探测与熔断状态' : 'Probe latency and breaker state'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleBatchTest}
            disabled={batchTesting || channels.length === 0}
            className="ui-btn-secondary ui-btn-sm"
          >
            <Activity className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${batchTesting ? 'animate-spin text-[#355CFF]' : ''}`} />
            <span>{batchTesting ? (isZh ? '体检中...' : 'Probing...') : (isZh ? '全渠道体检' : 'Probe All')}</span>
          </button>
          <button
            onClick={() => setCurrentTab('channels')}
            className="ui-btn-primary ui-btn-sm"
          >
            {isZh ? `渠道管理 (${channels.length}) →` : `Manage Channels (${channels.length}) →`}
          </button>
        </div>
      </div>

      {channels.length === 0 ? (
        <div className="p-10 text-center flex flex-col items-center justify-center space-y-3">
          <h4 className="font-medium text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '暂无服务商' : 'No Providers Connected'}</h4>
          {adminUser?.role === 'admin' && setShowChannelModal && (
            <button
              onClick={() => setShowChannelModal(true)}
              className="ui-btn-primary"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
            </button>
          )}
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr>
                <th className="ui-th">{isZh ? '服务商名称' : 'Provider Name'}</th>
                <th className="ui-th">{isZh ? '协议类型' : 'Protocol'}</th>
                <th className="ui-th">{isZh ? '接入 Base URL' : 'Base URL'}</th>
                <th className="ui-th text-right">{isZh ? '支持模型数' : 'Models'}</th>
                <th className="ui-th">{isZh ? '熔断状态' : 'Breaker State'}</th>
                <th className="ui-th text-right">{isZh ? '最近延迟' : 'Latency'}</th>
                <th className="ui-th text-right">{isZh ? '操作' : 'Action'}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60">
              {channels.map((ch) => {
                const latInfo = channelLatencies[ch.id];
                const breaker = ch.breaker_status || 'CLOSED';
                return (
                  <tr key={ch.id} className="ui-tr-hover">
                    <td className="ui-td font-medium text-[#202124] dark:text-[#F1F2F4]">
                      {ch.name}
                    </td>
                    <td className="ui-td font-mono text-xs text-[#656B75] dark:text-[#A1A7B0]">
                      {ch.type}
                    </td>
                    <td className="ui-td font-mono text-xs text-[#656B75] dark:text-[#A1A7B0] max-w-xs truncate">
                      {ch.base_url}
                    </td>
                    <td className="ui-td font-mono tabular-nums text-right text-[#656B75] dark:text-[#A1A7B0]">
                      {ch.models?.length || 0}
                    </td>
                    <td className="ui-td">
                      <span className="inline-flex items-center space-x-1.5 text-xs text-[#202124] dark:text-[#F1F2F4]">
                        <span
                          className={`w-1.5 h-1.5 rounded-full ${
                            breaker === 'CLOSED'
                              ? 'bg-emerald-500'
                              : breaker === 'HALF-OPEN'
                              ? 'bg-amber-500 animate-pulse'
                              : 'bg-rose-500'
                          }`}
                        />
                        <span>
                          {breaker === 'CLOSED' ? (isZh ? '正常' : 'Healthy') : breaker === 'HALF-OPEN' ? (isZh ? '探测中' : 'Probing') : (isZh ? '已熔断' : 'Tripped')}
                        </span>
                      </span>
                    </td>
                    <td className="ui-td font-mono tabular-nums text-right">
                      {latInfo ? (
                        latInfo.success ? (
                          <span className="text-emerald-600 dark:text-emerald-400 font-medium">{latInfo.latencyMs} ms</span>
                        ) : (
                          <span className="text-rose-500">{isZh ? '异常' : 'Error'}</span>
                        )
                      ) : (
                        <span className="text-[#656B75] dark:text-[#A1A7B0]">-</span>
                      )}
                    </td>
                    <td className="ui-td text-right">
                      <button
                        onClick={() => handleTestChannel && handleTestChannel(ch)}
                        disabled={testingId === ch.id}
                        className="ui-btn-secondary ui-btn-sm"
                      >
                        {testingId === ch.id ? <RefreshCw className="w-3 h-3 animate-spin text-[#656B75] dark:text-[#A1A7B0]" /> : <Play className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" />}
                        <span>Ping</span>
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
