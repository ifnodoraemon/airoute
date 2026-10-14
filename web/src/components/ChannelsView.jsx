import React, { useState } from 'react';
import {
  Server,
  Activity,
  Plus,
  Trash2,
  Copy,
  Play,
  Edit3,
  RefreshCw,
  Search,
  Filter,
  ShieldAlert,
  CheckCircle2,
  AlertTriangle,
  Cpu,
  Layers
} from 'lucide-react';

export default function ChannelsView({
  channels = [],
  batchTesting,
  handleBatchPing,
  setEditingChannelId,
  setNewChannel,
  setProbeAlert,
  setShowChannelModal,
  selectedChannelIds = [],
  setSelectedChannelIds,
  handleBatchStatusChannels,
  handleBatchDeleteChannels,
  channelLatencies = {},
  copyToClipboard,
  handleTestChannel,
  testingId,
  handleEditChannel,
  handleDeleteChannel,
  showToast,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  const [searchQuery, setSearchQuery] = useState('');
  const [engineFilter, setEngineFilter] = useState('all'); // 'all' | 'gpustack' | 'openai' | 'anthropic' | 'gemini' | 'sub2api'
  const [statusFilter, setStatusFilter] = useState('all'); // 'all' | 'active' | 'disabled' | 'breaker_open'

  const openNewChannelModal = () => {
    setEditingChannelId(null);
    setNewChannel({
      name: '',
      type: 'gpustack',
      base_url: '',
      api_key: '',
      priority: 1,
      weight: 10,
      timeout_seconds: 60,
      models_str: '',
      mapping_str: '',
      protocols: ['openai_chat', 'openai_response', 'openai_text', 'embeddings', 'rerank', 'images'],
    });
    setProbeAlert(null);
    setShowChannelModal(true);
  };

  // Filter channels
  const filteredChannels = channels.filter(ch => {
    if (engineFilter !== 'all' && ch.type !== engineFilter) return false;
    if (statusFilter === 'active' && ch.status !== 'active') return false;
    if (statusFilter === 'disabled' && ch.status === 'active') return false;
    if (statusFilter === 'breaker_open' && ch.breaker_status !== 'OPEN') return false;

    if (searchQuery.trim()) {
      const q = searchQuery.trim().toLowerCase();
      const matchName = (ch.name || '').toLowerCase().includes(q);
      const matchUrl = (ch.base_url || '').toLowerCase().includes(q);
      const matchType = (ch.type || '').toLowerCase().includes(q);
      const matchModels = (ch.models || []).some(m => m.toLowerCase().includes(q));
      if (!matchName && !matchUrl && !matchType && !matchModels) return false;
    }
    return true;
  });

  const activeChannels = channels.filter(ch => ch.status === 'active');
  const openBreakers = channels.filter(ch => ch.breaker_status === 'OPEN');

  const toggleSingleStatus = (ch) => {
    const nextStatus = ch.status === 'active' ? 'disabled' : 'active';
    if (handleBatchStatusChannels) {
      handleBatchStatusChannels(nextStatus, [ch.id]);
    }
  };

  return (
    <div className="space-y-6">
      {/* 1. Header (Directly on canvas) */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h2 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-white">
              {isZh ? '服务商管理' : 'Channel Providers'}
            </h2>
            <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-500 dark:text-slate-400">
              {channels.length} {isZh ? '个节点' : 'nodes'}
            </span>
            {openBreakers.length > 0 && (
              <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 border border-rose-200/60 dark:border-rose-900/40">
                {openBreakers.length} {isZh ? '个熔断' : 'tripped'}
              </span>
            )}
          </div>
          <p className="text-xs text-slate-400 mt-1">
            {isZh
              ? '管理 GPUStack、商业厂商大模型上游凭证与负载权重'
              : 'Manage provider credentials, health checks, and circuit-breaker weights'}
          </p>
        </div>

        <div className="flex items-center space-x-2.5">
          <button
            onClick={handleBatchPing}
            disabled={batchTesting || channels.length === 0}
            className="px-3.5 py-2 bg-white dark:bg-[#0F1420] hover:bg-slate-50 dark:hover:bg-slate-800/80 text-slate-700 dark:text-slate-300 border border-slate-200/70 dark:border-slate-800/80 rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer"
          >
            <Activity className={`w-3.5 h-3.5 text-slate-400 ${batchTesting ? 'animate-spin text-indigo-500' : ''}`} />
            <span>{batchTesting ? (isZh ? '巡检中...' : 'Probing...') : (isZh ? '全量体检' : 'Probe All Health')}</span>
          </button>
          <button
            onClick={openNewChannelModal}
            className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 dark:hover:bg-indigo-500 text-white rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
          </button>
        </div>
      </div>

      {/* 2. Filter Bar (Directly on canvas, no card) */}
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs">
        <div className="relative flex-1 max-w-sm">
          <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={isZh ? '搜索服务商名称 / 引擎 / 地址 / 模型...' : 'Search provider name, engine, URL, models...'}
            className="w-full bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-lg pl-8 pr-3 py-2 text-xs text-slate-800 dark:text-slate-100 shadow-sm focus:outline-none focus:border-indigo-500"
          />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* Engine Type Filter */}
          <select
            value={engineFilter}
            onChange={(e) => setEngineFilter(e.target.value)}
            className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-lg px-3 py-2 text-xs text-slate-700 dark:text-slate-200 font-medium shadow-sm focus:outline-none cursor-pointer"
          >
            <option value="all">{isZh ? '全部引擎' : 'All Engines'}</option>
            <option value="gpustack">GPUStack</option>
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic Claude</option>
            <option value="gemini">Google Gemini</option>
            <option value="sub2api">Sub2API</option>
          </select>

          {/* Status Filter */}
          <div className="inline-flex bg-slate-100 dark:bg-slate-800/60 p-0.5 rounded-lg border border-slate-200/60 dark:border-slate-800/60">
            <button
              onClick={() => setStatusFilter('all')}
              className={`px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                statusFilter === 'all'
                  ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              {isZh ? '全部' : 'All'}
            </button>
            <button
              onClick={() => setStatusFilter('active')}
              className={`px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                statusFilter === 'active'
                  ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              {isZh ? '正常' : 'Active'}
            </button>
            <button
              onClick={() => setStatusFilter('disabled')}
              className={`px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                statusFilter === 'disabled'
                  ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold'
                  : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
            >
              {isZh ? '停用' : 'Disabled'}
            </button>
            {openBreakers.length > 0 && (
              <button
                onClick={() => setStatusFilter('breaker_open')}
                className={`px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
                  statusFilter === 'breaker_open'
                    ? 'bg-rose-50 text-rose-700 shadow-sm font-semibold'
                    : 'text-rose-600 hover:text-rose-800'
                }`}
              >
                {isZh ? `熔断 (${openBreakers.length})` : `Tripped (${openBreakers.length})`}
              </button>
            )}
          </div>
        </div>
      </div>

      {/* 3. Channels Table Content */}
      {channels.length === 0 ? (
        <div className="bg-white dark:bg-[#0F1420] border border-dashed border-slate-200 dark:border-slate-800 rounded-2xl p-12 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-12 h-12 rounded-xl bg-slate-100 dark:bg-slate-800/80 flex items-center justify-center text-slate-400 shadow-sm">
            <Server className="w-6 h-6" />
          </div>
          <h4 className="font-semibold text-sm text-slate-900 dark:text-slate-100">{isZh ? '暂无模型服务商' : 'No Upstream Providers'}</h4>
          <p className="text-xs text-slate-400 max-w-sm">
            {isZh
              ? '接入服务商以挂载算力端点，支持 GPUStack、阿里云百炼、火山方舟、OpenAI 等任意兼容接口。'
              : 'Connect upstream providers to mount compute endpoints, supporting GPUStack, Volcengine, OpenAI, and compatible APIs.'}
          </p>
          <button
            onClick={openNewChannelModal}
            className="px-4 py-2 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 text-white rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {selectedChannelIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-slate-100 dark:bg-slate-800/70 border border-slate-200 dark:border-slate-700 px-5 py-3 rounded-xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-medium text-slate-800 dark:text-slate-200">
                <span>{isZh ? `已选中 ${selectedChannelIds.length} 个服务商` : `Selected ${selectedChannelIds.length} channels`}</span>
              </div>
              <div className="flex items-center space-x-2">
                <button
                  onClick={() => handleBatchStatusChannels('active')}
                  className="px-3 py-1.5 bg-slate-900 dark:bg-indigo-600 hover:bg-slate-800 text-white text-xs font-medium rounded-lg shadow-sm transition cursor-pointer"
                >
                  {isZh ? '批量启用' : 'Batch Enable'}
                </button>
                <button
                  onClick={() => handleBatchStatusChannels('disabled')}
                  className="px-3 py-1.5 bg-slate-200 dark:bg-slate-700 text-slate-700 dark:text-slate-200 hover:bg-slate-300 text-xs font-medium rounded-lg shadow-sm transition cursor-pointer"
                >
                  {isZh ? '批量停用' : 'Batch Disable'}
                </button>
                <button
                  onClick={handleBatchDeleteChannels}
                  className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-medium rounded-lg shadow-sm transition flex items-center space-x-1 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>{isZh ? '批量删除' : 'Batch Delete'}</span>
                </button>
                <button
                  onClick={() => setSelectedChannelIds([])}
                  className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-lg hover:bg-slate-50 transition cursor-pointer"
                >
                  {isZh ? '取消' : 'Cancel'}
                </button>
              </div>
            </div>
          )}

          <div className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-2xl overflow-hidden shadow-sm">
            <div className="overflow-x-auto w-full">
              <table className="w-full text-left border-collapse min-w-[850px]">
              <thead>
                <tr className="border-b border-slate-100 dark:border-slate-800/80 text-slate-400 text-xs font-medium">
                  <th className="py-3 px-4 w-10 text-center">
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-slate-900 focus:ring-slate-500 cursor-pointer"
                      checked={filteredChannels.length > 0 && selectedChannelIds.length === filteredChannels.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedChannelIds(filteredChannels.map((c) => c.id));
                        } else {
                          setSelectedChannelIds([]);
                        }
                      }}
                    />
                  </th>
                  <th className="py-3 px-6">{isZh ? '服务商名称' : 'Provider Name'}</th>
                  <th className="py-3 px-6">{isZh ? '服务引擎' : 'Engine'}</th>
                  <th className="py-3 px-6">{isZh ? 'Base URL' : 'Base URL'}</th>
                  <th className="py-3 px-6">{isZh ? '功能模态' : 'Modalities'}</th>
                  <th className="py-3 px-6">{isZh ? '支持模型' : 'Models'}</th>
                  <th className="py-3 px-6">{isZh ? '优先级 / 权重' : 'Priority / Weight'}</th>
                  <th className="py-3 px-6">{isZh ? '状态' : 'Status'}</th>
                  <th className="py-3 px-6 text-right">{isZh ? '操作' : 'Actions'}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 text-xs">
                {filteredChannels.map((ch) => (
                  <tr key={ch.id} className="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition">
                    <td className="py-4 px-4 text-center">
                      <input
                        type="checkbox"
                        className="rounded border-slate-300 text-slate-900 focus:ring-slate-500 cursor-pointer"
                        checked={selectedChannelIds.includes(ch.id)}
                        onChange={(e) => {
                          e.stopPropagation();
                          if (e.target.checked) {
                            setSelectedChannelIds((prev) => [...prev, ch.id]);
                          } else {
                            setSelectedChannelIds((prev) => prev.filter((x) => x !== ch.id));
                          }
                        }}
                      />
                    </td>
                    <td className="py-4 px-6 font-medium text-slate-900 dark:text-slate-100">
                      <div className="flex items-center space-x-2">
                        <span className="font-medium text-xs text-slate-900 dark:text-white">{ch.name}</span>
                      </div>
                      <div className="flex items-center space-x-2 mt-1">
                        {ch.breaker_status && (
                          <span className="inline-flex items-center space-x-1 text-[11px] font-mono text-slate-400">
                            <span
                              className={`w-1.5 h-1.5 rounded-full ${
                                ch.breaker_status === 'OPEN'
                                  ? 'bg-rose-500'
                                  : ch.breaker_status === 'HALF-OPEN'
                                  ? 'bg-amber-500 animate-pulse'
                                  : 'bg-emerald-500'
                              }`}
                            />
                            <span>{ch.breaker_status}</span>
                          </span>
                        )}
                        {channelLatencies[ch.id] && (
                          <span className="text-[11px] font-mono tabular-nums text-slate-400">
                            {channelLatencies[ch.id].success ? `${channelLatencies[ch.id].latency_ms}ms` : (isZh ? '超时' : 'Timeout')}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="py-4 px-6">
                      <span className="px-2 py-0.5 rounded text-xs font-mono text-slate-600 dark:text-slate-300 bg-slate-100 dark:bg-slate-800/80">
                        {ch.type}
                      </span>
                    </td>
                    <td className="py-4 px-6 text-slate-500 font-mono text-xs max-w-xs">
                      <div className="flex items-center space-x-1.5">
                        <span className="truncate">{ch.base_url}</span>
                        <button
                          onClick={() => copyToClipboard(ch.base_url)}
                          className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition p-0.5 cursor-pointer"
                          title={isZh ? "复制 Base URL" : "Copy Base URL"}
                        >
                          <Copy className="w-3 h-3" />
                        </button>
                      </div>
                    </td>
                    <td className="py-4 px-6">
                      <div className="flex flex-wrap gap-1 max-w-xs">
                        {!ch.protocols || ch.protocols.length === 0 ? (
                          <span className="text-slate-400 text-xs font-mono">
                            {isZh ? '全功能' : 'All'}
                          </span>
                        ) : (
                          ch.protocols.slice(0, 3).map((p) => {
                            let label = p;
                            if (p === 'openai_chat' || p === 'chat') label = isZh ? '对话' : 'Chat';
                            else if (p === 'openai_response' || p === 'responses' || p === 'response') label = 'Response';
                            else if (p === 'openai_text' || p === 'completion') label = isZh ? '补全' : 'Completion';
                            else if (p === 'anthropic_messages' || p === 'messages') label = 'Messages';
                            else if (p === 'images' || p === 'image_generation') label = isZh ? '生图' : 'Images';
                            else if (p === 'audio_speech' || p === 'tts') label = 'TTS';
                            else if (p === 'audio_transcription' || p === 'stt') label = 'STT';
                            else if (p === 'videos' || p === 'video_generation') label = isZh ? '视频' : 'Videos';
                            else if (p === 'embeddings' || p === 'embedding') label = isZh ? '向量' : 'Embeddings';
                            else if (p === 'rerank' || p === 'reranker') label = isZh ? '重排' : 'Rerank';
                            return (
                              <span key={p} className="px-1.5 py-0.5 rounded text-[11px] font-mono text-slate-500 bg-slate-100 dark:bg-slate-800/60">
                                {label}
                              </span>
                            );
                          })
                        )}
                        {ch.protocols && ch.protocols.length > 3 && (
                          <span className="text-[11px] font-mono text-slate-400">
                            +{ch.protocols.length - 3}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="py-4 px-6">
                      <div className="flex flex-wrap gap-1 max-w-xs">
                        {!ch.models || ch.models.length === 0 ? (
                          <span className="text-xs text-slate-400">-</span>
                        ) : (
                          ch.models.slice(0, 2).map((m) => (
                            <span
                              key={m}
                              className="px-1.5 py-0.5 rounded text-[11px] font-mono text-slate-600 dark:text-slate-300 bg-slate-100 dark:bg-slate-800/60"
                            >
                              {m}
                            </span>
                          ))
                        )}
                        {ch.models && ch.models.length > 2 && (
                          <span
                            className="px-1.5 py-0.5 rounded text-[11px] font-mono text-slate-400"
                            title={ch.models.slice(2).join(', ')}
                          >
                            +{ch.models.length - 2}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="py-4 px-6 text-slate-600 dark:text-slate-400 font-mono text-xs tabular-nums">
                      {ch.priority} / {ch.weight}
                    </td>
                    <td className="py-4 px-6">
                      <button
                        onClick={() => toggleSingleStatus(ch)}
                        className="inline-flex items-center space-x-1.5 text-xs text-slate-600 dark:text-slate-300 hover:text-slate-900 transition cursor-pointer"
                        title={isZh ? '点击切换状态' : 'Toggle status'}
                      >
                        <span className={`w-1.5 h-1.5 rounded-full ${ch.status === 'active' ? 'bg-emerald-500' : 'bg-slate-300'}`} />
                        <span>{ch.status === 'active' ? (isZh ? '正常' : 'Active') : (isZh ? '停用' : 'Disabled')}</span>
                      </button>
                    </td>
                    <td className="py-4 px-6 text-right space-x-2">
                      <button
                        onClick={() => handleTestChannel(ch)}
                        disabled={testingId === ch.id}
                        className="text-xs px-2.5 py-1 rounded-md bg-white dark:bg-[#0F1420] text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800/80 border border-slate-200/70 dark:border-slate-800/80 font-medium transition inline-flex items-center space-x-1 cursor-pointer shadow-sm"
                        title={isZh ? '连通性测试' : 'Ping test'}
                      >
                        {testingId === ch.id ? (
                          <RefreshCw className="w-3 h-3 animate-spin text-slate-400" />
                        ) : (
                          <Play className="w-3 h-3 text-slate-400" />
                        )}
                        <span>Ping</span>
                      </button>
                      <button
                        onClick={() => handleEditChannel(ch)}
                        className="text-xs px-2.5 py-1 rounded-md bg-white dark:bg-[#0F1420] text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800/80 border border-slate-200/70 dark:border-slate-800/80 font-medium transition cursor-pointer shadow-sm"
                      >
                        {isZh ? '编辑' : 'Edit'}
                      </button>
                      <button
                        onClick={() => handleDeleteChannel(ch.id)}
                        className="text-xs px-2 py-1 text-slate-400 hover:text-rose-600 transition font-medium cursor-pointer"
                      >
                        {isZh ? '删除' : 'Delete'}
                      </button>
                    </td>
                  </tr>
                ))}
                {filteredChannels.length === 0 && (
                  <tr>
                    <td colSpan="9" className="py-10 text-center text-slate-400 text-xs">
                      {isZh ? '没有找到符合条件的服务商' : 'No matching channels found'}
                      {(searchQuery || engineFilter !== 'all' || statusFilter !== 'all') && (
                        <button
                          onClick={() => { setSearchQuery(''); setEngineFilter('all'); setStatusFilter('all'); }}
                          className="ml-2 text-slate-900 dark:text-white hover:underline font-semibold"
                        >
                          {isZh ? '清除筛选' : 'Clear Filters'}
                        </button>
                      )}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
