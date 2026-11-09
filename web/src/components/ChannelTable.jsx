import { SortableTh } from './TableControls';

import { Trash2, Copy, Play, Edit3, RefreshCw } from 'lucide-react';

export default function ChannelTable({
  HEALTH_META, allVisibleSelected, channelLatencies, columnByKey, copyToClipboard, getChannelHealth,
  handleDeleteChannel, handleEditChannel, handleTestChannel, hasActiveFilters, isVisible, isZh,
  onNavigateToProcurement, resetFilters, selectedChannelIds, setSelectedChannelIds, sort, testingId,
  toggleSingleStatus, toggleSort, visibleChannels, visibleColumnCount, visibleIds
}) {
  return (
    <div className="ui-card overflow-hidden">
      <div className="overflow-x-auto w-full">
        <table className="w-full text-left border-collapse min-w-[380px]">
        <thead>
          <tr className="border-b border-[#E4E6E9] dark:border-[#2C3038] bg-[#F7F7F5] dark:bg-[#191B1F]">
            <th scope="col" className="ui-th w-10 px-2 text-center sm:px-4">
              <input
                type="checkbox"
                className="rounded border-[#E4E6E9] text-[#355CFF] focus:ring-[#355CFF] cursor-pointer"
                aria-label={isZh ? '全选当前列表' : 'Select all visible channels'}
                checked={allVisibleSelected}
                onChange={(e) => {
                  // Operates on what is currently filtered + sorted, so the
                  // header checkbox always describes the rows the user sees.
                  if (e.target.checked) {
                    setSelectedChannelIds((prev) => Array.from(new Set([...prev, ...visibleIds])));
                  } else {
                    setSelectedChannelIds((prev) => prev.filter((id) => !visibleIds.includes(id)));
                  }
                }}
              />
            </th>
            {isVisible('name') && (
              <SortableTh column={columnByKey.name} sort={sort} onSort={toggleSort} className="min-w-[120px]" />
            )}
            {isVisible('type') && (
              <SortableTh column={columnByKey.type} sort={sort} onSort={toggleSort} className="hidden sm:table-cell" />
            )}
            {isVisible('base_url') && (
              <SortableTh column={columnByKey.base_url} sort={sort} onSort={toggleSort} className="hidden lg:table-cell" />
            )}
            {isVisible('protocols') && (
              <SortableTh column={columnByKey.protocols} sort={sort} onSort={toggleSort} className="hidden xl:table-cell" />
            )}
            {isVisible('models') && (
              <SortableTh column={columnByKey.models} sort={sort} onSort={toggleSort} className="hidden md:table-cell" />
            )}
            {isVisible('priority') && (
              <SortableTh column={columnByKey.priority} sort={sort} onSort={toggleSort} className="hidden sm:table-cell" />
            )}
            {isVisible('weight') && (
              <SortableTh column={columnByKey.weight} sort={sort} onSort={toggleSort} className="hidden sm:table-cell" />
            )}
            {isVisible('enabled') && <SortableTh column={columnByKey.enabled} sort={sort} onSort={toggleSort} />}
            {isVisible('health') && <SortableTh column={columnByKey.health} sort={sort} onSort={toggleSort} />}
            {isVisible('latency') && <SortableTh column={columnByKey.latency} sort={sort} onSort={toggleSort} />}
            <th scope="col" className="ui-th text-right">{isZh ? '操作' : 'Actions'}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-[#E4E6E9]/60 dark:divide-[#2C3038]/60 text-xs">
          {visibleChannels.map((ch) => {
            const probe = channelLatencies[ch.id];
            const health = getChannelHealth(ch, channelLatencies);
            const healthMeta = HEALTH_META[health];
            const healthDetail = [
              `${isZh ? '熔断器' : 'Breaker'}: ${ch.breaker_status || (isZh ? '未知' : 'unknown')}`,
              probe
                ? `${isZh ? '探测' : 'Probe'}: ${probe.success ? `${probe.latency_ms} ms` : (probe.error || (isZh ? '失败' : 'failed'))}`
                : (isZh ? '本次会话未探测' : 'not probed in this session'),
            ].join(' · ');
            return (
              <tr key={ch.id} className="ui-tr-hover">
                <td className="ui-td w-10 px-2 text-center sm:px-4">
                  <input
                    type="checkbox"
                    className="rounded border-[#E4E6E9] dark:border-[#2C3038] text-[#355CFF] focus:ring-[#355CFF]/30 cursor-pointer"
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
                {isVisible('name') && (
                  <td className="ui-td min-w-[120px] font-medium text-[#202124] dark:text-[#F1F2F4]">
                    <span
                      className="block max-w-[200px] truncate text-xs font-medium text-[#202124] dark:text-[#F1F2F4]"
                      title={ch.name}
                    >
                      {ch.name}
                    </span>
                    {onNavigateToProcurement && <button type="button" className="mt-1 text-[11px] text-indigo-600 hover:underline" onClick={() => onNavigateToProcurement(ch.name)}>{isZh ? '模型采购报价' : 'Model purchase quotes'}</button>}
                  </td>
                )}
                {isVisible('type') && (
                  <td className="ui-td hidden sm:table-cell">
                    <span className="px-2 py-0.5 rounded text-xs font-mono text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9]/60 dark:border-[#2C3038]/60">
                      {ch.type}
                    </span>
                  </td>
                )}
                {isVisible('base_url') && (
                  <td className="ui-td hidden lg:table-cell max-w-xs font-mono text-xs text-[#656B75] dark:text-[#A1A7B0]">
                    <div className="flex items-center space-x-1.5">
                      <span className="truncate max-w-[220px]">{ch.base_url}</span>
                      <button
                        onClick={() => copyToClipboard(ch.base_url)}
                        className="text-[#656B75] hover:text-[#355CFF] dark:text-[#A1A7B0] dark:hover:text-[#91A7FF] transition p-0.5 cursor-pointer"
                        title={isZh ? "复制 Base URL" : "Copy Base URL"}
                      >
                        <Copy className="w-3 h-3" />
                      </button>
                    </div>
                  </td>
                )}
                {isVisible('protocols') && (
                  <td className="ui-td hidden xl:table-cell">
                    <div className="flex flex-wrap gap-1 max-w-xs">
                      {!ch.protocols || ch.protocols.length === 0 ? (
                        <span className="text-xs text-[#656B75] dark:text-[#A1A7B0] font-mono">
                          {isZh ? '全功能' : 'All'}
                        </span>
                      ) : (
                        ch.protocols.slice(0, 3).map((p) => {
                          let label = p;
                          if (p === 'openai_chat' || p === 'chat') label = isZh ? '对话' : 'Chat';
                          else if (p === 'openai_response' || p === 'responses' || p === 'response') label = 'Response';
                          else if (p === 'anthropic_messages' || p === 'messages') label = 'Messages';
                          else if (p === 'images' || p === 'image_generation') label = isZh ? '生图' : 'Images';
                          else if (p === 'audio_speech' || p === 'tts') label = 'TTS';
                          else if (p === 'audio_transcription' || p === 'stt') label = 'STT';
                          else if (p === 'videos' || p === 'video_generation') label = isZh ? '视频' : 'Videos';
                          else if (p === 'embeddings' || p === 'embedding') label = isZh ? '向量' : 'Embeddings';
                          else if (p === 'rerank' || p === 'reranker') label = isZh ? '重排' : 'Rerank';
                          return (
                            <span key={p} className="px-1.5 py-0.5 rounded text-xs font-mono text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9]/60 dark:border-[#2C3038]/60">
                              {label}
                            </span>
                          );
                        })
                      )}
                      {ch.protocols && ch.protocols.length > 3 && (
                        <span className="text-xs font-mono text-[#656B75] dark:text-[#A1A7B0]">
                          +{ch.protocols.length - 3}
                        </span>
                      )}
                    </div>
                  </td>
                )}
                {isVisible('models') && (
                  <td className="ui-td hidden md:table-cell">
                    <div className="flex flex-wrap gap-1 max-w-xs">
                      {!ch.models || ch.models.length === 0 ? (
                        <span className="text-xs text-[#656B75] dark:text-[#A1A7B0]">-</span>
                      ) : (
                        ch.models.slice(0, 2).map((m) => (
                          <span
                            key={m}
                            className="px-1.5 py-0.5 rounded text-xs font-mono text-[#656B75] dark:text-[#A1A7B0] bg-[#F0F1F2] dark:bg-[#22252B] border border-[#E4E6E9]/60 dark:border-[#2C3038]/60"
                          >
                            {m}
                          </span>
                        ))
                      )}
                      {ch.models && ch.models.length > 2 && (
                        <span
                          className="px-1.5 py-0.5 rounded text-xs font-mono text-[#656B75] dark:text-[#A1A7B0]"
                          title={ch.models.slice(2).join(', ')}
                        >
                          +{ch.models.length - 2}
                        </span>
                      )}
                    </div>
                  </td>
                )}
                {isVisible('priority') && (
                  <td className="ui-td hidden sm:table-cell font-mono text-xs tabular-nums text-[#656B75] dark:text-[#A1A7B0]">
                    {ch.priority}
                  </td>
                )}
                {isVisible('weight') && (
                  <td className="ui-td hidden sm:table-cell font-mono text-xs tabular-nums text-[#656B75] dark:text-[#A1A7B0]">
                    {ch.weight}
                  </td>
                )}
                {isVisible('enabled') && (
                  <td className="ui-td">
                    <button
                      onClick={() => toggleSingleStatus(ch)}
                      className="inline-flex items-center space-x-1.5 text-xs text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4] transition cursor-pointer"
                      title={isZh ? '点击切换启用状态' : 'Toggle enabled state'}
                    >
                      <span className={`w-1.5 h-1.5 rounded-full ${ch.status === 'active' ? 'bg-emerald-500' : 'bg-[#E4E6E9] dark:bg-[#2C3038]'}`} />
                      <span>{ch.status === 'active' ? (isZh ? '已启用' : 'Enabled') : (isZh ? '已停用' : 'Disabled')}</span>
                    </button>
                  </td>
                )}
                {isVisible('health') && (
                  <td className="ui-td">
                    <span
                      className={`inline-flex items-center space-x-1 rounded-md border px-1.5 py-0.5 text-xs font-medium ${healthMeta.cls}`}
                      title={healthDetail}
                    >
                      <span className={`w-1.5 h-1.5 rounded-full ${healthMeta.dot}`} />
                      <span>{isZh ? healthMeta.zh : healthMeta.en}</span>
                    </span>
                  </td>
                )}
                {isVisible('latency') && (
                  <td className="ui-td font-mono text-xs tabular-nums">
                    {probe ? (
                      probe.success ? (
                        <span className="text-emerald-600 dark:text-emerald-400">{probe.latency_ms} ms</span>
                      ) : (
                        <span
                          className="text-rose-600 dark:text-rose-400"
                          title={probe.error || (isZh ? '探测失败' : 'Probe failed')}
                        >
                          {isZh ? '失败' : 'Failed'}
                        </span>
                      )
                    ) : (
                      <span className="text-[#656B75] dark:text-[#A1A7B0]">—</span>
                    )}
                  </td>
                )}
                <td className="ui-td text-right">
                  <div className="flex items-center justify-end gap-1.5">
                    <button
                      onClick={() => handleTestChannel(ch)}
                      disabled={testingId === ch.id}
                      className="ui-btn-secondary ui-btn-sm"
                      title={isZh ? '连通性测试' : 'Ping test'}
                      aria-label={isZh ? '连通性测试' : 'Ping test'}
                    >
                      {testingId === ch.id ? (
                        <RefreshCw className="w-3 h-3 animate-spin text-[#656B75]" />
                      ) : (
                        <Play className="w-3 h-3 text-[#656B75]" />
                      )}
                      <span className="hidden sm:inline">Ping</span>
                    </button>
                    <button
                      onClick={() => handleEditChannel(ch)}
                      className="ui-btn-secondary ui-btn-sm"
                      title={isZh ? '编辑' : 'Edit'}
                      aria-label={isZh ? '编辑' : 'Edit'}
                    >
                      <Edit3 className="w-3 h-3 text-[#656B75] sm:hidden" />
                      <span className="hidden sm:inline">{isZh ? '编辑' : 'Edit'}</span>
                    </button>
                    <button
                      onClick={() => handleDeleteChannel(ch.id)}
                      className="text-xs px-2 py-1 text-[#656B75] dark:text-[#A1A7B0] hover:text-rose-600 dark:hover:text-rose-400 transition font-medium cursor-pointer"
                      title={isZh ? '删除' : 'Delete'}
                      aria-label={isZh ? '删除' : 'Delete'}
                    >
                      <Trash2 className="w-3.5 h-3.5 sm:hidden" />
                      <span className="hidden sm:inline">{isZh ? '删除' : 'Delete'}</span>
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
          {visibleChannels.length === 0 && (
            <tr>
              <td colSpan={visibleColumnCount} className="py-10 text-center">
                {hasActiveFilters ? (
                  <div className="flex flex-col items-center justify-center gap-2">
                    <span className="ui-meta">
                      {isZh ? '没有服务商匹配当前筛选条件' : 'No channels match the current filters'}
                    </span>
                    <button onClick={resetFilters} className="ui-btn-secondary ui-btn-sm">
                      {isZh ? '清除筛选' : 'Clear filters'}
                    </button>
                  </div>
                ) : (
                  <span className="ui-meta">
                    {isZh ? '暂无可显示的服务商' : 'No channels to display'}
                  </span>
                )}
              </td>
            </tr>
          )}
        </tbody>
      </table>
      </div>
    </div>
  );
}
