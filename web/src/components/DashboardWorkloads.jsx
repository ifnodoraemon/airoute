
export default function DashboardWorkloads({
  errorBudget, isZh, modalityStats, onOpenTopology, providerSlaStats, setCurrentTab, slaAvailability,
  topModels
}) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* Panel 1: Multimodal Modality Distribution */}
      <div className="ui-card p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '全模态负载分布' : 'Multimodal Workload Distribution'}
          </h4>
          <span className="ui-meta font-mono">
            {modalityStats.length} {isZh ? '类模态' : 'Modalities'}
          </span>
        </div>

        <div className="space-y-3.5 pt-1">
          {modalityStats.map(mod => (
            <div key={mod.key} className="space-y-1">
              <div className="flex justify-between text-xs">
                <span className="font-medium text-[#202124] dark:text-[#F1F2F4]">
                  {mod.label}
                </span>
                <span className="font-mono tabular-nums text-[#656B75] dark:text-[#A1A7B0]">
                  {mod.count} {isZh ? '次' : 'reqs'} ({mod.pct}%)
                </span>
              </div>
              <div className="w-full bg-[#F0F1F2] dark:bg-[#22252B] h-1.5 rounded-full overflow-hidden">
                <div
                  style={{ width: `${Math.max(4, mod.pct)}%` }}
                  className="h-full rounded-full transition-all duration-300 bg-[#355CFF]"
                />
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Panel 2: Provider Latency & SLA Benchmark */}
      <div className="ui-card p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '服务商实时时延与可用性跑分' : 'Provider Latency & SLA Benchmark'}
          </h4>
          <span className="ui-meta font-mono">
            {providerSlaStats.length} {isZh ? '个渠道' : 'Channels'}
          </span>
        </div>

        {providerSlaStats.length === 0 ? (
          <div className="p-8 text-center ui-meta">
            {isZh ? '暂无服务商测速数据' : 'No provider latency benchmark data'}
          </div>
        ) : (
          <div className="space-y-3.5 pt-1">
            {providerSlaStats.slice(0, 5).map((p, idx) => {
              const measured = providerSlaStats.filter(s => typeof s.avgTtft === 'number');
              const maxTtft = Math.max(...measured.map(s => s.avgTtft), 500);
              const hasLatency = typeof p.avgTtft === 'number';
              const barWidth = hasLatency
                ? Math.max(10, Math.min(100, Math.round((p.avgTtft / maxTtft) * 100)))
                : 0;

              return (
                <div key={p.id} className="space-y-1.5">
                  <div className="flex items-center justify-between text-xs">
                    <div className="flex items-center space-x-2">
                      <span className="font-medium text-[#202124] dark:text-[#F1F2F4]">
                        {p.name}
                      </span>
                    </div>
                    <div className="flex items-center space-x-3 font-mono tabular-nums">
                      <span className="font-medium text-[#202124] dark:text-[#F1F2F4]">
                        {hasLatency ? `${p.avgTtft} ms` : (isZh ? '无样本' : 'n/a')}
                      </span>
                      <span className={p.successRate === null ? 'text-[#656B75] dark:text-[#A1A7B0] font-medium' : 'text-emerald-600 dark:text-emerald-400 font-medium'}>
                        {p.successRate === null
                          ? (isZh ? 'SLA 未知' : 'SLA n/a')
                          : `SLA ${p.successRate}%`}
                      </span>
                    </div>
                  </div>
                  <div className="w-full bg-[#F0F1F2] dark:bg-[#22252B] h-1.5 rounded-full overflow-hidden">
                    <div
                      style={{ width: `${barWidth}%` }}
                      className="h-full rounded-full transition-all duration-300 bg-[#355CFF]"
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}

        <div className="pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038] flex justify-between items-center text-xs">
          <span className="ui-meta">{isZh ? '各服务商调用与支出核算' : 'Provider usage and spend'}</span>
          <button
            onClick={() => setCurrentTab('reconciliation')}
            className="text-[#355CFF] dark:text-[#91A7FF] hover:underline font-medium inline-flex items-center gap-1 cursor-pointer"
          >
            <span>{isZh ? '进入供应商整体对账 →' : 'Supplier Reconciliation →'}</span>
          </button>
        </div>
      </div>

      {/* Panel 3: Enterprise SLA Target & Error Budget Gauge */}
      <div className="ui-card p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '企业级 99.9% SLA 承诺与错误预算' : 'Enterprise 99.9% SLA & Error Budget'}
          </h4>
          <span className="ui-meta font-mono">
            Three-Nines
          </span>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
          <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="ui-meta font-medium block">
              {isZh ? 'SLA 目标承诺' : 'Committed Target'}
            </span>
            <div className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums mt-0.5">
              99.90%
            </div>
          </div>

          <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="ui-meta font-medium block">
              {isZh ? '当前实际可用率' : 'Actual Availability'}
            </span>
            <div className="text-lg font-semibold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums mt-0.5">
              {slaAvailability}{slaAvailability === '—' ? '' : '%'}
            </div>
          </div>

          <div className="p-3.5 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="ui-meta font-medium block">
              {isZh ? '容灾运行状态' : 'Live failover state'}
            </span>
            <div className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums mt-0.5">
              <button type="button" className="ui-btn-ghost ui-btn-sm text-[#355CFF] dark:text-[#91A7FF]" onClick={onOpenTopology}>{isZh ? '查看实时拓扑' : 'View live topology'}</button>
            </div>
          </div>
        </div>

        <div className="space-y-1.5 pt-2 border-t border-[#E4E6E9] dark:border-[#2C3038]">
          <div className="flex justify-between text-xs">
            <span className="text-[#656B75] dark:text-[#A1A7B0]">
              {isZh ? '剩余错误预算' : 'Error Budget Remaining'}
            </span>
            <span className="font-mono tabular-nums font-semibold text-[#202124] dark:text-[#F1F2F4]">
              {errorBudget.remainingBudgetPct === null ? '—' : `${errorBudget.remainingBudgetPct}%`}
            </span>
          </div>
          <div className="w-full bg-[#F0F1F2] dark:bg-[#22252B] h-2 rounded-full overflow-hidden">
            <div
              style={{ width: `${errorBudget.remainingBudgetPct ?? 0}%` }}
              className="h-full rounded-full transition-all duration-300 bg-[#355CFF]"
            />
          </div>
        </div>
      </div>

      {/* Panel 4: Top Models Breakdown */}
      <div className="ui-card p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
            {isZh ? '热门大模型 Token 占比' : 'Top Models by Token Usage'}
          </h4>
          <span className="ui-meta font-mono">Top 5</span>
        </div>

        {topModels.length === 0 ? (
          <div className="p-8 text-center ui-meta">
            {isZh ? '暂无模型调用数据' : 'No model invocation telemetry yet'}
          </div>
        ) : (
          <div className="space-y-3.5 pt-1">
            {topModels.map(m => (
              <div key={m.name} className="space-y-1">
                <div className="flex justify-between text-xs font-mono tabular-nums">
                  <span className="font-medium text-[#202124] dark:text-[#F1F2F4] truncate max-w-[200px]">
                    {m.name}
                  </span>
                  <span className="text-[#656B75] dark:text-[#A1A7B0]">
                    {m.tokens} tokens ({m.percent}%)
                  </span>
                </div>
                <div className="w-full bg-[#F0F1F2] dark:bg-[#22252B] h-1.5 rounded-full overflow-hidden">
                  <div
                    className="bg-[#355CFF] h-full rounded-full transition-all duration-300"
                    style={{ width: `${Math.max(6, m.percent)}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
