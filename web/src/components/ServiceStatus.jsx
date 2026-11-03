import { useState, useEffect, useCallback, useRef } from 'react';
import {
  CheckCircle2,
  AlertTriangle,
  XCircle,
  Activity,
  RefreshCw,
  Server,
  Layers,
  Search,
  Filter,
  Pause,
  Play,
  Copy,
  Check
} from 'lucide-react';
import { copyToClipboard as crossOriginCopy } from '../config';

// Public status polling cadence.
const AUTO_REFRESH_MS = 30000;

/**
 * Normalise a channel's circuit-breaker state.
 *
 * The gateway serialises two independent fields (controlplane/channel_view.go):
 *   status          -> 'active' | 'disabled'   (operator enablement)
 *   breaker_status  -> 'CLOSED' | 'OPEN' | 'HALF-OPEN' (routing health)
 * Only the second one says whether traffic is being routed around the channel, so
 * every health decision on this page derives from it — the same rule
 * ChannelsView.jsx (`channelHealth`) and DashboardView.jsx apply.
 *
 * @param {{breaker_status?: string}} channel
 * @returns {'open'|'half_open'|'closed'} 'closed' when no breaker has tripped.
 */
export function channelBreakerState(channel) {
  const breaker = String(channel?.breaker_status || '').toUpperCase();
  if (breaker === 'OPEN' || breaker === 'TRIPPED') return 'open';
  if (breaker === 'HALF-OPEN' || breaker === 'HALF_OPEN') return 'half_open';
  return 'closed';
}

export default function ServiceStatus({
  isStandalone = true,
  onBackHome,
  onEnterConsole,
  onOpenLogin,
  isLoggedIn = false,
  lang = 'zh',
  setLang,
  modelRoutes = [],
  channels = [],
  t,
  onRefresh,
  probeLatencies = {},
  showToast,
  onBatchProbe,
  batchTesting = false,
}) {
  const isZh = lang === 'zh';
  const [publicData, setPublicData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [lastUpdated, setLastUpdated] = useState(new Date());
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [countdown, setCountdown] = useState(30);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedModality, setSelectedModality] = useState('all');
  const [copiedModel, setCopiedModel] = useState('');

  const fetchPublicStatus = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch('/api/v1/public/status');
      if (res.ok) {
        const json = await res.json();
        if (json.code === 0 && json.data) {
          setPublicData(json.data);
          setLastUpdated(new Date());
        }
      }
    } catch (e) {
      console.warn('Failed to fetch public status:', e);
    } finally {
      setLoading(false);
    }
  }, []);

  // This page is public: an anonymous visitor must only ever reach public
  // endpoints. `onRefresh` is the console's tenant loader (it calls /admin/*),
  // which answered 401 four times per visitor every 30 seconds here, so the
  // console data is refreshed only for a signed-in session.
  const onRefreshRef = useRef(onRefresh);
  const isLoggedInRef = useRef(isLoggedIn);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
    isLoggedInRef.current = isLoggedIn;
  }, [onRefresh, isLoggedIn]);

  const nextAutoRefreshAtRef = useRef(Date.now() + AUTO_REFRESH_MS);

  const refreshAll = useCallback(() => {
    fetchPublicStatus();
    if (isLoggedInRef.current) onRefreshRef.current?.();
  }, [fetchPublicStatus]);

  useEffect(() => {
    refreshAll();
  }, [refreshAll]);

  // 30-second auto-refresh. The tick only computes the remaining time; the
  // requests run in the timer callback, never inside a state updater (a updater
  // may be replayed, which duplicated the requests).
  useEffect(() => {
    if (!autoRefresh) return undefined;
    const tick = () => {
      const remaining = Math.max(0, Math.ceil((nextAutoRefreshAtRef.current - Date.now()) / 1000));
      setCountdown(remaining);
      if (remaining > 0) return;
      nextAutoRefreshAtRef.current = Date.now() + AUTO_REFRESH_MS;
      refreshAll();
    };
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [autoRefresh, refreshAll]);

  const handleManualRefresh = () => {
    nextAutoRefreshAtRef.current = Date.now() + AUTO_REFRESH_MS;
    setCountdown(Math.ceil(AUTO_REFRESH_MS / 1000));
    refreshAll();
    if (showToast) showToast(isZh ? '服务状态已实时刷新' : 'Service status refreshed', 'info');
  };

  const copyToClipboard = async (text) => {
    await crossOriginCopy(text);
    setCopiedModel(text);
    if (showToast) showToast(isZh ? `已复制模型名称: ${text}` : `Copied model ID: ${text}`, 'success');
    setTimeout(() => setCopiedModel(''), 2000);
  };

  const getModalityFromModel = (modelName, existingModality) => {
    if (existingModality && existingModality !== 'chat') return existingModality;
    const lower = modelName.toLowerCase();
    if (lower.includes('rerank') || lower.includes('bge-reranker')) return 'rerank';
    if (lower.includes('embed') || lower.includes('bge-large') || lower.includes('bge-m3')) return 'embeddings';
    if (lower.includes('sora') || lower.includes('video') || lower.includes('seedance') || lower.includes('gen-3') || lower.includes('kling') || lower.includes('cogvideo') || lower.includes('视频')) return 'videos';
    if (lower.includes('tts') || lower.includes('whisper') || lower.includes('audio') || lower.includes('speech') || lower.includes('voice') || lower.includes('语音')) return 'audio';
    if (lower.includes('image') || lower.includes('dall-e') || lower.includes('flux') || lower.includes('midjourney') || lower.includes('sdxl') || lower.includes('图像')) return 'images';
    if (lower.includes('vision') || lower.includes('omni') || lower.includes('vl') || lower.includes('4o')) return 'vision';
    return 'chat';
  };

  // Determine display models: prefer modelRoutes if available, otherwise public models from /api/v1/public/status
  const rawModels = modelRoutes.length > 0
    ? modelRoutes.map(mr => {
        const activeCount = Array.isArray(mr.providers) ? mr.providers.filter(p => p.status === 'active').length : 0;
        return {
          model: mr.model,
          modality: getModalityFromModel(mr.model, mr.modality),
          status: activeCount > 0 ? 'operational' : 'degraded',
          fallback_model: mr.fallback_model,
          activeProviders: activeCount,
          totalProviders: Array.isArray(mr.providers) ? mr.providers.length : 0,
          providers: mr.providers || []
        };
      })
    : (publicData?.models && publicData.models.length > 0
        ? publicData.models.map(m => ({
            ...m,
            modality: getModalityFromModel(m.model, m.modality),
            providers: []
          }))
        : []);

  // Filter models based on search query and modality
  const displayModels = rawModels.filter(m => {
    const matchesSearch = !searchQuery.trim() || m.model.toLowerCase().includes(searchQuery.toLowerCase().trim());
    const matchesModality = selectedModality === 'all' || m.modality === selectedModality;
    return matchesSearch && matchesModality;
  });

  // Modality statistics
  const modalityCounts = rawModels.reduce((acc, m) => {
    acc[m.modality] = (acc[m.modality] || 0) + 1;
    return acc;
  }, {});

  // Check overall health.
  //
  // The circuit breaker lives in `breaker_status` (CLOSED / OPEN / HALF-OPEN), while
  // `status` is the channel's enablement (active / disabled) — the same two fields
  // ChannelsView and DashboardView read. Comparing `status` against 'tripped' could
  // never be true, so a breaker-open channel used to be reported as healthy here.
  const anyTripped = channels.some((c) => channelBreakerState(c) !== 'closed' || (probeLatencies[c.id] && probeLatencies[c.id].error))
    || (publicData && publicData.status === 'degraded');

  // Show only actual probe measurements; no synthetic SLA or latency values.
  const getModelMetrics = (modelName) => {
    let realProbeMs = null;
    if (channels && channels.length > 0 && probeLatencies) {
      for (const ch of channels) {
        if (ch.models && (ch.models.includes(modelName) || ch.models.includes('*'))) {
          const lat = probeLatencies[ch.id];
          if (lat && !lat.error && lat.latency_ms > 0) {
            if (realProbeMs === null || lat.latency_ms < realProbeMs) {
              realProbeMs = lat.latency_ms;
            }
          }
        }
      }
    }

    return { realProbeMs };
  };

  const modalitiesList = [
    { id: 'all', label: isZh ? '全部模态' : 'All Modalities', count: rawModels.length },
    { id: 'chat', label: isZh ? '文本对话' : 'Chat & Text', count: modalityCounts['chat'] || 0 },
    { id: 'vision', label: isZh ? '多模态视觉' : 'Vision', count: modalityCounts['vision'] || 0 },
    { id: 'images', label: isZh ? '图像生成' : 'Images', count: modalityCounts['images'] || 0 },
    { id: 'audio', label: isZh ? '语音音频' : 'Audio', count: modalityCounts['audio'] || 0 },
    { id: 'videos', label: isZh ? '视频生成' : 'Video', count: modalityCounts['videos'] || 0 },
    { id: 'embeddings', label: isZh ? '向量嵌入' : 'Embeddings', count: modalityCounts['embeddings'] || 0 },
  ].filter(item => item.id === 'all' || item.count > 0);


  const content = (
    <div className="space-y-6">
      {/* 1. Global System Status Banner */}
      <div
        className={`p-6 rounded-2xl border shadow-sm transition flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 ${
          anyTripped
            ? 'bg-amber-500/10 border-amber-500/30 text-amber-900 dark:text-amber-200'
            : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-950 dark:text-emerald-200'
        }`}
      >
        <div className="flex items-center space-x-4">
          <div
            className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
              anyTripped
                ? 'bg-amber-500 text-white animate-pulse'
                : 'bg-emerald-500 text-white'
            }`}
          >
            {anyTripped ? (
              <AlertTriangle className="w-5 h-5" />
            ) : (
              <CheckCircle2 className="w-5 h-5" />
            )}
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-2 mb-1">
              <h2 className="text-base font-semibold tracking-tight">
                {anyTripped
                  ? (t ? t.statusDegraded : (isZh ? '部分模型延迟偏高或熔断' : 'Partial degradation or breaker tripped'))
                  : (rawModels.length === 0 ? (isZh ? '暂无已配置模型' : 'No configured models') : (isZh ? '当前未发现熔断渠道' : 'No tripped channels reported'))}
              </h2>
              <span className="inline-flex items-center space-x-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium bg-white/40 dark:bg-black/20 border border-current/20 backdrop-blur-sm">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                <span>{isZh ? '当前渠道状态' : 'Current channel state'}</span>
              </span>
            </div>
            <p className="text-xs opacity-80 mt-0.5">
              {isZh ? '按当前配置与熔断状态推算' : 'Based on current routes and breakers'}
              <span className="ml-2 font-mono tabular-nums text-xs opacity-75">
                ({isZh ? '更新于 ' : 'Updated at '}{lastUpdated.toLocaleTimeString()})
              </span>
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-2.5 self-end sm:self-auto shrink-0">
          {/* Auto Refresh Toggle Badge */}
          <button
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={`flex items-center space-x-1.5 text-xs font-medium px-3 py-1.5 rounded-lg border transition cursor-pointer ${
              autoRefresh
                ? 'bg-white dark:bg-[#191B1F] border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4] shadow-2xs hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'
                : 'bg-[#F0F1F2] dark:bg-[#22252B] border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0]'
            }`}
            title={autoRefresh ? (isZh ? '点击暂停自动刷新' : 'Click to pause auto refresh') : (isZh ? '点击恢复 30 秒自动刷新' : 'Click to resume 30s auto refresh')}
          >
            {autoRefresh ? (
              <>
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
                <span>{isZh ? `自动刷新: ${countdown}s` : `Auto: ${countdown}s`}</span>
                <Pause className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0] ml-0.5" />
              </>
            ) : (
              <>
                <span className="w-2 h-2 rounded-full bg-slate-300 dark:bg-slate-600"></span>
                <span>{isZh ? '已暂停' : 'Paused'}</span>
                <Play className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0] ml-0.5" />
              </>
            )}
          </button>

          <button
            onClick={handleManualRefresh}
            disabled={loading}
            className="ui-btn-secondary ui-btn-sm"
            title={t ? t.statusRefresh : (isZh ? '刷新状态' : 'Refresh status')}
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin text-[#355CFF] dark:text-[#91A7FF]' : ''}`} />
            <span className="hidden sm:inline">{t ? t.statusRefresh : (isZh ? '刷新' : 'Refresh')}</span>
          </button>
        </div>
      </div>

      {/* Current configured routes and breaker state */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="ui-card p-5">
          <span className="ui-meta font-medium">{isZh ? '已配置模型路由' : 'Configured model routes'}</span>
          <div className="text-2xl font-bold text-[#355CFF] dark:text-[#91A7FF] font-mono mt-2">{rawModels.length}</div>
        </div>
        <div className="ui-card p-5">
          <span className="ui-meta font-medium">{isZh ? '当前熔断状态' : 'Current circuit state'}</span>
          <div className="text-lg font-bold text-[#202124] dark:text-[#F1F2F4] mt-2">{anyTripped ? (isZh ? '存在已熔断渠道' : 'A channel is tripped') : (isZh ? '未发现熔断' : 'No tripped channels reported')}</div>
        </div>
      </div>

      {/* Configured models and live probe results */}
      <div className="space-y-4">
        {/* Header & Search Bar */}
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4] flex items-center space-x-2">
              <Activity className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
              <span>{isZh ? '模型路由与探针结果' : 'Model routes and probe results'}</span>
            </h3>
          </div>

          <div className="flex items-center space-x-2.5">
            {/* Search Input */}
            <div className="relative">
              <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder={isZh ? '搜索模型名称...' : 'Search model ID...'}
                className="ui-field ui-field-sm pl-8 pr-3 w-48 sm:w-56"
              />
              {searchQuery && (
                <button
                  onClick={() => setSearchQuery('')}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[#656B75] hover:text-[#202124] dark:hover:text-[#F1F2F4] text-xs"
                >
                  ✕
                </button>
              )}
            </div>

            <span className="ui-meta font-mono tabular-nums hidden sm:inline">
              {isZh ? `显示 ${displayModels.length} / ${rawModels.length}` : `Showing ${displayModels.length} / ${rawModels.length}`}
            </span>
          </div>
        </div>

        {/* Modality Filter Pills */}
        <div className="flex items-center space-x-1.5 overflow-x-auto pb-1 text-xs">
          {modalitiesList.map(tab => (
            <button
              key={tab.id}
              onClick={() => setSelectedModality(tab.id)}
              className={`px-3 py-1.5 rounded-lg font-medium transition cursor-pointer shrink-0 flex items-center space-x-1.5 ${
                selectedModality === tab.id
                  ? 'bg-[#355CFF] text-white font-medium shadow-sm'
                  : 'bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B]'
              }`}
            >
              <span>{tab.label}</span>
              <span className={`text-xs px-1.5 py-0.2 rounded-full font-mono tabular-nums ${
                selectedModality === tab.id
                  ? 'bg-white/20 text-white'
                  : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]'
              }`}>
                {tab.count}
              </span>
            </button>
          ))}
        </div>

        {/* Models Grid */}
        {loading && displayModels.length === 0 ? (
          <div className="ui-card p-12 text-center ui-meta flex flex-col items-center justify-center space-y-2">
            <RefreshCw className="w-5 h-5 text-[#355CFF] dark:text-[#91A7FF] animate-spin" />
            <p>{isZh ? '正在获取实时模型可用性状态...' : 'Fetching live model health telemetry...'}</p>
          </div>
        ) : displayModels.length === 0 ? (
          <div className="ui-card p-12 text-center ui-meta space-y-2">
            <Filter className="w-8 h-8 text-[#A1A7B0] dark:text-[#656B75] mx-auto" />
            <p>{isZh ? '未找到符合条件的大模型服务' : 'No matching models found'}</p>
            {(searchQuery || selectedModality !== 'all') && (
              <button
                onClick={() => { setSearchQuery(''); setSelectedModality('all'); }}
                className="text-[#355CFF] dark:text-[#91A7FF] hover:underline text-xs font-semibold cursor-pointer"
              >
                {isZh ? '清空筛选条件' : 'Clear filters'}
              </button>
            )}
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {displayModels.map((mr) => {
              const isDegraded = mr.status === 'degraded';
              const metrics = getModelMetrics(mr.model);

              return (
                <div
                  key={mr.model}
                  className="ui-card p-5 hover:border-[#355CFF]/40 dark:hover:border-[#91A7FF]/40 transition space-y-4"
                >
                  {/* Model Header */}
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center space-x-2">
                        <span className="font-mono text-sm font-bold text-[#202124] dark:text-[#F1F2F4]">
                          {mr.model}
                        </span>
                        <button
                          onClick={() => copyToClipboard(mr.model)}
                          className="text-[#656B75] dark:text-[#A1A7B0] hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition cursor-pointer p-0.5"
                          title={isZh ? '复制模型名称' : 'Copy model ID'}
                        >
                          {copiedModel === mr.model ? (
                            <Check className="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" />
                          ) : (
                            <Copy className="w-3.5 h-3.5" />
                          )}
                        </button>
                        <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] uppercase">
                          {mr.modality}
                        </span>
                      </div>
                      {(mr.activeProviders !== undefined || mr.fallback_model) && (
                        <p className="ui-meta mt-1 flex items-center flex-wrap gap-1.5">
                          {mr.activeProviders !== undefined && (
                            <span>
                              {mr.activeProviders}/{mr.totalProviders} {t ? t.statusActivePool : (isZh ? '健康渠道提供商' : 'Healthy Channels')}
                            </span>
                          )}
                          {mr.fallback_model && (
                            <span className="text-amber-700 dark:text-amber-300 bg-amber-500/10 px-2 py-0.5 rounded-md font-mono text-xs border border-amber-500/20">
                              {isZh ? '➔ 容灾: ' : '➔ Fallback: '}{mr.fallback_model}
                            </span>
                          )}
                        </p>
                      )}

                      {/* Explicit Provider Channel Badges */}
                      {Array.isArray(mr.providers) && mr.providers.length > 0 && (
                        <div className="flex flex-wrap items-center gap-1.5 mt-2">
                          <span className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
                            {isZh ? '承载渠道:' : 'Providers:'}
                          </span>
                          {mr.providers.map(p => {
                            const isPActive = p.status === 'active';
                            return (
                              <span
                                key={p.channel_id || p.channel_name}
                                className={`px-1.5 py-0.5 rounded text-[11px] font-mono border ${
                                  isPActive
                                    ? 'bg-[#EEF2FF] dark:bg-[#242D48] text-[#355CFF] dark:text-[#91A7FF] border-[#355CFF]/20 dark:border-[#91A7FF]/30 font-medium'
                                    : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border-[#E4E6E9] dark:border-[#2C3038] line-through'
                                }`}
                                title={isPActive ? (isZh ? '提供商活跃' : 'Active provider') : (isZh ? '提供商渠道已停用' : 'Channel disabled')}
                              >
                                {p.channel_name || `#${p.channel_id}`}{!isPActive ? (isZh ? ' (已停用)' : ' (off)') : ''}
                              </span>
                            );
                          })}
                        </div>
                      )}
                    </div>

                    <span className={`px-2.5 py-1 rounded-full text-xs font-semibold border flex items-center space-x-1.5 shrink-0 ${
                      isDegraded
                        ? 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border-amber-500/30'
                        : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border-emerald-500/30'
                    }`}>
                      <span className={`w-1.5 h-1.5 rounded-full ${isDegraded ? 'bg-amber-500' : 'bg-emerald-500 animate-pulse'}`}></span>
                      <span>{isDegraded ? (isZh ? '不可用/降级' : 'Degraded') : (isZh ? '正常运行' : 'Operational')}</span>
                    </span>
                  </div>

                  <div className="rounded-2xl bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 border border-[#E4E6E9] dark:border-[#2C3038] px-3 py-2.5 text-xs">
                    <span className="text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '最近一次实测探针延迟' : 'Latest measured probe latency'}</span>
                    <span className="font-mono font-bold text-[#202124] dark:text-[#F1F2F4] ml-2">{metrics.realProbeMs ? `${metrics.realProbeMs} ms` : (isZh ? '暂无探针数据' : 'No probe data')}</span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* 6. Upstream Provider Channels Status */}
      {channels.length > 0 && (
        <div className="ui-card p-6 space-y-4">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2">
            <div className="flex items-center space-x-2">
              <Layers className="w-4 h-4 text-[#355CFF] dark:text-[#91A7FF]" />
              <h3 className="font-semibold text-[#202124] dark:text-[#F1F2F4] text-sm">
                {t ? t.statusCircuitBreakers : (isZh ? '上游提供商与熔断探针矩阵' : 'Upstream Channels & Circuit Breakers')}
              </h3>
            </div>

            {onBatchProbe && (
              <button
                onClick={onBatchProbe}
                disabled={batchTesting}
                className="px-3 py-1.5 rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038] text-[#202124] dark:text-[#F1F2F4] border border-[#E4E6E9] dark:border-[#2C3038] text-xs font-medium transition flex items-center space-x-1.5 cursor-pointer disabled:opacity-50 self-start sm:self-auto"
              >
                <Activity className={`w-3.5 h-3.5 ${batchTesting ? 'animate-spin' : ''}`} />
                <span>{batchTesting ? (isZh ? '正在并发探活...' : 'Probing in parallel...') : (isZh ? '一键探测全部渠道' : 'Probe All Channels')}</span>
              </button>
            )}
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] uppercase">
                  <th className="py-2.5 px-4 font-semibold">{t ? t.statusChannelName : (isZh ? '渠道名称' : 'Channel Name')}</th>
                  <th className="py-2.5 px-4 font-semibold">{t ? t.statusChannelType : (isZh ? '协议类型' : 'Protocol')}</th>
                  <th className="py-2.5 px-4 font-semibold">{t ? t.statusBreakerState : (isZh ? '状态与熔断器' : 'Status & Circuit Breaker')}</th>
                  <th className="py-2.5 px-4 font-semibold">{t ? t.statusProbeLatency : (isZh ? '实时探测延迟' : 'Live Latency')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#E4E6E9] dark:divide-[#2C3038] text-xs">
                {channels.map((ch) => {
                  const isDisabled = ch.status === 'disabled';
                  const breaker = channelBreakerState(ch);
                  const lat = probeLatencies[ch.id];

                  return (
                    <tr key={ch.id} className="hover:bg-[#F0F1F2]/60 dark:hover:bg-[#22252B]/60 transition">
                      <td className="py-3 px-4 font-medium">
                        <div className="flex items-center space-x-2">
                          <span className={`w-2 h-2 rounded-full ${
                            isDisabled
                              ? 'bg-[#A1A7B0] dark:bg-[#656B75]'
                              : breaker === 'open'
                                ? 'bg-rose-500'
                                : breaker === 'half_open'
                                  ? 'bg-amber-500'
                                  : 'bg-emerald-500'
                          }`}></span>
                          <span className={`font-semibold ${isDisabled ? 'text-[#656B75] dark:text-[#A1A7B0]' : 'text-[#202124] dark:text-[#F1F2F4]'}`}>{ch.name}</span>
                          {isDisabled && (
                            <span className="px-1.5 py-0.2 rounded text-[10px] font-mono bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038]">
                              {isZh ? '已停用' : 'Disabled'}
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="py-3 px-4 font-mono text-xs text-[#656B75] dark:text-[#A1A7B0]">
                        {(ch.type || 'openai').toUpperCase()}
                      </td>
                      <td className="py-3 px-4">
                        {isDisabled ? (
                          <span className="px-2 py-0.5 rounded-full text-xs font-semibold bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0] border border-[#E4E6E9] dark:border-[#2C3038] inline-flex items-center space-x-1">
                            <Pause className="w-3 h-3 text-[#656B75] dark:text-[#A1A7B0]" />
                            <span>{isZh ? '已停用 / 暂停服务' : 'Disabled / Inactive'}</span>
                          </span>
                        ) : breaker === 'open' ? (
                          <span className="px-2 py-0.5 rounded-full text-xs font-semibold bg-rose-500/10 dark:bg-rose-500/20 text-rose-700 dark:text-rose-300 border border-rose-500/30 inline-flex items-center space-x-1">
                            <XCircle className="w-3 h-3 text-rose-500" />
                            <span>{t ? t.statusStateOpen : (isZh ? '已熔断跳闸' : 'Tripped (Open)')}</span>
                          </span>
                        ) : breaker === 'half_open' ? (
                          <span className="px-2 py-0.5 rounded-full text-xs font-semibold bg-amber-500/10 dark:bg-amber-500/20 text-amber-700 dark:text-amber-300 border border-amber-500/30 inline-flex items-center space-x-1">
                            <AlertTriangle className="w-3 h-3 text-amber-500" />
                            <span>{isZh ? '半开探测中' : 'Half-open (probing)'}</span>
                          </span>
                        ) : (
                          <span className="px-2 py-0.5 rounded-full text-xs font-semibold bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30 inline-flex items-center space-x-1">
                            <CheckCircle2 className="w-3 h-3 text-emerald-500" />
                            <span>{t ? t.statusStateClosed : (isZh ? '健康闭合' : 'Closed (Healthy)')}</span>
                          </span>
                        )}
                      </td>
                      <td className="py-3 px-4 font-mono text-xs">
                        {isDisabled ? (
                          <span className="text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '渠道已停用' : 'Channel disabled'}</span>
                        ) : lat ? (
                          lat.error ? (
                            <span className="text-rose-600 dark:text-rose-400 font-semibold">{isZh ? '超时 / 探测失败' : 'Timeout / Failed'}</span>
                          ) : (
                            <span className="text-emerald-600 dark:text-emerald-400 font-semibold">{lat.latency_ms} ms</span>
                          )
                        ) : (
                          // No probe has run for this channel. Showing a made-up
                          // latency here ("~35 ms (Normal)") reported a measurement
                          // that was never taken.
                          <span className="text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '未探测' : 'Not probed'}</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );

  if (!isStandalone) {
    return content;
  }

  // Standalone Public Page View
  return (
    <div className="min-h-screen bg-[#F7F7F5] dark:bg-[#111214] text-[#202124] dark:text-[#F1F2F4] flex flex-col font-sans selection:bg-[#355CFF] selection:text-white">
      {/* Top Standalone Navigation Bar */}
      <header className="sticky top-0 z-40 bg-white/90 dark:bg-[#191B1F]/90 backdrop-blur-md border-b border-[#E4E6E9] dark:border-[#2C3038]">
        <div className="max-w-7xl mx-auto px-6 h-16 flex items-center justify-between">
          <div
            onClick={onBackHome}
            className="flex items-center space-x-3 cursor-pointer group transition duration-150 hover:opacity-90"
            title={lang === 'zh' ? '点击返回门户首页' : 'Return to Home'}
          >
            <div className="w-8 h-8 rounded-lg bg-[#355CFF] flex items-center justify-center text-white shadow-sm font-semibold text-sm">
              AR
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <span className="font-semibold text-base tracking-tight text-[#202124] dark:text-[#F1F2F4]">
                  Airoute
                </span>
                <span className="text-xs font-medium px-2 py-0.5 rounded-full bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border border-emerald-500/30">
                  {t ? t.statusOperationalBadge : (isZh ? '运行正常' : 'Operational')}
                </span>
              </div>
              <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] block">
                {lang === 'zh' ? '服务运行状态监控 (Status)' : 'Live System Status'}
              </span>
            </div>
          </div>

          <div className="flex items-center space-x-3">
            {setLang && (
              <button
                onClick={() => setLang(lang === 'zh' ? 'en' : 'zh')}
                className="px-3 py-1.5 rounded-lg bg-[#F0F1F2] dark:bg-[#22252B] hover:bg-[#E4E6E9] dark:hover:bg-[#2C3038] text-[#202124] dark:text-[#F1F2F4] text-xs font-medium transition cursor-pointer"
              >
                {lang === 'zh' ? 'EN' : '中'}
              </button>
            )}

            <button
              onClick={onBackHome}
              className="px-3.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#202124] dark:text-[#F1F2F4] text-xs font-medium transition cursor-pointer"
            >
              {lang === 'zh' ? '← 返回首页' : '← Home'}
            </button>

            {isLoggedIn ? (
              <button
                onClick={onEnterConsole}
                className="ui-btn-primary"
              >
                <Server className="w-3.5 h-3.5" />
                <span>{lang === 'zh' ? '进入工作台 →' : 'Console →'}</span>
              </button>
            ) : (
              <button
                onClick={onOpenLogin}
                className="ui-btn-primary"
              >
                <span>{lang === 'zh' ? '登录 / 注册' : 'Login / Register'}</span>
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Main Container */}
      <main className="max-w-6xl mx-auto w-full px-6 py-8 flex-1">
        {content}
      </main>

      {/* Footer */}
      <footer className="border-t border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] py-6 mt-12 text-center text-xs text-[#656B75] dark:text-[#A1A7B0]">
        <div className="max-w-6xl mx-auto px-6 flex flex-col sm:flex-row items-center justify-between gap-3">
          <p>© {new Date().getFullYear()} Airoute · {isZh ? '企业级大模型与多模态网关' : 'Enterprise AI Gateway'}</p>
          <div className="flex items-center space-x-4">
            <button onClick={onBackHome} className="hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition cursor-pointer">
              {isZh ? '门户首页' : 'Home'}
            </button>
            <span>·</span>
            <span className="text-[#656B75] dark:text-[#A1A7B0] font-medium">{isZh ? '状态来自当前网关' : 'Status from this gateway'}</span>
          </div>
        </div>
      </footer>
    </div>
  );
}
