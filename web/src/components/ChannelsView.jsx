import useChannelTable from '../hooks/useChannelTable';
import ChannelFilters from './ChannelFilters';
import ChannelTable from './ChannelTable';

import { Server, Activity, Plus, Trash2, Coins } from 'lucide-react';

// Enablement ("is this channel switched on?") and health ("is it working?")
// are different questions: an *enabled* channel can still be tripped by the
// circuit breaker or fail its last probe. These are the honest health states,
// derived only from data we actually have.
const HEALTH_KEYS = ['tripped', 'half_open', 'failed', 'healthy', 'unprobed'];

// Ascending sort puts problems first: tripped → half-open → probe failed →
// healthy → never probed. That is the default table order.
const HEALTH_RANK = { tripped: 0, half_open: 1, failed: 2, healthy: 3, unprobed: 4 };

const HEALTH_META = {
  tripped: {
    zh: '熔断',
    en: 'Tripped',
    dot: 'bg-rose-500',
    cls: 'border-rose-200/70 dark:border-rose-900/50 bg-rose-50 dark:bg-rose-950/40 text-rose-700 dark:text-rose-300',
  },
  half_open: {
    zh: '半开',
    en: 'Half-open',
    dot: 'bg-amber-500 animate-pulse',
    cls: 'border-amber-200/70 dark:border-amber-900/50 bg-amber-50 dark:bg-amber-950/40 text-amber-700 dark:text-amber-300',
  },
  failed: {
    zh: '探测失败',
    en: 'Probe failed',
    dot: 'bg-rose-500',
    cls: 'border-rose-200/70 dark:border-rose-900/50 bg-rose-50 dark:bg-rose-950/40 text-rose-700 dark:text-rose-300',
  },
  healthy: {
    zh: '健康',
    en: 'Healthy',
    dot: 'bg-emerald-500',
    cls: 'border-emerald-200/70 dark:border-emerald-900/50 bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300',
  },
  unprobed: {
    zh: '未探测',
    en: 'Not probed',
    dot: 'bg-slate-300 dark:bg-slate-600',
    cls: 'border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-900/60 text-slate-500 dark:text-slate-400',
  },
};

function getChannelHealth(ch, channelLatencies) {
  const breaker = String(ch.breaker_status || '').toUpperCase();
  if (breaker === 'OPEN') return 'tripped';
  if (breaker === 'HALF-OPEN') return 'half_open';
  const probe = channelLatencies[ch.id];
  // A failed probe is the most recent direct evidence, so it outranks a
  // closed breaker; a tripped breaker still outranks a lucky probe.
  if (probe && probe.success === false) return 'failed';
  if (probe && probe.success) return 'healthy';
  if (breaker === 'CLOSED') return 'healthy';
  return 'unprobed';
}

export default function ChannelsView({
  openCreateChannel,
  channels = [],
  batchTesting,
  handleBatchPing,
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
  onNavigateToReconciliation,
  onNavigateToProcurement,
  lang = 'zh',
}) {
  const isZh = lang === 'zh';
  // Filters live in the URL (`channels.<key>`), so switching modules no longer
  // silently drops them and the exact list can be shared as a link.
  const {
    allVisibleSelected, columnByKey, controls, enablementCounts, engineFilter, hasActiveFilters, healthFilter,
    healthOptions, healthTotal, isVisible, openBreakers, resetFilters, searchQuery, setFilters, sort,
    statusChipClass, toggleSingleStatus, toggleSort, visibleChannels, visibleColumnCount, visibleIds
  } = useChannelTable({
    HEALTH_KEYS, HEALTH_RANK, channelLatencies, channels, getChannelHealth, handleBatchStatusChannels, isZh,
    selectedChannelIds
  });

  return (
    <div className="space-y-6">
      {/* 1. Header (Directly on canvas) */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4">
        <div>
          <div className="flex items-center space-x-3">
            <h2 className="ui-page-title">
              {isZh ? '服务商管理' : 'Channel Providers'}
            </h2>
            <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]">
              {channels.length} {isZh ? '个节点' : 'nodes'}
            </span>
            {openBreakers.length > 0 && (
              <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-rose-500/10 text-rose-600 dark:text-rose-400 border border-rose-500/20">
                {openBreakers.length} {isZh ? '个熔断' : 'tripped'}
              </span>
            )}
          </div>
          <p className="ui-page-subtitle mt-1">
            {isZh
              ? '接入上游凭证与负载权重'
              : 'Provider credentials, health and weights'}
          </p>
        </div>

        <div className="flex items-center space-x-2.5">
          {onNavigateToProcurement && <button type="button" className="ui-btn-secondary ui-btn-sm" onClick={() => onNavigateToProcurement()}><Coins className="w-3.5 h-3.5" />{isZh ? '渠道采购报价' : 'Purchase quotes'}</button>}
          {onNavigateToReconciliation && (
            <button
              onClick={onNavigateToReconciliation}
              className="ui-btn-secondary"
            >
              <Coins className="w-3.5 h-3.5 text-[#355CFF] dark:text-[#91A7FF]" />
              <span>{isZh ? '供应商整体对账' : 'Supplier Reconciliation'}</span>
            </button>
          )}
          <button onClick={handleBatchPing} disabled={batchTesting || channels.length === 0} className="ui-btn-secondary">
            <Activity className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${batchTesting ? 'animate-spin text-[#355CFF]' : ''}`} />
            <span>{batchTesting ? (isZh ? '巡检中...' : 'Probing...') : (isZh ? '全量体检' : 'Probe All Health')}</span>
          </button>
          <button onClick={openCreateChannel} className="ui-btn-primary">
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
          </button>
        </div>
      </div>

      {/* 2. Filter Bar (Directly on canvas, no card) */}
      <ChannelFilters {...{
        controls, enablementCounts, engineFilter, hasActiveFilters, healthFilter, healthOptions, healthTotal, isZh,
        lang, resetFilters, searchQuery, setFilters, statusChipClass
      }} />

      {/* 3. Channels Table Content */}
      {channels.length === 0 ? (
        <div className="ui-card border-dashed p-12 text-center flex flex-col items-center justify-center space-y-3">
          <div className="w-12 h-12 rounded-xl bg-[#F0F1F2] dark:bg-[#22252B] flex items-center justify-center text-[#656B75] dark:text-[#A1A7B0] shadow-2xs">
            <Server className="w-6 h-6" />
          </div>
          <h4 className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4]">{isZh ? '暂无模型服务商' : 'No Upstream Providers'}</h4>
          <p className="ui-meta max-w-sm">
            {isZh
              ? '接入服务商以挂载算力端点，支持 GPUStack、阿里云百炼、火山方舟、OpenAI 等任意兼容接口。'
              : 'Connect upstream providers to mount compute endpoints, supporting GPUStack, Volcengine, OpenAI, and compatible APIs.'}
          </p>
          {typeof openCreateChannel === 'function' && (
            <button onClick={openCreateChannel} className="ui-btn-primary">
              <Plus className="w-4 h-4" />
              <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {selectedChannelIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-[#EEF2FF] dark:bg-[#242D48] border border-[#355CFF]/30 dark:border-[#91A7FF]/30 px-5 py-3 rounded-xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-semibold text-[#355CFF] dark:text-[#91A7FF]">
                <span>{isZh ? `已选中 ${selectedChannelIds.length} 个服务商` : `Selected ${selectedChannelIds.length} channels`}</span>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <button onClick={() => handleBatchStatusChannels('active')} className="ui-btn-primary ui-btn-sm">
                  {isZh ? '批量启用' : 'Batch Enable'}
                </button>
                <button onClick={() => handleBatchStatusChannels('disabled')} className="ui-btn-secondary ui-btn-sm">
                  {isZh ? '批量停用' : 'Batch Disable'}
                </button>
                <button onClick={handleBatchDeleteChannels} className="ui-btn-danger ui-btn-sm">
                  <Trash2 className="w-3.5 h-3.5" />
                  <span>{isZh ? '批量删除' : 'Batch Delete'}</span>
                </button>
                <button onClick={() => setSelectedChannelIds([])} className="ui-btn-secondary ui-btn-sm">
                  {isZh ? '取消' : 'Cancel'}
                </button>
              </div>
            </div>
          )}

          <ChannelTable {...{
            HEALTH_META, allVisibleSelected, channelLatencies, columnByKey, copyToClipboard, getChannelHealth,
            handleDeleteChannel, handleEditChannel, handleTestChannel, hasActiveFilters, isVisible, isZh,
            onNavigateToProcurement, resetFilters, selectedChannelIds, setSelectedChannelIds, sort, testingId,
            toggleSingleStatus, toggleSort, visibleChannels, visibleColumnCount, visibleIds
          }} />
        </div>
      )}
    </div>
  );
}
