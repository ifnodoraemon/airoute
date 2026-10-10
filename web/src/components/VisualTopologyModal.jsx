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
  showToast
}) {
  const [simulatedFailure, setSimulatedFailure] = useState(false);
  const [selectedRouteModel, setSelectedRouteModel] = useState('deepseek-v3');

  if (!isOpen) return null;

  const activeChannels = channels.filter(c => c.status === 'active');
  const primaryChannel = activeChannels[0] || { name: 'GPUStack 本地私有集群', type: 'gpustack', priority: 1, breaker_status: 'CLOSED' };
  const backupChannel = activeChannels[1] || { name: 'Sub2API 聚合冗余通道', type: 'sub2api', priority: 2, breaker_status: 'CLOSED' };
  const drChannel = activeChannels[2] || { name: '官方直连备灾通道', type: 'openai', priority: 3, breaker_status: 'CLOSED' };

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
                <span>智能容灾拓扑与模型调度流向图</span>
                <span className="text-[11px] font-semibold px-2 py-0.5 rounded-full bg-emerald-50 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800">
                  Pre-Token Fallback SLA 99.99%
                </span>
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                全链路可视化展现：入站协议转译 ➔ 级联前缀剥离 ➔ 优先级动态选路 ➔ 三态熔断器容灾
              </p>
            </div>
          </div>

          <div className="flex items-center space-x-2">
            <button
              onClick={() => {
                setSimulatedFailure(!simulatedFailure);
                if (showToast) {
                  showToast(simulatedFailure ? '已恢复正常生产调度拓扑' : '已注入首选渠道 429 故障，触发毫秒级透明漂移！', simulatedFailure ? 'info' : 'warning');
                }
              }}
              className={`px-3 py-1.5 rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer border ${
                simulatedFailure
                  ? 'bg-rose-50 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300 border-rose-300 dark:border-rose-800'
                  : 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800 hover:bg-indigo-100'
              }`}
            >
              {simulatedFailure ? <RotateCcw className="w-3.5 h-3.5 animate-spin" /> : <ShieldAlert className="w-3.5 h-3.5 text-amber-500" />}
              <span>{simulatedFailure ? '复位故障模拟' : '演练注入故障 (Chaos)'}</span>
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
                ⚠️ 首选提供商 (Priority 1) 发生 429 Rate Limit 限流 / 500 异常
              </div>
              <p className="text-[11px] leading-relaxed">
                Airoute 数据面拦截异常并在首字输出前（15ms 内）自动旁路故障节点，流量无感切换至 Priority 2 备用提供商。客户端请求保持 100% 连通与不中断！
              </p>
            </div>
          </div>
        )}

        {/* Interactive Topology Graph Flow */}
        <div className="space-y-6">
          {/* Layer 1: Client Inbound */}
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4 items-center">
            <div className="p-4 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 space-y-2">
              <span className="text-[10px] font-bold uppercase tracking-wider text-slate-400 block">
                1. 客户端多协议接入
              </span>
              <div className="font-bold text-xs text-slate-800 dark:text-slate-200 flex items-center space-x-1.5">
                <Zap className="w-3.5 h-3.5 text-indigo-500" />
                <span>OpenAI / Claude / Gemini SDK</span>
              </div>
              <div className="text-[11px] font-mono text-slate-500 dark:text-slate-400">
                /v1/chat/completions<br/>
                /v1/messages
              </div>
            </div>

            <div className="flex justify-center text-indigo-500 dark:text-indigo-400">
              <ArrowRight className="w-6 h-6 hidden md:block" />
              <div className="md:hidden font-mono text-xs">↓</div>
            </div>

            {/* Layer 2: Gateway Kernel & Cascading */}
            <div className="p-4 rounded-2xl bg-indigo-50/60 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800 space-y-2">
              <span className="text-[10px] font-bold uppercase tracking-wider text-indigo-600 dark:text-indigo-400 block">
                2. 网关内核与级联重写
              </span>
              <div className="font-bold text-xs text-indigo-950 dark:text-indigo-200 flex items-center space-x-1.5">
                <Cpu className="w-3.5 h-3.5 text-indigo-600" />
                <span>Zero-DB 路由决策引擎</span>
              </div>
              <div className="text-[11px] font-mono text-slate-600 dark:text-slate-300">
                通配剥离: org/* ➔ *<br/>
                决策延迟: &lt; 50μs
              </div>
            </div>

            <div className="flex justify-center text-indigo-500 dark:text-indigo-400">
              <ArrowRight className="w-6 h-6 hidden md:block" />
              <div className="md:hidden font-mono text-xs">↓</div>
            </div>
          </div>

          {/* Layer 3: Dynamic Priority Fallback Queue */}
          <div className="p-5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200 dark:border-slate-800 space-y-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Shield className="w-4 h-4 text-emerald-500" />
                <span className="font-bold text-xs text-slate-800 dark:text-slate-200">
                  3. 多渠道优先级分发队列 (Fallback & Circuit Breaker Matrix)
                </span>
              </div>
              <span className="text-[11px] text-slate-400 font-mono">
                当前排队渠道数: {activeChannels.length || 3}
              </span>
            </div>

            {/* Channel Cards */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
              {/* Primary Channel */}
              <div
                className={`p-4 rounded-2xl border transition relative ${
                  simulatedFailure
                    ? 'bg-rose-50/50 dark:bg-rose-950/30 border-rose-300 dark:border-rose-800 opacity-60'
                    : 'bg-emerald-50/60 dark:bg-emerald-950/40 border-emerald-300 dark:border-emerald-800 shadow-md ring-2 ring-emerald-500/20'
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <span className="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-indigo-100 dark:bg-indigo-900 text-indigo-700 dark:text-indigo-300">
                    优先级 1 (首选)
                  </span>
                  <span
                    className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold border ${
                      simulatedFailure
                        ? 'bg-rose-100 dark:bg-rose-900 text-rose-700 dark:text-rose-300 border-rose-300'
                        : 'bg-emerald-100 dark:bg-emerald-900 text-emerald-700 dark:text-emerald-300 border-emerald-300'
                    }`}
                  >
                    <span className={`w-1.5 h-1.5 rounded-full mr-1 ${simulatedFailure ? 'bg-rose-500' : 'bg-emerald-500 animate-pulse'}`}></span>
                    {simulatedFailure ? '熔断隔离 (OPEN)' : '正常命中 (ACTIVE)'}
                  </span>
                </div>
                <div className="font-bold text-xs text-slate-800 dark:text-slate-100 truncate">
                  {primaryChannel.name}
                </div>
                <div className="text-[11px] text-slate-500 dark:text-slate-400 font-mono mt-1">
                  类型: {primaryChannel.type} · 权重: 10
                </div>
                <div className="mt-2 text-[10px] text-slate-400">
                  {simulatedFailure ? '❌ 已旁路死节点避免雪崩' : '⚡ 承载 100% 活跃常规请求'}
                </div>
              </div>

              {/* Secondary Backup Channel */}
              <div
                className={`p-4 rounded-2xl border transition relative ${
                  simulatedFailure
                    ? 'bg-emerald-50/80 dark:bg-emerald-950/50 border-emerald-400 dark:border-emerald-700 shadow-lg ring-2 ring-emerald-500/30'
                    : 'bg-white dark:bg-slate-800/80 border-slate-200 dark:border-slate-700'
                }`}
              >
                <div className="flex items-center justify-between mb-2">
                  <span className="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300">
                    优先级 2 (备用)
                  </span>
                  <span
                    className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold border ${
                      simulatedFailure
                        ? 'bg-emerald-100 dark:bg-emerald-900 text-emerald-700 dark:text-emerald-300 border-emerald-300 animate-pulse'
                        : 'bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 border-slate-200'
                    }`}
                  >
                    <span className={`w-1.5 h-1.5 rounded-full mr-1 ${simulatedFailure ? 'bg-emerald-500' : 'bg-slate-400'}`}></span>
                    {simulatedFailure ? '接管接流 (ROUTED)' : '热备待命 (STANDBY)'}
                  </span>
                </div>
                <div className="font-bold text-xs text-slate-800 dark:text-slate-100 truncate">
                  {backupChannel.name}
                </div>
                <div className="text-[11px] text-slate-500 dark:text-slate-400 font-mono mt-1">
                  类型: {backupChannel.type} · 权重: 10
                </div>
                <div className="mt-2 text-[10px] text-slate-400">
                  {simulatedFailure ? '🚀 15ms 内无感漂移成功，保障调用成功' : '随时就绪，首字前无感容灾'}
                </div>
              </div>

              {/* Disaster Recovery Channel */}
              <div className="p-4 rounded-2xl border bg-white dark:bg-slate-800/80 border-slate-200 dark:border-slate-700">
                <div className="flex items-center justify-between mb-2">
                  <span className="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300">
                    优先级 3 (托底兜底)
                  </span>
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-slate-600">
                    兜底待命
                  </span>
                </div>
                <div className="font-bold text-xs text-slate-800 dark:text-slate-100 truncate">
                  {drChannel.name}
                </div>
                <div className="text-[11px] text-slate-500 dark:text-slate-400 font-mono mt-1">
                  类型: {drChannel.type} · 权重: 5
                </div>
                <div className="mt-2 text-[10px] text-slate-400">
                  多集群多供应商最后一道防线
                </div>
              </div>
            </div>
          </div>

          {/* SLA Protection Principles Footer */}
          <div className="p-4 bg-indigo-50/50 dark:bg-indigo-950/30 border border-indigo-200/80 dark:border-indigo-800/60 rounded-2xl text-xs space-y-2">
            <div className="font-bold text-indigo-950 dark:text-indigo-200 flex items-center space-x-2">
              <CheckCircle2 className="w-4 h-4 text-emerald-500" />
              <span>三态智能熔断与无感自愈核心算法 (Tri-State State Machine)</span>
            </div>
            <p className="text-slate-600 dark:text-slate-300 leading-relaxed text-[11px]">
              当某上游渠道连续发生 3 次网络超时或 5xx 错误时，该节点立即进入 <code>OPEN (熔断)</code> 状态并在内存中维持 30 秒冷却；冷却期结束后自动进入 <code>HALF-OPEN (半开)</code> 进行单次探活，验证健康后自动恢复 <code>CLOSED (闭合)</code>，实现全自动无人值守自愈。
            </p>
          </div>
        </div>

        {/* Action Button */}
        <div className="mt-6 flex justify-end">
          <button
            onClick={onClose}
            className="px-5 py-2.5 bg-slate-900 dark:bg-slate-100 hover:bg-slate-800 dark:hover:bg-white text-white dark:text-slate-900 rounded-xl text-xs font-bold transition shadow-xs cursor-pointer"
          >
            完成查看
          </button>
        </div>
      </div>
    </div>
  );
}
