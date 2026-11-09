import { useMemo } from 'react';

export default function useDashboardTraffic({
  chartMetric, isZh, keyStats, keys, liveStats, overviewLogs, overviewStats, recentLogs, selectedKey,
  statsKey, timeframe, timeseriesData
}) {
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
    return (keys || []).find(k => String(k.id) === selectedKey);
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
  return {
    activeKeyObj, activeRequests, activeStreams, currentRPM, currentTPM, errorLogs, liveDetail, livePhase, logs,
    maxMetricVal, peakRPM, peakTPM, slaAvailability, stats, timeBuckets, totalErrorRequests, totalRequests
  };
}
