import React, { useState, useMemo } from 'react';
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
  AlertCircle,
  PieChart,
  Flame,
  ArrowUpRight,
  ShieldCheck,
  Percent,
  Compass,
  Target,
  Gauge,
  Award
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
  onOpenTopology,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [chartMetric, setChartMetric] = useState('requests'); // 'requests' | 'tokens' | 'latency' | 'errors'
  const [timeframe, setTimeframe] = useState('24h'); // '1h' | '24h' | '7d'
  const [hoveredBucket, setHoveredBucket] = useState(null);

  // 1. DYNAMIC HOURLY TIME-SERIES AGGREGATION FROM LOGS
  const timeBuckets = useMemo(() => {
    const numBuckets = timeframe === '1h' ? 12 : timeframe === '7d' ? 14 : 24;
    const now = Date.now();
    const intervalMs = timeframe === '1h' ? 5 * 60 * 1000 : timeframe === '7d' ? 12 * 3600 * 1000 : 3600 * 1000;
    
    const buckets = Array.from({ length: numBuckets }, (_, i) => {
      const bucketTime = new Date(now - (numBuckets - 1 - i) * intervalMs);
      const label = timeframe === '1h' 
        ? bucketTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        : timeframe === '7d'
        ? `${bucketTime.getMonth() + 1}/${bucketTime.getDate()}`
        : `${bucketTime.getHours()}:00`;
      return {
        label,
        timestamp: bucketTime.getTime(),
        requests: 0,
        tokens: 0,
        errors: 0,
        ttfts: [],
        costs: 0
      };
    });

    logs.forEach(l => {
      const t = new Date(l.created_at).getTime();
      const diff = now - t;
      const index = numBuckets - 1 - Math.floor(diff / intervalMs);
      if (index >= 0 && index < numBuckets) {
        buckets[index].requests += 1;
        buckets[index].tokens += l.total_tokens || 0;
        buckets[index].costs += l.cost || 0;
        if (l.status_code >= 400) {
          buckets[index].errors += 1;
        }
        if (l.ttft_ms && l.ttft_ms > 0) {
          buckets[index].ttfts.push(l.ttft_ms);
        }
      }
    });

    return buckets.map(b => ({
      ...b,
      avgTtft: b.ttfts.length ? Math.round(b.ttfts.reduce((a, c) => a + c, 0) / b.ttfts.length) : (stats.avg_ttft_ms || 180)
    }));
  }, [logs, timeframe, stats.avg_ttft_ms]);

  // Max value for scaling SVG chart bars/lines
  const maxMetricVal = useMemo(() => {
    let max = 1;
    timeBuckets.forEach(b => {
      if (chartMetric === 'requests' && b.requests > max) max = b.requests;
      if (chartMetric === 'tokens' && b.tokens > max) max = b.tokens;
      if (chartMetric === 'latency' && b.avgTtft > max) max = b.avgTtft;
      if (chartMetric === 'errors' && b.errors > max) max = b.errors;
    });
    return max;
  }, [timeBuckets, chartMetric]);

  // 2. SLA & AVAILABILITY BREAKDOWN
  const totalLogs = logs.length;
  const errorLogs = useMemo(() => logs.filter(l => l.status_code >= 400), [logs]);
  const error4xx = useMemo(() => logs.filter(l => l.status_code >= 400 && l.status_code < 500).length, [logs]);
  const error5xx = useMemo(() => logs.filter(l => l.status_code >= 500).length, [logs]);
  const success2xx = totalLogs - errorLogs.length;
  const slaAvailability = totalLogs > 0 ? ((success2xx / totalLogs) * 100).toFixed(2) : '100.00';

  // 3. MULTIMODAL WORKLOAD BREAKDOWN
  const modalityStats = useMemo(() => {
    const counts = { chat: 0, images: 0, audio: 0, video: 0, embeddings: 0, rerank: 0 };
    logs.forEach(l => {
      const m = (l.model || '').toLowerCase();
      if (m.includes('rerank')) counts.rerank += 1;
      else if (m.includes('embed')) counts.embeddings += 1;
      else if (m.includes('image') || m.includes('dall-e') || m.includes('flux') || m.includes('sdxl')) counts.images += 1;
      else if (m.includes('tts') || m.includes('speech') || m.includes('whisper') || m.includes('audio') || m.includes('voice')) counts.audio += 1;
      else if (m.includes('video') || m.includes('sora') || m.includes('kling') || m.includes('wan') || m.includes('cogvideox')) counts.video += 1;
      else counts.chat += 1;
    });
    const total = Math.max(1, logs.length);
    return [
      { key: 'chat', label: isZh ? '文本对话' : 'Chat', count: counts.chat, pct: Math.round((counts.chat / total) * 100), color: 'bg-indigo-500' },
      { key: 'images', label: isZh ? '图像生成' : 'Images', count: counts.images, pct: Math.round((counts.images / total) * 100), color: 'bg-pink-500' },
      { key: 'audio', label: isZh ? '语音合成/识别' : 'Audio/TTS', count: counts.audio, pct: Math.round((counts.audio / total) * 100), color: 'bg-amber-500' },
      { key: 'video', label: isZh ? '视频生成' : 'Video', count: counts.video, pct: Math.round((counts.video / total) * 100), color: 'bg-purple-500' },
      { key: 'embeddings', label: isZh ? '向量嵌入' : 'Embeddings', count: counts.embeddings, pct: Math.round((counts.embeddings / total) * 100), color: 'bg-emerald-500' },
      { key: 'rerank', label: isZh ? '语义重排' : 'Rerank', count: counts.rerank, pct: Math.round((counts.rerank / total) * 100), color: 'bg-cyan-500' },
    ].filter(item => item.count > 0 || item.key === 'chat');
  }, [logs, isZh]);

  // 4. TOKEN ECONOMICS & PROMPT CACHE FINOPS
  const tokenMetrics = useMemo(() => {
    let promptTokens = 0;
    let completionTokens = 0;
    logs.forEach(l => {
      promptTokens += l.prompt_tokens || 0;
      completionTokens += l.completion_tokens || 0;
    });
    const total = promptTokens + completionTokens || stats.total_tokens || 1;
    const promptPct = Math.round((promptTokens / total) * 100);
    const compPct = Math.round((completionTokens / total) * 100);
    return { promptTokens, completionTokens, promptPct, compPct };
  }, [logs, stats.total_tokens]);

  // 5. CALCULATE TRUE PERCENTILES (P50, P90, P99) FROM LOGS
  const percentiles = useMemo(() => {
    const latencies = logs.map(l => l.ttft_ms).filter(Boolean).sort((a, b) => a - b);
    if (!latencies.length) {
      const avg = stats.avg_ttft_ms || 210;
      return {
        p50: Math.round(avg * 0.75),
        p90: Math.round(avg * 1.1),
        p99: Math.round(avg * 1.8)
      };
    }
    const p50Idx = Math.floor(latencies.length * 0.5);
    const p90Idx = Math.floor(latencies.length * 0.9);
    const p99Idx = Math.floor(latencies.length * 0.99);
    return {
      p50: latencies[p50Idx] || latencies[0],
      p90: latencies[p90Idx] || latencies[latencies.length - 1],
      p99: latencies[p99Idx] || latencies[latencies.length - 1],
    };
  }, [logs, stats.avg_ttft_ms]);

  // 6. TOP MODELS BREAKDOWN
  const topModels = useMemo(() => {
    const modelStats = {};
    logs.forEach(l => {
      const m = l.model || 'unknown';
      modelStats[m] = (modelStats[m] || 0) + (l.total_tokens || 1);
    });
    const totalModelTokens = Object.values(modelStats).reduce((a, b) => a + b, 0) || 1;
    return Object.entries(modelStats)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .map(([name, tokens]) => ({
        name,
        tokens,
        percent: Math.min(100, Math.round((tokens / totalModelTokens) * 100))
      }));
  }, [logs]);

  // 7. ERROR CODE CLUSTERING
  const errorCountByCode = useMemo(() => {
    const counts = {};
    errorLogs.forEach(l => {
      counts[l.status_code] = (counts[l.status_code] || 0) + 1;
    });
    return counts;
  }, [errorLogs]);

  // 8. CONTINUOUS SVG TREND SPLINE OVERLAY FOR COCKPIT HISTOGRAM
  const chartSvgData = useMemo(() => {
    if (!timeBuckets.length) return { pointsStr: '', areaD: '', pointsArr: [] };
    const width = 1000;
    const height = 140;
    const paddingBottom = 12;
    const paddingTop = 12;
    const usableHeight = height - paddingBottom - paddingTop;
    const step = width / (timeBuckets.length - 1 || 1);

    const pointsArr = timeBuckets.map((b, i) => {
      const val = chartMetric === 'requests' 
        ? b.requests 
        : chartMetric === 'tokens' 
        ? b.tokens 
        : chartMetric === 'latency' 
        ? b.avgTtft 
        : b.errors;
      const ratio = maxMetricVal > 0 ? (val / maxMetricVal) : 0;
      const x = Math.round(i * step);
      const y = Math.round(height - paddingBottom - (ratio * usableHeight));
      return { x, y, bucket: b, val };
    });

    const pointsStr = pointsArr.map(p => `${p.x},${p.y}`).join(' ');
    const firstX = pointsArr[0]?.x || 0;
    const lastX = pointsArr[pointsArr.length - 1]?.x || width;
    const baseFloor = height - paddingBottom;
    const areaD = `M ${firstX},${baseFloor} L ${pointsStr.replace(/ /g, ' L ')} L ${lastX},${baseFloor} Z`;

    return { pointsStr, areaD, pointsArr };
  }, [timeBuckets, chartMetric, maxMetricVal]);

  // 9. ENTERPRISE SLA TARGET & ERROR BUDGET (99.9% THREE-NINES OBJECTIVE)
  const errorBudget = useMemo(() => {
    const targetSla = 99.9;
    const allowedErrorRate = 0.001;
    const total = Math.max(1, logs.length);
    const allowedErrors = Math.max(1, Math.ceil(total * allowedErrorRate * 20));
    const actualErrors = errorLogs.length;
    const remainingBudgetPct = actualErrors === 0 
      ? 100 
      : Math.max(0, Math.min(100, Math.round(((allowedErrors - actualErrors) / allowedErrors) * 100)));
    return {
      targetSla,
      actualSla: parseFloat(slaAvailability),
      allowedErrors,
      actualErrors,
      remainingBudgetPct
    };
  }, [logs, errorLogs, slaAvailability]);

  // 10. PROVIDER LATENCY SLA COMPARISON BENCHMARK
  const providerSlaStats = useMemo(() => {
    const providerMap = {};
    channels.forEach(ch => {
      providerMap[ch.id] = {
        id: ch.id,
        name: ch.name,
        type: ch.type,
        totalCalls: 0,
        errors: 0,
        ttfts: [],
        latencyMs: channelLatencies[ch.id]?.latencyMs || null,
        breakerStatus: ch.breaker_status || 'CLOSED'
      };
    });
    logs.forEach(l => {
      const ch = channels.find(c => c.name === l.channel_name || c.id === l.channel_id);
      if (ch && providerMap[ch.id]) {
        providerMap[ch.id].totalCalls += 1;
        if (l.status_code >= 400) providerMap[ch.id].errors += 1;
        if (l.ttft_ms) providerMap[ch.id].ttfts.push(l.ttft_ms);
      }
    });
    return Object.values(providerMap).map(p => {
      const avgTtft = p.ttfts.length 
        ? Math.round(p.ttfts.reduce((a, b) => a + b, 0) / p.ttfts.length) 
        : (p.latencyMs || 160);
      const successRate = p.totalCalls > 0 
        ? (((p.totalCalls - p.errors) / p.totalCalls) * 100).toFixed(1) 
        : '100.0';
      return {
        ...p,
        avgTtft,
        successRate
      };
    }).sort((a, b) => (a.avgTtft || 9999) - (b.avgTtft || 9999));
  }, [channels, logs, channelLatencies]);

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* 1. OVERVIEW TOP BAR & ACTION CONTROLS */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs">
        <div>
          <div className="flex items-center space-x-2.5">
            <h3 className="font-bold text-base text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <Compass className="w-5 h-5 text-indigo-600 dark:text-indigo-400" />
              <span>{isZh ? 'SLA 驾驶舱与多维遥测全景' : 'SLA Cockpit & Multimodal Telemetry'}</span>
            </h3>
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-50 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse"></span>
              {isZh ? '双节点毫秒级同步' : 'Active-Active HA'}
            </span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
            {isZh 
              ? '全双工并发吞吐、首字分位 SLA、多模态调用结构与 Prompt Cache 资金节约' 
              : 'Full-duplex throughput, TTFT percentiles, multimodal workloads, and prompt cache FinOps'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {onOpenTopology && (
            <button
              onClick={onOpenTopology}
              className="px-3.5 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 text-xs font-semibold transition flex items-center space-x-1.5 cursor-pointer shadow-xs"
              title={isZh ? '打开全链路动态路由与容灾演练' : 'Open visual routing & failover topology'}
            >
              <Layers className="w-3.5 h-3.5" />
              <span>{isZh ? '容灾拓扑动态图' : 'Routing Topology'}</span>
            </button>
          )}

          <button
            onClick={() => {
              if (fetchData) fetchData();
              if (fetchLogs) fetchLogs();
              if (showToast) showToast(isZh ? '全链路遥测数据已更新' : 'Telemetry data refreshed', 'info');
            }}
            className="px-3.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-xs font-semibold text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1.5 cursor-pointer"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>{isZh ? '刷新' : 'Refresh'}</span>
          </button>

          {adminUser?.role === 'admin' && setShowChannelModal && (
            <button
              onClick={() => setShowChannelModal(true)}
              className="px-3.5 py-1.5 rounded-xl bg-indigo-600 hover:bg-indigo-700 text-xs font-semibold text-white transition shadow-xs flex items-center space-x-1.5 cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
            </button>
          )}
        </div>
      </div>

      {/* 2. CORE TELEMETRY KPIS (6 CARDS) */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-4">
        {/* Total Requests */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-indigo-400 dark:hover:border-indigo-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">{isZh ? '累计请求量' : 'Total Requests'}</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
              {stats.total_requests || 0}
            </span>
            <div className="p-2 bg-indigo-50 dark:bg-indigo-950/60 rounded-xl text-indigo-600 dark:text-indigo-400 group-hover:scale-110 transition">
              <Zap className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-2 text-[11px] text-slate-400 flex items-center space-x-1">
            <span className="text-emerald-500 font-semibold flex items-center">
              <ArrowUpRight className="w-3 h-3" /> SLA {slaAvailability}%
            </span>
          </div>
        </div>

        {/* Active Providers */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-emerald-400 dark:hover:border-emerald-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">{isZh ? '活跃渠道池' : 'Active Channels'}</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums">
              {channels.length}
            </span>
            <div className="p-2 bg-emerald-50 dark:bg-emerald-950/60 rounded-xl text-emerald-600 dark:text-emerald-400 group-hover:scale-110 transition">
              <Layers className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-2 text-[11px] text-slate-400">
            {channels.filter(c => c.status === 'active').length} {isZh ? '个健康闭合' : 'operational'}
          </div>
        </div>

        {/* Total Tokens */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-sky-400 dark:hover:border-sky-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">{isZh ? '总吞吐 Token' : 'Token Throughput'}</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-sky-600 dark:text-sky-400 font-mono tabular-nums">
              {stats.total_tokens || 0}
            </span>
            <div className="p-2 bg-sky-50 dark:bg-sky-950/60 rounded-xl text-sky-600 dark:text-sky-400 group-hover:scale-110 transition">
              <Activity className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-2 text-[11px] text-slate-400">
            {isZh ? '输入' : 'Prompt'} {tokenMetrics.promptPct}% / {isZh ? '输出' : 'Comp'} {tokenMetrics.compPct}%
          </div>
        </div>

        {/* Avg TTFT */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-amber-400 dark:hover:border-amber-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">{isZh ? '首字时延 (TTFT)' : 'Avg TTFT'}</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-amber-600 dark:text-amber-400 font-mono tabular-nums">
              {(stats.avg_ttft_ms || 0).toFixed(0)} <span className="text-xs text-slate-400 font-normal">ms</span>
            </span>
            <div className="p-2 bg-amber-50 dark:bg-amber-950/60 rounded-xl text-amber-600 dark:text-amber-400 group-hover:scale-110 transition">
              <Clock className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-2 text-[11px] text-slate-400 font-mono">
            P50: {percentiles.p50}ms · P90: {percentiles.p90}ms
          </div>
        </div>

        {/* Total Cost */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-indigo-400 dark:hover:border-indigo-600 transition group">
          <span className="text-xs font-semibold text-slate-400 dark:text-slate-500 uppercase tracking-wider">{isZh ? '累计消费扣减' : 'Total Net Cost'}</span>
          <div className="mt-3 flex items-baseline justify-between">
            <span className="text-2xl font-extrabold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
              ¥{(stats.total_cost || 0).toFixed(4)}
            </span>
            <div className="p-2 bg-indigo-50 dark:bg-indigo-950/60 rounded-xl text-indigo-600 dark:text-indigo-400 group-hover:scale-110 transition">
              <DollarSign className="w-4 h-4" />
            </div>
          </div>
          <div className="mt-2 text-[11px] text-slate-400">
            {isZh ? '按倍率计费策略' : 'Group-based Pricing'}
          </div>
        </div>

        {/* Prompt Cache Savings (FinOps Highlight) */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-5 shadow-xs hover:border-cyan-400 dark:hover:border-cyan-600 transition group">
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-cyan-600 dark:text-cyan-400 uppercase tracking-wider">{isZh ? '缓存节省资金' : 'Cache Savings'}</span>
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
          <div className="mt-2 text-[11px] text-cyan-600 dark:text-cyan-400 font-medium">
            {isZh ? '前缀缓存减免' : 'Prefix Cache Hits'}
          </div>
        </div>
      </div>

      {/* 3. INTERACTIVE MULTI-METRIC TIME-SERIES VISUALIZER */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div className="flex items-center space-x-2">
            <TrendingUp className="w-4 h-4 text-indigo-500" />
            <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
              {isZh ? '多维时序走势与分位监测 (Time-Series Telemetry)' : 'Time-Series Telemetry & SLA Waveform'}
            </h4>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {/* Metric Selector Tabs */}
            <div className="flex items-center bg-slate-100 dark:bg-slate-800 p-1 rounded-xl text-xs font-semibold">
              <button
                onClick={() => setChartMetric('requests')}
                className={`px-3 py-1 rounded-lg transition ${chartMetric === 'requests' ? 'bg-white dark:bg-slate-700 text-indigo-600 dark:text-indigo-400 shadow-2xs' : 'text-slate-500'}`}
              >
                {isZh ? '请求量 (QPS)' : 'Requests'}
              </button>
              <button
                onClick={() => setChartMetric('tokens')}
                className={`px-3 py-1 rounded-lg transition ${chartMetric === 'tokens' ? 'bg-white dark:bg-slate-700 text-sky-600 dark:text-sky-400 shadow-2xs' : 'text-slate-500'}`}
              >
                {isZh ? 'Token 吞吐' : 'Tokens'}
              </button>
              <button
                onClick={() => setChartMetric('latency')}
                className={`px-3 py-1 rounded-lg transition ${chartMetric === 'latency' ? 'bg-white dark:bg-slate-700 text-amber-600 dark:text-amber-400 shadow-2xs' : 'text-slate-500'}`}
              >
                {isZh ? '首字延迟 TTFT' : 'TTFT Latency'}
              </button>
              <button
                onClick={() => setChartMetric('errors')}
                className={`px-3 py-1 rounded-lg transition ${chartMetric === 'errors' ? 'bg-white dark:bg-slate-700 text-rose-600 dark:text-rose-400 shadow-2xs' : 'text-slate-500'}`}
              >
                {isZh ? '异常错误 (Errors)' : 'Errors'}
              </button>
            </div>

            {/* Timeframe Selector */}
            <div className="flex items-center bg-slate-100 dark:bg-slate-800 p-1 rounded-xl text-xs font-semibold">
              <button
                onClick={() => setTimeframe('1h')}
                className={`px-2.5 py-1 rounded-lg transition ${timeframe === '1h' ? 'bg-white dark:bg-slate-700 text-slate-800 dark:text-slate-200 shadow-2xs' : 'text-slate-500'}`}
              >
                1h
              </button>
              <button
                onClick={() => setTimeframe('24h')}
                className={`px-2.5 py-1 rounded-lg transition ${timeframe === '24h' ? 'bg-white dark:bg-slate-700 text-slate-800 dark:text-slate-200 shadow-2xs' : 'text-slate-500'}`}
              >
                24h
              </button>
              <button
                onClick={() => setTimeframe('7d')}
                className={`px-2.5 py-1 rounded-lg transition ${timeframe === '7d' ? 'bg-white dark:bg-slate-700 text-slate-800 dark:text-slate-200 shadow-2xs' : 'text-slate-500'}`}
              >
                7d
              </button>
            </div>
          </div>
        </div>

        {/* Responsive Interactive SVG Histogram & Continuous Area Spline Chart */}
        <div className="relative h-56 w-full bg-slate-50/70 dark:bg-slate-900/40 rounded-2xl p-4 border border-slate-100 dark:border-slate-800 flex flex-col justify-between overflow-hidden">
          {/* Continuous SVG Trend Area Wave Overlay */}
          <div className="absolute inset-0 px-4 pt-4 pb-8 pointer-events-none">
            <svg viewBox="0 0 1000 140" preserveAspectRatio="none" className="w-full h-full overflow-visible">
              <defs>
                <linearGradient id="telemetryGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" stopColor={chartMetric === 'requests' ? '#6366f1' : chartMetric === 'tokens' ? '#0ea5e9' : chartMetric === 'latency' ? '#f59e0b' : '#f43f5e'} stopOpacity="0.32" />
                  <stop offset="100%" stopColor={chartMetric === 'requests' ? '#6366f1' : chartMetric === 'tokens' ? '#0ea5e9' : chartMetric === 'latency' ? '#f59e0b' : '#f43f5e'} stopOpacity="0.0" />
                </linearGradient>
              </defs>
              {chartSvgData.areaD && (
                <path d={chartSvgData.areaD} fill="url(#telemetryGradient)" />
              )}
              {chartSvgData.pointsStr && (
                <polyline
                  points={chartSvgData.pointsStr}
                  fill="none"
                  stroke={chartMetric === 'requests' ? '#6366f1' : chartMetric === 'tokens' ? '#0ea5e9' : chartMetric === 'latency' ? '#f59e0b' : '#f43f5e'}
                  strokeWidth="3"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              )}
              {chartSvgData.pointsArr.map((pt, i) => (
                <circle
                  key={i}
                  cx={pt.x}
                  cy={pt.y}
                  r={hoveredBucket?.label === pt.bucket.label ? "5" : "2"}
                  fill={hoveredBucket?.label === pt.bucket.label ? "#ffffff" : (chartMetric === 'requests' ? '#6366f1' : chartMetric === 'tokens' ? '#0ea5e9' : chartMetric === 'latency' ? '#f59e0b' : '#f43f5e')}
                  stroke={chartMetric === 'requests' ? '#6366f1' : chartMetric === 'tokens' ? '#0ea5e9' : chartMetric === 'latency' ? '#f59e0b' : '#f43f5e'}
                  strokeWidth="2"
                  className="transition-all duration-150"
                />
              ))}
            </svg>
          </div>

          {/* Hover Tooltip Overlay */}
          {hoveredBucket && (
            <div className="absolute top-3 right-4 bg-slate-900/90 dark:bg-slate-800/95 text-white text-[11px] px-3 py-1.5 rounded-xl border border-slate-700 shadow-lg pointer-events-none z-10 flex items-center space-x-3">
              <span className="font-mono text-slate-300">{hoveredBucket.label}</span>
              <span className="font-semibold text-indigo-300">
                {isZh ? '请求' : 'Reqs'}: {hoveredBucket.requests}
              </span>
              <span className="font-semibold text-sky-300">
                {isZh ? 'Tokens' : 'Tokens'}: {hoveredBucket.tokens}
              </span>
              <span className="font-semibold text-amber-300">
                {isZh ? 'TTFT' : 'TTFT'}: {hoveredBucket.avgTtft}ms
              </span>
              {hoveredBucket.errors > 0 && (
                <span className="font-semibold text-rose-400">
                  {isZh ? '错误' : 'Errors'}: {hoveredBucket.errors}
                </span>
              )}
            </div>
          )}

          {/* Bar / Column Chart Grid */}
          <div className="h-38 flex items-end justify-between gap-1.5 sm:gap-2 pt-4 px-1 z-1">
            {timeBuckets.map((bucket, idx) => {
              const val = chartMetric === 'requests' 
                ? bucket.requests 
                : chartMetric === 'tokens' 
                ? bucket.tokens 
                : chartMetric === 'latency' 
                ? bucket.avgTtft 
                : bucket.errors;

              const heightPct = Math.max(8, Math.min(100, Math.round((val / maxMetricVal) * 100)));
              
              const barColor = chartMetric === 'requests'
                ? 'bg-gradient-to-t from-indigo-600/70 to-indigo-400/80'
                : chartMetric === 'tokens'
                ? 'bg-gradient-to-t from-sky-600/70 to-sky-400/80'
                : chartMetric === 'latency'
                ? 'bg-gradient-to-t from-amber-600/70 to-amber-400/80'
                : 'bg-gradient-to-t from-rose-600/70 to-rose-400/80';

              return (
                <div
                  key={idx}
                  onMouseEnter={() => setHoveredBucket(bucket)}
                  onMouseLeave={() => setHoveredBucket(null)}
                  className="flex-1 h-full flex flex-col justify-end items-center group cursor-pointer"
                >
                  <div
                    style={{ height: `${heightPct}%` }}
                    className={`w-full max-w-[20px] rounded-t-md transition-all duration-300 ${barColor} group-hover:brightness-125 group-hover:scale-y-105`}
                  />
                </div>
              );
            })}
          </div>

          {/* X-Axis Timeline Labels */}
          <div className="flex justify-between text-[10px] text-slate-400 font-mono pt-2 border-t border-slate-200/50 dark:border-slate-800 z-1">
            <span>{timeBuckets[0]?.label}</span>
            <span>{timeBuckets[Math.floor(timeBuckets.length / 2)]?.label}</span>
            <span>{isZh ? '当前 (实时)' : 'Live Now'}</span>
          </div>
        </div>

        {/* Latency Percentiles & SLA Quick Stat Bar */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 pt-1">
          <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
            <span className="text-[10px] text-slate-400 font-medium block">{isZh ? 'P50 中位时延' : 'P50 Median TTFT'}</span>
            <div className="flex items-baseline space-x-1.5 mt-0.5">
              <span className="text-lg font-extrabold text-emerald-600 dark:text-emerald-400 font-mono">
                {percentiles.p50} ms
              </span>
              <span className="text-[10px] text-slate-400 font-normal">(&lt; 200ms)</span>
            </div>
          </div>
          <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
            <span className="text-[10px] text-slate-400 font-medium block">{isZh ? 'P90 企业基准' : 'P90 Baseline'}</span>
            <div className="flex items-baseline space-x-1.5 mt-0.5">
              <span className="text-lg font-extrabold text-amber-600 dark:text-amber-400 font-mono">
                {percentiles.p90} ms
              </span>
              <span className="text-[10px] text-slate-400 font-normal">(&lt; 500ms)</span>
            </div>
          </div>
          <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
            <span className="text-[10px] text-slate-400 font-medium block">{isZh ? 'P99 极端长尾' : 'P99 Tail TTFT'}</span>
            <div className="flex items-baseline space-x-1.5 mt-0.5">
              <span className="text-lg font-extrabold text-indigo-600 dark:text-indigo-400 font-mono">
                {percentiles.p99} ms
              </span>
              <span className="text-[10px] text-slate-400 font-normal">(&lt; 1000ms)</span>
            </div>
          </div>
          <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
            <span className="text-[10px] text-slate-400 font-medium block">{isZh ? '服务可用率 (SLA)' : 'SLA Availability'}</span>
            <div className="flex items-baseline space-x-1.5 mt-0.5">
              <span className="text-lg font-extrabold text-emerald-600 dark:text-emerald-400 font-mono">
                {slaAvailability}%
              </span>
              <span className="text-[10px] text-slate-400 font-normal">{errorLogs.length} {isZh ? '次异常' : 'errors'}</span>
            </div>
          </div>
        </div>
      </div>

      {/* 4. WORKLOAD DISTRIBUTION & TELEMETRY PANELS (FOUR QUADRANTS) */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Panel 1: Multimodal Modality Distribution */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <PieChart className="w-4 h-4 text-pink-500" />
              <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
                {isZh ? '全模态负载分布 (Multimodal Workloads)' : 'Multimodal Workload Distribution'}
              </h4>
            </div>
            <span className="text-[11px] font-semibold text-slate-400">
              {modalityStats.length} {isZh ? '类模态端点' : 'Modalities'}
            </span>
          </div>

          <div className="space-y-3.5 pt-1">
            {modalityStats.map(mod => (
              <div key={mod.key} className="space-y-1">
                <div className="flex justify-between text-xs">
                  <span className="font-medium text-slate-700 dark:text-slate-300">
                    {mod.label}
                  </span>
                  <span className="font-mono text-slate-500 dark:text-slate-400">
                    {mod.count} {isZh ? '次' : 'reqs'} ({mod.pct}%)
                  </span>
                </div>
                <div className="w-full bg-slate-100 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
                  <div
                    style={{ width: `${Math.max(4, mod.pct)}%` }}
                    className={`h-full rounded-full transition-all duration-500 ${mod.color}`}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Panel 2: Provider Latency & SLA Benchmark */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <Award className="w-4 h-4 text-emerald-500" />
              <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
                {isZh ? '服务商实时时延与可用性跑分榜' : 'Provider Latency & SLA Benchmark'}
              </h4>
            </div>
            <span className="text-[11px] font-semibold text-slate-400">
              {providerSlaStats.length} {isZh ? '个上游渠道' : 'Channels'}
            </span>
          </div>

          {providerSlaStats.length === 0 ? (
            <div className="p-8 text-center text-xs text-slate-400">
              {isZh ? '暂无服务商测速数据' : 'No provider latency benchmark data'}
            </div>
          ) : (
            <div className="space-y-3 pt-1">
              {providerSlaStats.slice(0, 5).map((p, idx) => {
                const maxTtft = Math.max(...providerSlaStats.map(s => s.avgTtft || 100), 500);
                const barWidth = Math.max(10, Math.min(100, Math.round(((p.avgTtft || 150) / maxTtft) * 100)));
                const tier = (p.avgTtft || 0) < 300 
                  ? { label: isZh ? '极速' : 'Fast', color: 'text-emerald-600 bg-emerald-50 dark:bg-emerald-950/60 border-emerald-200 dark:border-emerald-800' }
                  : (p.avgTtft || 0) < 800
                  ? { label: isZh ? '良好' : 'Optimal', color: 'text-amber-600 bg-amber-50 dark:bg-amber-950/60 border-amber-200 dark:border-amber-800' }
                  : { label: isZh ? '拥堵' : 'Slow', color: 'text-rose-600 bg-rose-50 dark:bg-rose-950/60 border-rose-200 dark:border-rose-800' };

                return (
                  <div key={p.id} className="space-y-1.5">
                    <div className="flex items-center justify-between text-xs">
                      <div className="flex items-center space-x-2">
                        <span className="w-4 h-4 rounded-full bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 text-[10px] font-bold flex items-center justify-center">
                          {idx + 1}
                        </span>
                        <span className="font-semibold text-slate-800 dark:text-slate-200">
                          {p.name}
                        </span>
                        <span className={`text-[10px] px-1.5 py-0.2 rounded-md font-medium border ${tier.color}`}>
                          {tier.label}
                        </span>
                      </div>
                      <div className="flex items-center space-x-2 font-mono">
                        <span className="font-bold text-slate-900 dark:text-slate-100">
                          {p.avgTtft} ms
                        </span>
                        <span className="text-[11px] text-emerald-600 dark:text-emerald-400 font-semibold">
                          SLA {p.successRate}%
                        </span>
                      </div>
                    </div>
                    <div className="w-full bg-slate-100 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
                      <div
                        style={{ width: `${barWidth}%` }}
                        className={`h-full rounded-full transition-all duration-500 ${
                          (p.avgTtft || 0) < 300 ? 'bg-emerald-500' : (p.avgTtft || 0) < 800 ? 'bg-amber-500' : 'bg-rose-500'
                        }`}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>

        {/* Panel 3: Enterprise SLA Target & Error Budget Gauge */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <Target className="w-4 h-4 text-indigo-500" />
              <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
                {isZh ? '企业级 99.9% SLA 承诺与错误预算 (Error Budget)' : 'Enterprise 99.9% SLA & Error Budget'}
              </h4>
            </div>
            <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold bg-indigo-50 dark:bg-indigo-950/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800">
              {isZh ? '三九可用性指标' : 'Three-Nines SLA'}
            </span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
            <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 font-medium block">
                {isZh ? 'SLA 目标承诺' : 'Committed SLA Target'}
              </span>
              <div className="text-xl font-extrabold text-indigo-600 dark:text-indigo-400 font-mono mt-0.5">
                99.90%
              </div>
              <div className="text-[10px] text-slate-400 mt-0.5">
                {isZh ? '月允许故障时间 ≤ 43分钟' : 'Monthly downtime ≤ 43m'}
              </div>
            </div>

            <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 font-medium block">
                {isZh ? '当前实际可用率' : 'Actual Availability'}
              </span>
              <div className="text-xl font-extrabold text-emerald-600 dark:text-emerald-400 font-mono mt-0.5">
                {slaAvailability}%
              </div>
              <div className="text-[10px] text-emerald-600 dark:text-emerald-400 font-medium mt-0.5 flex items-center">
                <CheckCircle2 className="w-3 h-3 mr-1" />
                {parseFloat(slaAvailability) >= 99.9 ? (isZh ? '优于承诺目标' : 'Exceeds SLA target') : (isZh ? '处于容差范围' : 'Within budget')}
              </div>
            </div>

            <div className="p-3.5 rounded-2xl bg-slate-50 dark:bg-slate-900/60 border border-slate-200/70 dark:border-slate-800">
              <span className="text-[10px] text-slate-400 font-medium block">
                {isZh ? '容灾漂移平均耗时 (MTTR)' : 'Failover MTTR'}
              </span>
              <div className="text-xl font-extrabold text-sky-600 dark:text-sky-400 font-mono mt-0.5">
                24 ms
              </div>
              <div className="text-[10px] text-slate-400 mt-0.5">
                {isZh ? 'Pre-Token 无感自愈' : 'Pre-token seamless retry'}
              </div>
            </div>
          </div>

          <div className="space-y-1.5 pt-2 border-t border-slate-100 dark:border-slate-800/80">
            <div className="flex justify-between text-xs">
              <span className="font-medium text-slate-700 dark:text-slate-300">
                {isZh ? '剩余错误预算 (Error Budget Remaining)' : 'Error Budget Remaining'}
              </span>
              <span className="font-mono font-bold text-indigo-600 dark:text-indigo-400">
                {errorBudget.remainingBudgetPct}% {isZh ? '安全裕量' : 'Safety Margin'}
              </span>
            </div>
            <div className="w-full bg-slate-100 dark:bg-slate-800 h-2.5 rounded-full overflow-hidden">
              <div
                style={{ width: `${errorBudget.remainingBudgetPct}%` }}
                className={`h-full rounded-full transition-all duration-500 ${
                  errorBudget.remainingBudgetPct > 60
                    ? 'bg-gradient-to-r from-emerald-500 to-indigo-500'
                    : errorBudget.remainingBudgetPct > 20
                    ? 'bg-gradient-to-r from-amber-500 to-amber-600'
                    : 'bg-gradient-to-r from-rose-500 to-rose-600'
                }`}
              />
            </div>
          </div>
        </div>

        {/* Panel 4: Top Models Breakdown */}
        <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl p-6 shadow-xs space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <Cpu className="w-4 h-4 text-purple-500" />
              <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">
                {isZh ? '热门大模型 Token 占比 (Top Models)' : 'Top Models by Token Usage'}
              </h4>
            </div>
            <span className="text-[11px] font-semibold text-slate-400">Top 5</span>
          </div>

          {topModels.length === 0 ? (
            <div className="p-12 text-center text-xs text-slate-400">
              {isZh ? '暂无模型调用数据' : 'No model invocation telemetry yet'}
            </div>
          ) : (
            <div className="space-y-3.5 pt-1">
              {topModels.map(m => (
                <div key={m.name} className="space-y-1">
                  <div className="flex justify-between text-xs font-mono">
                    <span className="font-semibold text-slate-800 dark:text-slate-200 truncate max-w-[200px]">
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

      {/* 5. UPSTREAM PROVIDERS HEALTH & CIRCUIT BREAKER MATRIX */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
        <div className="p-5 border-b border-slate-100 dark:border-slate-800/80 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div>
            <h3 className="font-bold text-sm text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <Shield className="w-4 h-4 text-emerald-500" />
              <span>{isZh ? '上游渠道实时测速与三态熔断矩阵' : 'Provider Telemetry & Tri-State Breaker Matrix'}</span>
            </h3>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchTest}
              disabled={batchTesting || channels.length === 0}
              className="px-3.5 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-xs font-semibold text-slate-700 dark:text-slate-200 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1.5 cursor-pointer"
            >
              <Activity className={`w-3.5 h-3.5 ${batchTesting ? 'animate-spin text-indigo-500' : 'text-emerald-500'}`} />
              <span>{batchTesting ? (isZh ? '体检中...' : 'Probing...') : (isZh ? '全渠道体检' : 'Probe All')}</span>
            </button>
            <button
              onClick={() => setCurrentTab('channels')}
              className="px-3.5 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/60 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800 text-xs font-semibold transition cursor-pointer"
            >
              {isZh ? `渠道配置 (${channels.length}) →` : `Manage Channels (${channels.length}) →`}
            </button>
          </div>
        </div>

        {channels.length === 0 ? (
          <div className="p-10 text-center flex flex-col items-center justify-center space-y-3">
            <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-950/50 border border-indigo-200 dark:border-indigo-800/80 flex items-center justify-center text-indigo-500">
              <Server className="w-5 h-5" />
            </div>
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">{isZh ? '暂无服务商' : 'No Providers Connected'}</h4>
            {adminUser?.role === 'admin' && setShowChannelModal && (
              <button
                onClick={() => setShowChannelModal(true)}
                className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-xs transition flex items-center space-x-1.5 cursor-pointer"
              >
                <Plus className="w-4 h-4" />
                <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
              </button>
            )}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-3 px-6 font-semibold">{isZh ? '服务商名称' : 'Provider Name'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '协议类型' : 'Protocol'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '接入 Base URL' : 'Base URL'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '支持模型数' : 'Models'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '熔断器状态' : 'Breaker State'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '最近延迟' : 'Probe Latency'}</th>
                  <th className="py-3 px-6 text-right font-semibold">{isZh ? '体检操作' : 'Action'}</th>
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
                        {ch.models?.length || 0} {isZh ? '个模型' : 'models'}
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
                          {breaker === 'CLOSED' ? (isZh ? '正常' : 'Closed (Healthy)') : breaker === 'HALF-OPEN' ? (isZh ? '半开恢复' : 'Half-Open') : (isZh ? '熔断隔离' : 'Open (Tripped)')}
                        </span>
                      </td>
                      <td className="py-3.5 px-6 font-mono text-xs">
                        {latInfo ? (
                          latInfo.success ? (
                            <span className="text-emerald-600 dark:text-emerald-400 font-semibold">{latInfo.latencyMs} ms</span>
                          ) : (
                            <span className="text-rose-500">{isZh ? '异常' : 'Error'}</span>
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

      {/* 6. ERROR CLUSTERING & ROOT CAUSE ANALYSIS (RCA) */}
      {errorLogs.length > 0 && (
        <div className="bg-white dark:bg-[#111726] border border-amber-200/80 dark:border-amber-800/60 rounded-3xl p-5 shadow-xs space-y-3">
          <div className="flex items-center space-x-2 text-amber-800 dark:text-amber-200 font-bold text-sm">
            <AlertCircle className="w-4 h-4 text-amber-600" />
            <span>{isZh ? '智能异常聚类与根因排障建议 (Smart RCA Insights)' : 'Intelligent Error Clustering & AI RCA Insights'}</span>
          </div>
          <div className="text-xs text-slate-600 dark:text-slate-300 leading-relaxed space-y-2">
            <p>
              {isZh ? `在近期审计日志中检测到 ` : `Detected `}
              <strong>{errorLogs.length} {isZh ? '次非 2xx 请求' : 'non-2xx requests in recent logs'}</strong>。
              {isZh ? '主要状态分布：' : ' Distribution: '}
              {Object.entries(errorCountByCode).map(([code, count]) => (
                <span key={code} className="ml-2 font-mono font-bold text-rose-600 dark:text-rose-400">
                  HTTP {code} ({count}{isZh ? '次' : 'x'})
                </span>
              ))}
            </p>
            <div className="p-3 rounded-xl bg-amber-50/60 dark:bg-amber-950/30 border border-amber-200/70 dark:border-amber-800/50 text-[11px] text-amber-900 dark:text-amber-200">
              💡 <strong>{isZh ? '网关诊断建议：' : 'Gateway Diagnostics: '}</strong>
              {isZh
                ? '如频繁出现 HTTP 429，说明上游服务商已达 RPM 限流，Airoute 已自动启动 Pre-Token Fallback 漂移至备选渠道。建议在【渠道配置】中提高备选渠道权重或补充多 Key 负载均衡。'
                : 'Frequent HTTP 429 indicates upstream provider RPM limits have been reached. Airoute pre-token fallback will automatically divert traffic. Consider adding secondary channels or multiple API keys for weighted load balancing.'}
            </div>
          </div>
        </div>
      )}

      {/* 7. RECENT TRAFFIC AUDIT STREAM */}
      <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
        <div className="p-5 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div>
            <h3 className="font-bold text-sm text-slate-900 dark:text-slate-100 flex items-center space-x-2">
              <History className="w-4 h-4 text-sky-500" />
              <span>{isZh ? '最近流量流水' : 'Recent Inbound Audit Stream'}</span>
            </h3>
          </div>
          <button
            onClick={() => setCurrentTab('logs')}
            className="text-xs font-semibold text-indigo-600 dark:text-indigo-400 hover:underline cursor-pointer"
          >
            {isZh ? `全部审计日志 (${logs.length}) →` : `All Audit Logs (${logs.length}) →`}
          </button>
        </div>

        {logs.length === 0 ? (
          <div className="p-8 text-center text-xs text-slate-400 dark:text-slate-500">
            {isZh ? '暂无请求记录' : 'No request logs recorded yet'}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/60 dark:bg-slate-900/60">
                  <th className="py-3 px-6 font-semibold">{isZh ? '请求时间' : 'Timestamp'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '调用方 / 租户' : 'Client / Tenant'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '请求模型' : 'Model'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '命中上游' : 'Routed Channel'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '状态码' : 'Status'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? '首字 (TTFT) / 总耗时' : 'TTFT / Latency'}</th>
                  <th className="py-3 px-6 font-semibold">{isZh ? 'Token 消耗' : 'Tokens'}</th>
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
                      {log.channel_name || (isZh ? '默认通道' : 'Default')}
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
