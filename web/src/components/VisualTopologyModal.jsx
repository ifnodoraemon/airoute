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
  const isZh = lang === 'zh';
  const [simulatedFailure, setSimulatedFailure] = useState(false);
  const [selectedRouteModel, setSelectedRouteModel] = useState('deepseek-v3');

  if (!isOpen) return null;

  const activeChannels = channels.filter(c => c.status === 'active');
  const primaryChannel = activeChannels[0] || { name: isZh ? 'GPUStack 本地私有集群' : 'GPUStack Private Cluster', type: 'gpustack', priority: 1, breaker_status: 'CLOSED' };
  const backupChannel = activeChannels[1] || { name: isZh ? 'Sub2API 聚合冗余通道' : 'Sub2API Redundant Channel', type: 'sub2api', priority: 2, breaker_status: 'CLOSED' };
  const drChannel = activeChannels[2] || { name: isZh ? '官方直连备灾通道' : 'Official Direct DR Channel', type: 'openai', priority: 3, breaker_status: 'CLOSED' };

  return (
    <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
      <div className="bg-white dark:bg-[#111726] border border-slate-200 dark:border-slate-800 rounded-3xl p-6 sm:p-8 max-w-4xl w-full shadow-2xl relative max-h-[92vh] overflow-y-auto">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-slate-100 dark:border-slate-800 mb-6">
          <div className="flex items-center space-x-3">
            <div className="w-10 h-10 rounded-2xl bg-gradient-to-tr from-indigo-600 to-emerald-500 text-white flex items-center justify-center shadow-lg shadow-indigo-500/20 shrink-0">
              <Layers className="w-5 h-5 fill-white text-white" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-slate-900 dark:text-slate-100 tracking-tight flex items-center space-x-2">
                <span>{isZh ? '智能容灾拓扑与模型调度流向图' : 'Visual Routing & Failover Flow Topology'}</span>
                <span className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-emerald-50 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800">
                  Pre-Token Fallback SLA 99.99%
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
                setSimulatedFailure(!simulatedFailure);
                if (showToast) {
                  showToast(
                    simulatedFailure
                      ? (isZh ? '已恢复正常生产调度拓扑' : 'Restored normal production topology')
                      : (isZh ? '已注入首选渠道 429 故障，触发毫秒级透明漂移！' : 'Injected primary 429 outage: triggered zero-loss failover!'),
                    simulatedFailure ? 'info' : 'warning'
                  );
                }
              }}
              className={`px-3 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer border ${
                simulatedFailure
                  ? 'bg-rose-50 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300 border-rose-300 dark:border-rose-800'
                  : 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800 hover:bg-indigo-100'
              }`}
            >
              {simulatedFailure ? <RotateCcw className="w-3.5 h-3.5 animate-spin" /> : <ShieldAlert className="w-3.5 h-3.5 text-amber-500" />}
              <span>{simulatedFailure ? (isZh ? '复位故障模拟' : 'Reset Simulation') : (isZh ? '演练注入故障 (Chaos)' : 'Simulate Outage (Chaos)')}</span>
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
                  ? '⚠️ 首选提供商 (Priority 1) 发生 429 Rate Limit 限流 / 500 异常'
                  : '⚠️ Primary Provider (Priority 1) Hit 429 Rate Limit / 500 Outage'}
              </div>
              <p className="text-[11px] leading-relaxed">
                {isZh
                  ? 'Airoute 数据面拦截异常并在首字输出前（15ms 内）自动旁路故障节点，流量无感切换至 Priority 2 备用提供商。客户端请求保持 100% 连通与不中断！'
                  : 'Airoute intercepted the error before the first chunk (<15ms) and automatically bypassed the faulty node, shifting traffic smoothly to Priority 2. Zero client downtime!'}
              </p>
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
            <span className="text-[10px] text-slate-400 font-mono">OpenAI / Claude / Gemini SDK</span>
          </div>

          {/* Stage 2: Rewrite & Normalization */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-purple-50 dark:bg-purple-950 text-purple-600 dark:text-purple-400 flex items-center justify-center font-bold">
              <Cpu className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '2. 协议与前缀重写' : '2. Normalize & Rewrite'}</span>
            <span className="text-[10px] text-slate-400 font-mono">{isZh ? '剥离前缀 / 全双工互转' : 'Prefix strip / Translation'}</span>
          </div>

          {/* Stage 3: Priority Fallback Queue */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-emerald-50 dark:bg-emerald-950 text-emerald-600 dark:text-emerald-400 flex items-center justify-center font-bold">
              <Layers className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '3. 动态优先级队列' : '3. Fallback Queue'}</span>
            <span className="text-[10px] text-slate-400 font-mono">{isZh ? 'WRR 权重 + 优先级' : 'Priority + WRR Weights'}</span>
          </div>

          {/* Stage 4: Circuit Breaker Matrix */}
          <div className={`p-4 rounded-2xl border flex flex-col items-center text-center space-y-2 transition ${
            simulatedFailure 
              ? 'bg-rose-50/70 dark:bg-rose-950/40 border-rose-300 dark:border-rose-800 text-rose-950'
              : 'bg-slate-50 dark:bg-slate-900/60 border-slate-200 dark:border-slate-800'
          }`}>
            <div className={`w-10 h-10 rounded-xl flex items-center justify-center font-bold ${
              simulatedFailure
                ? 'bg-rose-100 text-rose-600 animate-pulse'
                : 'bg-amber-50 dark:bg-amber-950 text-amber-600'
            }`}>
              <Shield className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '4. 三态熔断器矩阵' : '4. Breaker Matrix'}</span>
            <span className="text-[10px] font-mono text-slate-400">
              {simulatedFailure ? (isZh ? '首选已熔断 ➔ 旁路' : 'Primary Tripped ➔ Bypassed') : (isZh ? '闭合 Closed (正常)' : 'Closed (Healthy)')}
            </span>
          </div>

          {/* Stage 5: Upstream Providers */}
          <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 flex flex-col items-center text-center space-y-2">
            <div className="w-10 h-10 rounded-xl bg-cyan-50 dark:bg-cyan-950 text-cyan-600 dark:text-cyan-400 flex items-center justify-center font-bold">
              <Server className="w-5 h-5" />
            </div>
            <span className="font-bold text-xs text-slate-900 dark:text-slate-100">{isZh ? '5. 目标上游集群' : '5. Upstream Cluster'}</span>
            <span className="text-[10px] text-slate-400 font-mono">{channels.length} {isZh ? '个承载节点' : 'Providers Active'}</span>
          </div>
        </div>

        {/* Live Fallback Routing Table */}
        <div className="mt-4 p-5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 space-y-3">
          <h4 className="font-bold text-xs text-slate-800 dark:text-slate-200 flex items-center justify-between">
            <span>{isZh ? '当前选路与容灾排队顺序 (Fallback Execution Queue)' : 'Fallback Execution Queue & Dispatch Status'}</span>
            <span className="text-[11px] text-indigo-600 dark:text-indigo-400 font-mono">Model: {selectedRouteModel}</span>
          </h4>

          <div className="space-y-2">
            {/* Priority 1 */}
            <div className={`p-3 rounded-xl border flex items-center justify-between text-xs transition ${
              simulatedFailure
                ? 'bg-rose-50/60 dark:bg-rose-950/30 border-rose-300 dark:border-rose-900/60 opacity-60'
                : 'bg-white dark:bg-slate-800 border-emerald-300 dark:border-emerald-800'
            }`}>
              <div className="flex items-center space-x-2.5">
                <span className="w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center font-bold text-[10px]">
                  1
                </span>
                <span className="font-semibold text-slate-800 dark:text-slate-200">{primaryChannel.name}</span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 font-mono text-slate-500">
                  {primaryChannel.type}
                </span>
              </div>
              <div className="flex items-center space-x-2">
                {simulatedFailure ? (
                  <span className="text-rose-600 dark:text-rose-400 font-bold text-[11px] flex items-center space-x-1">
                    <ShieldAlert className="w-3.5 h-3.5" />
                    <span>{isZh ? '已熔断跳闸 (旁路跳过)' : 'Tripped (Bypassed)'}</span>
                  </span>
                ) : (
                  <span className="text-emerald-600 dark:text-emerald-400 font-bold text-[11px] flex items-center space-x-1">
                    <CheckCircle2 className="w-3.5 h-3.5" />
                    <span>{isZh ? '首选承载通道 (Closed)' : 'Primary Active (Closed)'}</span>
                  </span>
                )}
              </div>
            </div>

            {/* Priority 2 */}
            <div className={`p-3 rounded-xl border flex items-center justify-between text-xs transition ${
              simulatedFailure
                ? 'bg-emerald-50/80 dark:bg-emerald-950/40 border-emerald-400 dark:border-emerald-800 shadow-xs'
                : 'bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700'
            }`}>
              <div className="flex items-center space-x-2.5">
                <span className="w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center font-bold text-[10px]">
                  2
                </span>
                <span className="font-semibold text-slate-800 dark:text-slate-200">{backupChannel.name}</span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 font-mono text-slate-500">
                  {backupChannel.type}
                </span>
              </div>
              <div className="flex items-center space-x-2">
                {simulatedFailure ? (
                  <span className="text-emerald-600 dark:text-emerald-400 font-bold text-[11px] flex items-center space-x-1 animate-pulse">
                    <Zap className="w-3.5 h-3.5" />
                    <span>{isZh ? '⚡ 自动接管流量中 (Active Target)' : '⚡ Active Target (Auto-Failover)'}</span>
                  </span>
                ) : (
                  <span className="text-slate-400 text-[11px]">{isZh ? '热备就绪 (Hot Standby)' : 'Hot Standby'}</span>
                )}
              </div>
            </div>

            {/* Priority 3 */}
            <div className="p-3 rounded-xl bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 flex items-center justify-between text-xs">
              <div className="flex items-center space-x-2.5">
                <span className="w-5 h-5 rounded-full bg-slate-100 dark:bg-slate-700 flex items-center justify-center font-bold text-[10px]">
                  3
                </span>
                <span className="font-semibold text-slate-800 dark:text-slate-200">{drChannel.name}</span>
                <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-700 font-mono text-slate-500">
                  {drChannel.type}
                </span>
              </div>
              <span className="text-slate-400 text-[11px]">{isZh ? '三级底线兜底 (Cold DR Backup)' : 'Cold DR Backup'}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
