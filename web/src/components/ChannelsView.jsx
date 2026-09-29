import React from 'react';
import { Server, Activity, Plus, Trash2, Copy, Play, Edit3, RefreshCw } from 'lucide-react';

export default function ChannelsView({
  channels,
  batchTesting,
  handleBatchPing,
  setEditingChannelId,
  setNewChannel,
  setProbeAlert,
  setShowChannelModal,
  selectedChannelIds,
  setSelectedChannelIds,
  handleBatchStatusChannels,
  handleBatchDeleteChannels,
  channelLatencies,
  copyToClipboard,
  handleTestChannel,
  testingId,
  handleEditChannel,
  handleDeleteChannel,
}) {
  const openNewChannelModal = () => {
    setEditingChannelId(null);
    setNewChannel({
      name: '',
      type: 'gpustack',
      base_url: 'http://10.232.16.83/v1-openai',
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

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center bg-white dark:bg-[#111726] p-5 rounded-3xl border border-slate-200/80 dark:border-slate-800/80 shadow-xs gap-3">
        <div className="flex items-center space-x-2">
          <Server className="w-4 h-4 text-emerald-500" />
          <h3 className="font-semibold text-slate-900 dark:text-slate-100 text-sm">
            模型服务商
          </h3>
        </div>
        <div className="flex items-center space-x-2.5">
          <button
            onClick={handleBatchPing}
            disabled={batchTesting || channels.length === 0}
            className="px-3.5 py-2 bg-emerald-50 dark:bg-emerald-950/40 hover:bg-emerald-100 dark:hover:bg-emerald-900/60 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800/60 rounded-xl text-xs font-semibold shadow-xs flex items-center space-x-1.5 transition disabled:opacity-50"
          >
            <Activity className={`w-3.5 h-3.5 ${batchTesting ? 'animate-spin' : ''}`} />
            <span>{batchTesting ? '体检中...' : '体检全部'}</span>
          </button>
          <button
            onClick={openNewChannelModal}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            <span>接入服务商</span>
          </button>
        </div>
      </div>

      {channels.length === 0 ? (
        <div className="bg-white dark:bg-[#111726] border border-dashed border-slate-300 dark:border-slate-800 rounded-3xl p-10 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-10 h-10 rounded-2xl bg-indigo-50 dark:bg-indigo-950/60 border border-indigo-200 dark:border-indigo-800/80 flex items-center justify-center text-indigo-600 dark:text-indigo-400">
            <Server className="w-5 h-5" />
          </div>
          <h4 className="font-bold text-sm text-slate-900 dark:text-slate-100">暂无服务商</h4>
          <button
            onClick={openNewChannelModal}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white rounded-xl text-xs font-semibold shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            <span>接入服务商</span>
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {selectedChannelIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-indigo-50 dark:bg-indigo-950/40 border border-indigo-200 dark:border-indigo-800/60 px-5 py-3 rounded-2xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-semibold text-indigo-900 dark:text-indigo-200">
                <span>已选中 {selectedChannelIds.length} 项</span>
              </div>
              <div className="flex items-center space-x-2">
                <button
                  onClick={() => handleBatchStatusChannels('active')}
                  className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                >
                  批量启用
                </button>
                <button
                  onClick={() => handleBatchStatusChannels('disabled')}
                  className="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-xs font-semibold rounded-xl shadow-xs transition cursor-pointer"
                >
                  批量停用
                </button>
                <button
                  onClick={handleBatchDeleteChannels}
                  className="px-3 py-1.5 bg-rose-600 hover:bg-rose-700 text-white text-xs font-semibold rounded-xl shadow-xs transition flex items-center space-x-1 cursor-pointer"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>批量删除</span>
                </button>
                <button
                  onClick={() => setSelectedChannelIds([])}
                  className="px-3 py-1.5 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700 text-xs font-medium rounded-xl hover:bg-slate-50 transition cursor-pointer"
                >
                  取消选择
                </button>
              </div>
            </div>
          )}

          <div className="bg-white dark:bg-[#111726] border border-slate-200/80 dark:border-slate-800/80 rounded-3xl overflow-hidden shadow-xs">
            <table className="w-full text-left border-collapse">
              <thead>
                <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 dark:text-slate-400 text-xs uppercase bg-slate-50/80 dark:bg-slate-900/80">
                  <th className="py-3.5 px-4 w-10 text-center">
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
                      checked={channels.length > 0 && selectedChannelIds.length === channels.length}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedChannelIds(channels.map((c) => c.id));
                        } else {
                          setSelectedChannelIds([]);
                        }
                      }}
                    />
                  </th>
                  <th className="py-3.5 px-6 font-semibold">服务商名称 & 健康状态</th>
                  <th className="py-3.5 px-6 font-semibold">服务引擎</th>
                  <th className="py-3.5 px-6 font-semibold">下游 Base URL</th>
                  <th className="py-3.5 px-6 font-semibold">开放功能模态</th>
                  <th className="py-3.5 px-6 font-semibold">挂载模型与别名</th>
                  <th className="py-3.5 px-6 font-semibold">路由优先级 / 权重</th>
                  <th className="py-3.5 px-6 text-right font-semibold">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60 text-sm">
                {channels.map((ch) => (
                  <tr key={ch.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/40 transition">
                    <td className="py-4 px-4 text-center">
                      <input
                        type="checkbox"
                        className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 cursor-pointer"
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
                        <span
                          className={`w-2.5 h-2.5 rounded-full ${
                            ch.breaker_status === 'OPEN'
                              ? 'bg-rose-500 animate-ping'
                              : ch.status === 'active'
                              ? 'bg-emerald-500'
                              : 'bg-slate-400'
                          }`}
                        ></span>
                        <span className="font-semibold">{ch.name}</span>
                      </div>
                      <div className="flex items-center space-x-1.5 mt-1">
                        {ch.breaker_status && (
                          <span
                            className={`inline-block text-[10px] px-2 py-0.5 rounded font-mono font-medium ${
                              ch.breaker_status === 'OPEN'
                                ? 'bg-rose-100 dark:bg-rose-950/60 text-rose-700 dark:text-rose-300'
                                : ch.breaker_status === 'HALF-OPEN'
                                ? 'bg-amber-100 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300'
                                : 'bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300'
                            }`}
                          >
                            Breaker: {ch.breaker_status}
                          </span>
                        )}
                        {channelLatencies[ch.id] && (
                          <span
                            className={`text-[10px] px-2 py-0.5 rounded font-mono font-medium ${
                              channelLatencies[ch.id].success
                                ? 'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800'
                                : 'bg-rose-50 dark:bg-rose-950/50 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-800'
                            }`}
                          >
                            {channelLatencies[ch.id].success ? `🟢 ${channelLatencies[ch.id].latency_ms}ms` : '🔴 异常'}
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="py-4 px-6">
                      <span
                        className={`px-2.5 py-1 rounded-lg text-xs font-mono font-medium border ${
                          ch.type === 'gemini'
                            ? 'bg-blue-50 dark:bg-blue-950/40 text-blue-700 dark:text-blue-300 border-blue-200 dark:border-blue-800'
                            : ch.type === 'anthropic'
                            ? 'bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300 border-amber-200 dark:border-amber-800'
                            : ch.type === 'gpustack'
                            ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 border-emerald-200 dark:border-emerald-800'
                            : ch.type === 'sub2api'
                            ? 'bg-purple-50 dark:bg-purple-950/40 text-purple-700 dark:text-purple-300 border-purple-200 dark:border-purple-800'
                            : 'bg-indigo-50 dark:bg-indigo-950/40 text-indigo-700 dark:text-indigo-300 border-indigo-200 dark:border-indigo-800'
                        }`}
                      >
                        {ch.type}
                      </span>
                    </td>
                    <td className="py-4 px-6 text-slate-600 dark:text-slate-400 font-mono text-xs max-w-xs">
                      <div className="flex items-center space-x-1.5">
                        <span className="truncate">{ch.base_url}</span>
                        <button
                          onClick={() => copyToClipboard(ch.base_url)}
                          className="text-slate-400 hover:text-indigo-500 transition p-1"
                          title="复制 Base URL"
                        >
                          <Copy className="w-3 h-3" />
                        </button>
                      </div>
                    </td>
                    <td className="py-4 px-6">
                      <div className="flex flex-wrap gap-1">
                        {!ch.protocols || ch.protocols.length === 0 ? (
                          <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-[11px] text-slate-600 dark:text-slate-300 font-medium">
                            全功能直通
                          </span>
                        ) : (
                          ch.protocols.map((p) => {
                            let label = p;
                            let colorClass =
                              'bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-200 dark:border-slate-700';
                            if (p === 'openai_chat' || p === 'chat') {
                              label = '💬 对话';
                              colorClass =
                                'bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 border border-indigo-200 dark:border-indigo-800';
                            } else if (p === 'openai_response' || p === 'responses' || p === 'response') {
                              label = '⚡ Responses';
                              colorClass =
                                'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800';
                            } else if (p === 'openai_text' || p === 'completion') {
                              label = '📝 补全';
                              colorClass =
                                'bg-sky-50 dark:bg-sky-950/50 text-sky-700 dark:text-sky-300 border border-sky-200 dark:border-sky-800';
                            } else if (p === 'anthropic_messages' || p === 'messages') {
                              label = '🧠 Claude';
                              colorClass =
                                'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800';
                            } else if (p === 'images' || p === 'image_generation') {
                              label = '🎨 生图';
                              colorClass =
                                'bg-pink-50 dark:bg-pink-950/50 text-pink-700 dark:text-pink-300 border border-pink-200 dark:border-pink-800';
                            } else if (p === 'audio_speech' || p === 'tts') {
                              label = '🔊 TTS';
                              colorClass =
                                'bg-cyan-50 dark:bg-cyan-950/50 text-cyan-700 dark:text-cyan-300 border border-cyan-200 dark:border-cyan-800';
                            } else if (p === 'audio_transcription' || p === 'stt') {
                              label = '🎙️ STT';
                              colorClass =
                                'bg-teal-50 dark:bg-teal-950/50 text-teal-700 dark:text-teal-300 border border-teal-200 dark:border-teal-800';
                            } else if (p === 'videos' || p === 'video_generation') {
                              label = '🎬 视频';
                              colorClass =
                                'bg-purple-50 dark:bg-purple-950/50 text-purple-700 dark:text-purple-300 border border-purple-200 dark:border-purple-800';
                            } else if (p === 'embeddings' || p === 'embedding') {
                              label = '🧠 向量';
                              colorClass =
                                'bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800';
                            } else if (p === 'rerank' || p === 'reranker') {
                              label = '🎯 重排';
                              colorClass =
                                'bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-800';
                            }
                            return (
                              <span key={p} className={`px-2 py-0.5 rounded text-[11px] font-medium ${colorClass}`}>
                                {label}
                              </span>
                            );
                          })
                        )}
                      </div>
                    </td>
                    <td className="py-4 px-6">
                      <div className="flex flex-wrap gap-1 max-w-xs">
                        {!ch.models || ch.models.length === 0 ? (
                          <span className="text-xs text-slate-400">未同步模型</span>
                        ) : (
                          ch.models.slice(0, 4).map((m) => (
                            <span
                              key={m}
                              className="px-2 py-0.5 rounded bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 text-xs font-mono border border-indigo-100 dark:border-indigo-800"
                            >
                              {m}
                            </span>
                          ))
                        )}
                        {ch.models && ch.models.length > 4 && (
                          <span
                            className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300 text-xs font-mono"
                            title={ch.models.slice(4).join(', ')}
                          >
                            +{ch.models.length - 4} 更多
                          </span>
                        )}
                        {ch.model_mapping &&
                          Object.keys(ch.model_mapping).length > 0 &&
                          Object.entries(ch.model_mapping).map(([k, v]) => (
                            <span
                              key={k}
                              className="px-2 py-0.5 rounded bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 text-[11px] font-mono border border-emerald-200 dark:border-emerald-800"
                              title={`别名映射: ${k} -> ${v}`}
                            >
                              🔗 {k} → {v}
                            </span>
                          ))}
                      </div>
                    </td>
                    <td className="py-4 px-6 text-slate-700 dark:text-slate-300 font-mono text-xs">
                      <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-800 dark:text-slate-200">
                        优先级: {ch.priority}
                      </span>
                      <span className="px-2 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-800 dark:text-slate-200 ml-1">
                        权重: {ch.weight}
                      </span>
                    </td>
                    <td className="py-4 px-6 text-right space-x-2">
                      <button
                        onClick={() => handleTestChannel(ch)}
                        disabled={testingId === ch.id}
                        className="text-xs px-3 py-1.5 rounded-xl bg-indigo-50 dark:bg-indigo-950/50 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-900/60 border border-indigo-200 dark:border-indigo-800 font-medium transition inline-flex items-center space-x-1"
                      >
                        {testingId === ch.id ? (
                          <RefreshCw className="w-3 h-3 animate-spin" />
                        ) : (
                          <Play className="w-3 h-3" />
                        )}
                        <span>Ping</span>
                      </button>
                      <button
                        onClick={() => handleEditChannel(ch)}
                        className="text-xs px-3 py-1.5 rounded-xl bg-amber-50 dark:bg-amber-950/50 text-amber-700 dark:text-amber-300 hover:bg-amber-100 dark:hover:bg-amber-900/60 border border-amber-200 dark:border-amber-800 font-medium transition inline-flex items-center space-x-1"
                      >
                        <Edit3 className="w-3 h-3" />
                        <span>编辑</span>
                      </button>
                      <button
                        onClick={() => handleDeleteChannel(ch.id)}
                        className="text-xs px-2.5 py-1.5 text-rose-600 dark:text-rose-400 hover:text-rose-800 dark:hover:text-rose-300 transition font-medium"
                      >
                        注销
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
