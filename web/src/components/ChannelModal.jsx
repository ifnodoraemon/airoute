import React, { useState } from 'react';
import { X, Sparkles, RefreshCw, Search, Eye, EyeOff } from 'lucide-react';
import ModelChipManager from './ModelChipManager';
import ModelMappingEditor from './ModelMappingEditor';
import CapabilitiesSelector from './CapabilitiesSelector';
import useModalA11y from '../hooks/useModalA11y';

export default function ChannelModal({
  isOpen,
  onClose,
  editingChannelId,
  newChannel,
  setNewChannel,
  probeAlert,
  probing,
  applyPreset,
  handleProbeChannel,
  toggleProtocol,
  onSubmit,
  lang = 'zh',
  t
}) {
  useModalA11y(isOpen, onClose);
  const [showApiKeyPlain, setShowApiKeyPlain] = useState(false);

  if (!isOpen) return null;
  const isZh = lang === 'zh';

  return (
    <div
      className="fixed inset-0 z-50 overflow-hidden bg-slate-900/40 backdrop-blur-sm animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <form
        onSubmit={onSubmit}
        className="fixed inset-y-0 right-0 w-full max-w-xl bg-white dark:bg-[#0F1420] border-l border-slate-200/70 dark:border-slate-800/80 shadow-2xl flex flex-col h-full animate-in slide-in-from-right duration-200"
      >
        <div className="p-6 border-b border-slate-100 dark:border-slate-800/80 flex items-center justify-between">
          <div>
            <h3 className="font-semibold text-base text-slate-900 dark:text-white tracking-tight">
              {editingChannelId ? (isZh ? '编辑服务商' : 'Edit Upstream Provider') : (isZh ? '接入服务商' : 'Connect New Provider')}
            </h3>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
              {isZh ? '配置上游模型服务商端点、鉴权密钥与协议映射' : 'Configure upstream endpoint, authentication credentials, and protocol mappings'}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Scrollable Body */}
        <div className="p-6 space-y-4 overflow-y-auto flex-1 text-xs">
          {/* Probe Notification */}
          {probeAlert && (
            <div
              className={`p-3.5 rounded-lg text-xs font-medium border ${
                probeAlert.type === 'success'
                  ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-800 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                  : probeAlert.type === 'warning'
                  ? 'bg-amber-50 dark:bg-amber-950/50 text-amber-800 dark:text-amber-300 border-amber-200 dark:border-amber-800'
                  : 'bg-rose-50 dark:bg-rose-950/50 text-rose-800 dark:text-rose-300 border-rose-200 dark:border-rose-800'
              }`}
            >
              {probeAlert.text}
            </div>
          )}

          {/* Field 0: 1-Click Provider Quick Presets */}
          {!editingChannelId && (
            <div className="p-3.5 bg-slate-50 dark:bg-slate-900/60 rounded-xl border border-slate-200/80 dark:border-slate-800/80 space-y-2">
              <div className="flex items-center justify-between">
                <span className="font-medium text-slate-700 dark:text-slate-300 text-[11px] flex items-center space-x-1.5">
                  <Sparkles className="w-3.5 h-3.5 text-slate-400" />
                  <span>{isZh ? '快捷预设服务商 (点击一键自动填入):' : 'Quick Presets (1-click autofill):'}</span>
                </span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {[
                  { key: 'deepseek', label: 'DeepSeek', badge: isZh ? '官方' : 'Official' },
                  { key: 'gpustack', label: 'GPUStack', badge: isZh ? '私有集群' : 'Cluster' },
                  { key: 'zhipu', label: '智谱 GLM', badge: isZh ? '主流' : 'GLM' },
                  { key: 'doubao', label: '火山豆包', badge: isZh ? '多模态' : 'Multimodal' },
                  { key: 'moonshot', label: '月之暗面 Kimi', badge: isZh ? '长文本' : 'Kimi' },
                  { key: 'openai', label: 'OpenAI', badge: isZh ? '全协议' : 'Full Suite' },
                  { key: 'anthropic', label: 'Claude', badge: 'Anthropic' },
                  { key: 'gemini', label: 'Google Gemini', badge: isZh ? '多模态' : 'Multimodal' },
                  { key: 'ollama', label: 'Ollama', badge: isZh ? '本地开源' : 'Local' },
                  { key: 'vllm', label: 'vLLM', badge: isZh ? '自建集群' : 'vLLM' },
                ].map(p => (
                  <button
                    key={p.key}
                    type="button"
                    onClick={() => applyPreset(p.key)}
                    className="px-2.5 py-1 rounded-md border border-slate-200/80 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-slate-50 dark:hover:bg-slate-800 text-slate-700 dark:text-slate-300 text-xs font-medium transition flex items-center space-x-1 cursor-pointer shadow-sm"
                  >
                    <span>{p.label}</span>
                    <span className="text-[10px] text-slate-400 dark:text-slate-500 font-normal">({p.badge})</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {/* Field 1: Name */}
          <div>
            <label className="block font-medium text-slate-700 dark:text-slate-300 mb-1">
              {isZh ? '服务商名称' : 'Provider Name'} <span className="text-rose-500">*</span>
            </label>
            <input
              required
              value={newChannel.name}
              onChange={(e) => setNewChannel({ ...newChannel, name: e.target.value })}
              placeholder="gpustack-primary"
              className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-3 py-2 text-slate-800 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:border-indigo-500 text-xs shadow-sm transition"
            />
          </div>

          {/* Field 2: Base URL + Test Ping */}
          <div>
            <label className="block font-medium text-slate-700 dark:text-slate-300 mb-1">
              {isZh ? '服务商 Base URL' : 'Provider Base URL'} <span className="text-rose-500">*</span>
            </label>
            <div className="flex space-x-2">
              <input
                required
                value={newChannel.base_url}
                onChange={(e) => {
                  const val = e.target.value;
                  const lower = val.toLowerCase();
                  const updates = { base_url: val };
                  if (!newChannel.name || newChannel.name === 'gpustack-primary') {
                    if (lower.includes('deepseek')) updates.name = 'deepseek-provider';
                    else if (lower.includes('anthropic')) updates.name = 'claude-provider';
                    else if (lower.includes('openai')) updates.name = 'openai-provider';
                    else if (lower.includes('gemini') || lower.includes('googleapis')) updates.name = 'gemini-provider';
                    else if (lower.includes('ollama') || lower.includes('11434')) updates.name = 'ollama-local';
                    else if (lower.includes('gpustack')) updates.name = 'gpustack-cluster';
                    else if (lower.includes('zhipu') || lower.includes('bigmodel')) updates.name = 'zhipu-glm';
                    else if (lower.includes('doubao') || lower.includes('volces')) updates.name = 'doubao-provider';
                    else if (lower.includes('moonshot') || lower.includes('kimi')) updates.name = 'kimi-provider';
                  }
                  if (lower.includes('anthropic')) updates.type = 'anthropic';
                  else if (lower.includes('deepseek')) updates.type = 'deepseek';
                  else if (lower.includes('gemini') || lower.includes('googleapis')) updates.type = 'gemini';
                  else if (lower.includes('ollama') || lower.includes('11434')) updates.type = 'ollama';
                  else if (lower.includes('gpustack')) updates.type = 'gpustack';
                  setNewChannel(prev => ({ ...prev, ...updates }));
                }}
                placeholder={
                  newChannel.type === 'ollama' ? (isZh ? '例如: http://<宿主机IP或容器服务名>:11434/v1' : 'e.g. http://<host-ip-or-container>:11434/v1') :
                  newChannel.type === 'vllm' ? (isZh ? '例如: http://<宿主机IP或集群域名>:8000/v1' : 'e.g. http://<host-ip-or-cluster>:8000/v1') :
                  newChannel.type === 'gpustack' ? (isZh ? '例如: http://<GPUStack服务IP或集群域名>/v1-openai' : 'e.g. http://<gpustack-ip-or-domain>/v1-openai') :
                  (isZh ? 'https://api.deepseek.com 或私有网关端点' : 'https://api.deepseek.com or private gateway endpoint')
                }
                className="flex-1 bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg px-3 py-2 text-slate-800 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:border-indigo-500 font-mono text-xs shadow-sm transition"
              />
              <button
                type="button"
                onClick={handleProbeChannel}
                disabled={probing}
                className="px-3.5 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium flex items-center space-x-1.5 shadow-sm transition disabled:opacity-50 shrink-0 cursor-pointer"
              >
                {probing ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
                <span>{probing ? (isZh ? '连通中...' : 'Probing...') : (isZh ? '测试连通性' : 'Test Ping')}</span>
              </button>
            </div>
          </div>

          {/* Field 3: API Key */}
          <div>
            <label className="block font-medium text-slate-700 dark:text-slate-300 mb-1">
              API Key
            </label>
            <div className="relative">
              <input
                type={showApiKeyPlain ? 'text' : 'password'}
                value={newChannel.api_key}
                onChange={(e) => setNewChannel({ ...newChannel, api_key: e.target.value })}
                placeholder={isZh ? '留空或填 none 表示免密' : 'Leave empty or none for no auth'}
                className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/80 dark:border-slate-800 rounded-lg pl-3 pr-10 py-2 text-slate-800 dark:text-slate-100 placeholder-slate-400 focus:outline-none focus:border-indigo-500 font-mono text-xs shadow-sm transition"
              />
              <button
                type="button"
                onClick={() => setShowApiKeyPlain(!showApiKeyPlain)}
                className="absolute right-3 top-2.5 text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition cursor-pointer"
                title={showApiKeyPlain ? (isZh ? '隐藏密钥' : 'Hide Key') : (isZh ? '显示明文' : 'Show Plaintext')}
              >
                {showApiKeyPlain ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          {/* Field 4: Models */}
          <ModelChipManager
            models={newChannel.models_str}
            onChange={(list) => setNewChannel(prev => ({ ...prev, models_str: list.join(', ') }))}
            lang={lang}
          />

          {/* Collapsible Advanced Settings */}
          <details className="group border border-slate-200/80 dark:border-slate-800 rounded-xl p-3.5 bg-slate-50/50 dark:bg-slate-900/40">
            <summary className="font-medium text-slate-700 dark:text-slate-300 cursor-pointer flex items-center justify-between select-none">
              <span>{isZh ? '高级设置' : 'Advanced Settings'}</span>
              <span className="text-slate-400 group-open:rotate-180 transition-transform text-xs">▼</span>
            </summary>

            <div className="pt-3 space-y-3.5 border-t border-slate-200/60 dark:border-slate-800 mt-2.5">
              <div>
                <label className="block font-medium text-slate-700 dark:text-slate-300 mb-1">{isZh ? '服务引擎类型' : 'Engine Type'}</label>
                <select
                  value={newChannel.type}
                  onChange={(e) => setNewChannel({ ...newChannel, type: e.target.value })}
                  className="w-full bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-3 py-2 text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 text-xs shadow-sm"
                >
                  <option value="gpustack">GPUStack</option>
                  <option value="deepseek">DeepSeek</option>
                  <option value="openai">{isZh ? 'OpenAI (及所有兼容服务商)' : 'OpenAI (and compatible)'}</option>
                  <option value="anthropic">Claude / Anthropic</option>
                  <option value="gemini">Gemini</option>
                  <option value="vllm">vLLM</option>
                  <option value="sglang">SGLang</option>
                  <option value="ollama">Ollama</option>
                  <option value="sub2api">Sub2API</option>
                  <option value="custom">{isZh ? '自定义兼容引擎 (Custom Engine)' : 'Custom / Compatible Engine'}</option>
                </select>
              </div>

              <ModelMappingEditor
                mappingStr={newChannel.mapping_str}
                onChange={(val) => setNewChannel(prev => ({ ...prev, mapping_str: val }))}
                lang={lang}
              />

              <CapabilitiesSelector
                protocols={newChannel.protocols}
                onToggle={toggleProtocol}
                onSetProtocols={(list) => setNewChannel(prev => ({ ...prev, protocols: list }))}
                lang={lang}
              />

              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-[11px] text-slate-500 mb-1">{isZh ? '优先级 (1最先)' : 'Priority (1 highest)'}</label>
                  <input
                    type="number"
                    value={newChannel.priority}
                    onChange={(e) => setNewChannel({ ...newChannel, priority: parseInt(e.target.value) || 1 })}
                    className="w-full bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 shadow-sm"
                  />
                </div>
                <div>
                  <label className="block text-[11px] text-slate-500 mb-1">{isZh ? '负载权重' : 'Weight'}</label>
                  <input
                    type="number"
                    value={newChannel.weight}
                    onChange={(e) => setNewChannel({ ...newChannel, weight: parseInt(e.target.value) || 10 })}
                    className="w-full bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 shadow-sm"
                  />
                </div>
                <div>
                  <label className="block text-[11px] text-slate-500 mb-1">{isZh ? '超时时间 (秒)' : 'Timeout (s)'}</label>
                  <input
                    type="number"
                    value={newChannel.timeout_seconds || 60}
                    onChange={(e) => setNewChannel({ ...newChannel, timeout_seconds: parseInt(e.target.value) || 60 })}
                    className="w-full bg-white dark:bg-[#0F1420] border border-slate-200 dark:border-slate-800 rounded-lg px-2.5 py-1.5 font-mono text-xs text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 shadow-sm"
                  />
                </div>
              </div>
            </div>
          </details>
        </div>

        {/* Sticky Footer */}
        <div className="p-4 bg-slate-50/70 dark:bg-slate-900/50 border-t border-slate-100 dark:border-slate-800/80 flex items-center justify-end space-x-3 mt-auto">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-xs text-slate-500 hover:text-slate-800 dark:hover:text-slate-200 font-medium transition cursor-pointer"
          >
            {isZh ? '取消' : 'Cancel'}
          </button>
          <button
            type="submit"
            className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium shadow-sm transition cursor-pointer"
          >
            {editingChannelId ? (isZh ? '保存配置' : 'Save Changes') : (isZh ? '确认接入' : 'Connect Provider')}
          </button>
        </div>
      </form>
    </div>
  );
}
