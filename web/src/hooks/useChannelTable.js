import useUrlFilters from './useUrlFilters';

import { useMemo } from 'react';
import useTableControls from './useTableControls';

export default function useChannelTable({
  HEALTH_KEYS, HEALTH_RANK, channelLatencies, channels, getChannelHealth, handleBatchStatusChannels, isZh,
  selectedChannelIds
}) {
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
  }, [baseChannels, statusFilter, getChannelHealth, channelLatencies]);

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
  }, [baseChannels, healthFilter, getChannelHealth, channelLatencies]);

  const filteredChannels = useMemo(
    () =>
      baseChannels.filter((ch) => {
        if (statusFilter === 'active' && ch.status !== 'active') return false;
        if (statusFilter === 'disabled' && ch.status === 'active') return false;
        if (healthFilter !== 'all' && getChannelHealth(ch, channelLatencies) !== healthFilter) return false;
        return true;
      }),
    [baseChannels, statusFilter, healthFilter, getChannelHealth, channelLatencies]
  );

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
    [isZh, HEALTH_RANK, getChannelHealth, channelLatencies]
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
  return {
    allVisibleSelected, columnByKey, controls, enablementCounts, engineFilter, hasActiveFilters, healthFilter,
    healthOptions, healthTotal, isVisible, openBreakers, resetFilters, searchQuery, setFilters, sort,
    statusChipClass, toggleSingleStatus, toggleSort, visibleChannels, visibleColumnCount, visibleIds
  };
}
