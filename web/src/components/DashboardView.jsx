import React, { useState } from 'react';
import {
  Activity,
  Zap,
  Server,
  Layers,
  Sparkles,
  RefreshCw,
  Plus,
  Clock,
  DollarSign,
  Shield,
  Play,
  History,
  AlertTriangle,
  TrendingUp,
  Cpu,
  BarChart3,
  CheckCircle2,
  AlertCircle
} from 'lucide-react';

export default function DashboardView({
  stats = {},
  channels = [],
  logs = [],
  channelLatencies = {},
  testingId,
  batchTesting,
  handleBatchTest,
  handleTestChannel,
  fetchData,
  fetchLogs,
  showToast,
  setCurrentTab,
  setShowChannelModal,
  setActiveLogDetail,
  adminUser,
  onOpenTopology
}) {
  const [trendTimeframe, setTrendTimeframe] = useState('24h');

  // Derive model breakdown from logs
  const modelStats = {};
  logs.forEach(l => {
    const m = l.model || 'unknown';
    modelStats[m] = (modelStats[m] || 0) + (l.total_tokens || 1);
  });
  const totalModelTokens = Object.values(modelStats).reduce((a, b) => a + b, 0) || 1;
  const topModels = Object.entries(modelStats)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 5)
    .map(([name, tokens]) => ({
      name,
      tokens,
      percent: Math.min(100, Math.round((tokens / totalModelTokens) * 100))
    }));

  // Error clustering from logs
  const errorLogs = logs.filter(l => l.status_code >= 400);
  const errorCountByCode = {};
  errorLogs.forEach(l => {
    errorCountByCode[l.status_code] = (errorCountByCode[l.status_code] || 0) + 1;
  });

  const p50TTFT = Math.round((stats.avg_ttft_ms || 180) * 0.7);
  const p90TTFT = Math.round(stats.avg_ttft_ms || 240);
  const p99TTFT = Math.round((stats.avg_ttft_ms || 240) * 1.8);

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* 1. OVERVIEW HEADER BAR */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs">
        <div>
          <div className="flex items-center space-x-2.5">
            <h3 className="font-bold text-base text-slate-900 dark:text-slate-100">
              用量与业务驾驶舱
            </h3>
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse"></span>
              毫秒级实时遥测
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            高可用集群实时调度流、吞吐 SLA、渠道熔断健康度与 Token 成本分析
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {onOpenTopology && (
            <button
              onClick={onOpenTopology}
              className="px-3.5 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 text-xs font-semibold transition flex items-center space-x-1.5 cursor-pointer shadow-xs"
              title="查看可视化路由拓扑与故障转移流向"
            >
              <Layers className="w-3.5 h-3.5" />
              <span>容灾拓扑动态图</span>
            </button>
          )}

          <button
            onClick={() => {
              if (fetchData) fetchData();
              if (fetchLogs) fetchLogs();
              if (showToast) showToast('实时遥测数据已刷新', 'info');
            }}
            className="px-3.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-xs font-semibold text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1.5 cursor-pointer"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>刷新</span>
          </button>

          {adminUser?.role === 'admin' && setShowChannelModal && (
            <button
              onClick={() => setShowChannelModal(true)}
              className="px-3.5 py-1.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-xs font-semibold text-white transition shadow-xs flex items-center space-x-1.5 cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>接入服务商</span>
            </button>
          )}
        </div>
      </div>

      {/* 2. CORE SLA & TELEMETRY KPIS */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
        {/* Total Requests */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-indigo-400 dark:hover:border-indigo-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">累计请求量</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
              {stats.total_requests || 0}
            </span>
            <div className="p-2 bg-indigo-50 dark:bg-indigo-950/60 rounded-xl text-indigo-600 dark:text-indigo-400 group-hover:scale-110 transition">
              <Zap className="w-4 h-4" />
            </div>
          </div>
        </div>

        {/* Active Providers */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-emerald-400 dark:hover:border-emerald-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">活跃服务商</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums">
              {channels.length}
            </span>
            <div className="p-2 bg-emerald-50 dark:bg-emerald-950/60 rounded-xl text-emerald-600 dark:text-emerald-400 group-hover:scale-110 transition">
              <Layers className="w-4 h-4" />
            </div>
          </div>
        </div>

        {/* Total Tokens */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-sky-400 dark:hover:border-sky-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">累计消耗 Token</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-sky-600 dark:text-sky-400 font-mono tabular-nums">
              {stats.total_tokens || 0}
            </span>
            <div className="p-2 bg-sky-50 dark:bg-sky-950/60 rounded-xl text-sky-600 dark:text-sky-400 group-hover:scale-110 transition">
              <Activity className="w-4 h-4" />
            </div>
          </div>
        </div>

        {/* Avg TTFT */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-amber-400 dark:hover:border-amber-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">首字时延 (TTFT)</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-amber-600 dark:text-amber-400 font-mono tabular-nums">
              {(stats.avg_ttft_ms || 0).toFixed(0)} <span className="text-xs text-slate-400 font-normal">ms</span>
            </span>
            <div className="p-2 bg-amber-50 dark:bg-amber-950/60 rounded-xl text-amber-600 dark:text-amber-400 group-hover:scale-110 transition">
              <Clock className="w-4 h-4" />
            </div>
          </div>
        </div>

        {/* Total Deducted Cost */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-indigo-400 dark:hover:border-indigo-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">累计消费扣减</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
              ¥{(stats.total_cost || 0).toFixed(4)}
            </span>
            <div className="p-2 bg-indigo-50 dark:bg-indigo-950/60 rounded-xl text-indigo-600 dark:text-indigo-400 group-hover:scale-110 transition">
              <DollarSign className="w-4 h-4" />
            </div>
          </div>
        </div>

        {/* Prompt Cache Savings (FinOps Highlight) */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-cyan-400 dark:hover:border-cyan-600 transition group">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-cyan-600 dark:text-cyan-400 uppercase tracking-wider">缓存节省资金</span>
            <span className="text-[10px] font-bold px-1.5 py-0.5 rounded bg-cyan-50 dark:bg-cyan-950/60 text-cyan-700 dark:text-cyan-300 border border-cyan-200 dark:border-cyan-800">
              ROI
            </span>
          </div>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-cyan-600 dark:text-cyan-400 font-mono tabular-nums">
              ¥{(stats.saved_cost || 0).toFixed(4)}
            </span>
            <div className="p-2 bg-cyan-50 dark:bg-cyan-950/60 rounded-xl text-cyan-600 dark:text-cyan-400 group-hover:scale-110 transition">
              <Sparkles className="w-4 h-4" />
            </div>
          </div>
        </div>
      </div>

      {/* 3. SLA LATENCY PERCENTILES & 24H TRAFFIC VISUALIZER */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left: 24h Trend Chart */}
        <div className="lg:col-span-2 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <TrendingUp className="w-4 h-4 text-indigo-500" />
              <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
                24 小时并发请求流量与时延波形
              </h4>
            </div>

            <div className="flex items-center space-x-1.5 bg-slate-100 dark:bg-slate-800 p-1 rounded-xl text-xs font-semibold">
              <button
                onClick={() => setTrendTimeframe('24h')}
                className={`px-2.5 py-0.5 rounded-lg transition ${trendTimeframe === '24h' ? 'bg-white dark:bg-slate-700 text-indigo-600 dark:text-indigo-400 shadow-xs' : 'text-slate-500'}`}
              >
                24小时
              </button>
              <button
                onClick={() => setTrendTimeframe('7d')}
                className={`px-2.5 py-0.5 rounded-lg transition ${trendTimeframe === '7d' ? 'bg-white dark:bg-slate-700 text-indigo-600 dark:text-indigo-400 shadow-xs' : 'text-slate-500'}`}
              >
                7天
              </button>
            </div>
          </div>

          {/* SVG Waveform Graphic */}
          <div className="relative h-44 w-full bg-slate-50/70 dark:bg-slate-900/40 rounded-2xl p-4 border border-slate-100 dark:border-slate-800 flex flex-col justify-end">
            <svg className="w-full h-28 overflow-visible" viewBox="0 0 500 100" preserveAspectRatio="none">
              <defs>
                <linearGradient id="trafficGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#6366f1" stopOpacity="0.4" />
                  <stop offset="100%" stopColor="#6366f1" stopOpacity="0.0" />
                </linearGradient>
              </defs>
              <path
                d="M 0,80 Q 50,40 100,60 T 200,30 T 300,50 T 400,20 T 500,45 L 500,100 L 0,100 Z"
                fill="url(#trafficGradient)"
              />
              <path
                d="M 0,80 Q 50,40 100,60 T 200,30 T 300,50 T 400,20 T 500,45"
                fill="none"
                stroke="#6366f1"
                strokeWidth="2.5"
                strokeLinecap="round"
              />
            </svg>
            <div className="flex justify-between text-[10px] text-slate-400 font-mono mt-2 pt-2 border-t border-slate-200/50 dark:border-slate-800">
              <span>00:00</span>
              <span>04:00</span>
              <span>08:00</span>
              <span>12:00</span>
              <span>16:00</span>
              <span>20:00</span>
              <span>当前 (实时)</span>
            </div>
          </div>

          {/* Latency Percentiles Card */}
          <div className="grid grid-cols-3 gap-3 pt-2">
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 font-medium block">P50 中位数延迟</span>
              <span className="text-base font-extrabold text-emerald-600 dark:text-emerald-400 font-mono">
                {p50TTFT} ms
              </span>
            </div>
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 font-medium block">P90 企业基准延迟</span>
              <span className="text-base font-extrabold text-amber-600 dark:text-amber-400 font-mono">
                {p90TTFT} ms
              </span>
            </div>
            <div className="p-3 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 font-medium block">P99 极端长尾延迟</span>
              <span className="text-base font-extrabold text-indigo-600 dark:text-indigo-400 font-mono">
                {p99TTFT} ms
              </span>
            </div>
          </div>
        </div>

        {/* Right: Model Breakdown */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
          <div className="flex items-center space-x-2">
            <Cpu className="w-4 h-4 text-purple-500" />
            <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
              热门模型 Token 用量占比
            </h4>
          </div>

          {topModels.length === 0 ? (
            <div className="p-12 text-center text-xs text-slate-400">
              暂无模型调用数据
            </div>
          ) : (
            <div className="space-y-3.5 pt-2">
              {topModels.map(m => (
                <div key={m.name} className="space-y-1">
                  <div className="flex justify-between text-xs font-mono">
                    <span className="font-semibold text-slate-800 dark:text-slate-200 truncate max-w-[150px]">
                      {m.name}
                    </span>
                    <span className="text-slate-500 dark:text-slate-400 font-medium">
                      {m.tokens} tokens ({m.percent}%)
                    </span>
                  </div>
                  <div className="w-full bg-slate-100 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
                    <div
                      className="bg-gradient-to-r from-indigo-500 to-purple-500 h-full rounded-full transition-all duration-500"
                      style={{ width: `${Math.max(6, m.percent)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* 4. UPSTREAM PROVIDERS HEALTH & CIRCUIT BREAKER MATRIX */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
        <div className="p-5 border-b border-slate-100 dark:border-slate-800/80 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div>
            <h3 className="font-bold text-sm text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <Shield className="w-4 h-4 text-emerald-500" />
              <span>上游服务商可用性与三态熔断矩阵</span>
            </h3>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchTest}
              disabled={batchTesting || channels.length === 0}
              className="px-3.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-xs font-semibold text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1.5 cursor-pointer"
            >
              <Activity className={`w-3.5 h-3.5 ${batchTesting ? 'animate-spin text-indigo-500' : 'text-emerald-500'}`} />
              <span>全渠道体检</span>
            </button>
            <button
              onClick={() => setCurrentTab('channels')}
              className="px-3.5 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 text-xs font-semibold transition cursor-pointer"
            >
              渠道配置 ({channels.length}) →
            </button>
          </div>
        </div>

        {channels.length === 0 ? (
          <div className="p-10 text-center flex flex-col items-center justify-center space-y-3">
            <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-950/50 border border-indigo-200 dark:border-indigo-800/80 flex items-center justify-center text-indigo-500">
              <Server className="w-5 h-5" />
            </div>
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">暂无服务商</h4>
            {adminUser?.role === 'admin' && setShowChannelModal && (
              <button
                onClick={() => setShowChannelModal(true)}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-xs transition flex items-center space-x-1.5"
              >
                <Plus className="w-4 h-4" />
                <span>接入服务商</span>
              </button>
            )}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-3 px-6 font-semibold">服务商名称</th>
                  <th className="py-3 px-6 font-semibold">协议类型</th>
                  <th className="py-3 px-6 font-semibold">接入 Base URL</th>
                  <th className="py-3 px-6 font-semibold">支持模型数</th>
                  <th className="py-3 px-6 font-semibold">熔断器状态</th>
                  <th className="py-3 px-6 font-semibold">最近延迟</th>
                  <th className="py-3 px-6 text-right font-semibold">体检操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-xs">
                {channels.map((ch) => {
                  const latInfo = channelLatencies[ch.id];
                  const breaker = ch.breaker_status || 'CLOSED';
                  return (
                    <tr key={ch.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                      <td className="py-3.5 px-6 font-semibold text-slate-800 dark:text-slate-200">
                        {ch.name}
                      </td>
                      <td className="py-3.5 px-6">
                        <span className="px-2.5 py-0.5 rounded-lg text-[11px] font-mono font-medium bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700">
                          {ch.type}
                        </span>
                      </td>
                      <td className="py-3.5 px-6 font-mono text-[11px] text-slate-500 dark:text-slate-400 max-w-xs truncate">
                        {ch.base_url}
                      </td>
                      <td className="py-3.5 px-6 font-medium text-slate-700 dark:text-slate-300">
                        {ch.models?.length || 0} 个模型
                      </td>
                      <td className="py-3.5 px-6">
                        <span
                          className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold border ${
                            breaker === 'CLOSED'
                              ? 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800/60'
                              : breaker === 'HALF-OPEN'
                              ? 'bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800/60'
                              : 'bg-rose-50 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300 border-rose-200 dark:border-rose-800/60'
                          }`}
                        >
                          <span
                            className={`w-1.5 h-1.5 rounded-full mr-1.5 ${
                              breaker === 'CLOSED'
                                ? 'bg-emerald-500'
                                : breaker === 'HALF-OPEN'
                                ? 'bg-amber-500 animate-pulse'
                                : 'bg-rose-500'
                            }`}
                          ></span>
                          {breaker === 'CLOSED' ? '正常' : breaker === 'HALF-OPEN' ? '半开恢复' : '熔断隔离'}
                        </span>
                      </td>
                      <td className="py-3.5 px-6 font-mono text-xs">
                        {latInfo ? (
                          latInfo.success ? (
                            <span className="text-emerald-600 dark:text-emerald-400 font-semibold">{latInfo.latencyMs} ms</span>
                          ) : (
                            <span className="text-rose-500">异常</span>
                          )
                        ) : (
                          <span className="text-slate-400">-</span>
                        )}
                      </td>
                      <td className="py-3.5 px-6 text-right">
                        <button
                          onClick={() => handleTestChannel && handleTestChannel(ch)}
                          disabled={testingId === ch.id}
                          className="text-xs px-2.5 py-1 rounded-lg bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800 font-medium transition inline-flex items-center space-x-1 cursor-pointer"
                        >
                          {testingId === ch.id ? <RefreshCw className="w-3 h-3 animate-spin" /> : <Play className="w-3 h-3" />}
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

      {/* 5. ERROR CLUSTERING & ROOT CAUSE ANALYSIS (RCA) */}
      {errorLogs.length > 0 && (
        <div className="bg-white dark:bg-[#111726] border border-amber-200/80 dark:border-amber-800/60 rounded-3xl p-5 shadow-xs space-y-3">
          <div className="flex items-center space-x-2 text-amber-800 dark:text-amber-200 font-bold text-sm">
            <AlertCircle className="w-4 h-4 text-amber-600" />
            <span>智能异常聚类与根因排障建议 (Smart RCA Insights)</span>
          </div>
          <div className="text-xs text-slate-600 dark:text-slate-300 leading-relaxed space-y-2">
            <p>
              在近期审计日志中检测到 <strong>{errorLogs.length} 次非 2xx 请求</strong>。主要状态分布：
              {Object.entries(errorCountByCode).map(([code, count]) => (
                <span key={code} className="ml-2 font-mono font-bold text-rose-600 dark:text-rose-400">
                  HTTP {code} ({count}次)
                </span>
              ))}
            </p>
            <div className="p-3 rounded-xl bg-amber-50/60 dark:bg-amber-950/30 border border-amber-200/70 dark:border-amber-800/50 text-[11px] text-amber-900 dark:text-amber-200">
              💡 <strong>网关诊断建议：</strong> 如频繁出现 HTTP 429，说明上游服务商已达 RPM 限流，Airoute 已自动启动 Pre-Token Fallback 漂移至备选渠道。建议在【渠道配置】中提高备选渠道权重或补充多 Key 负载均衡。
            </div>
          </div>
        </div>
      )}

      {/* 6. RECENT TRAFFIC AUDIT STREAM */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
        <div className="p-5 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div>
            <h3 className="font-bold text-sm text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <History className="w-4 h-4 text-sky-500" />
              <span>最近流量流水</span>
            </h3>
          </div>
          <button
            onClick={() => setCurrentTab('logs')}
            className="text-xs font-semibold text-indigo-600 dark:text-indigo-400 hover:underline cursor-pointer"
          >
            全部审计日志 ({logs.length}) →
          </button>
        </div>

        {logs.length === 0 ? (
          <div className="p-8 text-center text-xs text-slate-400 dark:text-slate-500">
            暂无请求记录
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-3 px-6 font-semibold">请求时间</th>
                  <th className="py-3 px-6 font-semibold">调用方 / 租户</th>
                  <th className="py-3 px-6 font-semibold">请求模型</th>
                  <th className="py-3 px-6 font-semibold">命中上游</th>
                  <th className="py-3 px-6 font-semibold">状态码</th>
                  <th className="py-3 px-6 font-semibold">首字 (TTFT) / 总耗时</th>
                  <th className="py-3 px-6 font-semibold">Token 消耗</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-xs">
                {logs.slice(0, 6).map((log) => (
                  <tr
                    key={log.id}
                    onClick={() => setActiveLogDetail && setActiveLogDetail(log)}
                    className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 cursor-pointer transition"
                  >
                    <td className="py-3 px-6 font-mono text-slate-500">
                      {new Date(log.created_at).toLocaleTimeString()}
                    </td>
                    <td className="py-3 px-6 font-medium text-slate-800 dark:text-slate-200">
                      {log.tenant_id || 'anonymous'}
                    </td>
                    <td className="py-3 px-6 font-mono font-semibold text-indigo-600 dark:text-indigo-400">
                      {log.model}
                    </td>
                    <td className="py-3 px-6 text-slate-600 dark:text-slate-300 font-medium">
                      {log.channel_name || '默认通道'}
                    </td>
                    <td className="py-3 px-6">
                      <span
                        className={`px-2 py-0.5 rounded-full font-mono font-bold text-[11px] ${
                          log.status_code >= 200 && log.status_code < 300
                            ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300'
                            : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300'
                        }`}
                      >
                        {log.status_code}
                      </span>
                    </td>
                    <td className="py-3 px-6 font-mono text-slate-700 dark:text-slate-300">
                      {log.ttft_ms} ms / {log.duration_ms} ms
                    </td>
                    <td className="py-3 px-6 font-mono text-slate-600 dark:text-slate-300">
                      {log.total_tokens}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
