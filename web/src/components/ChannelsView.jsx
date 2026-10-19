import React, { useMemo } from 'react';
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
  Layers,
  Coins
} from 'lucide-react';
import useTableControls from '../hooks/useTableControls';
import useUrlFilters from '../hooks/useUrlFilters';
import { SortableTh, ColumnToggle } from './TableControls';

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
  onNavigateToReconciliation,
  showToast,
  lang = 'zh',
  t
}) {
  const isZh = lang === 'zh';
  // Filters live in the URL (`channels.<key>`), so switching modules no longer
  // silently drops them and the exact list can be shared as a link.
  const [filters, setFilters, resetFilters] = useUrlFilters('channels', {
    q: '',
    engine: 'all',
    status: 'all',
    health: 'all',
  });

  // Links shared before the health filter existed stored the tripped filter as
  // `channels.status=breaker_open`; map it onto the new health dimension.
  const legacyTripped = filters.status === 'breaker_open';
  const searchQuery = filters.q;
  const engineFilter = filters.engine;
  const statusFilter = legacyTripped
    ? 'all'
    : (filters.status === 'active' || filters.status === 'disabled' ? filters.status : 'all');
  const healthFilter = legacyTripped
    ? 'tripped'
    : (HEALTH_KEYS.includes(filters.health) ? filters.health : 'all');

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

  // Search + engine narrow the pool first; the two status dimensions then
  // filter it. Keeping them apart is what lets "enabled but tripped" show up.
  const baseChannels = useMemo(() => {
    const q = (filters.q || '').trim().toLowerCase();
    return channels.filter((ch) => {
      if (engineFilter !== 'all' && ch.type !== engineFilter) return false;
      if (!q) return true;
      const matchName = (ch.name || '').toLowerCase().includes(q);
      const matchUrl = (ch.base_url || '').toLowerCase().includes(q);
      const matchType = (ch.type || '').toLowerCase().includes(q);
      const matchModels = (ch.models || []).some((m) => String(m).toLowerCase().includes(q));
      return matchName || matchUrl || matchType || matchModels;
    });
  }, [channels, engineFilter, filters.q]);

  // Facet counts are computed against the *other* active filters, so the number
  // on a control always equals the number of rows clicking it will show.
  const healthCounts = useMemo(() => {
    const counts = { tripped: 0, half_open: 0, failed: 0, healthy: 0, unprobed: 0 };
    for (const ch of baseChannels) {
      if (statusFilter === 'active' && ch.status !== 'active') continue;
      if (statusFilter === 'disabled' && ch.status === 'active') continue;
      counts[getChannelHealth(ch, channelLatencies)] += 1;
    }
    return counts;
  }, [baseChannels, statusFilter, channelLatencies]);

  const healthTotal =
    healthCounts.tripped + healthCounts.half_open + healthCounts.failed + healthCounts.healthy + healthCounts.unprobed;

  const enablementCounts = useMemo(() => {
    let enabled = 0;
    let total = 0;
    for (const ch of baseChannels) {
      if (healthFilter !== 'all' && getChannelHealth(ch, channelLatencies) !== healthFilter) continue;
      total += 1;
      if (ch.status === 'active') enabled += 1;
    }
    return { enabled, disabled: total - enabled };
  }, [baseChannels, healthFilter, channelLatencies]);

  const filteredChannels = useMemo(
    () =>
      baseChannels.filter((ch) => {
        if (statusFilter === 'active' && ch.status !== 'active') return false;
        if (statusFilter === 'disabled' && ch.status === 'active') return false;
        if (healthFilter !== 'all' && getChannelHealth(ch, channelLatencies) !== healthFilter) return false;
        return true;
      }),
    [baseChannels, statusFilter, healthFilter, channelLatencies]
  );

  const activeChannels = channels.filter(ch => ch.status === 'active');
  const openBreakers = channels.filter(ch => String(ch.breaker_status || '').toUpperCase() === 'OPEN');

  // ---- Sorting + column visibility -----------------------------------------
  const tableColumns = useMemo(
    () => [
      { key: 'name', label: isZh ? '服务商' : 'Provider', sortValue: (ch) => ch.name || '' },
      { key: 'type', label: isZh ? '引擎' : 'Engine', sortValue: (ch) => ch.type || '' },
      { key: 'base_url', label: 'Base URL', sortable: false },
      { key: 'protocols', label: isZh ? '模态' : 'Modalities', defaultVisible: false, sortValue: (ch) => (ch.protocols || []).length },
      { key: 'models', label: isZh ? '支持模型' : 'Models', sortValue: (ch) => (ch.models || []).length },
      { key: 'priority', label: isZh ? '优先级' : 'Priority', sortValue: (ch) => Number(ch.priority ?? 0) },
      { key: 'weight', label: isZh ? '权重' : 'Weight', sortValue: (ch) => Number(ch.weight ?? 0) },
      { key: 'enabled', label: isZh ? '启用' : 'Enabled', sortValue: (ch) => (ch.status === 'active' ? 0 : 1) },
      {
        key: 'health',
        label: isZh ? '健康' : 'Health',
        sortValue: (ch) => HEALTH_RANK[getChannelHealth(ch, channelLatencies)] ?? 9,
      },
      {
        key: 'latency',
        label: isZh ? '延迟' : 'Latency',
        sortValue: (ch) => {
          const probe = channelLatencies[ch.id];
          return probe && probe.success ? Number(probe.latency_ms) : null;
        },
      },
    ],
    [isZh, channelLatencies]
  );

  // Default order is health ascending: tripped first, then half-open, probe
  // failures, healthy and finally channels we have no data for yet.
  const controls = useTableControls({
    columns: tableColumns,
    storageKey: 'airoute.channels.columns',
    initialSort: { key: 'health', dir: 'asc' },
  });
  const { sort, toggleSort, sortRows, isVisible } = controls;
  const columnByKey = useMemo(
    () => Object.fromEntries(tableColumns.map((column) => [column.key, column])),
    [tableColumns]
  );
  const visibleChannels = sortRows(filteredChannels);
  const visibleIds = visibleChannels.map((ch) => ch.id);
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedChannelIds.includes(id));
  const visibleColumnCount = 2 + tableColumns.filter((column) => isVisible(column.key)).length;
  const hasActiveFilters =
    Boolean((searchQuery || '').trim()) || engineFilter !== 'all' || statusFilter !== 'all' || healthFilter !== 'all';

  const toggleSingleStatus = (ch) => {
    const nextStatus = ch.status === 'active' ? 'disabled' : 'active';
    if (handleBatchStatusChannels) {
      handleBatchStatusChannels(nextStatus, [ch.id]);
    }
  };

  const healthOptions = [
    { value: 'tripped', label: isZh ? '熔断' : 'Tripped', count: healthCounts.tripped },
    { value: 'half_open', label: isZh ? '半开' : 'Half-open', count: healthCounts.half_open },
    { value: 'failed', label: isZh ? '探测失败' : 'Probe failed', count: healthCounts.failed },
    { value: 'healthy', label: isZh ? '健康' : 'Healthy', count: healthCounts.healthy },
    { value: 'unprobed', label: isZh ? '未探测' : 'Not probed', count: healthCounts.unprobed },
  ];

  const statusChipClass = (value) =>
    `px-2.5 py-1 rounded-md text-xs font-medium transition cursor-pointer ${
      statusFilter === value
        ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm font-semibold'
        : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
    }`;

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
          <p className="ui-meta mt-1">
            {isZh
              ? '管理 GPUStack、商业厂商大模型上游凭证与负载权重'
              : 'Manage provider credentials, health checks, and circuit-breaker weights'}
          </p>
        </div>

        <div className="flex items-center space-x-2.5">
          {onNavigateToReconciliation && (
            <button
              onClick={onNavigateToReconciliation}
              className="px-3.5 py-2 bg-indigo-50 hover:bg-indigo-100 dark:bg-indigo-950/50 dark:hover:bg-indigo-900/60 text-indigo-700 dark:text-indigo-300 border border-indigo-200/80 dark:border-indigo-800/80 rounded-lg text-xs font-medium shadow-sm flex items-center space-x-1.5 transition cursor-pointer"
            >
              <Coins className="w-3.5 h-3.5 text-indigo-600 dark:text-indigo-400" />
              <span>{isZh ? '供应商整体对账' : 'Supplier Reconciliation'}</span>
            </button>
          )}
          <button onClick={handleBatchPing} disabled={batchTesting || channels.length === 0} className="ui-btn-secondary">
            <Activity className={`w-3.5 h-3.5 text-slate-400 ${batchTesting ? 'animate-spin text-indigo-500' : ''}`} />
            <span>{batchTesting ? (isZh ? '巡检中...' : 'Probing...') : (isZh ? '全量体检' : 'Probe All Health')}</span>
          </button>
          <button onClick={openNewChannelModal} className="ui-btn-primary">
            <Plus className="w-3.5 h-3.5" />
            <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
          </button>
        </div>
      </div>

      {/* 2. Filter Bar (Directly on canvas, no card) */}
      <div className="flex flex-col gap-2.5 text-xs">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative flex-1 min-w-[180px] max-w-sm">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setFilters({ q: e.target.value })}
              placeholder={isZh ? '搜索服务商名称 / 引擎 / 地址 / 模型...' : 'Search provider name, engine, URL, models...'}
              className="ui-field pl-8"
            />
          </div>

          {/* Engine Type Filter */}
          <select
            value={engineFilter}
            onChange={(e) => setFilters({ engine: e.target.value })}
            className="ui-field w-auto cursor-pointer font-medium"
            aria-label={isZh ? '按引擎筛选' : 'Filter by engine'}
          >
            <option value="all">{isZh ? '全部引擎' : 'All Engines'}</option>
            <option value="gpustack">GPUStack</option>
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic Claude</option>
            <option value="gemini">Google Gemini</option>
            <option value="sub2api">Sub2API</option>
          </select>

          {/* Enablement filter: only on/off, says nothing about health. */}
          <div className="inline-flex items-center bg-slate-100 dark:bg-slate-800/60 p-0.5 rounded-lg border border-slate-200/60 dark:border-slate-800/60">
            <span className="hidden sm:inline px-1.5 text-xs text-slate-500 dark:text-slate-400">
              {isZh ? '启用' : 'Enabled'}
            </span>
            <button type="button" onClick={() => setFilters({ status: 'all' })} className={statusChipClass('all')}>
              {isZh ? '全部' : 'All'}
            </button>
            <button type="button" onClick={() => setFilters({ status: 'active' })} className={statusChipClass('active')}>
              {isZh ? `已启用 (${enablementCounts.enabled})` : `Enabled (${enablementCounts.enabled})`}
            </button>
            <button type="button" onClick={() => setFilters({ status: 'disabled' })} className={statusChipClass('disabled')}>
              {isZh ? `已停用 (${enablementCounts.disabled})` : `Disabled (${enablementCounts.disabled})`}
            </button>
          </div>

          {/* Health filter: breaker state + last probe result, counted honestly. */}
          <label className="inline-flex items-center gap-1.5">
            <span className="text-xs text-slate-500 dark:text-slate-400">{isZh ? '健康' : 'Health'}</span>
            <select
              value={healthFilter}
              onChange={(e) => setFilters({ health: e.target.value })}
              className={`ui-field w-auto cursor-pointer font-medium ${
                healthFilter === 'tripped' || healthFilter === 'failed'
                  ? 'border-rose-300 dark:border-rose-900/60 text-rose-700 dark:text-rose-300'
                  : healthFilter === 'half_open'
                  ? 'border-amber-300 dark:border-amber-900/60 text-amber-700 dark:text-amber-300'
                  : ''
              }`}
            >
              <option value="all">
                {isZh ? `全部健康状态 (${healthTotal})` : `All health states (${healthTotal})`}
              </option>
              {healthOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {`${option.label} (${option.count})`}
                </option>
              ))}
            </select>
          </label>

          <div className="flex items-center gap-2 sm:ml-auto">
            {hasActiveFilters && (
              <button type="button" onClick={resetFilters} className="ui-btn-ghost ui-btn-sm">
                {isZh ? '清除筛选' : 'Clear filters'}
              </button>
            )}
            <ColumnToggle controls={controls} lang={lang} />
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
          <p className="ui-meta max-w-sm">
            {isZh
              ? '接入服务商以挂载算力端点，支持 GPUStack、阿里云百炼、火山方舟、OpenAI 等任意兼容接口。'
              : 'Connect upstream providers to mount compute endpoints, supporting GPUStack, Volcengine, OpenAI, and compatible APIs.'}
          </p>
          {typeof setShowChannelModal === 'function' && (
            <button onClick={openNewChannelModal} className="ui-btn-primary">
              <Plus className="w-4 h-4" />
              <span>{isZh ? '接入服务商' : 'Add Channel'}</span>
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          {selectedChannelIds.length > 0 && (
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between bg-slate-100 dark:bg-slate-800/70 border border-slate-200 dark:border-slate-700 px-5 py-3 rounded-xl animate-in fade-in gap-3">
              <div className="flex items-center space-x-2 text-xs font-medium text-slate-800 dark:text-slate-200">
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

          <div className="bg-white dark:bg-[#0F1420] border border-slate-200/70 dark:border-slate-800/80 rounded-2xl overflow-hidden shadow-sm">
            <div className="overflow-x-auto w-full">
              <table className="w-full text-left border-collapse min-w-[380px]">
              <thead>
                <tr className="border-b border-slate-100 dark:border-slate-800/80">
                  <th scope="col" className="ui-th w-10 px-2 text-center sm:px-4">
                    <input
                      type="checkbox"
                      className="rounded border-slate-300 text-slate-900 focus:ring-slate-500 cursor-pointer"
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
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70 text-xs">
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
                    <tr key={ch.id} className="hover:bg-slate-50/50 dark:hover:bg-slate-800/30 transition">
                      <td className="ui-td w-10 px-2 text-center sm:px-4">
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
                      {isVisible('name') && (
                        <td className="ui-td min-w-[120px] font-medium text-slate-900 dark:text-slate-100">
                          <span
                            className="block max-w-[200px] truncate text-xs font-medium text-slate-900 dark:text-white"
                            title={ch.name}
                          >
                            {ch.name}
                          </span>
                        </td>
                      )}
                      {isVisible('type') && (
                        <td className="ui-td hidden sm:table-cell">
                          <span className="px-2 py-0.5 rounded text-xs font-mono text-slate-600 dark:text-slate-300 bg-slate-100 dark:bg-slate-800/80">
                            {ch.type}
                          </span>
                        </td>
                      )}
                      {isVisible('base_url') && (
                        <td className="ui-td hidden lg:table-cell max-w-xs font-mono text-xs text-slate-500 dark:text-slate-400">
                          <div className="flex items-center space-x-1.5">
                            <span className="truncate max-w-[220px]">{ch.base_url}</span>
                            <button
                              onClick={() => copyToClipboard(ch.base_url)}
                              className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition p-0.5 cursor-pointer"
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
                              <span className="text-xs text-slate-500 dark:text-slate-400 font-mono">
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
                                  <span key={p} className="px-1.5 py-0.5 rounded text-xs font-mono text-slate-600 dark:text-slate-300 bg-slate-100 dark:bg-slate-800/60">
                                    {label}
                                  </span>
                                );
                              })
                            )}
                            {ch.protocols && ch.protocols.length > 3 && (
                              <span className="text-xs font-mono text-slate-500 dark:text-slate-400">
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
                              <span className="text-xs text-slate-500 dark:text-slate-400">-</span>
                            ) : (
                              ch.models.slice(0, 2).map((m) => (
                                <span
                                  key={m}
                                  className="px-1.5 py-0.5 rounded text-xs font-mono text-slate-600 dark:text-slate-300 bg-slate-100 dark:bg-slate-800/60"
                                >
                                  {m}
                                </span>
                              ))
                            )}
                            {ch.models && ch.models.length > 2 && (
                              <span
                                className="px-1.5 py-0.5 rounded text-xs font-mono text-slate-500 dark:text-slate-400"
                                title={ch.models.slice(2).join(', ')}
                              >
                                +{ch.models.length - 2}
                              </span>
                            )}
                          </div>
                        </td>
                      )}
                      {isVisible('priority') && (
                        <td className="ui-td hidden sm:table-cell font-mono text-xs tabular-nums text-slate-600 dark:text-slate-400">
                          {ch.priority}
                        </td>
                      )}
                      {isVisible('weight') && (
                        <td className="ui-td hidden sm:table-cell font-mono text-xs tabular-nums text-slate-600 dark:text-slate-400">
                          {ch.weight}
                        </td>
                      )}
                      {isVisible('enabled') && (
                        <td className="ui-td">
                          <button
                            onClick={() => toggleSingleStatus(ch)}
                            className="inline-flex items-center space-x-1.5 text-xs text-slate-600 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white transition cursor-pointer"
                            title={isZh ? '点击切换启用状态' : 'Toggle enabled state'}
                          >
                            <span className={`w-1.5 h-1.5 rounded-full ${ch.status === 'active' ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-600'}`} />
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
                            <span className="text-slate-500 dark:text-slate-400">—</span>
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
                              <RefreshCw className="w-3 h-3 animate-spin text-slate-400" />
                            ) : (
                              <Play className="w-3 h-3 text-slate-400" />
                            )}
                            <span className="hidden sm:inline">Ping</span>
                          </button>
                          <button
                            onClick={() => handleEditChannel(ch)}
                            className="ui-btn-secondary ui-btn-sm"
                            title={isZh ? '编辑' : 'Edit'}
                            aria-label={isZh ? '编辑' : 'Edit'}
                          >
                            <Edit3 className="w-3 h-3 text-slate-400 sm:hidden" />
                            <span className="hidden sm:inline">{isZh ? '编辑' : 'Edit'}</span>
                          </button>
                          <button
                            onClick={() => handleDeleteChannel(ch.id)}
                            className="text-xs px-2 py-1 text-slate-500 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-400 transition font-medium cursor-pointer"
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
        </div>
      )}
    </div>
  );
}
