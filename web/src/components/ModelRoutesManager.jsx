import React, { useState, useEffect, useCallback } from 'react';
import {
  Cpu,
  Shield,
  ShieldAlert,
  ArrowRight,
  RefreshCw,
  Search,
  Check,
  Copy,
  Zap,
  Plus,
  Trash2,
  ChevronDown,
  ChevronUp
} from 'lucide-react';
import ModelRouteModal, { getChannelModels } from './ModelRouteModal';
import { copyToClipboard as crossOriginCopy } from '../config';
import useUrlFilters from '../hooks/useUrlFilters';

export default function ModelRoutesManager({
  adminFetch,
  showToast,
  setCurrentTab,
  lang = 'zh',
  isAdmin = false
}) {
  const isZh = lang === 'zh';
  const [modelRoutes, setModelRoutes] = useState([]);
  const [loading, setLoading] = useState(false);
  const [permissionDenied, setPermissionDenied] = useState(false);
  // Filter state lives in the URL so a filtered route list survives module
  // switches and can be shared as a link.
  const [filters, setFilters, resetFilters] = useUrlFilters('routes', { q: '', modality: 'all' });
  const searchQuery = filters.q;
  const selectedModality = filters.modality || 'all';
  const [activeEditRoute, setActiveEditRoute] = useState(null);
  const [probingModel, setProbingModel] = useState(null);
  const [probeResults, setProbeResults] = useState({});
  const [copiedModel, setCopiedModel] = useState('');
  const [expandedModel, setExpandedModel] = useState(null);

  const [allChannels, setAllChannels] = useState([]);
  const [pricingRates, setPricingRates] = useState([]);
  const [selectedModels, setSelectedModels] = useState([]);

  const handleBatchDeleteRoutes = async () => {
    if (selectedModels.length === 0) return;
    if (!window.confirm(isZh ? `确定批量删除选中的 ${selectedModels.length} 个模型路由配置？渠道支持的模型配置将保留。` : `Delete selected ${selectedModels.length} model route configs? Channel capabilities will be retained.`)) {
      return;
    }
    try {
      const res = await adminFetch('/api/v1/admin/models/routes/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ models: selectedModels })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.warning || data.message || (isZh ? '已批量删除模型路由' : 'Selected model routes deleted'), data.warning ? 'warning' : 'success');
        setSelectedModels([]);
        fetchModelRoutes();
      } else {
        showToast(data.error || (isZh ? '批量删除失败' : 'Failed to delete routes'), 'warning');
      }
    } catch (err) {
      showToast((isZh ? '删除请求异常: ' : 'Delete error: ') + err.message, 'warning');
    }
  };

  const fetchModelRoutes = useCallback(async () => {
    // /admin/models/routes and /admin/channels are admin-only. Calling them as a
    // regular user produced a wall of permission errors instead of a clear
    // message, so fail fast with an explanation.
    if (!isAdmin) {
      setPermissionDenied(true);
      setLoading(false);
      return;
    }
    setPermissionDenied(false);
    setLoading(true);
    try {
      const [routesRes, channelsRes, pricesRes] = await Promise.all([
        adminFetch('/api/v1/admin/models/routes'),
        adminFetch('/api/v1/admin/channels'),
        adminFetch('/api/v1/admin/pricing')
      ]);
      const data = await routesRes.json();
      if (routesRes.ok && data.code === 0) {
        setModelRoutes(data.data || []);
      } else {
        showToast(isZh ? '获取模型路由拓扑失败' : 'Failed to fetch model routes topology', 'warning');
      }
      if (channelsRes.ok) {
        const cData = await channelsRes.json();
        setAllChannels(cData.data || []);
      }
      if (pricesRes.ok) {
        const pData = await pricesRes.json();
        setPricingRates(pData.data || []);
      }
    } catch (err) {
      showToast((isZh ? '网络请求异常: ' : 'Network error: ') + err.message, 'warning');
    } finally {
      setLoading(false);
    }
  }, [isAdmin, adminFetch, showToast, isZh]);

  const handleDeleteRoute = async (modelName) => {
    if (!window.confirm(isZh ? `确定删除模型 [${modelName}] 路由配置？渠道支持的模型配置将保留。` : `Delete route config for [${modelName}]? Channel capabilities will be retained.`)) {
      return;
    }
    try {
      const query = new URLSearchParams({ model: modelName });
      const res = await adminFetch(`/api/v1/admin/models/routes?${query}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.warning || (isZh ? `已删除模型 [${modelName}] 路由，渠道模型配置已保留` : `Deleted route for [${modelName}]; channel capabilities retained`), data.warning ? 'warning' : 'success');
        fetchModelRoutes();
      } else {
        showToast(data.error || (isZh ? '删除失败' : 'Failed to delete route'), 'warning');
      }
    } catch (err) {
      showToast((isZh ? '删除请求异常: ' : 'Delete error: ') + err.message, 'warning');
    }
  };

  const handleOpenEdit = (route) => {
    // Price rows are keyed by (model, group). Matching on model alone could pick
    // a VIP/enterprise row and then write its rates into the default group.
    const targetGroup = route.group_name || 'default';
    const price = pricingRates.find(p => p.model === route.model && (p.group_name || 'default') === targetGroup)
      || pricingRates.find(p => p.model === route.model && (p.group_name || 'default') === 'default');
    setActiveEditRoute({
      group_name: targetGroup,
      ...route,
      prompt_price: price ? price.prompt_price : 2.0,
      completion_price: price ? price.completion_price : 8.0,
      cache_read_price: price ? price.cache_read_price : 0.2,
      fixed_price: price ? price.fixed_price : 0.0,
      currency: price ? price.currency : 'CNY',
      off_peak_enabled: price ? (price.off_peak_enabled !== false) : true,
      off_peak_mode: price?.off_peak_mode || 'custom',
      off_peak_start: price?.off_peak_start || '00:00',
      off_peak_end: price?.off_peak_end || '08:30',
      off_peak_discount: price?.off_peak_discount ?? 0.5,
      off_peak_slots: price?.off_peak_slots || '',
      isNew: false
    });
  };

  const handleOpenCreate = () => {
    setActiveEditRoute({
      model: '',
      modality: 'chat',
      fallback_model: '',
      providers: [],
      prompt_price: 2.0,
      completion_price: 8.0,
      cache_read_price: 0.2,
      fixed_price: 0.0,
      currency: 'CNY',
      off_peak_enabled: true,
      off_peak_mode: 'custom',
      off_peak_start: '00:00',
      off_peak_end: '08:30',
      off_peak_discount: 0.5,
      off_peak_slots: '',
      isNew: true
    });
  };

  useEffect(() => {
    fetchModelRoutes();
  }, [fetchModelRoutes]);

  const handleProbeRoute = async (modelName) => {
    setProbingModel(modelName);
    try {
      const res = await adminFetch('/api/v1/admin/models/routes/probe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model: modelName })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        setProbeResults(prev => ({
          ...prev,
          [modelName]: data.data || []
        }));
        showToast(isZh ? `已完成 [${modelName}] 下游服务商实时连通性探测` : `Latency probe complete for [${modelName}] downstreams`, 'success');
      } else {
        showToast(data.error || (isZh ? '探测失败' : 'Probe failed'), 'warning');
      }
    } catch (err) {
      showToast((isZh ? '探测异常: ' : 'Probe error: ') + err.message, 'warning');
    } finally {
      setProbingModel(null);
    }
  };

  const copyToClipboard = async (text) => {
    await crossOriginCopy(text);
    setCopiedModel(text);
    setTimeout(() => setCopiedModel(''), 2000);
  };

  const spofRoutes = modelRoutes.filter(r => (r.providers || []).length <= 1);

  // Facts we can actually derive from the loaded configuration. Provider count
  // alone says nothing about health, so these feed a configuration summary
  // rather than a resilience verdict.
  const providerBindings = modelRoutes.reduce((sum, r) => sum + (r.providers || []).length, 0);
  const boundChannelCount = new Set(modelRoutes.flatMap(r => (r.providers || []).map(p => p.channel_id))).size;
  const modalityCount = new Set(modelRoutes.map(r => r.modality || 'chat')).size;
  const fallbackRouteCount = modelRoutes.filter(r => r.fallback_model).length;
  const probedModelCount = Object.keys(probeResults).filter(m => (probeResults[m] || []).length > 0).length;

  const modalityTabs = [
    { id: 'all', label: isZh ? '全部模态' : 'All' },
    { id: 'chat', label: isZh ? '对话' : 'Chat' },
    { id: 'images', label: isZh ? '生图' : 'Images' },
    { id: 'videos', label: isZh ? '视频' : 'Video' },
    { id: 'audio_speech', label: isZh ? '语音' : 'Audio' },
    { id: 'embeddings', label: isZh ? '向量' : 'Embedding' }
  ];
  const selectedModalityLabel = (modalityTabs.find(tab => tab.id === selectedModality) || modalityTabs[0]).label;

  const filteredRoutes = modelRoutes.filter(r => {
    if (selectedModality !== 'all' && r.modality !== selectedModality) {
      return false;
    }
    if (!searchQuery) return true;
    const q = searchQuery.toLowerCase();
    return r.model.toLowerCase().includes(q) || (r.providers || []).some(p => p.channel_name.toLowerCase().includes(q));
  });

  const getModalityBadge = (modality) => {
    switch (modality) {
      case 'images':
        return { label: isZh ? '生图' : 'Image', color: 'text-amber-700 bg-amber-50 dark:bg-amber-950/40 dark:text-amber-300 border-amber-200 dark:border-amber-800' };
      case 'audio_speech':
        return { label: isZh ? '语音TTS' : 'TTS', color: 'text-rose-700 bg-rose-50 dark:bg-rose-950/40 dark:text-rose-300 border-rose-200 dark:border-rose-800' };
      case 'audio_transcription':
        return { label: isZh ? '语音STT' : 'STT', color: 'text-orange-700 bg-orange-50 dark:bg-orange-950/40 dark:text-orange-300 border-orange-200 dark:border-orange-800' };
      case 'videos':
        return { label: isZh ? '视频' : 'Video', color: 'text-purple-700 bg-purple-50 dark:bg-purple-950/40 dark:text-purple-300 border-purple-200 dark:border-purple-800' };
      case 'embeddings':
        return { label: isZh ? '嵌入' : 'Embedding', color: 'text-cyan-700 bg-cyan-50 dark:bg-cyan-950/40 dark:text-cyan-300 border-cyan-200 dark:border-cyan-800' };
      case 'rerank':
        return { label: isZh ? '重排' : 'Rerank', color: 'text-emerald-700 bg-emerald-50 dark:bg-emerald-950/40 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800' };
      default:
        return { label: isZh ? '对话/文本' : 'Chat', color: 'text-[#355CFF] bg-[#EEF2FF] dark:bg-[#242D48] dark:text-[#91A7FF] border-[#355CFF]/20 dark:border-[#91A7FF]/20' };
    }
  };

  const allModelNames = modelRoutes.map(r => r.model);

  if (permissionDenied) {
    return (
      <div className="space-y-6">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div>
            <h1 className="ui-page-title">
              {isZh ? '模型路由与分发' : 'Model Routes & Dispatch'}
            </h1>
            <p className="ui-page-subtitle">
              {isZh ? '仅超级管理员可查看与配置集群路由拓扑' : 'Cluster routing topology is restricted to administrators'}
            </p>
          </div>
        </div>
        <div className="rounded-xl border border-amber-200 dark:border-amber-900/60 bg-amber-50/60 dark:bg-amber-950/30 p-5 flex items-start space-x-3">
          <ShieldAlert className="w-5 h-5 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
          <div className="text-sm text-amber-800 dark:text-amber-200">
            <p className="font-semibold">
              {isZh ? '权限不足：无法加载模型路由配置' : 'Insufficient permissions to load model routes'}
            </p>
            <p className="mt-1 text-amber-700 dark:text-amber-300 text-xs">
              {isZh
                ? '该页面依赖管理员接口 (/admin/models/routes 与 /admin/channels)。你仍可在「模型定价」查看自己的费率，或在「Playground」直接调用模型。'
                : 'This page relies on admin-only endpoints. You can still review your own rates under Pricing, or call models from the Playground.'}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 1. Header (Skeleton aligned) */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="ui-page-title">
            {isZh ? '模型路由与分发' : 'Model Routes & Dispatch'}
          </h1>
          <p className="ui-page-subtitle">
            {isZh ? '配置路由绑定、降级与定价' : 'Routing weights, fallbacks and pricing'}
          </p>
        </div>

        <div className="flex items-center space-x-2 shrink-0">
          <button
            onClick={fetchModelRoutes}
            disabled={loading}
            className="ui-btn-secondary ui-btn-sm"
          >
            <RefreshCw className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${loading ? 'animate-spin' : ''}`} />
            <span>{isZh ? '刷新' : 'Refresh'}</span>
          </button>
          <button
            onClick={handleOpenCreate}
            className="ui-btn-primary ui-btn-sm"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '新建模型路由' : 'New Model Route'}</span>
          </button>
        </div>
      </div>

      {/* Search & Modality Bar */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3 text-xs">
        <div className="inline-flex bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] overflow-x-auto max-w-full">
          {modalityTabs.map(tab => (
            <button
              key={tab.id}
              onClick={() => setFilters({ modality: tab.id })}
              className={`px-3 py-1 rounded-md text-xs font-medium whitespace-nowrap transition cursor-pointer ${
                selectedModality === tab.id
                  ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-semibold'
                  : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="relative w-full md:w-64 shrink-0">
          <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] absolute left-3 top-2.5" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setFilters({ q: e.target.value })}
            placeholder={isZh ? '搜索模型或服务商...' : 'Search model or provider...'}
            className="ui-field pl-9 pr-3 py-1.5 text-xs w-full"
          />
        </div>
      </div>

      {/* Batch Action Bar */}
      {selectedModels.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 px-4 py-2.5 rounded-xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-medium text-[#355CFF] dark:text-[#91A7FF]">
            <span>{isZh ? `已选中 ${selectedModels.length} 个模型路由` : `Selected ${selectedModels.length} routes`}</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDeleteRoutes}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-medium rounded-lg shadow-xs transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>{isZh ? '批量删除' : 'Batch Delete'}</span>
            </button>
            <button
              onClick={() => setSelectedModels([])}
              className="ui-btn-secondary ui-btn-sm"
            >
              {isZh ? '取消选择' : 'Deselect All'}
            </button>
          </div>
        </div>
      )}

      {/* Configuration summary */}
      {modelRoutes.length > 0 && (
        <div className={`p-4 rounded-xl border text-xs flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 ${
          spofRoutes.length > 0
            ? 'bg-amber-50/60 dark:bg-amber-950/20 border-amber-200/80 dark:border-amber-900/40 text-amber-900 dark:text-amber-200'
            : 'bg-white dark:bg-[#191B1F] border-[#E4E6E9] dark:border-[#2C3038] text-[#202124] dark:text-[#F1F2F4]'
        }`}>
          <div className="flex items-start space-x-2.5">
            <div className={`w-8 h-8 rounded-lg flex items-center justify-center shrink-0 ${
              spofRoutes.length > 0 ? 'bg-amber-100/80 dark:bg-amber-900/40 text-amber-700 dark:text-amber-300' : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]'
            }`}>
              <Shield className="w-4 h-4" />
            </div>
            <div className="space-y-0.5">
              <div className="flex flex-wrap items-center gap-2 font-semibold">
                <span>{isZh ? '路由配置摘要' : 'Route Configuration Summary'}</span>
                <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                  spofRoutes.length > 0
                    ? 'bg-amber-100 dark:bg-amber-900/60 text-amber-800 dark:text-amber-200'
                    : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]'
                }`}>
                  {spofRoutes.length > 0
                    ? (isZh ? `配置层：${spofRoutes.length} 个模型仅单一服务商` : `Config: ${spofRoutes.length} single-provider`)
                    : (isZh ? '配置层：均已配置备用服务商' : 'Config: all have backups')}
                </span>
              </div>
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-0.5">
                {isZh
                  ? `${modelRoutes.length} 条模型路由 · ${providerBindings} 条上游绑定（${boundChannelCount} 个渠道）· 覆盖 ${modalityCount} 类模态 · ${fallbackRouteCount} 条降级路由`
                  : `${modelRoutes.length} routes · ${providerBindings} upstream bindings across ${boundChannelCount} channels · ${modalityCount} modalities · ${fallbackRouteCount} fallback routes`}
              </p>
              {spofRoutes.length > 0 && (
                <p className="text-xs text-amber-800 dark:text-amber-300 mt-0.5">
                  {isZh
                    ? `其中 ${spofRoutes.slice(0, 3).map(r => r.model).join(', ')} 仅绑定了单一上游，没有可切换的备用渠道。`
                    : `${spofRoutes.slice(0, 3).map(r => r.model).join(', ')} bind a single upstream, so there is no backup channel to switch to.`}
                </p>
              )}
              <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-0.5">
                {probedModelCount > 0
                  ? (isZh
                      ? `已在本页手动探测 ${probedModelCount} 个模型的下游连通性（结果为本次会话内实测），其余模型未验证。`
                      : `Connectivity was manually probed for ${probedModelCount} model(s) in this session; the remaining models are unverified.`)
                  : (isZh
                      ? '尚未进行连通性探测，尚未进行故障切换演练——以上仅为已保存配置的统计，不代表服务健康状况。'
                      : 'No connectivity probes or failover drills have been run — this counts saved configuration only and is not a health guarantee.')}
              </p>
            </div>
          </div>
          {spofRoutes.length > 0 && (
            <button
              onClick={() => handleOpenEdit(spofRoutes[0])}
              className="px-3 py-1.5 rounded-lg bg-amber-600 hover:bg-amber-700 text-white font-medium text-xs shrink-0 cursor-pointer shadow-xs transition"
            >
              {isZh ? `为 [${spofRoutes[0].model}] 配置备用服务商` : `Add backup for [${spofRoutes[0].model}]`}
            </button>
          )}
        </div>
      )}

      {/* 2. Model Routes Table (Clean Master-Detail Layout) */}
      <div className="ui-card overflow-hidden">
        {filteredRoutes.length === 0 ? (
          <div className="p-12 text-center flex flex-col items-center justify-center space-y-3">
            <Cpu className="w-10 h-10 text-[#656B75]/40 dark:text-[#A1A7B0]/40" />
            {modelRoutes.length === 0 ? (
              <>
                <h4 className="font-medium text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '尚未配置模型路由' : 'No model routes configured yet'}</h4>
                <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] max-w-sm">
                  {isZh
                    ? '新建模型路由后，即可为模型绑定上游服务商、分流权重与降级链路。'
                    : 'Create a route to bind upstream providers, dispatch weights, and fallback chains to a model.'}
                </p>
                <button onClick={handleOpenCreate} className="ui-btn-primary ui-btn-sm">
                  <Plus className="w-3.5 h-3.5" />
                  <span>{isZh ? '新建模型路由' : 'New Model Route'}</span>
                </button>
              </>
            ) : (
              <>
                <Search className="w-10 h-10 text-[#656B75]/40 dark:text-[#A1A7B0]/40" />
                <h4 className="font-medium text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '当前筛选没有匹配的路由' : 'No routes match the current filters'}</h4>
                <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] max-w-md">
                  {isZh
                    ? `已配置的 ${modelRoutes.length} 条模型路由中，没有同时匹配关键词「${searchQuery || '全部'}」与模态「${selectedModalityLabel}」的项。`
                    : `None of the ${modelRoutes.length} configured routes match "${searchQuery || 'all'}" under "${selectedModalityLabel}".`}
                </p>
                <button onClick={resetFilters} className="ui-btn-secondary ui-btn-sm">
                  <RefreshCw className="w-3.5 h-3.5" />
                  <span>{isZh ? '清除筛选' : 'Clear filters'}</span>
                </button>
              </>
            )}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2]/50 dark:bg-[#22252B]/50 uppercase">
                  <th className="ui-th w-10 text-center">
                    <input
                      type="checkbox"
                      className="rounded border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-[#355CFF]/30 cursor-pointer"
                      checked={filteredRoutes.length > 0 && selectedModels.length === filteredRoutes.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedModels(filteredRoutes.map(r => r.model));
                        } else {
                          setSelectedModels([]);
                        }
                      }}
                    />
                  </th>
                  <th className="ui-th">{isZh ? '模型标识' : 'Model ID'}</th>
                  <th className="ui-th">{isZh ? '上游服务商 (权重)' : 'Upstream Channels (Weight)'}</th>
                  <th className="ui-th">{isZh ? '容灾降级' : 'Failover Target'}</th>
                  <th className="ui-th">{isZh ? '费率 (输入/输出)' : 'Rates (Prompt/Comp)'}</th>
                  <th className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60">
                {filteredRoutes.map((route) => {
                  const badge = getModalityBadge(route.modality);
                  const isExpanded = expandedModel === route.model;
                  const price = pricingRates.find(p => p.model === route.model);
                  const probes = probeResults[route.model] || [];

                  return (
                    <React.Fragment key={route.model}>
                      <tr className="ui-tr-hover group">
                        <td className="ui-td text-center" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            className="rounded border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-[#355CFF]/30 cursor-pointer"
                            checked={selectedModels.includes(route.model)}
                            onChange={(e) => {
                              e.stopPropagation();
                              if (e.target.checked) {
                                setSelectedModels(prev => [...prev, route.model]);
                              } else {
                                setSelectedModels(prev => prev.filter(m => m !== route.model));
                              }
                            }}
                          />
                        </td>
                        {/* Model name */}
                        <td className="ui-td font-medium text-[#202124] dark:text-[#F1F2F4]">
                          <div className="flex items-center space-x-1.5">
                            <span className="font-mono text-xs text-[#202124] dark:text-[#F1F2F4] font-semibold">
                              {route.model}
                            </span>
                            <button
                              onClick={() => copyToClipboard(route.model)}
                              className="text-[#656B75] hover:text-[#355CFF] dark:text-[#A1A7B0] dark:hover:text-[#91A7FF] transition cursor-pointer p-0.5 rounded"
                              title={isZh ? "复制模型标识" : "Copy model ID"}
                            >
                              {copiedModel === route.model ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                            </button>
                            <span className={`text-xs px-1.5 py-0.5 rounded border font-medium ${badge.color}`}>
                              {badge.label}
                            </span>
                          </div>
                        </td>

                        {/* Providers & Weights */}
                        <td className="ui-td">
                          <div className="flex flex-wrap items-center gap-1.5">
                            {(route.providers || []).map((p) => {
                              const ch = allChannels.find(c => c.id === p.channel_id);
                              let warnMsg = null;
                              if (!ch) {
                                warnMsg = isZh ? '渠道不存在或已删除' : 'Channel missing';
                              } else if (ch.status === 'inactive' || ch.status === 'disabled') {
                                warnMsg = isZh ? '渠道已停用' : 'Channel disabled';
                              } else {
                                const chModels = getChannelModels(ch);
                                const mapped = p.mapped_model || route.model;
                                if (chModels.length > 0 && !chModels.includes('*') && !chModels.includes(mapped)) {
                                  warnMsg = isZh ? `渠道未声明模型: ${mapped}` : `Model undeclared: ${mapped}`;
                                }
                              }

                              return (
                                <span
                                  key={p.channel_id}
                                  className={`inline-flex items-center space-x-1 px-2 py-0.5 rounded-md border text-xs font-medium ${
                                    warnMsg
                                      ? 'bg-amber-50 dark:bg-amber-950/40 border-amber-300 dark:border-amber-800 text-amber-900 dark:text-amber-200'
                                      : 'bg-[#F0F1F2] dark:bg-[#22252B] border-[#E4E6E9]/60 dark:border-[#2C3038]/60 text-[#202124] dark:text-[#F1F2F4]'
                                  }`}
                                  title={warnMsg ? warnMsg : p.mapped_model ? `${isZh ? '上游模型: ' : 'Upstream: '}${p.mapped_model}` : ''}
                                >
                                  {warnMsg && <ShieldAlert className="w-3 h-3 text-amber-500 shrink-0" />}
                                  <span>{p.channel_name}</span>
                                  <strong className={`${warnMsg ? 'text-amber-700 dark:text-amber-300' : 'text-[#355CFF] dark:text-[#91A7FF]'} font-mono tabular-nums font-semibold`}>
                                    {p.weight_percent ? `${p.weight_percent}%` : `${isZh ? '权重' : 'w:'}${p.weight}`}
                                  </strong>
                                </span>
                              );
                            })}
                            {(route.providers || []).length === 0 && (
                              <span className="text-rose-500 text-xs font-medium">{isZh ? '未绑定上游' : 'No Upstream'}</span>
                            )}
                          </div>
                        </td>

                        {/* Fallback */}
                        <td className="ui-td font-mono text-xs">
                          {route.fallback_model ? (
                            <span className="inline-flex items-center space-x-1 text-sky-700 dark:text-sky-300 bg-sky-50 dark:bg-sky-950/40 border border-sky-200 dark:border-sky-800 px-2 py-0.5 rounded-md text-xs font-medium">
                              <Shield className="w-3 h-3 text-sky-600 dark:text-sky-400" />
                              <span>{isZh ? '降级: ' : 'Fallback: '}{route.fallback_model}</span>
                            </span>
                          ) : (
                            <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-sans">{isZh ? '直通返回' : 'Direct Return'}</span>
                          )}
                        </td>

                        {/* Pricing */}
                        <td className="ui-td text-xs">
                          {price ? (
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation();
                                if (setCurrentTab) setCurrentTab('pricing');
                              }}
                              className="flex items-center space-x-1.5 text-xs hover:text-[#355CFF] dark:hover:text-[#91A7FF] transition cursor-pointer group text-left"
                              title={isZh ? '已配置专属定价，点击前往【定价管理】查看与编辑' : 'Click to view pricing'}
                            >
                              <span className="text-[#202124] dark:text-[#F1F2F4] font-mono tabular-nums font-semibold group-hover:text-[#355CFF] dark:group-hover:text-[#91A7FF]">
                                ¥{price.prompt_price.toFixed(1)}/¥{price.completion_price.toFixed(1)}
                              </span>
                              {price.off_peak_enabled !== false && (
                                <span className="bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] px-1.5 py-0.5 rounded font-medium text-xs">
                                  {isZh ? '闲时优惠' : 'Off-peak'}
                                </span>
                              )}
                              <ArrowRight className="w-2.5 h-2.5 opacity-0 group-hover:opacity-100 text-[#355CFF] dark:text-[#91A7FF] transition-opacity" />
                            </button>
                          ) : (
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation();
                                if (setCurrentTab) setCurrentTab('pricing');
                              }}
                              className="inline-flex items-center space-x-1 px-1.5 py-0.5 rounded text-xs font-medium bg-amber-50 dark:bg-amber-950/40 hover:bg-amber-100 dark:hover:bg-amber-900/60 text-amber-700 dark:text-amber-300 border border-amber-200/80 dark:border-amber-800 cursor-pointer transition text-left"
                              title={isZh ? '未配置专属费率，当前回退至系统基准定价 (¥2.0/¥8.0)。点击前往【定价管理】配置专属定价！' : 'No explicit price set, fallback to system default. Click to configure.'}
                            >
                              <ShieldAlert className="w-2.5 h-2.5 text-amber-500 shrink-0" />
                              <span>{isZh ? '未配专属价(基准)' : 'No Pricing'}</span>
                              <ArrowRight className="w-2 h-2 text-amber-600 ml-0.5" />
                            </button>
                          )}
                        </td>

                        {/* Actions */}
                        <td className="ui-td text-right space-x-1.5">
                          <button
                            onClick={() => handleOpenEdit(route)}
                            className="ui-btn-secondary ui-btn-sm"
                          >
                            {isZh ? '配置分发' : 'Configure'}
                          </button>
                          <button
                            onClick={() => setExpandedModel(isExpanded ? null : route.model)}
                            className="p-1 rounded-lg hover:bg-[#F0F1F2] dark:hover:bg-[#22252B] text-[#656B75] hover:text-[#202124] dark:text-[#A1A7B0] dark:hover:text-[#F1F2F4] transition cursor-pointer inline-flex items-center align-middle"
                            title={isExpanded ? (isZh ? '收起详情' : 'Collapse') : (isZh ? '展开服务商明细' : 'Expand Details')}
                          >
                            {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                          </button>
                          <button
                            onClick={() => handleDeleteRoute(route.model)}
                            className="p-1 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-950/50 text-[#656B75] hover:text-rose-600 dark:text-[#A1A7B0] dark:hover:text-rose-400 transition cursor-pointer inline-flex items-center align-middle"
                            title={isZh ? '删除路由' : 'Delete Route'}
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </td>
                      </tr>

                      {/* Expandable Provider Topology Detail Row */}
                      {isExpanded && (
                        <tr className="bg-[#F0F1F2]/40 dark:bg-[#22252B]/40 border-b border-[#E4E6E9] dark:border-[#2C3038]">
                          <td colSpan="6" className="py-4 px-8 space-y-3">
                            <div className="flex items-center justify-between text-xs">
                              <span className="font-semibold text-[#202124] dark:text-[#F1F2F4]">
                                {isZh
                                  ? `上游服务商明细与实时连通性探测 (${route.providers?.length || 0})`
                                  : `Upstream Providers & Health Probing (${route.providers?.length || 0})`}
                              </span>
                              <button
                                onClick={() => handleProbeRoute(route.model)}
                                disabled={probingModel === route.model}
                                className="ui-btn-secondary ui-btn-sm"
                              >
                                <Zap className={`w-3 h-3 text-amber-500 ${probingModel === route.model ? 'animate-pulse' : ''}`} />
                                <span>{probingModel === route.model ? (isZh ? '探测中...' : 'Probing...') : (isZh ? '测试所有下游延迟' : 'Test Downstream Latency')}</span>
                              </button>
                            </div>

                            <div className="border border-[#E4E6E9] dark:border-[#2C3038] rounded-xl overflow-hidden bg-white dark:bg-[#191B1F] text-xs">
                              <table className="w-full text-left">
                                <thead className="bg-[#F0F1F2]/60 dark:bg-[#22252B]/60 text-[#656B75] dark:text-[#A1A7B0] text-xs border-b border-[#E4E6E9] dark:border-[#2C3038]">
                                  <tr>
                                    <th className="ui-th">{isZh ? '服务商名称' : 'Provider Name'}</th>
                                    <th className="ui-th">{isZh ? '服务引擎' : 'Engine'}</th>
                                    <th className="ui-th">{isZh ? '分流权重' : 'Weight'}</th>
                                    <th className="ui-th">{isZh ? '远端模型映射' : 'Remote Model'}</th>
                                    <th className="ui-th text-right">{isZh ? '健康状态 / 探测延迟' : 'Health / Latency'}</th>
                                  </tr>
                                </thead>
                                <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60 font-mono text-xs">
                                  {(route.providers || []).map((p) => {
                                    const probe = probes.find(pr => pr.channel_id === p.channel_id);
                                    return (
                                      <tr key={p.channel_id} className="ui-tr-hover">
                                        <td className="ui-td font-sans font-medium text-[#202124] dark:text-[#F1F2F4]">
                                          {p.channel_name}
                                        </td>
                                        <td className="ui-td text-[#656B75] dark:text-[#A1A7B0]">
                                          {p.channel_type}
                                        </td>
                                        <td className="ui-td text-[#355CFF] dark:text-[#91A7FF] font-semibold tabular-nums">
                                          {p.weight_percent ? `${p.weight_percent}%` : `${isZh ? '权重: ' : 'Weight: '}${p.weight}`}
                                        </td>
                                        <td className="ui-td text-[#656B75] dark:text-[#A1A7B0]">
                                          {p.mapped_model || route.model}
                                        </td>
                                        <td className="ui-td text-right font-sans">
                                          {probe ? (
                                            <span className={`font-mono tabular-nums font-semibold ${probe.status === 'healthy' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'}`}>
                                              {probe.latency_ms} ms ({probe.status})
                                            </span>
                                          ) : (
                                            <span className="text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2] dark:bg-[#22252B] px-1.5 py-0.5 rounded border border-[#E4E6E9] dark:border-[#2C3038] text-xs font-medium">
                                              {isZh ? '未探测' : 'Not probed'}
                                            </span>
                                          )}
                                        </td>
                                      </tr>
                                    );
                                  })}
                                </tbody>
                              </table>
                            </div>
                          </td>
                        </tr>
                      )}
                    </React.Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* 3. Model Route Config Modal */}
      {activeEditRoute && (
        <ModelRouteModal
          isOpen={!!activeEditRoute}
          onClose={() => setActiveEditRoute(null)}
          modelRoute={activeEditRoute}
          allModels={allModelNames}
          allChannels={allChannels}
          onSaveSuccess={() => {
            fetchModelRoutes();
            setActiveEditRoute(null);
          }}
          adminFetch={adminFetch}
          showToast={showToast}
          lang={lang}
        />
      )}
    </div>
  );
}
