// Owns the audit-log data and the two loaders that fill it.
//
// Extracted from App.jsx, where six pieces of state and a hundred and forty lines
// of request building sat in the middle of a 2700-line component.
//
// The filter state deliberately stays with the caller: the filters are bound to the
// URL and rendered by the log toolbar, so the console owns them. The loaders read
// the current filters through a ref, which is what keeps their identity stable
// while a poll is in flight.
//
// One subtlety is load-bearing. fetchLogs is recreated when the time range, the
// session filter or the custom dates change, and the polling effects depend on
// fetchLogs. That is how changing a custom date triggers a refetch: those dates
// appear in no effect's dependency array. Collapsing them into the ref would make
// fetchLogs stable and silently stop refetching when the operator picks a custom
// window.

import { useCallback, useEffect, useRef, useState } from 'react';
import { apiFetch } from '../api/client';

// Server-side page size for the audit log table. Exported because the view renders
// the same number in its pagination control.
export const LOG_PAGE_SIZE = 50;

// The repository caps a single log request at this many rows.
const EXPORT_PAGE_SIZE = 200;

// Ceiling for the CSV export, so a pathological filter cannot pull an unbounded
// result set.
const EXPORT_MAX_ROWS = 5000;

const correlationField = (value) => ['trace_id', 'chat_id', 'session_id'].includes(value) ? value : 'session_id';

export function useLogsData({ filters, showToast, isZh }) {
  const [logs, setLogs] = useState([]);
  const [recentLogs, setRecentLogs] = useState([]);
  const [logTotal, setLogTotal] = useState(0);
  const [logOffset, setLogOffset] = useState(0);
  const [logLoading, setLogLoading] = useState(false);
  // Last load failure, so the console can tell "no logs match" apart from
  // "the request failed" — they used to share the same empty state.
  const [logError, setLogError] = useState('');
  const [logFacets, setLogFacets] = useState({ models: [], channels: [] });
  const [selectedLogIds, setSelectedLogIds] = useState([]);

  // Mirror of logOffset for the polling interval: the interval closure captures the
  // fetchLogs of the render it was created in, so a state read inside fetchLogs
  // would go stale — polls must fetch the page being *viewed*.
  const logOffsetRef = useRef(0);

  // The session generation. Clearing the rows is not enough: a request already in
  // flight still resolves, and without this it would write the previous account's
  // audit rows back into the cleared table.
  const generationRef = useRef(0);
  const requestRef = useRef(0);

  // Mirror of the whole filter object so a poll always reads the current facets
  // rather than the ones captured when its closure was created.
  const filtersRef = useRef(filters);
  useEffect(() => {
    filtersRef.current = filters;
  }, [filters]);

  const fetchLogs = useCallback(async (override = {}, overrideToken) => {
    const opts = overrideToken ? { token: overrideToken } : {};
    const generation = generationRef.current;
    const request = ++requestRef.current;
    const current = filtersRef.current || {};
    setLogLoading(true);
    try {
      const p = new URLSearchParams();
      p.set('limit', String(LOG_PAGE_SIZE));
      // Server-side pagination. Offset resolution rules:
      //  - explicit override.offset wins (page buttons);
      //  - a filter override (timeRange/sessionFilter) resets to page 1 —
      //    the old offset is meaningless for the new result set;
      //  - otherwise keep the currently viewed page (polling refresh).
      const filterReset =
        override.timeRange !== undefined || override.sessionFilter !== undefined || override.keyId !== undefined || override.q !== undefined;
      const off =
        override.offset !== undefined ? override.offset : (filterReset ? 0 : logOffsetRef.current);
      p.set('offset', String(off));
      logOffsetRef.current = off;
      setLogOffset((prev) => (prev === off ? prev : off));

      const tr = override.timeRange !== undefined ? override.timeRange : filters.range;
      const sf = override.sessionFilter !== undefined ? override.sessionFilter : filters.session;

      if (sf && sf.trim()) {
        p.set(correlationField(override.correlationType ?? current.correlation_type), sf.trim());
      }

      const keyId = override.keyId !== undefined ? override.keyId : current.key_id;
      if (keyId) p.set('key_id', String(keyId));

      // Keyword and facet filters are applied SERVER-SIDE. Filtering only the
      // loaded page made search/filter/export silently miss every match on the
      // other pages.
      const kw = override.q !== undefined ? override.q : current.q;
      if (kw && kw.trim()) p.set('q', kw.trim());
      if (current.model && current.model !== 'all') p.set('model', current.model);
      if (current.channel && current.channel !== 'all') p.set('channel', current.channel);
      if (current.status && current.status !== 'all') p.set('status', current.status);
      if (current.cache && current.cache !== 'all') p.set('cache', current.cache);
      if (current.latency && current.latency !== 'all') p.set('latency', current.latency);

      const now = new Date();
      if (tr === '1h') {
        p.set('start_time', new Date(now.getTime() - 3600000).toISOString());
      } else if (tr === 'today') {
        const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0);
        p.set('start_time', startOfToday.toISOString());
      } else if (tr === '7d') {
        p.set('start_time', new Date(now.getTime() - 7 * 86400000).toISOString());
      } else if (tr === 'custom') {
        const st = override.customStartTime !== undefined ? override.customStartTime : filters.start;
        const et = override.customEndTime !== undefined ? override.customEndTime : filters.end;
        if (st) p.set('start_time', new Date(st).toISOString());
        if (et) p.set('end_time', new Date(et).toISOString());
      }

      const res = await apiFetch(`/api/v1/admin/logs?${p.toString()}`, opts);
      const data = await res.json();
      if (generation !== generationRef.current || request !== requestRef.current) {
        // The session was cleared while this page was in the air.
        return false;
      }
      if (res.ok && data.code === 0) {
        setLogs(data.data || []);
        setLogFacets(data.facets || { models: [], channels: [] });
        // Total rows for the current filter, used for honest pagination.
        setLogTotal(typeof data.total === 'number' ? data.total : (data.data || []).length);
        setLogError('');
        return true;
      }
      setLogError(data?.error || (isZh ? '加载审计日志失败' : 'Failed to load audit logs'));
      return false;
    } catch (e) {
      console.error('Fetch logs failed:', e);
      if (generation === generationRef.current && request === requestRef.current) {
        setLogError((isZh ? '加载审计日志失败: ' : 'Failed to load audit logs: ') + e.message);
      }
      return false;
    } finally {
      if (generation === generationRef.current && request === requestRef.current) setLogLoading(false);
    }
    // The four date/range values are dependencies on purpose: see the note at the
    // top of this file.
  }, [filters.range, filters.session, filters.start, filters.end, isZh]);

  // Fetch EVERY row matching the active log filters (used by the CSV export).
  // Returns { rows, truncated }: `truncated` is true when the result set is
  // larger than the export cap, so the UI can say so instead of silently
  // shipping a shortened report. `onProgress` receives the running row count.
  const fetchAllLogsForExport = useCallback(async (maxRows = EXPORT_MAX_ROWS, onProgress) => {
    const generation = generationRef.current;
    const collected = [];
    let offset = 0;
    let truncated = false;
    // Freeze the relative window once: re-evaluating "last 7 days" on every page
    // shifted the boundary mid-export, which duplicated or skipped rows.
    const frozenAt = new Date();
    const facets = filtersRef.current || {};
    const range = facets.range;
    let startTime = '';
    let endTime = '';
    if (range === '1h') {
      startTime = new Date(frozenAt.getTime() - 3600000).toISOString();
      endTime = frozenAt.toISOString();
    } else if (range === 'today') {
      startTime = new Date(frozenAt.getFullYear(), frozenAt.getMonth(), frozenAt.getDate()).toISOString();
      endTime = frozenAt.toISOString();
    } else if (range === '7d') {
      startTime = new Date(frozenAt.getTime() - 7 * 86400000).toISOString();
      endTime = frozenAt.toISOString();
    } else if (range === 'custom') {
      if (facets.start) startTime = new Date(facets.start).toISOString();
      if (facets.end) endTime = new Date(facets.end).toISOString();
    }

    for (;;) {
      const p = new URLSearchParams();
      p.set('limit', String(EXPORT_PAGE_SIZE));
      p.set('offset', String(offset));
      if (facets.q && facets.q.trim()) p.set('q', facets.q.trim());
      if (facets.model && facets.model !== 'all') p.set('model', facets.model);
      if (facets.channel && facets.channel !== 'all') p.set('channel', facets.channel);
      if (facets.status && facets.status !== 'all') p.set('status', facets.status);
      if (facets.cache && facets.cache !== 'all') p.set('cache', facets.cache);
      if (facets.latency && facets.latency !== 'all') p.set('latency', facets.latency);
      if (facets.session && facets.session.trim()) p.set(correlationField(facets.correlation_type), facets.session.trim());
      if (facets.key_id) p.set('key_id', String(facets.key_id));
      if (startTime) p.set('start_time', startTime);
      if (endTime) p.set('end_time', endTime);

      const res = await apiFetch(`/api/v1/admin/logs?${p.toString()}`);
      const data = await res.json();
      // An export can span many pages; stop as soon as the session it belongs to
      // is gone rather than assembling a report from another account's rows.
      if (generation !== generationRef.current) break;
      if (!res.ok || data.code !== 0) break;
      const batch = data.data || [];
      collected.push(...batch);
      if (onProgress) onProgress({ collected: Math.min(collected.length, maxRows), maxRows });
      if (batch.length < EXPORT_PAGE_SIZE) break;
      if (collected.length >= maxRows) {
        // More rows exist than the cap allows; the caller must surface this.
        truncated = true;
        break;
      }
      offset += EXPORT_PAGE_SIZE;
    }
    return { rows: collected.slice(0, maxRows), truncated };
  }, []);

  // Move to a page. The ref and the state must move together: the ref is what a
  // poll reads, the state is what the pagination control renders.
  const goToOffset = useCallback((nextOffset) => {
    logOffsetRef.current = nextOffset;
    setLogOffset(nextOffset);
  }, []);

  const resetOffset = useCallback(() => {
    logOffsetRef.current = 0;
    setLogOffset(0);
  }, []);

  // Clear every piece of tenant-scoped log data so a following session cannot
  // observe the previous account's audit trail.
  const reset = useCallback(() => {
    generationRef.current += 1;
    requestRef.current += 1;
    setLogFacets({ models: [], channels: [] });
    setLogs([]);
    setRecentLogs([]);
    setLogTotal(0);
    setLogError('');
    logOffsetRef.current = 0;
    setLogOffset(0);
    setSelectedLogIds([]);
  }, []);

  // Dedicated un-filtered loader for the overview dashboard's recent stream,
  // ensuring the dashboard is never constrained by logs-view filter criteria.
  const fetchRecentLogs = useCallback(async (overrideToken) => {
    const opts = overrideToken ? { token: overrideToken } : {};
    const generation = generationRef.current;
    try {
      const res = await apiFetch('/api/v1/admin/logs?limit=20&offset=0', opts);
      const data = await res.json();
      if (generation !== generationRef.current) return;
      if (res.ok && data.code === 0 && Array.isArray(data.data)) {
        setRecentLogs(data.data);
      }
    } catch {}
  }, []);

  // Batch log operations. Each one clears the affected selection and refetches, so
  // the table never shows a row that no longer exists.
  const handleBatchDeleteLogs = useCallback(async () => {
    if (selectedLogIds.length === 0) return;
    if (!window.confirm(isZh ? `确认批量删除选中的 ${selectedLogIds.length} 条调用日志？` : `Confirm delete selected ${selectedLogIds.length} request logs?`)) return;
    try {
      const res = await apiFetch('/api/v1/admin/logs/batch-delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: selectedLogIds })
      });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(data.message || (isZh ? '已批量删除所选日志' : 'Selected logs deleted successfully'), 'success');
        setSelectedLogIds([]);
        fetchLogs();
      } else {
        showToast(data.error || (isZh ? '批量删除日志失败' : 'Failed to delete logs'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [selectedLogIds, isZh, showToast, fetchLogs]);

  const handleClearLogs = useCallback(async () => {
    if (!window.confirm(isZh ? '⚠️ 确认清空所有调用日志？此操作无法撤销！' : '⚠️ Confirm clear all request logs? This action cannot be undone!')) return;
    try {
      const res = await apiFetch('/api/v1/admin/logs/clear', { method: 'POST' });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? '已清空全部调用历史日志' : 'All request logs cleared', 'success');
        setSelectedLogIds([]);
        fetchLogs();
      } else {
        showToast(data.error || (isZh ? '清空日志失败' : 'Failed to clear logs'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [isZh, showToast, fetchLogs]);

  const handleDeleteSingleLog = useCallback(async (id) => {
    try {
      const res = await apiFetch(`/api/v1/admin/logs/${id}`, { method: 'DELETE' });
      const data = await res.json();
      if (res.ok && data.code === 0) {
        showToast(isZh ? '已删除该条调用日志' : 'Request log deleted', 'success');
        setSelectedLogIds(prev => prev.filter(x => x !== id));
        fetchLogs();
      } else {
        showToast(data.error || (isZh ? '删除日志失败' : 'Failed to delete log'), 'error');
      }
    } catch (e) {
      showToast((isZh ? '请求异常: ' : 'Request error: ') + e.message, 'error');
    }
  }, [isZh, showToast, fetchLogs]);

  return {
    selectedLogIds,
    setSelectedLogIds,
    handleBatchDeleteLogs,
    handleClearLogs,
    handleDeleteSingleLog,
    logs,
    recentLogs,
    fetchRecentLogs,
    logTotal,
    logOffset,
    logLoading,
    logError,
    logFacets,
    fetchLogs,
    fetchAllLogsForExport,
    goToOffset,
    resetOffset,
    reset,
  };
}
