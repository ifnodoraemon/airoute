import React, { useState } from 'react';
import {
  X,
  Plus,
  Trash2,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  Cpu,
  DollarSign,
  Shield,
  Layers,
  Moon
} from 'lucide-react';

export default function ModelRouteModal({
  isOpen,
  onClose,
  modelRoute,
  allModels = [],
  allChannels = [],
  onSaveSuccess,
  adminFetch,
  showToast
}) {
  if (!isOpen) return null;

  const isNew = !modelRoute || modelRoute.isNew;
  const [modelName, setModelName] = useState(modelRoute?.model || '');
  const [fallbackModel, setFallbackModel] = useState(modelRoute?.fallback_model || '');
  
  // Model pricing rates & Time-of-use
  const [promptPrice, setPromptPrice] = useState(modelRoute?.prompt_price ?? 2.0);
  const [completionPrice, setCompletionPrice] = useState(modelRoute?.completion_price ?? 8.0);
  const [cacheReadPrice, setCacheReadPrice] = useState(modelRoute?.cache_read_price ?? 0.2);
  const [currency, setCurrency] = useState(modelRoute?.currency || 'CNY');
  const [offPeakEnabled, setOffPeakEnabled] = useState(modelRoute?.off_peak_enabled !== false);

  // Bound upstream providers
  const [providers, setProviders] = useState(() => {
    return (modelRoute?.providers || []).map(p => ({
      channel_id: p.channel_id,
      channel_name: p.channel_name,
      channel_type: p.channel_type,
      priority: p.priority || 1,
      weight: p.weight || 50,
      mapped_model: p.mapped_model || modelRoute?.model || ''
    }));
  });

  const [selectedChannelToAdd, setSelectedChannelToAdd] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleWeightChange = (channelId, val) => {
    const num = Math.max(1, parseInt(val) || 1);
    setProviders(prev => prev.map(p => p.channel_id === channelId ? { ...p, weight: num } : p));
  };

  const handleMappedModelChange = (channelId, name) => {
    setProviders(prev => prev.map(p => p.channel_id === channelId ? { ...p, mapped_model: name } : p));
  };

  const handleRemoveProvider = (channelId) => {
    setProviders(prev => prev.filter(p => p.channel_id !== channelId));
  };

  const handleAddChannel = () => {
    if (!selectedChannelToAdd) return;
    const chId = parseInt(selectedChannelToAdd);
    const targetChannel = allChannels.find(c => c.id === chId);
    if (!targetChannel) return;

    if (providers.some(p => p.channel_id === chId)) {
      showToast('该服务商渠道已绑定在此模型中', 'warning');
      return;
    }

    setProviders(prev => [
      ...prev,
      {
        channel_id: targetChannel.id,
        channel_name: targetChannel.name,
        channel_type: targetChannel.type,
        priority: 1,
        weight: 50,
        mapped_model: modelName.trim() || targetChannel.name
      }
    ]);
    setSelectedChannelToAdd('');
  };

  const availableChannelsToAdd = allChannels.filter(
    ch => !providers.some(p => p.channel_id === ch.id)
  );

  // Extract all distinct models from connected channels for zero-typing 1-click routing
  const channelModelCandidates = React.useMemo(() => {
    const list = [];
    (allChannels || []).forEach(ch => {
      let mList = [];
      if (Array.isArray(ch.models)) mList = ch.models;
      else if (typeof ch.models === 'string') mList = ch.models.split(',').map(s => s.trim()).filter(Boolean);
      else if (typeof ch.models_str === 'string') mList = ch.models_str.split(',').map(s => s.trim()).filter(Boolean);

      mList.forEach(m => {
        if (!list.some(item => item.model === m)) {
          list.push({ model: m, channel: ch });
        }
      });
    });
    return list;
  }, [allChannels]);

  const handleSelectCandidateModel = (item) => {
    setModelName(item.model);
    setProviders([{
      channel_id: item.channel.id,
      channel_name: item.channel.name,
      channel_type: item.channel.type,
      priority: 1,
      weight: 50,
      mapped_model: item.model
    }]);

    const lower = item.model.toLowerCase();
    if (lower.includes('image') || lower.includes('seedream') || lower.includes('dall-e') || lower.includes('flux')) {
      setPromptPrice(0.10);
      setCompletionPrice(0.10);
      setCacheReadPrice(0.01);
    } else if (lower.includes('video') || lower.includes('seedance') || lower.includes('wan') || lower.includes('kling') || lower.includes('cogvideo') || lower.includes('sora')) {
      setPromptPrice(0.50);
      setCompletionPrice(0.50);
      setCacheReadPrice(0.05);
    } else if (lower.includes('embed') || lower.includes('bge-') || lower.includes('rerank')) {
      setPromptPrice(0.05);
      setCompletionPrice(0.05);
      setCacheReadPrice(0.005);
    }
  };

  const handleSave = async (e) => {
    e.preventDefault();
    setError('');

    const targetModel = modelName.trim();
    if (!targetModel) {
      setError('请输入模型标识 (例如 deepseek-chat 或 gpt-4o)');
      return;
    }

    setSaving(true);
    try {
      const payload = {
        model: targetModel,
        fallback_model: fallbackModel === targetModel ? '' : fallbackModel,
        provider_updates: providers.map(p => ({
          channel_id: p.channel_id,
          priority: p.priority || 1,
          weight: parseInt(p.weight) || 50,
          mapped_model: (p.mapped_model || '').trim() || targetModel
        })),
        prompt_price: parseFloat(promptPrice) || 0,
        completion_price: parseFloat(completionPrice) || 0,
        cache_read_price: parseFloat(cacheReadPrice) || 0,
        fixed_price: 0,
        currency: currency,
        off_peak_enabled: offPeakEnabled,
        off_peak_mode: offPeakEnabled ? (modelRoute?.off_peak_mode || 'custom') : 'none',
        off_peak_discount: modelRoute?.off_peak_discount ?? 0.5,
        off_peak_start: modelRoute?.off_peak_start || '00:00',
        off_peak_end: modelRoute?.off_peak_end || '08:30',
        off_peak_slots: modelRoute?.off_peak_slots || ''
      };

      const res = await adminFetch('/api/v1/admin/models/routes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const data = await res.json();

      if (res.ok && data.code === 0) {
        showToast(`模型 [${targetModel}] 路由配置已保存并完成热重载！`, 'success');
        onSaveSuccess();
        onClose();
      } else {
        setError(data.error || '保存失败，请稍后重试');
      }
    } catch (err) {
      setError('网络异常: ' + err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-4 z-50 backdrop-blur-sm animate-in fade-in">
      <div className="bg-white border border-slate-200 rounded-3xl p-6 max-w-xl w-full shadow-2xl space-y-4 animate-in zoom-in-95 duration-150 max-h-[90vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-100 pb-3 shrink-0">
          <div className="flex items-center space-x-2.5">
            <div className="w-8 h-8 rounded-xl bg-indigo-50 border border-indigo-200 flex items-center justify-center text-indigo-600 font-bold">
              <Cpu className="w-4 h-4" />
            </div>
            <div>
              <h3 className="font-bold text-base text-slate-900 tracking-tight">
                {isNew ? '新建模型路由' : '配置模型路由'}
              </h3>
              {!isNew && (
                <span className="font-mono text-xs text-indigo-600 font-medium">
                  {modelName}
                </span>
              )}
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-slate-100 rounded-xl text-slate-400 hover:text-slate-600 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {error && (
          <div className="p-3 rounded-xl bg-rose-50 border border-rose-200 text-rose-800 text-xs flex items-center space-x-2 shrink-0">
            <AlertCircle className="w-4 h-4 shrink-0 text-rose-600" />
            <span>{error}</span>
          </div>
        )}

        {/* Form Body */}
        <form onSubmit={handleSave} className="space-y-4 overflow-y-auto pr-1 flex-1 text-xs">
          {/* Field 1: Model Identifier */}
          <div>
            <label className="block font-semibold text-slate-800 mb-1">
              模型标识 <span className="text-rose-500">*</span>
            </label>
            <input
              type="text"
              required
              disabled={!isNew}
              list="channel-models-datalist"
              value={modelName}
              onChange={(e) => setModelName(e.target.value)}
              placeholder="如 deepseek-chat 或从下方一键选择"
              className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-2 font-mono text-xs text-slate-900 focus:outline-none focus:border-indigo-500 disabled:bg-slate-100 disabled:text-slate-500"
            />
            <datalist id="channel-models-datalist">
              {channelModelCandidates.map(c => (
                <option key={c.model} value={c.model}>
                  {c.channel.name} ({c.channel.type})
                </option>
              ))}
            </datalist>

            {/* Zero-typing: Quick select from connected channels */}
            {isNew && channelModelCandidates.length > 0 && (
              <div className="mt-2 pt-2 border-t border-slate-100">
                <span className="text-[11px] text-slate-500 font-medium block mb-1.5">
                  已连通服务商的模型 (点击自动填入并直接绑定服务商):
                </span>
                <div className="flex flex-wrap gap-1.5 max-h-24 overflow-y-auto">
                  {channelModelCandidates.map((c) => {
                    const isSelected = modelName === c.model;
                    return (
                      <button
                        key={c.model}
                        type="button"
                        onClick={() => handleSelectCandidateModel(c)}
                        className={`px-2 py-1 rounded-lg border text-[11px] font-mono transition cursor-pointer flex items-center space-x-1 ${
                          isSelected
                            ? 'bg-indigo-600 text-white border-indigo-600 font-semibold shadow-2xs'
                            : 'bg-white hover:bg-indigo-50 border-slate-200 text-slate-700 hover:text-indigo-700 hover:border-indigo-200'
                        }`}
                      >
                        <span>{c.model}</span>
                        <span className={`text-[10px] ${isSelected ? 'text-indigo-200' : 'text-slate-400'}`}>
                          ({c.channel.name})
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>

          {/* Field 2: Upstream Providers & Weights */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="font-semibold text-slate-800 flex items-center space-x-1.5">
                <Layers className="w-3.5 h-3.5 text-indigo-500" />
                <span>上游服务商 ({providers.length})</span>
              </label>
            </div>

            {providers.length === 0 ? (
              <div className="p-3 rounded-xl bg-slate-50 border border-dashed border-slate-200 text-center text-slate-400 text-xs">
                未绑定服务商
              </div>
            ) : (
              <div className="border border-slate-200 rounded-xl overflow-hidden divide-y divide-slate-100">
                {providers.map((p) => (
                  <div key={p.channel_id} className="p-2.5 flex items-center justify-between gap-3 bg-white hover:bg-slate-50/50">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center space-x-2">
                        <span className="font-semibold text-slate-800 truncate">{p.channel_name}</span>
                        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-slate-100 text-slate-500">
                          {p.channel_type}
                        </span>
                      </div>
                      <input
                        type="text"
                        value={p.mapped_model}
                        onChange={(e) => handleMappedModelChange(p.channel_id, e.target.value)}
                        placeholder="远端模型标识 (可选)"
                        className="mt-1 w-full bg-slate-50 border border-slate-200 rounded px-2 py-0.5 text-[11px] font-mono text-slate-700 focus:outline-none focus:border-indigo-400"
                      />
                    </div>

                    <div className="flex items-center space-x-2 shrink-0">
                      <div className="flex items-center space-x-1">
                        <span className="text-[11px] text-slate-400">权重:</span>
                        <input
                          type="number"
                          min="1"
                          max="1000"
                          value={p.weight}
                          onChange={(e) => handleWeightChange(p.channel_id, e.target.value)}
                          className="w-14 bg-slate-50 border border-slate-200 rounded px-1.5 py-1 text-center font-mono text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                        />
                      </div>

                      <button
                        type="button"
                        onClick={() => handleRemoveProvider(p.channel_id)}
                        className="p-1 text-slate-400 hover:text-rose-600 hover:bg-rose-50 rounded transition cursor-pointer"
                        title="移除服务商"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* Quick Add Provider Row */}
            {availableChannelsToAdd.length > 0 && (
              <div className="flex items-center space-x-2 pt-1">
                <select
                  value={selectedChannelToAdd}
                  onChange={(e) => setSelectedChannelToAdd(e.target.value)}
                  className="flex-1 bg-slate-50 border border-slate-200 rounded-xl px-3 py-1.5 text-xs text-slate-800 focus:outline-none focus:border-indigo-500"
                >
                  <option value="">-- 选择上游服务商 --</option>
                  {availableChannelsToAdd.map((ch) => (
                    <option key={ch.id} value={ch.id}>
                      {ch.name} ({ch.type})
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  onClick={handleAddChannel}
                  disabled={!selectedChannelToAdd}
                  className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-700 disabled:bg-slate-200 disabled:text-slate-400 text-white rounded-xl font-semibold shadow-xs flex items-center space-x-1 transition shrink-0 cursor-pointer"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>添加</span>
                </button>
              </div>
            )}
          </div>

          {/* Field 3: Fallback & Pricing (Clean and Compact) */}
          <div className="border-t border-slate-100 pt-3 space-y-3">
            {/* Fallback Model */}
            <div>
              <label className="font-semibold text-slate-800 flex items-center space-x-1.5 mb-1">
                <Shield className="w-3.5 h-3.5 text-indigo-500" />
                <span>容灾降级</span>
              </label>
              <select
                value={fallbackModel}
                onChange={(e) => setFallbackModel(e.target.value)}
                className="w-full bg-slate-50 border border-slate-200 rounded-xl px-3 py-1.5 text-xs font-mono text-slate-800 focus:outline-none focus:border-indigo-500"
              >
                <option value="">-- 无 (直接返回错误) --</option>
                {allModels
                  .filter(m => m !== modelName)
                  .map(m => (
                    <option key={m} value={m}>
                      降级至: {m}
                    </option>
                  ))}
              </select>
            </div>

            {/* Pricing Row */}
            <div>
              <label className="font-semibold text-slate-800 flex items-center space-x-1.5 mb-1.5">
                <DollarSign className="w-3.5 h-3.5 text-emerald-600" />
                <span>计费单价 (元/1M)</span>
              </label>
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <span className="block text-[10px] text-slate-400 mb-0.5">输入</span>
                  <input
                    type="number"
                    step="0.01"
                    min="0"
                    value={promptPrice}
                    onChange={(e) => setPromptPrice(e.target.value)}
                    placeholder="2.00"
                    className="w-full bg-slate-50 border border-slate-200 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-900 focus:outline-none focus:border-indigo-500"
                  />
                </div>
                <div>
                  <span className="block text-[10px] text-slate-400 mb-0.5">输出</span>
                  <input
                    type="number"
                    step="0.01"
                    min="0"
                    value={completionPrice}
                    onChange={(e) => setCompletionPrice(e.target.value)}
                    placeholder="8.00"
                    className="w-full bg-slate-50 border border-slate-200 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-900 focus:outline-none focus:border-indigo-500"
                  />
                </div>
                <div>
                  <span className="block text-[10px] text-emerald-700 mb-0.5">缓存读取</span>
                  <input
                    type="number"
                    step="0.01"
                    min="0"
                    value={cacheReadPrice}
                    onChange={(e) => setCacheReadPrice(e.target.value)}
                    placeholder="0.20"
                    className="w-full bg-slate-50 border border-emerald-300 rounded-xl px-2.5 py-1.5 font-mono text-xs text-slate-900 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              {/* Time-of-use discount toggle */}
              <div className="mt-2.5 p-3 bg-slate-50 border border-slate-200 rounded-xl flex items-center justify-between">
                <div>
                  <span className="block text-xs font-semibold text-slate-700">分时优惠</span>
                  <span className="block text-[11px] text-slate-400">开启后享受优惠费率，可在「模型定价」自定义任意时间段</span>
                </div>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={offPeakEnabled}
                    onChange={(e) => setOffPeakEnabled(e.target.checked)}
                    className="sr-only peer"
                  />
                  <div className="w-9 h-5 bg-slate-200 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-slate-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-indigo-600"></div>
                </label>
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="flex justify-end space-x-3 pt-3 border-t border-slate-100 shrink-0">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-1.5 text-slate-500 hover:text-slate-800 font-medium transition cursor-pointer"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={saving}
              className="px-5 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl font-semibold shadow-xs transition flex items-center space-x-1.5 disabled:opacity-50 cursor-pointer"
            >
              {saving ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <CheckCircle2 className="w-3.5 h-3.5" />}
              <span>{isNew ? '创建模型' : '保存'}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
