// Audit-log row selection: a selection belongs to the query it was made in.
//
// Regression for BUG-H01 (was: selection survived paging and filter changes, so a
// batch delete could act on rows that were no longer on screen). These assertions
// fail on the pre-fix code.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/hooks/useLogsData.selection.test.jsx

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLogsData } from './useLogsData';

function filters(overrides = {}) {
  return {
    q: '', range: 'all', start: '', end: '', session: '', correlation_type: 'session_id',
    model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'all',
    ...overrides,
  };
}

function rowsForPage(page) {
  return Array.from({ length: 3 }, (_, i) => ({ id: page * 100 + i, model: `model-p${page}` }));
}

function logResponse(rows, total = 500) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data: rows, total }) };
}

function lastQuery() {
  const url = String(globalThis.fetch.mock.calls.at(-1)[0]);
  return new URLSearchParams(url.split('?')[1] || '');
}

describe('useLogsData selection lifetime', () => {
  beforeEach(() => {
    window.confirm = vi.fn(() => true);
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(logResponse(rowsForPage(1))));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('clears the selection when the operator pages to another page', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: () => {}, isZh: true }));

    await act(async () => { await result.current.fetchLogs(); });
    expect(result.current.logs.map((r) => r.id)).toEqual([100, 101, 102]);
    act(() => { result.current.setSelectedLogIds([100, 101]); });
    expect(result.current.selectedLogIds).toEqual([100, 101]);

    globalThis.fetch.mockResolvedValue(logResponse(rowsForPage(3)));
    await act(async () => { await result.current.fetchLogs({ offset: 100 }); });
    expect(lastQuery().get('offset')).toBe('100');

    // The page-1 rows are no longer rendered, so their ids must not stay selected.
    expect(result.current.selectedLogIds).toEqual([]);
  });

  it('clears the selection when a filter changes the result set', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: () => {}, isZh: true }));

    await act(async () => { await result.current.fetchLogs(); });
    act(() => { result.current.setSelectedLogIds([100]); });

    globalThis.fetch.mockResolvedValue(logResponse([{ id: 999, model: 'other' }]));
    await act(async () => { await result.current.fetchLogs({ q: 'boom' }); });

    expect(result.current.logs.map((r) => r.id)).toEqual([999]);
    expect(result.current.selectedLogIds).toEqual([]);
  });

  it('keeps the selection across a poll of the same query', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: () => {}, isZh: true }));

    await act(async () => { await result.current.fetchLogs(); });
    act(() => { result.current.setSelectedLogIds([100]); });

    // A refresh of the same page/filter (the 10s poll) must not drop the ticks.
    await act(async () => { await result.current.fetchLogs(); });

    expect(result.current.selectedLogIds).toEqual([100]);
  });

  it('does not send a batch delete after the selection was invalidated by paging', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: () => {}, isZh: true }));

    await act(async () => { await result.current.fetchLogs(); });
    act(() => { result.current.setSelectedLogIds([100]); });

    globalThis.fetch.mockResolvedValue(logResponse(rowsForPage(2)));
    await act(async () => { await result.current.fetchLogs({ offset: 50 }); });

    await act(async () => { await result.current.handleBatchDeleteLogs(); });

    const deleteCall = globalThis.fetch.mock.calls.find(([url, opts]) =>
      String(url).includes('/logs/batch-delete') && opts?.method === 'POST');
    expect(deleteCall).toBeUndefined();
  });
});
