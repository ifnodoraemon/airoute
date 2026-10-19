import React, { useState } from 'react';
import {
  X,
  Server,
  Cpu,
  Layers,
  Shield,
  ShieldAlert,
  ArrowRight,
  Zap,
  RefreshCw,
  Activity,
  AlertTriangle,
  CheckCircle2,
  Play,
  RotateCcw
} from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';

export default function VisualTopologyModal({
  isOpen,
  onClose,
  channels = [],
  modelRoutes = [],
  onTestChannel,
  showToast,
  lang = 'zh',
  t
}) {
  useModalA11y(isOpen, onClose);

  const isZh = lang === 'zh';
  const [simulatedFailure, setSimulatedFailure] = useState(false);

  if (!isOpen) return null;

  // Derive the displayed model from real route data instead of a hardcoded name.
  const routedModels = (modelRoutes || []).map(r => r.model).filter(Boolean);
  const previewModel = routedModels.length
    ? (routedModels.length === 1 ? routedModels[0] : `${routedModels[0]} +${routedModels.length - 1}`)
    : (isZh ? '未配置模型路由' : 'No model routes');

  const activeChannels = channels.filter(c => c.status === 'active');
  // Slots without a real channel are rendered as "not configured". They used to
  // be filled with invented channel names (GPUStack / Sub2API / Official DR),
  // which made an unconfigured deployment look like a resilient cluster.
  const emptySlot = (tier) => ({
    name: isZh ? `未配置 (Priority ${tier})` : `Not configured (Priority ${tier})`,
    type: '—',
    priority: tier,
    breaker_status: 'CLOSED',
    __placeholder: true,
  });
  const primaryChannel = activeChannels[0] || emptySlot(1);
  const backupChannel = activeChannels[1] || emptySlot(2);
  const drChannel = activeChannels[2] || emptySlot(3);

  return (
    <div
      className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="ui-card p-6 sm:p-8 max-w-4xl w-full shadow-2xl relative max-h-[92vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-slate-100 dark:border-slate-800 mb-6">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-xl bg-slate-900 dark:bg-indigo-600 text-white flex items-center justify-center shrink-0 shadow-sm">
              <Layers className="w-5 h-5 text-white" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-slate-900 dark:text-slate-100 tracking-tight flex items-center space-x-2">
                <span>{isZh ? '智能容灾拓扑与模型调度流向图' : 'Visual Routing & Failover Flow Topology'}</span>
                <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-emerald-50 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800">
                  Pre-Token Fallback
                </span>
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                {isZh
                  ? '全链路可视化展现：入站协议转译 ➔ 级联前缀剥离 ➔ 优先级动态选路 ➔ 三态熔断器容灾'
                  : 'Full-duplex pipeline: Inbound translation ➔ Prefix stripping ➔ Dynamic priority queue ➔ Tri-state breaker fallback'}
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-2">
            <button
              onClick={() => {
                // This toggle only re-renders the diagram with a hypothetical
                // outage. It does NOT inject anything, so the copy must not claim
                // that failover was exercised — doing so made the picture look
                // like proof of resilience.
                setSimulatedFailure(!simulatedFailure);
                if (showToast) {
                  showToast(
                    simulatedFailure
                      ? (isZh ? '已退出故障推演视图' : 'Exited outage what-if view')
                      : (isZh ? '已进入故障推演视图（仅示意，未真实注入）' : 'Outage what-if view (illustration only, nothing was injected)'),
                    'info'
                  );
                }
              }}
              className={`ui-btn ui-btn-sm rounded-xl font-semibold border ${
                simulatedFailure
                  ? 'bg-rose-50 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300 border-rose-300 dark:border-rose-800'
                  : 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800 hover:bg-indigo-100 dark:hover:bg-indigo-900/60'
              }`}
            >
              {simulatedFailure ? <RotateCcw className="w-3.5 h-3.5 animate-spin" /> : <ShieldAlert className="w-3.5 h-3.5 text-amber-500" />}
              <span>{simulatedFailure ? (isZh ? '退出推演' : 'Exit What-if') : (isZh ? '故障推演 (仅示意)' : 'Outage What-if (illustrative)')}</span>
            </button>

            <button
              onClick={onClose}
              className="p-2 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition cursor-pointer"
            >
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Chaos Alert Banner */}
        {simulatedFailure && (
          <div className="mb-6 p-4 rounded-2xl bg-amber-50 dark:bg-amber-950/40 border border-amber-300 dark:border-amber-800/80 text-xs text-amber-900 dark:text-amber-200 flex items-start space-x-3 animate-in slide-in-from-top-2">
            <AlertTriangle className="w-5 h-5 text-amber-600 shrink-0 mt-0.5" />
            <div className="space-y-1">
              <div className="font-bold">
                {isZh
                  ? '⚠️ 推演场景：首选提供商 (Priority 1) 遭遇 429 限流 / 500 异常'
                  : '⚠️ What-if scenario: Primary Provider (Priority 1) hits 429 / 500'}
              </div>
              <p className="text-xs leading-relaxed">
                {isZh
                  ? '若首个内容分块尚未发出，网关会尝试 Priority 2 备用渠道；切换耗时取决于上游响应和超时配置。'
                  : 'If no content has been sent, the gateway tries the Priority 2 backup. Failover time depends on upstream response and configured timeouts.'}
              </p>
              <p className="text-xs leading-relaxed font-medium opacity-90">
                {isZh
                  ? '注意：此视图仅演示配置层面的降级路径，并未真实注入故障。要验证容灾是否有效，请对渠道执行实时连通性探测。'
                  : 'Note: this view only illustrates the configured fallback path; no fault was actually injected. To verify failover for real, run a live connectivity probe on the channel.'}
              </p>
              {onTestChannel && primaryChannel && !primaryChannel.__placeholder && (
                <button
                  type="button"
                  onClick={() => onTestChannel(primaryChannel)}
                  className="ui-btn ui-btn-sm mt-1 bg-amber-600 hover:bg-amber-700 text-white font-semibold"
                >
                  {isZh ? '对首选渠道执行实时探测' : 'Run live probe on primary channel'}
                </button>
              )}
            </div>
          </div>
        )}

        {/* 5-Stage Visual Pipeline Graph */}
        <div className="grid grid-cols-1 md:grid-cols-5 gap-3 relative py-4">
          {/* Stage 1: Client Inbound */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-indigo-50 dark:bg-indigo-950 text-indigo-600 dark:text-indigo-400 flex items-center justify-center font-bold">
              <Zap className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '1. 客户端 Inbound' : '1. Client Inbound'}</span>
            <span className="ui-meta font-mono">OpenAI / Claude / Gemini SDK</span>
          </div>

          {/* Stage 2: Rewrite & Normalization */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-purple-50 dark:bg-purple-950 text-purple-600 dark:text-purple-400 flex items-center justify-center font-bold">
              <Cpu className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '2. 协议与前缀重写' : '2. Normalize & Rewrite'}</span>
            <span className="ui-meta font-mono">{isZh ? '剥离前缀 / 全双工互转' : 'Prefix strip / Translation'}</span>
          </div>

          {/* Stage 3: Priority Fallback Queue */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-950 text-emerald-600 dark:text-emerald-400 flex items-center justify-center font-bold">
              <Layers className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '3. 动态优先级队列' : '3. Fallback Queue'}</span>
            <span className="ui-meta font-mono">{isZh ? 'WRR 权重 + 优先级' : 'Priority + WRR Weights'}</span>
          </div>

          {/* Stage 4: Circuit Breaker Matrix */}
          <div className={`p-4 rounded-2xl border flex flex-col items-center text-center space-y-2 transition ${
            simulatedFailure 
              ? 'bg-rose-50/70 dark:bg-rose-950/40 border-rose-300 dark:border-rose-800 text-rose-950 dark:text-rose-100'
              : 'bg-slate-50 dark:bg-slate-900/60 border-slate-200 dark:border-slate-800'
          }`}>
            <div className={`w-10 h-10 rounded-xl flex items-center justify-center font-bold ${
              simulatedFailure
                ? 'bg-rose-100 dark:bg-rose-950/60 text-rose-600 dark:text-rose-300 animate-pulse'
                : 'bg-amber-50 dark:bg-amber-950 text-amber-600 dark:text-amber-400'
            }`}>
              <Shield className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '4. 三态熔断器矩阵' : '4. Breaker Matrix'}</span>
            <span className="ui-meta font-mono">
              {simulatedFailure ? (isZh ? '首选已熔断 ➔ 旁路' : 'Primary Tripped ➔ Bypassed') : (isZh ? '闭合 Closed (正常)' : 'Closed (Healthy)')}
            </span>
          </div>

          {/* Stage 5: Upstream Providers */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-cyan-50 dark:bg-cyan-950 text-cyan-600 dark:text-cyan-400 flex items-center justify-center font-bold">
              <Server className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '5. 目标上游集群' : '5. Upstream Cluster'}</span>
            <span className="ui-meta font-mono">{channels.length} {isZh ? '个承载节点' : 'Providers Active'}</span>
          </div>
        </div>

        {/* Live Fallback Routing Table */}
        <div className="mt-4 p-5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 space-y-3">
          <h4 className="font-bold text-xs text-slate-800 dark:text-slate-200 flex items-center justify-between">
            <span>{isZh ? '当前选路与容灾排队顺序 (Fallback Execution Queue)' : 'Fallback Execution Queue & Dispatch Status'}</span>
            <span className="text-xs text-indigo-600 dark:text-indigo-400 font-mono">Model: {previewModel}</span>
          </h4>

          <div className="space-y-2">
            {/* Priority 1 */}
            <div className={`p-3 rounded-xl border flex items-center justify-between text-xs transition ${
              simulatedFailure
                ? 'bg-rose-50/60 dark:bg-rose-950/30 border-rose-300 dark:border-rose-900/60 opacity-60'
                : 'bg-white dark:bg-slate-800 border-emerald-300 dark:border-emerald-800'
            }`}>
              <div className="flex items-center space-x-2.5">
                <span className="w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center font-bold text-xs">
                  1
                </span>
                <span className="font-semibold text-slate-800 dark:text-slate-200">{primaryChannel.name}</span>
                <span className="ui-chip font-mono">
                  {primaryChannel.type}
                </span>
              </div>
              <div className="flex items-center space-x-2">
                {simulatedFailure ? (
                  <span className="text-rose-600 dark:text-rose-400 font-bold text-xs flex items-center space-x-1">
                    <ShieldAlert className="w-3.5 h-3.5" />
                    <span>{isZh ? '已熔断跳闸 (旁路跳过)' : 'Tripped (Bypassed)'}</span>
                  </span>
                ) : (
                  <span className="text-emerald-600 dark:text-emerald-400 font-bold text-xs flex items-center space-x-1">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    <span>{isZh ? '首选承载通道 (Closed)' : 'Primary Active (Closed)'}</span>
                  </span>
                )}
              </div>
            </div>

            {/* Priority 2 */}
            <div className={`p-3 rounded-xl border flex items-center justify-between text-xs transition ${
              simulatedFailure
                ? 'bg-emerald-50/80 dark:bg-emerald-950/40 border-emerald-400 dark:border-emerald-800 shadow-sm'
                : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700'
            }`}>
              <div className="flex items-center space-x-2.5">
                <span className="w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center font-bold text-xs">
                  2
                </span>
                <span className="font-semibold text-slate-800 dark:text-slate-200">{backupChannel.name}</span>
                <span className="ui-chip font-mono">
                  {backupChannel.type}
                </span>
              </div>
              <div className="flex items-center space-x-2">
                {simulatedFailure ? (
                  <span className="text-emerald-600 dark:text-emerald-400 font-bold text-xs flex items-center space-x-1 animate-pulse">
                    <Zap className="w-3.5 h-3.5" />
                    <span>{isZh ? '⚡ 自动接管流量中 (Active Target)' : '⚡ Active Target (Auto-Failover)'}</span>
                  </span>
                ) : (
                  <span className="ui-meta">{isZh ? '热备就绪 (Hot Standby)' : 'Hot Standby'}</span>
                )}
              </div>
            </div>

            {/* Priority 3 */}
            <div className="p-3 rounded-xl bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 flex items-center justify-between text-xs">
              <div className="flex items-center space-x-2.5">
                <span className="w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center font-bold text-xs">
                  3
                </span>
                <span className="font-semibold text-slate-800 dark:text-slate-200">{drChannel.name}</span>
                <span className="ui-chip font-mono">
                  {drChannel.type}
                </span>
              </div>
              <span className="ui-meta">{isZh ? '三级底线兜底 (Cold DR Backup)' : 'Cold DR Backup'}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
