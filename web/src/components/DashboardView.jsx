import { useState, useMemo, useEffect, useCallback, useRef } from 'react';
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
  Flame,
  Gauge,
  X,
  Sliders
} from 'lucide-react';
import { copyToClipboard } from '../config';
import { apiFetch } from '../api/client';

// The live endpoint is in-memory and cheap; 2s keeps the badge honest during a
// stream without hammering the gateway.
const LIVE_POLL_MS = 2000;
// The chart and the audit stream are aggregates, so they move far slower than
// the speedometers and are refreshed on their own, slower cadence.
const DASHBOARD_REFRESH_MS = 15000;

export default function DashboardView({
  stats: overviewStats = {},
  channels = [],
  logs: overviewLogs = [],
  keys = [],
  channelLatencies = {},
  testingId,
  batchTesting,
  handleBatchTest,
  handleTestChannel,
  fetchData,
  fetchLogs,
  showToast,
  setCurrentTab,
  onNavigateToErrors,
  setShowChannelModal,
  setActiveLogDetail,
  adminUser,
  onOpenTopology,
  adminFetch,
  selectedKey: propSelectedKey,
  onSelectKey,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [chartMetric, setChartMetric] = useState('requests'); // 'requests' | 'tokens' | 'latency' | 'errors' | 'rpm' | 'tpm'
  const [timeframe, setTimeframe] = useState('24h'); // '1h' | '24h' | '7d'
  const [hoveredBucket, setHoveredBucket] = useState(null);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefreshed, setLastRefreshed] = useState(null);
  const [timeseriesData, setTimeseriesData] = useState(null);
  const [selectedKey, setSelectedKey] = useState(propSelectedKey || '');
  const [keyStats, setKeyStats] = useState(null);
  const [dismissedAttentions, setDismissedAttentions] = useState(() => {
    try {
      return JSON.parse(localStorage.getItem('airoute_dismissed_attention') || '[]');
    } catch {
      return [];
    }
  });

  const handleDismissAttention = (itemKey) => {
    setDismissedAttentions(prev => {
      const next = [...new Set([...prev, itemKey])];
      try { localStorage.setItem('airoute_dismissed_attention', JSON.stringify(next)); } catch {}
      return next;
    });
  };

  const [recentLogs, setRecentLogs] = useState([]);
  const [recentLogsLoading, setRecentLogsLoading] = useState(false);
  const [liveStats, setLiveStats] = useState(null);

  const dashboardRequest = useRef(0);
  const [statsKey, setStatsKey] = useState(null);
  const fetchDashboard = useCallback(async () => {
    const request = ++dashboardRequest.current;
    const fetcher = adminFetch || apiFetch;
    const suffix = selectedKey ? `&key=${encodeURIComponent(selectedKey)}` : '';
    const read = async (url) => {
      const res = await fetcher(url);
      const json = await res.json();
      if (!res.ok || json.code !== 0) throw new Error(json.error || 'Dashboard request failed');
      return json.data;
    };
    setRecentLogsLoading(true);
    const results = await Promise.allSettled([
      read(`/api/v1/admin/logs?limit=10${suffix}`),
      read(`/api/v1/admin/stats/timeseries?timeframe=${timeframe}${suffix}`),
      selectedKey ? read(`/api/v1/admin/stats/overview?key=${encodeURIComponent(selectedKey)}`) : Promise.resolve(null),
    ]);
    if (request !== dashboardRequest.current) return false;
    setRecentLogs(results[0].status === 'fulfilled' && Array.isArray(results[0].value) ? results[0].value : []);
    setTimeseriesData(results[1].status === 'fulfilled' ? results[1].value : null);
    setKeyStats(results[2].status === 'fulfilled' ? results[2].value : null);
    setStatsKey(selectedKey);
    setRecentLogsLoading(false);
    return results.every(r => r.status === 'fulfilled');
  }, [selectedKey, timeframe, adminFetch]);

  const handleSelectKeyDrilldown = (key) => {
    setSelectedKey(key);
    onSelectKey?.(key);
  };
  useEffect(() => {
    if (propSelectedKey !== undefined) setSelectedKey(propSelectedKey || '');
  }, [propSelectedKey]);
  useEffect(() => {
    const requestRef = dashboardRequest;
    setKeyStats(null);
    setTimeseriesData(null);
    setRecentLogs([]);
    fetchDashboard();
    const timer = setInterval(() => {
      if (!document.hidden) fetchDashboard();
    }, DASHBOARD_REFRESH_MS);
    return () => { ++requestRef.current; clearInterval(timer); };
  }, [fetchDashboard]);

  // Real-time gateway telemetry (in-flight requests and streams).
  //
  // The 60s window only sees completed requests, so during a long stream the
  // speedometers read zero while the gateway is in fact busy. The live endpoint
  // is purely in-memory and reports what is happening right now. It is optional:
  // when it is missing (older gateway) or fails, the dashboard silently keeps
  // its previous 60s-window behaviour — no toast, no error state, no blocking of
  // the first paint.
  useEffect(() => {
    let mounted = true;
    let timer = null;
    let inFlight = false;
    const fetcher = adminFetch || apiFetch;

    const load = async () => {
      // Never stack requests: on a slow gateway the 2s tick would otherwise pile
      // up and resolve out of order.
      if (inFlight) return;
      inFlight = true;
      try {
        const res = await fetcher('/api/v1/admin/stats/live');
        const json = res && res.ok ? await res.json() : null;
        if (!mounted) return;
        const data = json && json.code === 0 ? json.data : null;
        setLiveStats(data && typeof data === 'object' && !Array.isArray(data) ? data : null);
      } catch {
        // Endpoint absent or gateway unreachable: degrade to the 60s window.
        if (mounted) setLiveStats(null);
      } finally {
        inFlight = false;
      }
    };

    const stop = () => {
      if (timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    };
    const start = () => {
      if (timer === null) timer = setInterval(load, LIVE_POLL_MS);
    };
    // A hidden tab has nobody watching it; polling there only burns requests.
    // Coming back visible catches up immediately instead of waiting a tick.
    const onVisibilityChange = () => {
      if (document.hidden) {
        stop();
        return;
      }
      start();
      load();
    };

    load();
    if (!document.hidden) start();
    document.addEventListener('visibilitychange', onVisibilityChange);

    return () => {
      mounted = false;
      stop();
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [adminFetch]);

  const stats = useMemo(() => selectedKey ? (statsKey === selectedKey ? keyStats || {} : {}) : overviewStats, [selectedKey, statsKey, keyStats, overviewStats]);
  const logs = useMemo(() => selectedKey ? (statsKey === selectedKey ? recentLogs : []) : overviewLogs, [selectedKey, statsKey, recentLogs, overviewLogs]);
  const currentRPM = stats.current_rpm ?? timeseriesData?.current_rpm ?? 0;
  const currentTPM = stats.current_tpm ?? timeseriesData?.current_tpm ?? 0;
  const peakRPM = timeseriesData?.peak_rpm ?? stats.peak_rpm ?? 0;
  const peakTPM = timeseriesData?.peak_tpm ?? stats.peak_tpm ?? 0;

  // Live gateway state, or null when the endpoint is unavailable. `livePhase`
  // collapses it into the four states the header badge and the TPM card speak:
  // streaming (tokens arriving), busy (request in flight, nothing counted yet),
  // live (traffic in the last 60s), idle (nothing).
  const hasLiveStats = Boolean(liveStats);
  const activeStreams = hasLiveStats ? Math.max(0, Math.round(Number(liveStats.active_streams) || 0)) : 0;
  const activeRequests = hasLiveStats ? Math.max(0, Math.round(Number(liveStats.active_requests) || 0)) : 0;
  const liveTokensLast60s = hasLiveStats ? Number(liveStats.tokens_last_60s) || 0 : 0;
  const liveTokensPerSec10s = hasLiveStats ? Number(liveStats.tokens_per_sec_10s) || 0 : 0;
  // -1 means "no token has ever been observed" and must stay distinct from a
  // genuine 0, otherwise a fresh gateway claims a request finished a second ago.
  const rawIdleSeconds = hasLiveStats ? Number(liveStats.idle_seconds) : Number.NaN;
  const liveIdleSeconds = Number.isFinite(rawIdleSeconds) ? Math.round(rawIdleSeconds) : -1;
  // A key drill-down is scoped to one key while these counters are platform-wide,
  // so the live layer is muted there rather than reporting the wrong scope.
  const livePhase = !hasLiveStats || selectedKey
    ? null
    : activeStreams > 0
      ? 'streaming'
      : activeRequests > 0
        ? 'busy'
        : liveTokensLast60s > 0
          ? 'live'
          : 'idle';

  // One string for the TPM card's secondary line, so the JSX stays readable.
  const liveDetail = (() => {
    if (livePhase === 'streaming') {
      const tps = Math.round(liveTokensPerSec10s);
      return isZh ? `当前 ${tps} tok/s · ${activeStreams} 个生成中` : `Current ${tps} tok/s · ${activeStreams} streaming`;
    }
    if (livePhase === 'busy') {
      return isZh ? `处理中 ${activeRequests} 个请求` : `${activeRequests} in flight`;
    }
    if (livePhase === 'idle') {
      if (liveIdleSeconds < 0) return isZh ? '空闲 · 尚无请求' : 'Idle · no requests yet';
      if (liveIdleSeconds > 3600) return isZh ? '空闲 · 最近请求超过 1 小时' : 'Idle · last request over 1h ago';
      return isZh ? `空闲 · 最近请求 ${liveIdleSeconds}s 前` : `Idle · last request ${liveIdleSeconds}s ago`;
    }
    return null;
  })();

  const activeKeyObj = useMemo(() => {
    return (keys || []).find(k => k.key === selectedKey);
  }, [keys, selectedKey]);

  // 1. DYNAMIC HOURLY TIME-SERIES AGGREGATION FROM SERVER OR LOGS FALLBACK
  const timeBuckets = useMemo(() => {
    if (timeseriesData && Array.isArray(timeseriesData.buckets) && timeseriesData.buckets.length > 0 && timeseriesData.timeframe === timeframe) {
      return timeseriesData.buckets.map(b => ({
        ...b,
        avgTtft: typeof b.avgTtft === 'number' ? b.avgTtft : 0,
        rpm: typeof b.rpm === 'number' ? b.rpm : 0,
        tpm: typeof b.tpm === 'number' ? b.tpm : 0
      }));
    }

    const numBuckets = timeframe === '1h' ? 12 : timeframe === '7d' ? 14 : 24;
    const now = Date.now();
    const intervalMs = timeframe === '1h' ? 5 * 60 * 1000 : timeframe === '7d' ? 12 * 3600 * 1000 : 3600 * 1000;
    
    const buckets = Array.from({ length: numBuckets }, (_, i) => {
      const bucketTime = new Date(now - (numBuckets - 1 - i) * intervalMs);
      const label = timeframe === '1h' 
        ? bucketTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        : timeframe === '7d'
        ? `${bucketTime.getMonth() + 1}/${bucketTime.getDate()} ${String(bucketTime.getHours()).padStart(2, '0')}:00`
        : `${bucketTime.getHours()}:00`;
      return {
        label,
        timestamp: bucketTime.getTime(),
        requests: 0,
        tokens: 0,
        errors: 0,
        ttfts: [],
        costs: 0,
        rpm: 0,
        tpm: 0
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
        if ((l.status_code >= 400 || (l.status_code ?? 200) < 200 || l.error_message)) {
          buckets[index].errors += 1;
        }
        if (l.ttft_ms && l.ttft_ms > 0) {
          buckets[index].ttfts.push(l.ttft_ms);
        }
      }
    });

    const mins = Math.max(1, intervalMs / (60 * 1000));
    return buckets.map(b => ({
      ...b,
      avgTtft: b.ttfts.length ? Math.round(b.ttfts.reduce((a, c) => a + c, 0) / b.ttfts.length) : 0,
      rpm: Math.round(b.requests / mins),
      tpm: Math.round(b.tokens / mins)
    }));
  }, [timeseriesData, logs, timeframe]);

  // Max value for scaling SVG chart bars/lines
  const maxMetricVal = useMemo(() => {
    let max = 1;
    (timeBuckets || []).forEach(b => {
      if (!b) return;
      if (chartMetric === 'requests' && b.requests > max) max = b.requests;
      if (chartMetric === 'tokens' && b.tokens > max) max = b.tokens;
      if (chartMetric === 'rpm' && (b.rpm || 0) > max) max = b.rpm;
      if (chartMetric === 'tpm' && (b.tpm || 0) > max) max = b.tpm;
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
  const errorLogs = useMemo(() => (logs || []).filter(l => l && (l.status_code >= 400 || (l.status_code ?? 200) < 200 || l.error_message)), [logs]);
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
    if (timeseriesData && timeseriesData.modality_stats && timeseriesData.timeframe === timeframe) {
      const counts = timeseriesData.modality_stats;
      const total = Math.max(1, (counts.chat || 0) + (counts.images || 0) + (counts.audio || 0) + (counts.video || 0) + (counts.embeddings || 0) + (counts.rerank || 0));
      return [
        { key: 'chat', label: isZh ? '文本对话' : 'Chat', count: counts.chat || 0, pct: Math.round(((counts.chat || 0) / total) * 100), color: 'bg-indigo-500' },
        { key: 'images', label: isZh ? '图像生成' : 'Images', count: counts.images || 0, pct: Math.round(((counts.images || 0) / total) * 100), color: 'bg-pink-500' },
        { key: 'audio', label: isZh ? '语音合成/识别' : 'Audio/TTS', count: counts.audio || 0, pct: Math.round(((counts.audio || 0) / total) * 100), color: 'bg-amber-500' },
        { key: 'video', label: isZh ? '视频生成' : 'Video', count: counts.video || 0, pct: Math.round(((counts.video || 0) / total) * 100), color: 'bg-purple-500' },
        { key: 'embeddings', label: isZh ? '向量嵌入' : 'Embeddings', count: counts.embeddings || 0, pct: Math.round(((counts.embeddings || 0) / total) * 100), color: 'bg-emerald-500' },
        { key: 'rerank', label: isZh ? '语义重排' : 'Rerank', count: counts.rerank || 0, pct: Math.round(((counts.rerank || 0) / total) * 100), color: 'bg-cyan-500' },
      ].filter(item => item.count > 0 || item.key === 'chat');
    }

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
  }, [timeseriesData, timeframe, logs, isZh]);

  // 4. TOKEN ECONOMICS & PROMPT CACHE FINOPS
  const tokenMetrics = useMemo(() => {
    let promptTokens = stats.prompt_tokens || 0;
    let completionTokens = stats.completion_tokens || 0;
    if (promptTokens === 0 && completionTokens === 0) {
      (logs || []).forEach(l => {
        if (!l) return;
        promptTokens += l.prompt_tokens || 0;
        completionTokens += l.completion_tokens || 0;
      });
    }
    const total = promptTokens + completionTokens || stats.total_tokens || 1;
    const rawPromptPct = (promptTokens / total) * 100;
    const rawCompPct = (completionTokens / total) * 100;

    let promptPctStr = '';
    let compPctStr = '';
    if (completionTokens > 0 && rawCompPct < 1) {
      compPctStr = rawCompPct < 0.1 ? '<0.1%' : `${rawCompPct.toFixed(1)}%`;
      promptPctStr = rawCompPct < 0.1 ? '>99.9%' : `${(100 - parseFloat(rawCompPct.toFixed(1))).toFixed(1)}%`;
    } else if (promptTokens > 0 && rawPromptPct < 1) {
      promptPctStr = rawPromptPct < 0.1 ? '<0.1%' : `${rawPromptPct.toFixed(1)}%`;
      compPctStr = rawPromptPct < 0.1 ? '>99.9%' : `${(100 - parseFloat(rawPromptPct.toFixed(1))).toFixed(1)}%`;
    } else {
      const roundedComp = Math.round(rawCompPct);
      const roundedPrompt = 100 - roundedComp;
      promptPctStr = `${roundedPrompt}%`;
      compPctStr = `${roundedComp}%`;
    }

    return {
      promptTokens,
      completionTokens,
      promptPct: Math.round(rawPromptPct),
      compPct: Math.round(rawCompPct),
      promptPctStr,
      compPctStr,
    };
  }, [logs, stats.prompt_tokens, stats.completion_tokens, stats.total_tokens]);

  // 5. CALCULATE TRUE PERCENTILES (P50, P90, P99)
  // Renders a percentile value, or an explicit "no data" marker when there are
  // no latency samples to compute it from.
  const fmtPct = (v) => (typeof v === 'number' && v > 0 ? `${v}ms` : (isZh ? '无样本' : 'n/a'));

  const percentiles = useMemo(() => {
    if (timeseriesData && timeseriesData.percentiles && timeseriesData.percentiles.sampled > 0 && timeseriesData.timeframe === timeframe) {
      return timeseriesData.percentiles;
    }

    const latencies = (logs || []).map(l => l?.ttft_ms).filter(v => typeof v === 'number' && v > 0).sort((a, b) => a - b);
    if (!latencies.length) {
      return { p50: null, p90: null, p99: null, sampled: 0, total: stats.ttft_samples || 0 };
    }
    const at = (q) => {
      const idx = Math.min(latencies.length - 1, Math.floor(latencies.length * q));
      return latencies[idx];
    };
    return { p50: at(0.5), p90: at(0.9), p99: at(0.99), sampled: latencies.length, total: stats.ttft_samples || latencies.length };
  }, [timeseriesData, timeframe, logs, stats.ttft_samples]);

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
        : chartMetric === 'rpm'
        ? (b.rpm || 0)
        : chartMetric === 'tpm'
        ? (b.tpm || 0)
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
    // A 99.9% target allows 0.1% of requests to fail. The old expression also
    // multiplied by 20, so the budget bar read "healthy" at twenty times the
    // error rate its own label promised.
    const allowedErrorRate = 1 - targetSla / 100;
    const total = totalRequests > 0 ? totalRequests : Math.max(1, (logs || []).length);
    const allowedErrors = Math.max(1, Math.ceil(total * allowedErrorRate));
    const actualErrors = totalRequests > 0 ? totalErrorRequests : errorLogs.length;
    // No traffic means the SLA is unknown, not perfect. parseFloat('—') is NaN, and
    // `NaN || 100` used to report a flawless error budget for a gateway that had
    // served nothing at all.
    const parsedSla = parseFloat(slaAvailability);
    const slaKnown = Number.isFinite(parsedSla);
    const remainingBudgetPct = !slaKnown
      ? null
      : actualErrors === 0
        ? 100
        : Math.max(0, Math.min(100, Math.round(((allowedErrors - actualErrors) / allowedErrors) * 100)));
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
        if ((l.status_code >= 400 || (l.status_code ?? 200) < 200 || l.error_message)) providerMap[ch.id].errors += 1;
        if (l.ttft_ms) providerMap[ch.id].ttfts.push(l.ttft_ms);
      }
    });
    return Object.values(providerMap).map(p => {
      // Unknown is not "fast" or "perfect": a channel with no samples used to be
      // reported as 160 ms with a 100.0% SLA and could outrank measured providers.
      const avgTtft = p.ttfts.length
        ? Math.round(p.ttfts.reduce((a, b) => a + b, 0) / p.ttfts.length)
        : (typeof p.latencyMs === 'number' && p.latencyMs > 0 ? p.latencyMs : null);
      const successRate = p.totalCalls > 0
        ? Number((((p.totalCalls - p.errors) / p.totalCalls) * 100).toFixed(1))
        : null;
      return {
        ...p,
        avgTtft,
        // A probe-only latency is not a success rate either: keep it unknown
        // unless there is at least one logged call to judge.
        successRate
      };
    }).sort((a, b) => {
      // Measured providers first (fastest first); unknowns last.
      if (a.avgTtft === null && b.avgTtft === null) return 0;
      if (a.avgTtft === null) return 1;
      if (b.avgTtft === null) return -1;
      return a.avgTtft - b.avgTtft;
    });
  }, [channels, logs, channelLatencies]);

  const [copiedReport, setCopiedReport] = useState(false);

  const handleExportExecutiveReport = async () => {
    const totalCalls = stats.total_requests || logs.length;
    // No sample means unknown, not a made-up 180 ms in a report an operator may
    // paste into a ticket or send to a customer.
    const avgTtftValue = typeof stats.avg_ttft_ms === 'number' && stats.avg_ttft_ms > 0
      ? stats.avg_ttft_ms
      : null;
    const avgTtftText = avgTtftValue === null
      ? (isZh ? '无样本' : 'n/a')
      : `${avgTtftValue}ms`;
    const report = isZh
      ? `# 📊 Airoute AI 网关集群运营与 SLA 遥测报告

**生成时间**：${new Date().toLocaleString()}
**高可用集群架构**：双活双节点负载均衡 (Active-Active HA)
**整体 SLA 可用性**：${slaAvailability}${slaAvailability === '—' ? '' : '%'} (目标: 99.9%, 基于全部 ${totalRequests} 次请求)

---

## 1. 核心关键指标 (KPIs)
- **累计调用量**：${totalCalls.toLocaleString()} 次
- **Token 总吞吐**：${(stats.total_tokens || 0).toLocaleString()} Tokens (输入占比 ${tokenMetrics.promptPctStr}, 输出占比 ${tokenMetrics.compPctStr})
- **首字时延 SLA (TTFT)**：平均 ${avgTtftText} (P50: ${fmtPct(percentiles.p50)}, P90: ${fmtPct(percentiles.p90)}, P99: ${fmtPct(percentiles.p99)}; 样本 ${percentiles.sampled}/${percentiles.total})
- **接入上游通道**：${channels.length} 个服务商 (活跃: ${channels.filter(c => c.status === 'active').length} 个, 熔断: ${channels.filter(c => ['OPEN', 'HALF-OPEN'].includes(String(c.breaker_status || '').toUpperCase())).length} 个)

## 2. FinOps 成本优化与缓存成效
- **Prompt Caching 命中**：累计命中识别 ${logs.filter(l => l.cached_tokens > 0).length} 次
- **闲时优惠覆盖**：夜间/周末闲时请求享受时段费率折扣，大幅削减峰值推理成本

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
- **Token Throughput**: ${(stats.total_tokens || 0).toLocaleString()} Tokens (Prompt: ${tokenMetrics.promptPctStr}, Completion: ${tokenMetrics.compPctStr})
- **TTFT SLA Latency**: Avg ${avgTtftText} (P50: ${fmtPct(percentiles.p50)}, P90: ${fmtPct(percentiles.p90)}, P99: ${fmtPct(percentiles.p99)}; ${percentiles.sampled}/${percentiles.total} samples)
- **Upstream Channels**: ${channels.length} providers (${channels.filter(c => c.status === 'active').length} active, ${channels.filter(c => ['OPEN', 'HALF-OPEN'].includes(String(c.breaker_status || '').toUpperCase())).length} tripped)

## 2. FinOps Cost Optimization
- **Prompt Cache Hits**: ${logs.filter(l => l.cached_tokens > 0).length} requests accelerated
- **Off-Peak Economics**: Time-of-use rate optimization during off-peak windows

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
      const [a, b, c] = await Promise.all([fetchData?.(), fetchLogs?.(), fetchDashboard()]);
      const ok = a !== false && b !== false && c !== false;
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
      onAction: () => onNavigateToErrors ? onNavigateToErrors() : setCurrentTab?.('logs')
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
      detail: isZh ? '统计覆盖全部调用日志，点击查看全部异常记录。' : 'Counts cover all invocation logs. View all failed requests.',
      actionLabel: isZh ? '查看日志' : 'View logs',
      onAction: () => onNavigateToErrors ? onNavigateToErrors() : setCurrentTab?.('logs')
    });
  }

  const attentionTones = {
    danger: {
      card: 'border-rose-200/80 dark:border-rose-900/60 border-l-rose-500 bg-rose-50/50 dark:bg-rose-950/20',
      icon: 'text-rose-500 dark:text-rose-400',
      title: 'text-rose-900 dark:text-rose-100'
    },
    warn: {
      card: 'border-amber-200/80 dark:border-amber-900/60 border-l-amber-500 bg-amber-50/50 dark:bg-amber-950/20',
      icon: 'text-amber-500 dark:text-amber-400',
      title: 'text-amber-900 dark:text-amber-100'
    }
  };

  return (
    <div className="space-y-6 animate-in fade-in duration-200">
      {/* 1. Page Header (Directly on canvas, no box-in-box card) */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="ui-page-title">
            {isZh ? '运营总览' : 'Dashboard'}
          </h2>
          <p className="ui-page-subtitle mt-1">
            {isZh 
              ? '并发、首字延迟、模态分布与缓存节省' 
              : 'Concurrency, TTFT, modality mix and cache savings'}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {onOpenTopology && (
            <button
              onClick={onOpenTopology}
              className="ui-btn-secondary"
              title={isZh ? '打开全链路动态路由与容灾演练' : 'Open visual routing & failover topology'}
            >
              <Layers className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
              <span>{isZh ? '运行时容灾拓扑' : 'Live Routing Topology'}</span>
            </button>
          )}

          {/* The executive report was written but never reachable: no button called
              it, so the generator and its copied-state indicator were both dead. */}
          <button
            onClick={handleExportExecutiveReport}
            className="ui-btn-secondary"
            title={isZh ? '复制一份包含 KPI、SLA 与建议的 Markdown 运营报告' : 'Copy a Markdown operations report with KPIs, SLA and recommendations'}
          >
            <FileText className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0]" />
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
              <RefreshCw className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${refreshing ? 'animate-spin' : ''}`} />
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
        <div className="flex items-center justify-between">
          <h3 className="ui-section-title">{isZh ? '需要关注' : 'Needs attention'}</h3>
        </div>

        {(() => {
          const visibleAttentionItems = attentionItems.filter(item => !dismissedAttentions.includes(item.key));
          if (visibleAttentionItems.length === 0) {
            return (
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
            );
          }

          return (
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3">
              {visibleAttentionItems.map(item => {
                const tone = attentionTones[item.tone];
                const ItemIcon = item.icon;
                return (
                  <div key={item.key} className={`rounded-xl border border-l-2 px-3.5 py-3 flex flex-col relative group ${tone.card}`}>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDismissAttention(item.key);
                      }}
                      className="absolute top-2 right-2 p-1 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 rounded-md transition opacity-70 group-hover:opacity-100 cursor-pointer"
                      title={isZh ? '忽略此项关注' : 'Dismiss notice'}
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                    <div className="flex items-start gap-2 pr-5">
                      <ItemIcon className={`w-4 h-4 shrink-0 mt-0.5 ${tone.icon}`} />
                      <span className={`text-xs font-semibold leading-5 ${tone.title}`}>{item.title}</span>
                    </div>
                    <p className="ui-meta mt-1 flex-1">{item.detail}</p>
                    {item.actionLabel && (
                      <button
                        onClick={() => {
                          handleDismissAttention(item.key);
                          item.onAction?.();
                        }}
                        className="ui-btn-ghost ui-btn-sm -ml-2.5 mt-1 self-start text-indigo-600 dark:text-indigo-400 hover:text-indigo-700 dark:hover:text-indigo-300"
                      >
                        {item.actionLabel} →
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          );
        })()}
      </section>

      {/* 2.5 REAL-TIME TELEMETRY COCKPIT & KEY DRILLDOWN */}
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
                    ? (isZh ? `密钥实时速率遥测 · ${activeKeyObj?.tenant_id || activeKeyObj?.name || selectedKey.slice(0, 16)}` : `Key Telemetry · ${activeKeyObj?.tenant_id || activeKeyObj?.name || selectedKey.slice(0, 16)}`)
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
              >
                <option value="">
                  {adminUser?.role === 'admin' 
                    ? (isZh ? '🌐 全平台聚合遥测 (Platform Overall)' : '🌐 All Platform Telemetry')
                    : (isZh ? '👤 我的全量密钥聚合 (My Keys Overall)' : '👤 All My Keys Telemetry')}
                </option>
                {(keys || []).map(k => (
                  <option key={k.id || k.key} value={k.key}>
                    🔑 {k.tenant_id || k.name || '—'} · {k.key_fingerprint || (isZh ? '无指纹' : 'no fingerprint')} {k.rpm ? `(${k.rpm} RPM)` : ''}
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

      {/* 3. CORE TELEMETRY KPIS (4 CARDS) */}
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

      {/* 4. INTERACTIVE MULTI-METRIC TIME-SERIES VISUALIZER */}
      <div className="ui-card p-6 space-y-6">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
          <div>
            <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] tracking-tight">
              {isZh ? '多维时序走势与分位监测' : 'Time-Series Telemetry & SLA Waveform'}
            </h4>
            <p className="ui-meta mt-0.5">
              {isZh ? '吞吐与首字时延趋势' : 'Throughput and TTFT trends'}
            </p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {/* Metric Selector Tabs */}
            <div className="flex items-center bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg text-xs font-medium">
              <button
                onClick={() => setChartMetric('requests')}
                className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'requests' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? '请求量 (QPS)' : 'Requests'}
              </button>
              <button
                onClick={() => setChartMetric('tokens')}
                className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'tokens' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? 'Token 吞吐' : 'Tokens'}
              </button>
              <button
                onClick={() => setChartMetric('rpm')}
                className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'rpm' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? '实时 RPM' : 'RPM'}
              </button>
              <button
                onClick={() => setChartMetric('tpm')}
                className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'tpm' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? '实时 TPM' : 'TPM'}
              </button>
              <button
                onClick={() => setChartMetric('latency')}
                className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'latency' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? '首字延迟 TTFT' : 'TTFT Latency'}
              </button>
              <button
                onClick={() => setChartMetric('errors')}
                className={`px-3 py-1 rounded-md transition-colors ${chartMetric === 'errors' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'}`}
              >
                {isZh ? '异常错误' : 'Errors'}
              </button>
            </div>

            {/* Timeframe Selector */}
            <div className="flex items-center bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg text-xs font-medium">
              <button
                onClick={() => setTimeframe('1h')}
                className={`px-2.5 py-1 rounded-md transition-colors ${timeframe === '1h' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}
              >
                1h
              </button>
              <button
                onClick={() => setTimeframe('24h')}
                className={`px-2.5 py-1 rounded-md transition-colors ${timeframe === '24h' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}
              >
                24h
              </button>
              <button
                onClick={() => setTimeframe('7d')}
                className={`px-2.5 py-1 rounded-md transition-colors ${timeframe === '7d' ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-2xs font-semibold' : 'text-[#656B75] dark:text-[#A1A7B0]'}`}
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
            <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
            <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
            <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
            <div className="w-full border-b border-[#E4E6E9]/60 dark:border-[#2C3038]/60"></div>
          </div>

          {/* Continuous SVG Trend Area Wave Overlay */}
          <div className="absolute inset-0 px-2 pt-4 pb-8 pointer-events-none">
            <svg viewBox="0 0 1000 140" preserveAspectRatio="none" className="w-full h-full overflow-visible">
              <defs>
                <linearGradient id="telemetryGradient" x1="0%" y1="0%" x2="0%" y2="100%">
                  <stop offset="0%" stopColor="#355CFF" stopOpacity="0.16" />
                  <stop offset="100%" stopColor="#355CFF" stopOpacity="0.0" />
                </linearGradient>
              </defs>
              {chartSvgData.areaD && (
                <path d={chartSvgData.areaD} fill="url(#telemetryGradient)" />
              )}
              {chartSvgData.pointsStr && (
                <polyline
                  points={chartSvgData.pointsStr}
                  fill="none"
                  stroke="#355CFF"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              )}
              {chartSvgData.pointsArr.map((pt, i) => {
                const isHovered = hoveredBucket?.timestamp != null
                  ? hoveredBucket.timestamp === pt.bucket.timestamp
                  : hoveredBucket?.label === pt.bucket.label;
                return (
                  <circle
                    key={i}
                    cx={pt.x}
                    cy={pt.y}
                    r={isHovered ? "4.5" : "2"}
                    fill={isHovered ? "#ffffff" : "#355CFF"}
                    stroke="#355CFF"
                    strokeWidth="2"
                    className="transition-all duration-150"
                  />
                );
              })}
            </svg>
          </div>

          {/* Hover Tooltip Overlay */}
          {hoveredBucket && (
            <div className="absolute top-2 right-2 bg-[#191B1F] text-[#F1F2F4] text-xs px-3 py-1.5 rounded-lg border border-[#2C3038] shadow-lg pointer-events-none z-10 flex items-center gap-3">
              <span className="font-mono text-[#A1A7B0]">{hoveredBucket.label}</span>
              <span className="font-semibold text-[#91A7FF] font-mono">
                {isZh ? '请求' : 'Reqs'}: {hoveredBucket.requests} ({((hoveredBucket.rpm ?? 0) / 60).toFixed(1)} req/s · {hoveredBucket.rpm ?? 0} RPM)
              </span>
              <span className="font-semibold text-sky-400 font-mono">
                Tokens: {hoveredBucket.tokens.toLocaleString()} ({hoveredBucket.tpm ?? 0} TPM)
              </span>
              <span className="font-semibold text-amber-400 font-mono">
                TTFT: {hoveredBucket.avgTtft > 0 ? `${hoveredBucket.avgTtft}ms` : (isZh ? '无样本' : 'n/a')}
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
                : chartMetric === 'rpm'
                ? (bucket.rpm || 0)
                : chartMetric === 'tpm'
                ? (bucket.tpm || 0)
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
                    className="w-full max-w-[18px] rounded-t-sm transition-all duration-150 bg-[#E4E6E9] dark:bg-[#2C3038] group-hover:bg-[#355CFF]"
                  />
                </div>
              );
            })}
          </div>

          {/* X-Axis Timeline Labels */}
          <div className="flex justify-between ui-meta font-mono pt-3 border-t border-[#E4E6E9] dark:border-[#2C3038] z-1">
            <span>{timeBuckets[0]?.label}</span>
            <span>{timeBuckets[Math.floor(timeBuckets.length / 2)]?.label}</span>
            <span>{isZh ? '当前 (实时)' : 'Live Now'}</span>
          </div>
        </div>

        {/* Latency Percentiles & SLA Quick Stat Bar */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 pt-1">
          <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="ui-meta font-medium block">{isZh ? 'P50 中位时延' : 'P50 Median TTFT'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">
                {fmtPct(percentiles.p50)}
              </span>
              <span className="ui-meta font-normal">(&lt; 200ms)</span>
            </div>
          </div>
          <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="ui-meta font-medium block">{isZh ? 'P90 企业基准' : 'P90 Baseline'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">
                {fmtPct(percentiles.p90)}
              </span>
              <span className="ui-meta font-normal">(&lt; 500ms)</span>
            </div>
          </div>
          <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
            <span className="ui-meta font-medium block">{isZh ? 'P99 极端长尾' : 'P99 Tail TTFT'}</span>
            <div className="flex items-baseline gap-1.5 mt-1">
              <span className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums">
                {fmtPct(percentiles.p99)}
              </span>
              <span className="ui-meta font-normal">(&lt; 1000ms)</span>
            </div>
          </div>
          <div className="p-4 rounded-xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038]">
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

      {/* 6. UPSTREAM PROVIDERS HEALTH & CIRCUIT BREAKER MATRIX */}
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

      {/* 7. RECENT TRAFFIC AUDIT STREAM */}
      <div className="ui-card overflow-hidden">
        <div className="p-6 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h3 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">
                {isZh ? '最近调用流水' : 'Recent Inbound Audit Stream'}
              </h3>
              <span className="text-[11px] font-mono px-1.5 py-0.5 rounded bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]">
                TOP 10
              </span>
            </div>
            <p className="ui-meta mt-0.5">
              {selectedKey
                ? (isZh ? '所选密钥最近调用，不受筛选影响' : 'Latest calls for selected key, ignoring filters')
                : (isZh ? '最近调用流水，按时间倒序' : 'Latest requests, newest first')}
            </p>
          </div>
          <button
            onClick={() => setCurrentTab('logs')}
            className="text-xs font-medium text-[#355CFF] dark:text-[#91A7FF] hover:underline transition-colors cursor-pointer"
          >
            {isZh ? '查看全部调用日志 →' : 'All Audit Logs →'}
          </button>
        </div>

        {(() => {
          const displayLogs = recentLogs.length > 0 ? recentLogs : (logs || []).slice(0, 10);
          if (recentLogsLoading && recentLogs.length === 0) {
            return (
              <div className="p-8 text-center ui-meta flex items-center justify-center gap-2">
                <RefreshCw className="w-4 h-4 animate-spin text-[#656B75] dark:text-[#A1A7B0]" />
                <span>{isZh ? '正在加载最新流水...' : 'Loading recent stream...'}</span>
              </div>
            );
          }
          if (displayLogs.length === 0) {
            return (
              <div className="p-8 text-center ui-meta">
                {isZh ? '暂无请求记录' : 'No request logs recorded yet'}
              </div>
            );
          }
          return (
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr>
                    <th className="ui-th">{isZh ? '时间' : 'Timestamp'}</th>
                    <th className="ui-th">{isZh ? '调用方' : 'Client'}</th>
                    <th className="ui-th">{isZh ? '模型' : 'Model'}</th>
                    <th className="ui-th">{isZh ? '渠道' : 'Channel'}</th>
                    <th className="ui-th">{isZh ? '状态' : 'Status'}</th>
                    <th className="ui-th text-right">{isZh ? '首字 / 耗时' : 'TTFT / Latency'}</th>
                    <th className="ui-th text-right">{isZh ? 'Token 消耗' : 'Tokens'}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60">
                  {displayLogs.map((log) => (
                    <tr
                      key={log.id}
                      onClick={() => setActiveLogDetail && setActiveLogDetail(log)}
                      className="ui-tr-hover cursor-pointer"
                    >
                      <td className="ui-td font-mono text-[#656B75] dark:text-[#A1A7B0] text-xs">
                        {new Date(log.created_at).toLocaleTimeString()}
                      </td>
                      <td className="ui-td font-medium text-[#202124] dark:text-[#F1F2F4]">
                        {log.tenant_id || (log.api_key ? (log.api_key.length > 12 ? `${log.api_key.slice(0, 8)}...` : log.api_key) : 'anonymous')}
                      </td>
                      <td className="ui-td font-mono font-medium text-[#202124] dark:text-[#F1F2F4]">
                        {log.model}
                      </td>
                      <td className="ui-td text-[#656B75] dark:text-[#A1A7B0] font-mono text-xs">
                        {log.channel || log.channel_name || (isZh ? '默认通道' : 'Default')}
                      </td>
                      <td className="ui-td">
                        <span
                          className={`inline-flex items-center px-2 py-0.5 rounded font-mono text-[11px] font-medium ${
                            log.status_code >= 200 && log.status_code < 300
                              ? 'bg-emerald-500/10 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20'
                              : 'bg-rose-500/10 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 border border-rose-500/20'
                          }`}
                        >
                          {log.status_code}
                        </span>
                      </td>
                      <td className="ui-td font-mono tabular-nums text-right text-[#202124] dark:text-[#F1F2F4]">
                        {log.ttft_ms > 0 ? `${log.ttft_ms}ms` : '—'} / {log.duration_ms ? `${log.duration_ms}ms` : '—'}
                      </td>
                      <td className="ui-td font-mono tabular-nums text-right text-[#202124] dark:text-[#F1F2F4]">
                        {(log.total_tokens || 0).toLocaleString()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          );
        })()}
      </div>
    </div>
  );
}
