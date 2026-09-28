import React, { useState, useEffect } from 'react';
import {
  Layers,
  Cpu,
  Shield,
  Sliders,
  Sparkles,
  ArrowRight,
  RefreshCw,
  Search,
  Check,
  Copy,
  Zap,
  Clock,
  MessageSquare,
  Image as ImageIcon,
  Volume2,
  Mic,
  Video,
  Database,
  Terminal,
  Activity,
  Plus,
  Trash2,
  DollarSign,
  ChevronDown,
  ChevronUp
} from 'lucide-react';
import ModelRouteModal from './ModelRouteModal';

export default function ModelRoutesManager({
  adminFetch,
  showToast,
  onNavigateToPlayground
}) {
  const [modelRoutes, setModelRoutes] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedModality, setSelectedModality] = useState('all');
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
    if (!window.confirm(`确定批量删除选中的 ${selectedModels.length} 个模型路由配置？`)) {
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
        showToast(data.message || '已批量删除模型路由', 'success');
        setSelectedModels([]);
        fetchModelRoutes();
      } else {
        showToast(data.error || '批量删除失败', 'warning');
      }
    } catch (err) {
      showToast('删除请求异常: ' + err.message, 'warning');
    }
  };

  const fetchModelRoutes = async () => {
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
        showToast('获取模型路由拓扑失败', 'warning');
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
      showToast('网络请求异常: ' + err.message, 'warning');
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteRoute = async (modelName) => {
    if (!window.confirm(`确定删除模型 [${modelName}] 路由配置及其服务商绑定？`)) {
      return;
    }
    try {
      const res = await adminFetch(`/api/v1/admin/models/routes/${encodeURIComponent(modelName)}`, {
        method: 'DELETE'
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(`已删除模型 [${modelName}] 路由`, 'success');
        fetchModelRoutes();
      } else {
        showToast(data.error || '删除失败', 'warning');
      }
    } catch (err) {
      showToast('删除请求异常: ' + err.message, 'warning');
    }
  };

  const handleOpenEdit = (route) => {
    const price = pricingRates.find(p => p.model === route.model);
    setActiveEditRoute({
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
  }, []);

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
        showToast(`已完成 [${modelName}] 下游服务商实时连通性探测`, 'success');
      } else {
        showToast(data.error || '探测失败', 'warning');
      }
    } catch (err) {
      showToast('探测异常: ' + err.message, 'warning');
    } finally {
      setProbingModel(null);
    }
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
    setCopiedModel(text);
    setTimeout(() => setCopiedModel(''), 2000);
  };

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
        return { label: '生图', color: 'text-amber-700 bg-amber-50 border-amber-200' };
      case 'audio_speech':
        return { label: '语音TTS', color: 'text-rose-700 bg-rose-50 border-rose-200' };
      case 'audio_transcription':
        return { label: '语音STT', color: 'text-orange-700 bg-orange-50 border-orange-200' };
      case 'videos':
        return { label: '视频', color: 'text-purple-700 bg-purple-50 border-purple-200' };
      case 'embeddings':
        return { label: '嵌入', color: 'text-cyan-700 bg-cyan-50 border-cyan-200' };
      case 'rerank':
        return { label: '重排', color: 'text-emerald-700 bg-emerald-50 border-emerald-200' };
      default:
        return { label: '对话/文本', color: 'text-indigo-700 bg-indigo-50 border-indigo-200' };
    }
  };

  const allModelNames = modelRoutes.map(r => r.model);

  return (
    <div className="space-y-4">
      {/* 1. Clean Top Header */}
      <div className="bg-white border border-slate-200/80 rounded-2xl p-5 shadow-xs">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center space-x-2.5">
            <h2 className="text-lg font-bold text-slate-900 tracking-tight">模型路由与分发</h2>
          </div>

          <div className="flex items-center space-x-2 shrink-0">
            <button
              onClick={handleOpenCreate}
              className="px-3.5 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-xs flex items-center space-x-1.5 transition cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>新建模型路由</span>
            </button>
            <button
              onClick={fetchModelRoutes}
              disabled={loading}
              className="px-3 py-1.5 rounded-xl bg-slate-100 hover:bg-slate-200 text-xs font-semibold text-slate-700 border border-slate-200 transition flex items-center space-x-1.5 cursor-pointer"
            >
              <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
              <span>刷新</span>
            </button>
          </div>
        </div>

        {/* Search & Modality Bar */}
        <div className="mt-4 pt-3.5 border-t border-slate-100 flex flex-col md:flex-row md:items-center justify-between gap-3 text-xs">
          <div className="flex items-center space-x-1 overflow-x-auto pb-1 max-w-full">
            {[
              { id: 'all', label: '全部模态' },
              { id: 'chat', label: '对话' },
              { id: 'images', label: '生图' },
              { id: 'videos', label: '视频' },
              { id: 'audio_speech', label: '语音' },
              { id: 'embeddings', label: '向量' }
            ].map(tab => (
              <button
                key={tab.id}
                onClick={() => setSelectedModality(tab.id)}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium whitespace-nowrap transition cursor-pointer ${
                  selectedModality === tab.id
                    ? 'bg-indigo-600 text-white shadow-2xs'
                    : 'text-slate-600 hover:bg-slate-100'
                }`}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <div className="relative w-full md:w-64 shrink-0">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="搜索模型或服务商..."
              className="w-full bg-slate-50 border border-slate-200 rounded-xl pl-8 pr-3 py-1.5 text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
            />
          </div>
        </div>
      </div>

      {/* Batch Action Bar */}
      {selectedModels.length > 0 && (
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 border border-indigo-200 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
          <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900">
            <span>已选中 {selectedModels.length} 个模型路由</span>
          </div>
          <div className="flex items-center space-x-2">
            <button
              onClick={handleBatchDeleteRoutes}
              className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>批量删除</span>
            </button>
            <button
              onClick={() => setSelectedModels([])}
              className="px-3 py-1.5 bg-white text-slate-700 border border-slate-200 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
            >
              取消选择
            </button>
          </div>
        </div>
      )}

      {/* 2. Model Routes Table (Clean Master-Detail Layout) */}
      <div className="bg-white border border-slate-200/80 rounded-2xl overflow-hidden shadow-xs">
        {filteredRoutes.length === 0 ? (
          <div className="p-10 text-center flex flex-col items-center justify-center space-y-2">
            <Cpu className="w-10 h-10 text-slate-300" />
            <h4 className="font-semibold text-xs text-slate-800">暂无模型路由</h4>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left border-collapse text-xs">
              <thead>
                <tr className="border-b border-slate-200 text-slate-500 bg-slate-50/80">
                  <th className="py-3 px-3 w-10 text-center">
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                  <th className="py-3 px-5 font-semibold">模型标识</th>
                  <th className="py-3 px-4 font-semibold">上游服务商 (权重)</th>
                  <th className="py-3 px-4 font-semibold">容灾降级</th>
                  <th className="py-3 px-4 font-semibold">费率 (输入/输出)</th>
                  <th className="py-3 px-5 text-right font-semibold">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {filteredRoutes.map((route) => {
                  const badge = getModalityBadge(route.modality);
                  const isExpanded = expandedModel === route.model;
                  const price = pricingRates.find(p => p.model === route.model);
                  const probes = probeResults[route.model] || [];

                  return (
                    <React.Fragment key={route.model}>
                      <tr className="hover:bg-slate-50/60 transition group">
                        <td className="py-3.5 px-3 text-center" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                        <td className="py-3.5 px-5 font-bold text-slate-900 font-sans">
                          <div className="flex items-center space-x-1.5">
                            <span className="font-mono text-xs text-slate-800 font-bold">
                              {route.model}
                            </span>
                            <button
                              onClick={() => copyToClipboard(route.model)}
                              className="text-slate-400 hover:text-slate-600 transition cursor-pointer p-0.5"
                              title="复制模型标识"
                            >
                              {copiedModel === route.model ? <Check className="w-3 h-3 text-emerald-600" /> : <Copy className="w-3 h-3" />}
                            </button>
                            <span className={`text-[10px] px-1.5 py-0.2 rounded border ${badge.color}`}>
                              {badge.label}
                            </span>
                          </div>
                        </td>

                        {/* Providers & Weights */}
                        <td className="py-3.5 px-4">
                          <div className="flex flex-wrap items-center gap-1.5">
                            {(route.providers || []).map((p) => (
                              <span
                                key={p.channel_id}
                                className="inline-flex items-center space-x-1 px-2 py-0.5 rounded-md bg-slate-100 border border-slate-200 text-[11px] text-slate-700 font-medium"
                              >
                                <span>{p.channel_name}</span>
                                <strong className="text-indigo-600 font-mono font-bold">
                                  {p.weight_percent ? `${p.weight_percent}%` : `权重${p.weight}`}
                                </strong>
                              </span>
                            ))}
                            {(route.providers || []).length === 0 && (
                              <span className="text-rose-500 text-[11px]">未绑定上游</span>
                            )}
                          </div>
                        </td>

                        {/* Fallback */}
                        <td className="py-3.5 px-4 font-mono text-xs">
                          {route.fallback_model ? (
                            <span className="inline-flex items-center space-x-1 text-sky-700 bg-sky-50 border border-sky-200 px-2 py-0.5 rounded-lg text-[11px] font-medium">
                              <Shield className="w-3 h-3 text-sky-600" />
                              <span>降级: {route.fallback_model}</span>
                            </span>
                          ) : (
                            <span className="text-slate-400 text-[11px]">直通返回</span>
                          )}
                        </td>

                        {/* Pricing */}
                        <td className="py-3.5 px-4 font-sans text-xs">
                          {price ? (
                            <div className="flex items-center space-x-1.5 text-[11px]">
                              <span className="text-slate-700 font-mono">
                                ¥{price.prompt_price.toFixed(1)}/¥{price.completion_price.toFixed(1)}
                              </span>
                              {price.off_peak_enabled !== false && (
                                <span className="bg-indigo-50 border border-indigo-200 text-indigo-700 px-1.5 py-0.2 rounded font-medium text-[10px]">
                                  🌙 闲时优惠
                                </span>
                              )}
                            </div>
                          ) : (
                            <span className="text-slate-400 text-[11px]">基准计费</span>
                          )}
                        </td>

                        {/* Actions */}
                        <td className="py-3.5 px-5 text-right space-x-1.5 font-sans">
                          <button
                            onClick={() => handleOpenEdit(route)}
                            className="px-2.5 py-1 rounded-lg bg-indigo-50 hover:bg-indigo-100 text-indigo-700 text-xs font-medium transition cursor-pointer"
                          >
                            配置分发
                          </button>
                          <button
                            onClick={() => setExpandedModel(isExpanded ? null : route.model)}
                            className="p-1 rounded-lg hover:bg-slate-100 text-slate-400 hover:text-slate-700 transition cursor-pointer inline-flex items-center"
                            title={isExpanded ? '收起详情' : '展开服务商明细'}
                          >
                            {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                          </button>
                          <button
                            onClick={() => handleDeleteRoute(route.model)}
                            className="p-1 rounded-lg hover:bg-rose-50 text-slate-400 hover:text-rose-600 transition cursor-pointer inline-flex items-center"
                            title="删除路由"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </td>
                      </tr>

                      {/* Expandable Provider Topology Detail Row */}
                      {isExpanded && (
                        <tr className="bg-slate-50/70 border-b border-slate-200">
                          <td colSpan="6" className="p-4 pl-8 space-y-3">
                            <div className="flex items-center justify-between text-xs">
                              <span className="font-semibold text-slate-800">
                                上游服务商明细与实时连通性探测 ({route.providers?.length || 0})
                              </span>
                              <button
                                onClick={() => handleProbeRoute(route.model)}
                                disabled={probingModel === route.model}
                                className="px-2.5 py-1 rounded-lg bg-white border border-slate-200 hover:bg-slate-100 text-xs font-medium text-slate-700 flex items-center space-x-1 transition cursor-pointer"
                              >
                                <Zap className={`w-3 h-3 text-amber-500 ${probingModel === route.model ? 'animate-pulse' : ''}`} />
                                <span>{probingModel === route.model ? '探测中...' : '测试所有下游延迟'}</span>
                              </button>
                            </div>

                            <div className="border border-slate-200 rounded-xl overflow-hidden bg-white text-xs">
                              <table className="w-full text-left">
                                <thead className="bg-slate-50 text-slate-500 text-[11px] border-b border-slate-200">
                                  <tr>
                                    <th className="py-2 px-4 font-semibold">服务商名称</th>
                                    <th className="py-2 px-4 font-semibold">服务引擎</th>
                                    <th className="py-2 px-4 font-semibold">分流权重</th>
                                    <th className="py-2 px-4 font-semibold">远端模型映射</th>
                                    <th className="py-2 px-4 font-semibold text-right">健康状态 / 探测延迟</th>
                                  </tr>
                                </thead>
                                <tbody className="divide-y divide-slate-100 font-mono text-[11px]">
                                  {(route.providers || []).map((p) => {
                                    const probe = probes.find(pr => pr.channel_id === p.channel_id);
                                    return (
                                      <tr key={p.channel_id} className="hover:bg-slate-50/50">
                                        <td className="py-2 px-4 font-sans font-semibold text-slate-800">
                                          {p.channel_name}
                                        </td>
                                        <td className="py-2 px-4 text-slate-500">
                                          {p.channel_type}
                                        </td>
                                        <td className="py-2 px-4 text-indigo-600 font-bold">
                                          {p.weight_percent ? `${p.weight_percent}%` : `权重: ${p.weight}`}
                                        </td>
                                        <td className="py-2 px-4 text-slate-600">
                                          {p.mapped_model || route.model}
                                        </td>
                                        <td className="py-2 px-4 text-right font-sans">
                                          {probe ? (
                                            <span className={`font-mono font-semibold ${probe.status === 'healthy' ? 'text-emerald-600' : 'text-rose-600'}`}>
                                              {probe.latency_ms} ms ({probe.status})
                                            </span>
                                          ) : (
                                            <span className="text-emerald-700 bg-emerald-50 px-1.5 py-0.2 rounded border border-emerald-200 text-[10px]">
                                              正常就绪
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
        />
      )}
    </div>
  );
}
