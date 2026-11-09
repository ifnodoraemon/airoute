import { useMemo } from 'react';

export default function useDashboardSummary({
  channelLatencies, channels, chartMetric, errorLogs, isZh, logs, maxMetricVal, slaAvailability, stats,
  timeBuckets, timeframe, timeseriesData, totalErrorRequests, totalRequests
}) {
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
  return {
    attention, chartSvgData, errorBudget, errorCountByCode, fmtPct, modalityStats, percentiles,
    providerSlaStats, tokenMetrics, topModels
  };
}
