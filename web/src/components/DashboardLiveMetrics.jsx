import { Gauge, X, Sliders } from 'lucide-react';
import { keyDisplayRef } from '../lib/apiKey';

export default function DashboardLiveMetrics({
  activeKeyObj, activeRequests, activeStreams, adminUser, attention, channels, currentRPM, currentTPM,
  handleSelectKeyDrilldown, isZh, keys, liveDetail, livePhase, peakRPM, peakTPM, selectedKey, slaAvailability,
  stats
}) {
  return (
    <section className="ui-card p-5">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-4 border-b border-[#E4E6E9] dark:border-[#2C3038]">
        <div className="flex items-center gap-2.5">
          <div className="p-2 rounded-xl bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF]">
            <Gauge className="w-5 h-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="text-base font-semibold text-[#202124] dark:text-[#F1F2F4]">
                {selectedKey
                  ? (isZh ? `密钥实时速率遥测 · ${activeKeyObj?.tenant_id || activeKeyObj?.name || `密钥 #${selectedKey}`}` : `Key Telemetry · ${activeKeyObj?.tenant_id || activeKeyObj?.name || `Key #${selectedKey}`}`)
                  : (adminUser?.role === 'admin'
                      ? (isZh ? '全平台实时速率遥测 (Platform Telemetry)' : 'Platform Real-time Telemetry')
                      : (isZh ? '我的密钥聚合实时遥测 (Account Telemetry)' : 'My Keys Real-time Telemetry')
                    )
                }
              </h3>
              <span
                className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-mono font-medium border ${
                  livePhase === 'idle'
                    ? 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border-[#E4E6E9] dark:border-[#2C3038]'
                    : livePhase === 'busy'
                      ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border-[#355CFF]/25'
                      : 'bg-emerald-500/10 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                }`}
              >
                <span
                  className={`w-1.5 h-1.5 rounded-full ${
                    livePhase === 'idle'
                      ? 'bg-[#9AA0A6] dark:bg-[#7C828B]'
                      : livePhase === 'busy'
                        ? 'bg-[#355CFF] animate-pulse'
                        : 'bg-emerald-500 animate-pulse'
                  }`}
                ></span>
                {livePhase === 'streaming'
                  ? (isZh ? `生成中 ${activeStreams}` : `Generating ${activeStreams}`)
                  : livePhase === 'busy'
                    ? (isZh ? `处理中 ${activeRequests}` : `Working ${activeRequests}`)
                    : livePhase === 'idle'
                      ? (isZh ? '空闲' : 'Idle')
                      : 'LIVE 60s'}
              </span>
            </div>
            <p className="ui-meta mt-0.5">
              {selectedKey
                ? (isZh ? '该密钥近 60 秒速率与限流水位' : 'Last 60s key rate and quota')
                : (isZh ? '近 60 秒滑动窗口 · 含周期峰值' : '60s sliding window with peak')
              }
            </p>
          </div>
        </div>

        {/* Drill-down selector */}
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-[#656B75] dark:text-[#A1A7B0] whitespace-nowrap">
            {isZh ? '下钻分析:' : 'Drill-down:'}
          </span>
          <div className="relative min-w-[220px]">
            <select
              value={selectedKey}
              onChange={(e) => handleSelectKeyDrilldown(e.target.value)}
              className="ui-field ui-field-sm font-mono text-xs w-full pr-8 cursor-pointer"
              aria-label={isZh ? '下钻密钥' : 'Drill-down key'}
            >
              <option value="">
                {adminUser?.role === 'admin'
                  ? (isZh ? '🌐 全平台聚合遥测 (Platform Overall)' : '🌐 All Platform Telemetry')
                  : (isZh ? '👤 我的全量密钥聚合 (My Keys Overall)' : '👤 All My Keys Telemetry')}
              </option>
              {(keys || []).map(k => (
                <option key={String(k.id)} value={String(k.id)}>
                  🔑 {k.tenant_id || k.name || '—'} · {keyDisplayRef(k)} {k.rpm ? `(${k.rpm} RPM)` : ''}
                </option>
              ))}
            </select>
          </div>
          {selectedKey && (
            <button
              onClick={() => handleSelectKeyDrilldown('')}
              className="ui-btn-secondary ui-btn-sm"
              title={isZh ? '返回全局大盘' : 'Reset drill-down'}
            >
              <X className="w-3.5 h-3.5" />
              <span>{isZh ? '重置' : 'Reset'}</span>
            </button>
          )}
        </div>
      </div>

      {/* 3 Telemetry Speedometers */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mt-4">
        {/* Card 1: Requests per sec & RPM */}
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038] flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-xs text-[#656B75] dark:text-[#A1A7B0]">
              <span className="font-medium">{isZh ? '实时请求速率 (RPS / RPM)' : 'Real-time RPS / RPM'}</span>
              <span className="font-mono text-[#355CFF] dark:text-[#91A7FF] font-semibold">
                {currentRPM} RPM
              </span>
            </div>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-3xl font-bold font-mono text-[#202124] dark:text-[#F1F2F4] tracking-tight">
                {(currentRPM / 60).toFixed(1)}
              </span>
              <span className="text-sm font-medium text-[#656B75] dark:text-[#A1A7B0]">
                req/s
              </span>
            </div>
          </div>

          <div className="mt-3 pt-2.5 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] flex items-center justify-between">
            <span>{isZh ? '周期峰值' : 'Peak RPM'}: <strong className="font-mono text-[#202124] dark:text-[#F1F2F4]">{peakRPM}</strong> RPM</span>
            {activeKeyObj?.rpm ? (
              <span className="font-mono text-xs">
                {isZh ? '限流水位' : 'Limit'}: {Math.min(100, Math.round((currentRPM / activeKeyObj.rpm) * 100))}%
              </span>
            ) : null}
          </div>

          {/* Quota Progress Bar if key has RPM limit */}
          {activeKeyObj?.rpm > 0 && (
            <div className="mt-2 w-full bg-[#E4E6E9] dark:bg-[#2C3038] rounded-full h-1.5 overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-300 ${
                  currentRPM >= activeKeyObj.rpm
                    ? 'bg-rose-500'
                    : currentRPM >= activeKeyObj.rpm * 0.8
                    ? 'bg-amber-500'
                    : 'bg-[#355CFF]'
                }`}
                style={{ width: `${Math.min(100, (currentRPM / activeKeyObj.rpm) * 100)}%` }}
              />
            </div>
          )}
        </div>

        {/* Card 2: Tokens per min (TPM) */}
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038] flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-xs text-[#656B75] dark:text-[#A1A7B0]">
              <span className="font-medium">{isZh ? '实时 Token 吞吐 (TPM)' : 'Real-time TPM'}</span>
              <span className="font-mono text-[#355CFF] dark:text-[#91A7FF] font-semibold">
                {currentTPM >= 1000 ? `${(currentTPM / 1000).toFixed(1)}k` : currentTPM} /min
              </span>
            </div>
            <div className="mt-2 flex items-baseline gap-2">
              <span className="text-3xl font-bold font-mono text-[#202124] dark:text-[#F1F2F4] tracking-tight">
                {currentTPM.toLocaleString()}
              </span>
              <span className="text-sm font-medium text-[#656B75] dark:text-[#A1A7B0]">
                Tokens/min
              </span>
            </div>
          </div>

          <div className="mt-3 pt-2.5 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] flex flex-wrap items-center justify-between gap-2">
            <span>{isZh ? '周期峰值' : 'Peak TPM'}: <strong className="font-mono text-[#202124] dark:text-[#F1F2F4]">{peakTPM.toLocaleString()}</strong> TPM</span>
            <span className="font-mono text-xs text-right">
              {liveDetail ?? `~${Math.round(currentTPM / 60)} tok/s`}
            </span>
          </div>

          {/* Quota Progress Bar if key has TPM limit */}
          {activeKeyObj?.tpm > 0 && (
            <div className="mt-2 w-full bg-[#E4E6E9] dark:bg-[#2C3038] rounded-full h-1.5 overflow-hidden">
              <div
                className={`h-full rounded-full transition-all duration-300 ${
                  currentTPM >= activeKeyObj.tpm
                    ? 'bg-rose-500'
                    : currentTPM >= activeKeyObj.tpm * 0.8
                    ? 'bg-amber-500'
                    : 'bg-[#355CFF]'
                }`}
                style={{ width: `${Math.min(100, (currentTPM / activeKeyObj.tpm) * 100)}%` }}
              />
            </div>
          )}
        </div>

        {/* Card 3: Key Status / Overall Concurrency Summary */}
        <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038] flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between text-xs text-[#656B75] dark:text-[#A1A7B0]">
              <span className="font-medium">
                {selectedKey ? (isZh ? '密钥运行状态与分组' : 'Key Status & Group') : (isZh ? '网关并发健康度' : 'Gateway Concurrency')}
              </span>
              <Sliders className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
            </div>
            {selectedKey ? (
              <div className="mt-2 space-y-1.5">
                <div className="flex items-center gap-2">
                  <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${
                    activeKeyObj?.status === 'disabled'
                      ? 'bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                      : 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                  }`}>
                    <span className={`w-1.5 h-1.5 rounded-full ${activeKeyObj?.status === 'disabled' ? 'bg-rose-500' : 'bg-emerald-500'}`} />
                    {activeKeyObj?.status === 'disabled' ? (isZh ? '已停用' : 'Disabled') : (isZh ? '运行中' : 'Active')}
                  </span>
                  <span className="font-mono text-xs px-2 py-0.5 rounded bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF]">
                    {activeKeyObj?.group_name || 'default'} 组
                  </span>
                </div>
                <div className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-mono">
                  {isZh ? '消耗额度' : 'Spent'}: ¥{Number(activeKeyObj?.used_cost || 0).toFixed(4)}
                  {Number(activeKeyObj?.budget) > 0 ? ` / ¥${Number(activeKeyObj.budget).toFixed(2)}` : ` (${isZh ? '不限预算' : 'No cap'})`}
                </div>
              </div>
            ) : (
              <div className="mt-2">
                <div className="flex items-baseline flex-wrap gap-x-2 gap-y-1">
                  <div className="text-2xl font-bold font-mono text-[#202124] dark:text-[#F1F2F4]">
                    {channels.filter(c => c.status === 'active').length} <span className="text-sm font-normal text-[#656B75] dark:text-[#A1A7B0]">/ {channels.length} {isZh ? '渠道在线' : 'channels active'}</span>
                  </div>
                  {livePhase && (
                    <span className="ml-auto px-1.5 py-0.5 rounded border border-[#E4E6E9] dark:border-[#2C3038] font-mono text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
                      {isZh ? `当前并发 ${activeRequests}` : `Concurrency ${activeRequests}`}
                    </span>
                  )}
                </div>
                <div className="ui-meta mt-1">
                  {attention.tripped === 0
                    ? (isZh ? '全链路断路器健康闭合' : 'All circuit breakers closed')
                    : (isZh ? `${attention.tripped} 个渠道触发断路保护` : `${attention.tripped} breaker(s) open`)}
                </div>
              </div>
            )}
          </div>

          <div className="mt-3 pt-2.5 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] flex items-center justify-between">
            <span>{isZh ? '调用成功率' : 'Success Rate'}: <strong className="font-mono text-emerald-600 dark:text-emerald-400">{slaAvailability}{slaAvailability === '—' ? '' : '%'}</strong></span>
            <span className="font-mono">TTFT ~{Math.round(stats.avg_ttft_ms || 0)}ms</span>
          </div>
        </div>
      </div>
    </section>
  );
}
