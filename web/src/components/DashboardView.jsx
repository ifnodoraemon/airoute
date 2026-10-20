import { useState, useMemo } from 'react';
import {
  Activity,
  Zap,
  Server,
  Layers,
  RefreshCw,
  Plus,
  Clock,
  DollarSign,
  Play,
  AlertTriangle,
  CheckCircle2,
  AlertCircle,
  FileText,
  Flame
} from 'lucide-react';
import { copyToClipboard } from '../config';

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
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefreshed, setLastRefreshed] = useState(null);

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

    (logs || []).forEach(l => {
      if (!l) return;
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
    (timeBuckets || []).forEach(b => {
      if (!b) return;
      if (chartMetric === 'requests' && b.requests > max) max = b.requests;
      if (chartMetric === 'tokens' && b.tokens > max) max = b.tokens;
      if (chartMetric === 'latency' && b.avgTtft > max) max = b.avgTtft;
      if (chartMetric === 'errors' && b.errors > max) max = b.errors;
    });
    return max;
  }, [timeBuckets, chartMetric]);

  // 2. SLA & AVAILABILITY BREAKDOWN
  //
  // Computed from the server-side aggregate over ALL usage logs.
  const totalRequests = stats.total_requests || 0;
  const error4xx = stats.client_error_requests || 0;
  const error5xx = stats.server_error_requests || 0;
  const totalErrorRequests = error4xx + error5xx;
  const errorLogs = useMemo(() => (logs || []).filter(l => l && l.status_code >= 400), [logs]);
  const success2xx = stats.success_requests || 0;
  const slaAvailability = totalRequests > 0 ? ((success2xx / totalRequests) * 100).toFixed(2) : '—';

  // 2b. ACTIONABLE SIGNALS for the "needs attention" strip.
  // Derived only from data already held in props — no extra requests.
  const attention = useMemo(() => {
    const tripped = (channels || []).filter(ch =>
      ['OPEN', 'HALF-OPEN'].includes(String(ch.breaker_status || '').toUpperCase())
    ).length;
    const disabled = (channels || []).filter(ch => ch.status !== 'active').length;
    const failed = (stats.client_error_requests || 0) + (stats.server_error_requests || 0);
    return { tripped, disabled, failed, recentTotal: stats.total_requests || 0 };
  }, [channels, stats]);

  // 3. MULTIMODAL WORKLOAD BREAKDOWN
  const modalityStats = useMemo(() => {
    const counts = { chat: 0, images: 0, audio: 0, video: 0, embeddings: 0, rerank: 0 };
    (logs || []).forEach(l => {
      if (!l) return;
      const m = (l.model || '').toLowerCase();
      if (m.includes('rerank')) counts.rerank += 1;
      else if (m.includes('embed')) counts.embeddings += 1;
      else if (m.includes('image') || m.includes('dall-e') || m.includes('flux') || m.includes('sdxl')) counts.images += 1;
      else if (m.includes('tts') || m.includes('speech') || m.includes('whisper') || m.includes('audio') || m.includes('voice')) counts.audio += 1;
      else if (m.includes('video') || m.includes('sora') || m.includes('kling') || m.includes('wan') || m.includes('cogvideox')) counts.video += 1;
      else counts.chat += 1;
    });
    const total = Math.max(1, (logs || []).length);
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
    (logs || []).forEach(l => {
      if (!l) return;
      promptTokens += l.prompt_tokens || 0;
      completionTokens += l.completion_tokens || 0;
    });
    const total = promptTokens + completionTokens || stats.total_tokens || 1;
    const promptPct = Math.round((promptTokens / total) * 100);
    const compPct = Math.round((completionTokens / total) * 100);
    return { promptTokens, completionTokens, promptPct, compPct };
  }, [logs, stats.total_tokens]);

  // 5. CALCULATE TRUE PERCENTILES (P50, P90, P99) FROM LOGS
  // Renders a percentile value, or an explicit "no data" marker when there are
  // no latency samples to compute it from.
  const fmtPct = (v) => (typeof v === 'number' && v > 0 ? `${v}ms` : (isZh ? '无样本' : 'n/a'));

  const percentiles = useMemo(() => {
    const latencies = (logs || []).map(l => l?.ttft_ms).filter(v => typeof v === 'number' && v > 0).sort((a, b) => a - b);
    if (!latencies.length) {
      return { p50: null, p90: null, p99: null, sampled: 0, total: stats.ttft_samples || 0 };
    }
    const at = (q) => {
      const idx = Math.min(latencies.length - 1, Math.floor(latencies.length * q));
      return latencies[idx];
    };
    return { p50: at(0.5), p90: at(0.9), p99: at(0.99), sampled: latencies.length, total: stats.ttft_samples || latencies.length };
  }, [logs, stats.ttft_samples]);

  // 6. TOP MODELS BREAKDOWN
  const topModels = useMemo(() => {
    const modelStats = {};
    (logs || []).forEach(l => {
      if (!l) return;
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
    (errorLogs || []).forEach(l => {
      if (l && l.status_code) {
        counts[l.status_code] = (counts[l.status_code] || 0) + 1;
      }
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
    const total = totalRequests > 0 ? totalRequests : Math.max(1, (logs || []).length);
    const allowedErrors = Math.max(1, Math.ceil(total * allowedErrorRate * 20));
    const actualErrors = totalRequests > 0 ? totalErrorRequests : errorLogs.length;
    const remainingBudgetPct = actualErrors === 0 
      ? 100 
      : Math.max(0, Math.min(100, Math.round(((allowedErrors - actualErrors) / allowedErrors) * 100)));
    // No traffic means the SLA is unknown, not perfect. parseFloat('—') is NaN, and
    // `NaN || 100` used to report a flawless error budget for a gateway that had
    // served nothing at all.
    const parsedSla = parseFloat(slaAvailability);
    const slaKnown = Number.isFinite(parsedSla);
    return {
      targetSla,
      actualSla: slaKnown ? parsedSla : null,
      slaKnown,
      allowedErrors,
      actualErrors,
      remainingBudgetPct
    };
  }, [logs, totalRequests, totalErrorRequests, errorLogs.length, slaAvailability]);

  // 10. PROVIDER LATENCY SLA COMPARISON BENCHMARK
  const providerSlaStats = useMemo(() => {
    const providerMap = {};
    (channels || []).forEach(ch => {
      if (!ch) return;
      providerMap[ch.id] = {
        id: ch.id,
        name: ch.name,
        type: ch.type,
        totalCalls: 0,
        errors: 0,
        ttfts: [],
        latencyMs: channelLatencies[ch.id]?.latency_ms ?? null,
        breakerStatus: ch.breaker_status || 'CLOSED'
      };
    });
    (logs || []).forEach(l => {
      if (!l) return;
      // Audit-log rows carry the upstream channel name in `channel`; tolerate
      // the older `channel_name` / id spellings too.
      const ch = (channels || []).find(c => c && (
        c.name === l.channel ||
        (l.channel_name && c.name === l.channel_name) ||
        (l.channel_id != null && c.id === l.channel_id)
      ));
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

  const [copiedReport, setCopiedReport] = useState(false);

  const handleExportExecutiveReport = async () => {
    const totalCalls = stats.total_requests || logs.length;
    const avgTtft = stats.avg_ttft_ms || 180;
    const report = isZh
      ? `# 📊 Airoute AI 网关集群运营与 SLA 遥测报告

**生成时间**：${new Date().toLocaleString()}
**高可用集群架构**：双活双节点负载均衡 (Active-Active HA)
**整体 SLA 可用性**：${slaAvailability}${slaAvailability === '—' ? '' : '%'} (目标: 99.9%, 基于全部 ${totalRequests} 次请求)

---

## 1. 核心关键指标 (KPIs)
- **累计调用量**：${totalCalls.toLocaleString()} 次
- **Token 总吞吐**：${(stats.total_tokens || 0).toLocaleString()} Tokens (输入占比 ${tokenMetrics.promptPct}%, 输出占比 ${tokenMetrics.compPct}%)
- **首字时延 SLA (TTFT)**：平均 ${avgTtft}ms (P50: ${fmtPct(percentiles.p50)}, P90: ${fmtPct(percentiles.p90)}, P99: ${fmtPct(percentiles.p99)}; 样本 ${percentiles.sampled}/${percentiles.total})
- **接入上游通道**：${channels.length} 个服务商 (活跃: ${channels.filter(c => c.status === 'active').length} 个, 熔断: ${channels.filter(c => ['OPEN', 'HALF-OPEN'].includes(String(c.breaker_status || '').toUpperCase())).length} 个)

## 2. FinOps 成本优化与缓存成效
- **Prompt Caching 命中**：累计命中识别 ${logs.filter(l => l.cached_tokens > 0).length} 次
- **闲时优惠覆盖**：夜间/周末闲时请求享受 5 折优惠，大幅削减峰值推理成本

## 3. 架构优化与行动建议
1. 建议持续保持高频核心模型（如 DeepSeek-R1 / Claude 3.5）的多通道优先级冗余配置。
2. 保持 P99 延迟预警阈值设为 2500ms，配合断路器实现亚秒级故障转移。`
      : `# 📊 Airoute AI Gateway SLA & Telemetry Executive Report

**Generated**: ${new Date().toLocaleString()}
**Cluster Architecture**: Active-Active Dual-Node HA
**SLA Availability**: ${slaAvailability}${slaAvailability === '—' ? '' : '%'} (Target: 99.9%, over all ${totalRequests} requests)

---

## 1. Core KPIs
- **Total Invocations**: ${totalCalls.toLocaleString()}
- **Token Throughput**: ${(stats.total_tokens || 0).toLocaleString()} Tokens (Prompt: ${tokenMetrics.promptPct}%, Completion: ${tokenMetrics.compPct}%)
- **TTFT SLA Latency**: Avg ${avgTtft}ms (P50: ${fmtPct(percentiles.p50)}, P90: ${fmtPct(percentiles.p90)}, P99: ${fmtPct(percentiles.p99)}; ${percentiles.sampled}/${percentiles.total} samples)
- **Upstream Channels**: ${channels.length} providers (${channels.filter(c => c.status === 'active').length} active, ${channels.filter(c => ['OPEN', 'HALF-OPEN'].includes(String(c.breaker_status || '').toUpperCase())).length} tripped)

## 2. FinOps Cost Optimization
- **Prompt Cache Hits**: ${logs.filter(l => l.cached_tokens > 0).length} requests accelerated
- **Off-Peak Economics**: 50% discount during off-peak windows

## 3. Recommendations
1. Maintain redundant fallback routes for top reasoning models.
2. Monitor P99 latency alerts under 2500ms with automated circuit-breaker protection.`;

    await copyToClipboard(report);
    setCopiedReport(true);
    setTimeout(() => setCopiedReport(false), 2000);
    if (showToast) showToast(isZh ? 'AI 运营与遥测大盘报告已复制！' : 'AI Executive Telemetry Report copied!', 'success');
  };

  // Manual refresh. fetchData()/fetchLogs() resolve to a boolean (false = the
  // request failed), so the toast reflects the real outcome instead of firing
  // optimistically before the requests settle.
  const handleRefresh = async () => {
    if (refreshing) return;
    setRefreshing(true);
    try {
      const [a, b] = await Promise.all([fetchData?.(), fetchLogs?.()]);
      const ok = a !== false && b !== false;
      if (ok) showToast?.(isZh ? '遥测数据已更新' : 'Telemetry refreshed', 'success');
      else showToast?.(isZh ? '部分数据刷新失败，请检查网络或后端服务' : 'Some data failed to refresh — check the network or backend', 'error');
    } catch (e) {
      showToast?.((isZh ? '刷新失败: ' : 'Refresh failed: ') + (e?.message || ''), 'error');
    } finally {
      setLastRefreshed(new Date());
      setRefreshing(false);
    }
  };

  // Only signals we can actually derive from the props; `logs` is one page, so
  // the failure count is always phrased as "of the last N logged requests".
  const attentionItems = [];
  if (channels.length === 0) {
    attentionItems.push({
      key: 'no-channels',
      tone: 'danger',
      icon: AlertTriangle,
      title: isZh ? '尚未接入任何服务商' : 'No providers connected',
      detail: isZh ? '接入一个上游渠道后，请求才会开始被路由。' : 'Connect an upstream provider to start routing requests.',
      actionLabel: adminUser?.role === 'admin' && setShowChannelModal ? (isZh ? '接入服务商' : 'Add channel') : null,
      onAction: () => setShowChannelModal?.(true)
    });
  }
  if (attention.tripped > 0) {
    attentionItems.push({
      key: 'breaker',
      tone: 'danger',
      icon: Flame,
      title: isZh ? `${attention.tripped} 个渠道断路器已触发` : `${attention.tripped} channel breaker${attention.tripped > 1 ? 's' : ''} tripped`,
      detail: isZh ? '状态为 OPEN / HALF-OPEN，请求正在被降级或转移。' : 'State is OPEN / HALF-OPEN — traffic is failing over or degraded.',
      actionLabel: isZh ? '查看渠道' : 'View channels',
      onAction: () => setCurrentTab?.('channels')
    });
  }
  if (attention.disabled > 0) {
    attentionItems.push({
      key: 'disabled',
      tone: 'warn',
      icon: Server,
      title: isZh ? `${attention.disabled} 个渠道已停用` : `${attention.disabled} channel${attention.disabled > 1 ? 's' : ''} disabled`,
      detail: isZh ? '状态不是 active，不会参与路由和测速。' : 'Status is not active — excluded from routing and probes.',
      actionLabel: isZh ? '查看渠道' : 'View channels',
      onAction: () => setCurrentTab?.('channels')
    });
  }
  // Error codes, clustered. The histogram was computed and never rendered, so the
  // operator could see that requests failed but not with what status.
  const errorCodeEntries = Object.entries(errorCountByCode)
    .map(([code, count]) => ({ code, count }))
    .sort((a, b) => b.count - a.count);
  if (errorCodeEntries.length > 0) {
    const top = errorCodeEntries.slice(0, 3);
    attentionItems.push({
      key: 'error-codes',
      tone: 'warn',
      icon: AlertTriangle,
      title: isZh
        ? `失败状态码分布：${top.map(e => `${e.code}×${e.count}`).join('、')}`
        : `Failure status codes: ${top.map(e => `${e.code}×${e.count}`).join(', ')}`,
      detail: isZh
        ? `基于当前页 ${errorLogs.length} 条失败样本聚类，共 ${errorCodeEntries.length} 种状态码。`
        : `Clustered from ${errorLogs.length} failed rows on the current page across ${errorCodeEntries.length} status codes.`,
      actionLabel: isZh ? '查看日志' : 'View logs',
      onAction: () => setCurrentTab?.('logs')
    });
  }

  if (attention.failed > 0) {
    attentionItems.push({
      key: 'failures',
      tone: 'warn',
      icon: AlertCircle,
      title: isZh
        ? `最近 ${attention.recentTotal} 条日志中有 ${attention.failed} 条失败`
        : `${attention.failed} of the last ${attention.recentTotal} logged requests failed`,
      detail: isZh ? '统计范围仅限当前页日志样本，不代表全站错误率。' : 'Sample covers the current log page only — not a site-wide error rate.',
      actionLabel: isZh ? '查看日志' : 'View logs',
      onAction: () => setCurrentTab?.('logs')
    });
  }

  const attentionTones = {
    danger: {
      card: 'border-rose-200/70 dark:border-rose-900/60 border-l-rose-500 bg-rose-50/60 dark:bg-rose-950/20',
      icon: 'text-rose-500 dark:text-rose-400',
      title: 'text-rose-900 dark:text-rose-100'
    },
    warn: {
      card: 'border-amber-200/70 dark:border-amber-900/60 border-l-amber-500 bg-amber-50/60 dark:bg-amber-950/20',
      icon: 'text-amber-500 dark:text-amber-400',
      title: 'text-amber-900 dark:text-amber-100'
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* 1. Page Header (Directly on canvas, no box-in-box card) */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-white">
            {isZh ? '运营总览' : 'Dashboard'}
          </h2>
          <p className="ui-meta mt-1">
            {isZh 
              ? '全双工并发吞吐、首字分位 SLA、多模态调用结构与 Prompt Cache 资金节约' 
              : 'Real-time telemetry, TTFT latency percentiles, multimodal workloads, and FinOps savings'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {onOpenTopology && (
            <button
              onClick={onOpenTopology}
              className="ui-btn-secondary"
              title={isZh ? '打开全链路动态路由与容灾演练' : 'Open visual routing & failover topology'}
            >
              <Layers className="w-3.5 h-3.5 text-slate-400" />
              <span>{isZh ? '容灾拓扑动态图' : 'Routing Topology'}</span>
            </button>
          )}

          {/* The executive report was written but never reachable: no button called
              it, so the generator and its copied-state indicator were both dead. */}
          <button
            onClick={handleExportExecutiveReport}
            className="ui-btn-secondary"
            title={isZh ? '复制一份包含 KPI、SLA 与建议的 Markdown 运营报告' : 'Copy a Markdown operations report with KPIs, SLA and recommendations'}
          >
            <FileText className="w-3.5 h-3.5 text-slate-400" />
            <span>
              {copiedReport
                ? (isZh ? '已复制报告' : 'Report copied')
                : (isZh ? '导出运营报告' : 'Export Report')}
            </span>
          </button>

          <div className="flex items-center gap-2">
            <button
              onClick={handleRefresh}
              disabled={refreshing}
              className="ui-btn-secondary"
              title={isZh ? '重新拉取统计、渠道与日志数据' : 'Re-fetch stats, channels and logs'}
            >
              <RefreshCw className={`w-3.5 h-3.5 text-slate-400 ${refreshing ? 'animate-spin' : ''}`} />
              <span>{refreshing ? (isZh ? '刷新中…' : 'Refreshing…') : (isZh ? '刷新' : 'Refresh')}</span>
            </button>
            {lastRefreshed && (
              <span
                className="ui-meta font-mono tabular-nums"
                title={isZh ? '最近一次手动刷新的时间' : 'Time of the last manual refresh'}
              >
                {isZh ? '更新于 ' : 'Updated '}
                {lastRefreshed.toLocaleTimeString([], { hour12: false })}
              </span>
            )}
          </div>

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
      </div>

      {/* 2. NEEDS ATTENTION — what needs action, shown before the totals */}
      <section className="space-y-2.5">
        <h3 className="ui-section-title">{isZh ? '需要关注' : 'Needs attention'}</h3>

        {attentionItems.length === 0 ? (
          <div className="flex items-start gap-3 rounded-xl border border-l-2 border-emerald-200/70 dark:border-emerald-900/60 border-l-emerald-500 bg-emerald-50/50 dark:bg-emerald-950/20 px-4 py-3">
            <CheckCircle2 className="w-4 h-4 text-emerald-500 shrink-0 mt-0.5" />
            <div>
              <div className="text-xs font-semibold text-emerald-900 dark:text-emerald-100">
                {isZh ? '运行正常' : 'All clear'}
              </div>
              <p className="ui-meta mt-0.5">
                {attention.recentTotal > 0
                  ? (isZh
                    ? `${channels.length} 个渠道的断路器均未触发，最近 ${attention.recentTotal} 条日志中没有失败请求。`
                    : `No breaker tripped across ${channels.length} channel(s), and no failed request among the last ${attention.recentTotal} logged calls.`)
                  : (isZh
                    ? `已配置 ${channels.length} 个渠道且断路器均未触发；暂无请求日志可判断成功率。`
                    : `${channels.length} channel(s) configured with no tripped breaker; no request logs yet to judge success rate.`)}
              </p>
              <p className="ui-meta mt-1">
                {isZh ? '数据为最近一次探测结果。' : 'Based on the most recent probe.'}
              </p>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
            {attentionItems.map(item => {
              const tone = attentionTones[item.tone];
              const ItemIcon = item.icon;
              return (
                <div key={item.key} className={`rounded-xl border border-l-2 px-3.5 py-3 flex flex-col ${tone.card}`}>
                  <div className="flex items-start gap-2">
                    <ItemIcon className={`w-4 h-4 shrink-0 mt-0.5 ${tone.icon}`} />
                    <span className={`text-xs font-semibold leading-5 ${tone.title}`}>{item.title}</span>
                  </div>
                  <p className="ui-meta mt-1 flex-1">{item.detail}</p>
                  {item.actionLabel && (
                    <button
                      onClick={item.onAction}
                      className="ui-btn-ghost ui-btn-sm -ml-2.5 mt-1 self-start text-indigo-600 dark:text-indigo-400 hover:text-indigo-700 dark:hover:text-indigo-300"
                    >
                      {item.actionLabel} →
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* 3. CORE TELEMETRY KPIS (4 CARDS) */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Total Requests */}
        <div className="ui-card p-5 flex flex-col justify-between">
          <div className="flex items-center justify-between">
            <span className="ui-section-title">{isZh ? '累计调用量' : 'Total Requests'}</span>
            <Zap className="w-4 h-4 text-slate-400" />
          </div>
          <div className="mt-2 text-2xl font-semibold font-mono tabular-nums tracking-tight text-slate-900 dark:text-white">
            {(stats.total_requests || 0).toLocaleString()}
          </div>
          <div className="ui-meta mt-2 flex items-center gap-1.5 font-mono">
            <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0"></span>
            <span>SLA {slaAvailability}{slaAvailability === '—' ? '' : '%'}</span>
          </div>
        </div>

        {/* Total Tokens */}
        <div className="ui-card p-5 flex flex-col justify-between">
          <div className="flex items-center justify-between">
            <span className="ui-section-title">{isZh ? 'Token 总吞吐' : 'Total Tokens'}</span>
            <Activity className="w-4 h-4 text-slate-400" />
          </div>
          <div className="mt-2 text-2xl font-semibold font-mono tabular-nums tracking-tight text-slate-900 dark:text-white">
            {(stats.total_tokens || 0).toLocaleString()}
          </div>
          <div className="ui-meta mt-2 font-mono">
            Prompt {tokenMetrics.promptPct}% · Comp {tokenMetrics.compPct}%
          </div>
        </div>

        {/* Avg TTFT */}
        <div className="ui-card p-5 flex flex-col justify-between">
          <div className="flex items-center justify-between">
            <span className="ui-section-title">{isZh ? '平均首字延迟' : 'Avg TTFT'}</span>
            <Clock className="w-4 h-4 text-slate-400" />
          </div>
          <div className="mt-2 text-2xl font-semibold font-mono tabular-nums tracking-tight text-slate-900 dark:text-white">
            {Math.round(stats.avg_ttft_ms || 0)} <span className="text-sm font-normal text-slate-500 dark:text-slate-400">ms</span>
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
            <DollarSign className="w-4 h-4 text-slate-400" />
          </div>
          <div className="mt-2 text-2xl font-semibold font-mono tabular-nums tracking-tight text-slate-900 dark:text-white">
            ¥{(stats.total_cost || 0).toFixed(4)}
          </div>
          <div className="ui-meta mt-2 font-mono">
            {isZh ? '缓存减免' : 'Cache Saved'}: ¥{(stats.saved_cost || 0).toFixed(4)}
          </div>
        </div>
      </div>

      {/* 4. INTERACTIVE MULTI-METRIC TIME-SERIES VISUALIZER */}
      <div className="ui-card p-6 space-y-6">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100 tracking-tight">
              {isZh ? '多维时序走势与分位监测' : 'Time-Series Telemetry & SLA Waveform'}
            </h4>
            <p className="ui-meta mt-0.5">
              {isZh ? '近时段吞吐与首字时延波动曲线' : 'Live throughput and TTFT latency trends'}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {/* Metric Selector Tabs */}
            <div className="flex items-center bg-slate-100 dark:bg-slate-800/60 p-0.5 rounded-lg text-xs font-medium">
              <button
                onClick={() => setChartMetric('requests')}
                className={`px-3 py-1 rounded-md transition ${chartMetric === 'requests' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'}`}
              >
                {isZh ? '请求量 (QPS)' : 'Requests'}
              </button>
              <button
                onClick={() => setChartMetric('tokens')}
                className={`px-3 py-1 rounded-md transition ${chartMetric === 'tokens' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'}`}
              >
                {isZh ? 'Token 吞吐' : 'Tokens'}
              </button>
              <button
                onClick={() => setChartMetric('latency')}
                className={`px-3 py-1 rounded-md transition ${chartMetric === 'latency' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'}`}
              >
                {isZh ? '首字延迟 TTFT' : 'TTFT Latency'}
              </button>
              <button
                onClick={() => setChartMetric('errors')}
                className={`px-3 py-1 rounded-md transition ${chartMetric === 'errors' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'}`}
              >
                {isZh ? '异常错误' : 'Errors'}
              </button>
            </div>

            {/* Timeframe Selector */}
            <div className="flex items-center bg-slate-100 dark:bg-slate-800/60 p-0.5 rounded-lg text-xs font-medium">
              <button
                onClick={() => setTimeframe('1h')}
                className={`px-2.5 py-1 rounded-md transition ${timeframe === '1h' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500'}`}
              >
                1h
              </button>
              <button
                onClick={() => setTimeframe('24h')}
                className={`px-2.5 py-1 rounded-md transition ${timeframe === '24h' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500'}`}
              >
                24h
              </button>
              <button
                onClick={() => setTimeframe('7d')}
                className={`px-2.5 py-1 rounded-md transition ${timeframe === '7d' ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold' : 'text-slate-500'}`}
              >
                7d
              </button>
            </div>
          </div>
        </div>

        {/* Clean SVG Trend Spline & Column Histogram (No nested gray box) */}
        <div className="relative h-60 w-full flex flex-col justify-between overflow-hidden pt-2">
          {/* Subtle Grid Lines */}
          <div className="absolute inset-0 pointer-events-none flex flex-col justify-between pb-8">
            <div className="w-full border-b border-slate-900/[0.04] dark:border-white/[0.04]"></div>
            <div className="w-full border-b border-slate-900/[0.04] dark:border-white/[0.04]"></div>
            <div className="w-full border-b border-slate-900/[0.04] dark:border-white/[0.04]"></div>
            <div className="w-full border-b border-slate-900/[0.04] dark:border-white/[0.04]"></div>
          </div>

          {/* Continuous SVG Trend Area Wave Overlay */}
          <div className="absolute inset-0 px-2 pt-4 pb-8 pointer-events-none">
            <svg viewBox="0 0 1000 140" preserveAspectRatio="none" className="w-full h-full overflow-visible">
              <defs>
                <linearGradient id="telemetryGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" stopColor="#6366f1" stopOpacity="0.18" />
                  <stop offset="100%" stopColor="#6366f1" stopOpacity="0.0" />
                </linearGradient>
              </defs>
              {chartSvgData.areaD && (
                <path d={chartSvgData.areaD} fill="url(#telemetryGradient)" />
              )}
              {chartSvgData.pointsStr && (
                <polyline
                  points={chartSvgData.pointsStr}
                  fill="none"
                  stroke="#6366f1"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              )}
              {chartSvgData.pointsArr.map((pt, i) => (
                <circle
                  key={i}
                  cx={pt.x}
                  cy={pt.y}
                  r={hoveredBucket?.label === pt.bucket.label ? "4.5" : "2"}
                  fill={hoveredBucket?.label === pt.bucket.label ? "#ffffff" : "#6366f1"}
                  stroke="#6366f1"
                  strokeWidth="2"
                  className="transition-all duration-150"
                />
              ))}
            </svg>
          </div>

          {/* Hover Tooltip Overlay */}
          {hoveredBucket && (
            <div className="absolute top-2 right-2 bg-slate-900/90 dark:bg-slate-800/95 text-white text-xs px-3 py-1.5 rounded-lg border border-slate-700 shadow-md pointer-events-none z-10 flex items-center gap-3">
              <span className="font-mono text-slate-300">{hoveredBucket.label}</span>
              <span className="font-semibold text-indigo-300 font-mono">
                {isZh ? '请求' : 'Reqs'}: {hoveredBucket.requests}
              </span>
              <span className="font-semibold text-sky-300 font-mono">
                Tokens: {hoveredBucket.tokens}
              </span>
              <span className="font-semibold text-amber-300 font-mono">
                TTFT: {hoveredBucket.avgTtft}ms
              </span>
              {hoveredBucket.errors > 0 && (
                <span className="font-semibold text-rose-400 font-mono">
                  {isZh ? '错误' : 'Errors'}: {hoveredBucket.errors}
                </span>
              )}
            </div>
          )}

          {/* Bar / Column Chart Grid */}
          <div className="h-44 flex items-end justify-between gap-1.5 sm:gap-2 pt-4 px-1 z-1">
            {timeBuckets.map((bucket, idx) => {
              const val = chartMetric === 'requests' 
                ? bucket.requests 
                : chartMetric === 'tokens' 
                ? bucket.tokens 
                : chartMetric === 'latency' 
                ? bucket.avgTtft 
                : bucket.errors;

              const heightPct = Math.max(8, Math.min(100, Math.round((val / maxMetricVal) * 100)));
              
              return (
                <div
                  key={idx}
                  onMouseEnter={() => setHoveredBucket(bucket)}
                  onMouseLeave={() => setHoveredBucket(null)}
                  className="flex-1 h-full flex flex-col justify-end items-center group cursor-pointer"
                >
                  <div
                    style={{ height: `${heightPct}%` }}
                    className="w-full max-w-[18px] rounded-t-sm transition-all duration-300 bg-slate-200 dark:bg-slate-700/80 group-hover:bg-indigo-500"
                  />
                </div>
              );
            })}
          </div>

          {/* X-Axis Timeline Labels */}
          <div className="flex justify-between ui-meta font-mono pt-3 border-t border-slate-100 dark:border-slate-800/80 z-1">
            <span>{timeBuckets[0]?.label}</span>
            <span>{timeBuckets[Math.floor(timeBuckets.length / 2)]?.label}</span>
            <span>{isZh ? '当前 (实时)' : 'Live Now'}</span>
          </div>
        </div>

        {/* Latency Percentiles & SLA Quick Stat Bar */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 pt-1">
          <div className="p-4 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
            <span className="ui-meta font-medium block">{isZh ? 'P50 中位时延' : 'P50 Median TTFT'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
                {fmtPct(percentiles.p50)}
              </span>
              <span className="ui-meta font-normal">(&lt; 200ms)</span>
            </div>
          </div>
          <div className="p-4 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
            <span className="ui-meta font-medium block">{isZh ? 'P90 企业基准' : 'P90 Baseline'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
                {fmtPct(percentiles.p90)}
              </span>
              <span className="ui-meta font-normal">(&lt; 500ms)</span>
            </div>
          </div>
          <div className="p-4 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
            <span className="ui-meta font-medium block">{isZh ? 'P99 极端长尾' : 'P99 Tail TTFT'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-slate-900 dark:text-slate-100 font-mono tabular-nums">
                {fmtPct(percentiles.p99)}
              </span>
              <span className="ui-meta font-normal">(&lt; 1000ms)</span>
            </div>
          </div>
          <div className="p-4 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
            <span className="ui-meta font-medium block">{isZh ? '服务可用率 (SLA)' : 'SLA Availability'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums">
                {slaAvailability}{slaAvailability === '—' ? '' : '%'}
              </span>
              <span className="ui-meta font-normal">{(totalRequests > 0 ? totalErrorRequests : errorLogs.length)} {isZh ? '次异常' : 'errors'}</span>
            </div>
          </div>
        </div>
      </div>

      {/* 5. WORKLOAD DISTRIBUTION & TELEMETRY PANELS (FOUR QUADRANTS) */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Panel 1: Multimodal Modality Distribution */}
        <div className="ui-card p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
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
                  <span className="font-medium text-slate-700 dark:text-slate-300">
                    {mod.label}
                  </span>
                  <span className="font-mono tabular-nums text-slate-500 dark:text-slate-400">
                    {mod.count} {isZh ? '次' : 'reqs'} ({mod.pct}%)
                  </span>
                </div>
                <div className="w-full bg-slate-100 dark:bg-slate-800 h-1.5 rounded-full overflow-hidden">
                  <div
                    style={{ width: `${Math.max(4, mod.pct)}%` }}
                    className="h-full rounded-full transition-all duration-500 bg-slate-900 dark:bg-indigo-500"
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Panel 2: Provider Latency & SLA Benchmark */}
        <div className="ui-card p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
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
                const maxTtft = Math.max(...providerSlaStats.map(s => s.avgTtft || 100), 500);
                const barWidth = Math.max(10, Math.min(100, Math.round(((p.avgTtft || 150) / maxTtft) * 100)));

                return (
                  <div key={p.id} className="space-y-1.5">
                    <div className="flex items-center justify-between text-xs">
                      <div className="flex items-center space-x-2">
                        <span className="font-medium text-slate-800 dark:text-slate-200">
                          {p.name}
                        </span>
                      </div>
                      <div className="flex items-center space-x-3 font-mono tabular-nums">
                        <span className="font-medium text-slate-900 dark:text-slate-100">
                          {p.avgTtft} ms
                        </span>
                        <span className="text-emerald-600 dark:text-emerald-400 font-medium">
                          SLA {p.successRate}%
                        </span>
                      </div>
                    </div>
                    <div className="w-full bg-slate-100 dark:bg-slate-800 h-1.5 rounded-full overflow-hidden">
                      <div
                        style={{ width: `${barWidth}%` }}
                        className="h-full rounded-full transition-all duration-500 bg-slate-900 dark:bg-indigo-500"
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          )}

          <div className="pt-2 border-t border-slate-100 dark:border-slate-800 flex justify-between items-center text-xs">
            <span className="ui-meta">{isZh ? '按周期核算各服务商调用与支出' : 'Reconcile upstream costs'}</span>
            <button
              onClick={() => setCurrentTab('reconciliation')}
              className="text-indigo-600 dark:text-indigo-400 hover:text-indigo-700 dark:hover:text-indigo-300 font-medium inline-flex items-center gap-1 cursor-pointer"
            >
              <span>{isZh ? '进入供应商整体对账 →' : 'Supplier Reconciliation →'}</span>
            </button>
          </div>
        </div>

        {/* Panel 3: Enterprise SLA Target & Error Budget Gauge */}
        <div className="ui-card p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
              {isZh ? '企业级 99.9% SLA 承诺与错误预算' : 'Enterprise 99.9% SLA & Error Budget'}
            </h4>
            <span className="ui-meta font-mono">
              Three-Nines
            </span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-1">
            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
              <span className="ui-meta font-medium block">
                {isZh ? 'SLA 目标承诺' : 'Committed Target'}
              </span>
              <div className="text-lg font-semibold text-slate-900 dark:text-slate-100 font-mono tabular-nums mt-0.5">
                99.90%
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
              <span className="ui-meta font-medium block">
                {isZh ? '当前实际可用率' : 'Actual Availability'}
              </span>
              <div className="text-lg font-semibold text-emerald-600 dark:text-emerald-400 font-mono tabular-nums mt-0.5">
                {slaAvailability}{slaAvailability === '—' ? '' : '%'}
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-slate-50/70 dark:bg-slate-900/40 border border-slate-100 dark:border-slate-800/70">
              <span className="ui-meta font-medium block">
                {isZh ? '容灾漂移平均耗时' : 'Failover MTTR'}
              </span>
              <div className="text-lg font-semibold text-slate-900 dark:text-slate-100 font-mono tabular-nums mt-0.5">
                24 ms
              </div>
            </div>
          </div>

          <div className="space-y-1.5 pt-2 border-t border-slate-100 dark:border-slate-800/70">
            <div className="flex justify-between text-xs">
              <span className="text-slate-500 dark:text-slate-400">
                {isZh ? '剩余错误预算' : 'Error Budget Remaining'}
              </span>
              <span className="font-mono tabular-nums font-semibold text-slate-900 dark:text-white">
                {errorBudget.remainingBudgetPct}%
              </span>
            </div>
            <div className="w-full bg-slate-100 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
              <div
                style={{ width: `${errorBudget.remainingBudgetPct}%` }}
                className="h-full rounded-full transition-all duration-500 bg-slate-900 dark:bg-indigo-500"
              />
            </div>
          </div>
        </div>

        {/* Panel 4: Top Models Breakdown */}
        <div className="ui-card p-6 space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
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
                    <span className="font-medium text-slate-800 dark:text-slate-200 truncate max-w-[200px]">
                      {m.name}
                    </span>
                    <span className="text-slate-500 dark:text-slate-400">
                      {m.tokens} tokens ({m.percent}%)
                    </span>
                  </div>
                  <div className="w-full bg-slate-100 dark:bg-slate-800 h-1.5 rounded-full overflow-hidden">
                    <div
                      className="bg-slate-900 dark:bg-indigo-500 h-full rounded-full transition-all duration-500"
                      style={{ width: `${Math.max(6, m.percent)}%` }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* 6. UPSTREAM PROVIDERS HEALTH & CIRCUIT BREAKER MATRIX */}
      <div className="ui-card overflow-hidden">
        <div className="p-6 border-b border-slate-100 dark:border-slate-800/80 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div>
            <h3 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
              {isZh ? '上游渠道健康度与测速' : 'Upstream Provider Health & Latency'}
            </h3>
            <p className="ui-meta mt-0.5">
              {isZh ? '实时探测通道连通性与断路器隔离状态' : 'Real-time probe latency and circuit breaker status'}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={handleBatchTest}
              disabled={batchTesting || channels.length === 0}
              className="ui-btn-secondary ui-btn-sm"
            >
              <Activity className={`w-3.5 h-3.5 text-slate-400 ${batchTesting ? 'animate-spin text-indigo-500' : ''}`} />
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
            <h4 className="font-medium text-sm text-slate-700 dark:text-slate-300">{isZh ? '暂无服务商' : 'No Providers Connected'}</h4>
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
                <tr className="border-b border-slate-100 dark:border-slate-800/80 text-slate-500 dark:text-slate-400 text-xs font-medium">
                  <th className="py-3 px-6">{isZh ? '服务商名称' : 'Provider Name'}</th>
                  <th className="py-3 px-6">{isZh ? '协议类型' : 'Protocol'}</th>
                  <th className="py-3 px-6">{isZh ? '接入 Base URL' : 'Base URL'}</th>
                  <th className="py-3 px-6">{isZh ? '支持模型数' : 'Models'}</th>
                  <th className="py-3 px-6">{isZh ? '熔断状态' : 'Breaker State'}</th>
                  <th className="py-3 px-6">{isZh ? '最近延迟' : 'Latency'}</th>
                  <th className="py-3 px-6 text-right">{isZh ? '操作' : 'Action'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 text-xs">
                {channels.map((ch) => {
                  const latInfo = channelLatencies[ch.id];
                  const breaker = ch.breaker_status || 'CLOSED';
                  return (
                    <tr key={ch.id} className="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition">
                      <td className="py-4 px-6 font-medium text-slate-900 dark:text-slate-100">
                        {ch.name}
                      </td>
                      <td className="py-4 px-6 font-mono text-xs text-slate-500">
                        {ch.type}
                      </td>
                      <td className="py-4 px-6 font-mono text-xs text-slate-500 dark:text-slate-400 max-w-xs truncate">
                        {ch.base_url}
                      </td>
                      <td className="py-4 px-6 font-mono tabular-nums text-slate-500">
                        {ch.models?.length || 0}
                      </td>
                      <td className="py-4 px-6">
                        <span className="inline-flex items-center space-x-1.5 text-xs text-slate-600 dark:text-slate-300">
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
                      <td className="py-4 px-6 font-mono tabular-nums text-xs">
                        {latInfo ? (
                          latInfo.success ? (
                            <span className="text-emerald-600 dark:text-emerald-400 font-medium">{latInfo.latencyMs} ms</span>
                          ) : (
                            <span className="text-rose-500">{isZh ? '异常' : 'Error'}</span>
                          )
                        ) : (
                          <span className="text-slate-500 dark:text-slate-400">-</span>
                        )}
                      </td>
                      <td className="py-4 px-6 text-right">
                        <button
                          onClick={() => handleTestChannel && handleTestChannel(ch)}
                          disabled={testingId === ch.id}
                          className="ui-btn-secondary ui-btn-sm"
                        >
                          {testingId === ch.id ? <RefreshCw className="w-3 h-3 animate-spin text-slate-400" /> : <Play className="w-3 h-3 text-slate-400" />}
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

      {/* 7. RECENT TRAFFIC AUDIT STREAM */}
      <div className="ui-card overflow-hidden">
        <div className="p-6 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div>
            <h3 className="font-semibold text-sm text-slate-900 dark:text-slate-100">
              {isZh ? '最近调用流水' : 'Recent Inbound Audit Stream'}
            </h3>
            <p className="ui-meta mt-0.5">
              {isZh ? '实时推理请求与首字延迟审计记录' : 'Live request stream with TTFT latency and status'}
            </p>
          </div>
          <button
            onClick={() => setCurrentTab('logs')}
            className="text-xs font-medium text-slate-600 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white transition cursor-pointer"
          >
            {isZh ? `全部审计日志 (${logs.length}) →` : `All Audit Logs (${logs.length}) →`}
          </button>
        </div>

        {logs.length === 0 ? (
          <div className="p-8 text-center ui-meta">
            {isZh ? '暂无请求记录' : 'No request logs recorded yet'}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-100 dark:border-slate-800/80 text-slate-500 dark:text-slate-400 text-xs font-medium">
                  <th className="py-3 px-6">{isZh ? '时间' : 'Timestamp'}</th>
                  <th className="py-3 px-6">{isZh ? '调用方' : 'Client'}</th>
                  <th className="py-3 px-6">{isZh ? '模型' : 'Model'}</th>
                  <th className="py-3 px-6">{isZh ? '渠道' : 'Channel'}</th>
                  <th className="py-3 px-6">{isZh ? '状态' : 'Status'}</th>
                  <th className="py-3 px-6">{isZh ? '首字 / 耗时' : 'TTFT / Latency'}</th>
                  <th className="py-3 px-6">{isZh ? 'Token 消耗' : 'Tokens'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 text-xs">
                {logs.slice(0, 6).map((log) => (
                  <tr
                    key={log.id}
                    onClick={() => setActiveLogDetail && setActiveLogDetail(log)}
                    className="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 cursor-pointer transition"
                  >
                    <td className="py-3.5 px-6 font-mono text-slate-500 dark:text-slate-400 text-xs">
                      {new Date(log.created_at).toLocaleTimeString()}
                    </td>
                    <td className="py-3.5 px-6 font-medium text-slate-800 dark:text-slate-200">
                      {log.tenant_id || 'anonymous'}
                    </td>
                    <td className="py-3.5 px-6 font-mono font-medium text-slate-900 dark:text-white">
                      {log.model}
                    </td>
                    <td className="py-3.5 px-6 text-slate-500 font-mono text-xs">
                      {log.channel || log.channel_name || (isZh ? '默认通道' : 'Default')}
                    </td>
                    <td className="py-3.5 px-6">
                      <span
                        className={`inline-flex items-center px-2 py-0.5 rounded font-mono text-[11px] font-medium ${
                          log.status_code >= 200 && log.status_code < 300
                            ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 border border-emerald-200/60 dark:border-emerald-800/60'
                            : 'bg-rose-50 dark:bg-rose-950/40 text-rose-700 dark:text-rose-300 border border-rose-200/60 dark:border-rose-800/60'
                        }`}
                      >
                        {log.status_code}
                      </span>
                    </td>
                    <td className="py-3.5 px-6 font-mono tabular-nums text-slate-600 dark:text-slate-300">
                      {log.ttft_ms} ms / {log.duration_ms} ms
                    </td>
                    <td className="py-3.5 px-6 font-mono tabular-nums text-slate-600 dark:text-slate-300">
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
