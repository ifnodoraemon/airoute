import { Activity, Zap, Clock, DollarSign } from 'lucide-react';

export default function DashboardKpis({
  currentRPM, currentTPM, fmtPct, isZh, percentiles, slaAvailability, stats, tokenMetrics
}) {
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      {/* Total Requests */}
      <div className="ui-card p-5 flex flex-col justify-between">
        <div className="flex items-center justify-between">
          <span className="ui-section-title">{isZh ? '累计调用量' : 'Total Requests'}</span>
          <Zap className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
        </div>
        <div className="mt-2 ui-metric-number font-mono tabular-nums">
          {(stats.total_requests || 0).toLocaleString()}
        </div>
        <div className="ui-meta mt-2 flex items-center justify-between font-mono">
          <div className="flex items-center gap-1.5">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0"></span>
            <span>SLA {slaAvailability}{slaAvailability === '—' ? '' : '%'}</span>
          </div>
          <span className="text-[#355CFF] dark:text-[#91A7FF] font-semibold">{(currentRPM / 60).toFixed(1)} req/s</span>
        </div>
      </div>

      {/* Total Tokens */}
      <div className="ui-card p-5 flex flex-col justify-between">
        <div className="flex items-center justify-between">
          <span className="ui-section-title">{isZh ? 'Token 总吞吐' : 'Total Tokens'}</span>
          <Activity className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
        </div>
        <div className="mt-2 ui-metric-number font-mono tabular-nums">
          {(stats.total_tokens || 0).toLocaleString()}
        </div>
        <div className="ui-meta mt-2 flex items-center justify-between font-mono">
          <span>Prompt {tokenMetrics.promptPctStr} · Comp {tokenMetrics.compPctStr}</span>
          <span className="text-[#355CFF] dark:text-[#91A7FF] font-semibold">{currentTPM >= 1000 ? `${(currentTPM / 1000).toFixed(1)}k` : currentTPM} TPM</span>
        </div>
      </div>

      {/* Avg TTFT */}
      <div className="ui-card p-5 flex flex-col justify-between">
        <div className="flex items-center justify-between">
          <span className="ui-section-title">{isZh ? '平均首字延迟' : 'Avg TTFT'}</span>
          <Clock className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
        </div>
        <div className="mt-2 ui-metric-number font-mono tabular-nums">
          {Math.round(stats.avg_ttft_ms || 0)} <span className="text-sm font-normal text-[#656B75] dark:text-[#A1A7B0]">ms</span>
        </div>
        <div className="ui-meta mt-2 font-mono">
          {percentiles.sampled > 0
            ? `P50: ${fmtPct(percentiles.p50)} · P90: ${fmtPct(percentiles.p90)} (${isZh ? `样本 ${percentiles.sampled}` : `${percentiles.sampled} samples`})`
            : (isZh ? 'P50/P90: 暂无延迟样本' : 'P50/P90: no latency samples yet')}
        </div>
      </div>

      {/* Total Cost & Saved */}
      <div className="ui-card p-5 flex flex-col justify-between">
        <div className="flex items-center justify-between">
          <span className="ui-section-title">{isZh ? '累计消耗与节约' : 'Total Cost & Saved'}</span>
          <DollarSign className="w-4 h-4 text-[#656B75] dark:text-[#A1A7B0]" />
        </div>
        <div className="mt-2 ui-metric-number font-mono tabular-nums">
          ¥{(stats.total_cost || 0).toFixed(4)}
        </div>
        <div className="ui-meta mt-2 font-mono">
          {isZh ? '缓存减免' : 'Cache Saved'}: ¥{(stats.saved_cost || 0).toFixed(4)}
        </div>
      </div>
    </div>
  );
}
