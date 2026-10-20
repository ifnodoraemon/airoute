import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useLogsData, LOG_PAGE_SIZE } from './useLogsData';

// The audit-log query builder used to live inside App.jsx, where the only way to
// exercise it was to drive the whole console. Its comment records two bugs that
// shipped from getting it wrong: filtering only the loaded page hid matches on
// other pages, and the poll closure read a stale offset so a refresh jumped back
// to page one.

function filters(overrides = {}) {
  return {
    q: '', range: 'all', start: '', end: '', session: '',
    model: 'all', channel: 'all', status: 'all', cache: 'all', latency: 'all',
    ...overrides,
  };
}

function logResponse(rows, total) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data: rows, total }) };
}

function lastQuery() {
  const url = String(globalThis.fetch.mock.calls.at(-1)[0]);
  return new URLSearchParams(url.split('?')[1] || '');
}

describe('useLogsData', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(logResponse([], 0)));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('requests the configured page size', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(lastQuery().get('limit')).toBe(String(LOG_PAGE_SIZE));
    expect(lastQuery().get('offset')).toBe('0');
  });

  it('stores the rows and the honest total', async () => {
    globalThis.fetch.mockResolvedValue(logResponse([{ id: 1 }], 400));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(result.current.logs).toEqual([{ id: 1 }]);
    expect(result.current.logTotal).toBe(400);
  });

  it('falls back to the row count when the gateway sends no total', async () => {
    globalThis.fetch.mockResolvedValue({ ok: true, status: 200, json: async () => ({ code: 0, data: [{ id: 1 }, { id: 2 }] }) });
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(result.current.logTotal).toBe(2);
  });

  it('sends every facet to the server', async () => {
    // The regression: facets were applied to the loaded page in memory, so matches
    // on other pages were invisible.
    const { result } = renderHook(() => useLogsData({
      filters: filters({ q: 'boom', model: 'gpt-x', channel: 'c1', status: '500', cache: 'miss', latency: 'slow', session: 'sess-1' }),
    }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    const q = lastQuery();
    expect(q.get('q')).toBe('boom');
    expect(q.get('model')).toBe('gpt-x');
    expect(q.get('channel')).toBe('c1');
    expect(q.get('status')).toBe('500');
    expect(q.get('cache')).toBe('miss');
    expect(q.get('latency')).toBe('slow');
    expect(q.get('session_id')).toBe('sess-1');
  });

  it('omits the facets that mean "all"', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    const q = lastQuery();
    for (const key of ['q', 'model', 'channel', 'status', 'cache', 'latency', 'session_id']) {
      expect(q.get(key)).toBeNull();
    }
  });

  it('translates the relative ranges into a start time', async () => {
    for (const range of ['1h', 'today', '7d']) {
      globalThis.fetch.mockClear();
      const { result } = renderHook(() => useLogsData({ filters: filters({ range }) }));

      await act(async () => {
        await result.current.fetchLogs();
      });

      expect(lastQuery().get('start_time')).toBeTruthy();
    }
  });

  it('sends both bounds for a custom range', async () => {
    const { result } = renderHook(() => useLogsData({
      filters: filters({ range: 'custom', start: '2026-01-02T03:04:05Z', end: '2026-01-03T03:04:05Z' }),
    }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(lastQuery().get('start_time')).toBeTruthy();
    expect(lastQuery().get('end_time')).toBeTruthy();
  });

  it('sends no time bound for the "all" range', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters({ range: 'all' }) }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(lastQuery().get('start_time')).toBeNull();
  });

  it('resets to the first page when the range changes', async () => {
    // A filter override invalidates the offset.
    const { result } = renderHook(() => useLogsData({ filters: filters({ range: '7d' }) }));
    await act(async () => {
      await result.current.fetchLogs({ offset: 300 });
    });
    expect(lastQuery().get('offset')).toBe('300');

    await act(async () => {
      await result.current.fetchLogs({ timeRange: '1h' });
    });
    expect(lastQuery().get('offset')).toBe('0');
  });

  it('keeps the viewed page on a plain refresh', async () => {
    // The regression: a poll used a stale offset and jumped to page one.
    const { result } = renderHook(() => useLogsData({ filters: filters() }));
    await act(async () => {
      await result.current.fetchLogs({ offset: 250 });
    });

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(lastQuery().get('offset')).toBe('250');
    expect(result.current.logOffset).toBe(250);
  });

  it('an explicit offset wins over everything', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    await act(async () => {
      await result.current.fetchLogs({ offset: 99, timeRange: '1h' });
    });

    expect(lastQuery().get('offset')).toBe('99');
  });

  it('reports a rejected request without throwing', async () => {
    globalThis.fetch.mockRejectedValue(new Error('offline'));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let ok;
    await act(async () => {
      ok = await result.current.fetchLogs();
    });

    expect(ok).toBe(false);
    expect(result.current.logLoading).toBe(false);
  });

  it('reports a non-zero envelope as a failure', async () => {
    globalThis.fetch.mockResolvedValue({ ok: true, status: 200, json: async () => ({ code: 500 }) });
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let ok;
    await act(async () => {
      ok = await result.current.fetchLogs();
    });

    expect(ok).toBe(false);
  });

  it('moves the page cursor and the rendered offset together', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    act(() => {
      result.current.goToOffset(150);
    });

    expect(result.current.logOffset).toBe(150);
    // And a following refresh fetches that page, which is what proves the cursor
    // moved with it.
    await act(async () => {
      await result.current.fetchLogs();
    });
    expect(lastQuery().get('offset')).toBe('150');
  });

  it('resets the cursor to the first page', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters() }));
    act(() => {
      result.current.goToOffset(500);
    });

    act(() => {
      result.current.resetOffset();
    });

    expect(result.current.logOffset).toBe(0);
  });

  it('clears every tenant-scoped slice on reset', async () => {
    globalThis.fetch.mockResolvedValue(logResponse([{ id: 1 }], 7));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));
    await act(async () => {
      await result.current.fetchLogs({ offset: 100 });
    });
    expect(result.current.logs).toHaveLength(1);
    expect(result.current.logTotal).toBe(7);

    act(() => {
      result.current.reset();
    });

    expect(result.current.logs).toEqual([]);
    expect(result.current.logTotal).toBe(0);
    expect(result.current.logOffset).toBe(0);
  });

  it('pages through every matching row for the export', async () => {
    // Two full pages then a short one: the loop must stop on the short page.
    const pages = [
      Array.from({ length: 200 }, (_, i) => ({ id: i })),
      Array.from({ length: 200 }, (_, i) => ({ id: 200 + i })),
      Array.from({ length: 5 }, (_, i) => ({ id: 400 + i })),
    ];
    let call = 0;
    globalThis.fetch.mockImplementation(async () => {
      const body = pages[Math.min(call, pages.length - 1)];
      call += 1;
      return { ok: true, status: 200, json: async () => ({ code: 0, data: body }) };
    });
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let rows;
    await act(async () => {
      rows = await result.current.fetchAllLogsForExport();
    });

    expect(rows).toHaveLength(405);
    expect(globalThis.fetch).toHaveBeenCalledTimes(3);
  });

  it('stops the export at the requested ceiling', async () => {
    globalThis.fetch.mockImplementation(async () => ({
      ok: true, status: 200, json: async () => ({ code: 0, data: Array.from({ length: 200 }, (_, i) => ({ id: i })) }),
    }));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let rows;
    await act(async () => {
      rows = await result.current.fetchAllLogsForExport(300);
    });

    expect(rows).toHaveLength(400);
    expect(globalThis.fetch).toHaveBeenCalledTimes(2);
  });

  it('applies the filters to the export too', async () => {
    globalThis.fetch.mockResolvedValue({ ok: true, status: 200, json: async () => ({ code: 0, data: [] }) });
    const { result } = renderHook(() => useLogsData({ filters: filters({ q: 'boom', range: 'today' }) }));

    await act(async () => {
      await result.current.fetchAllLogsForExport();
    });

    const q = lastQuery();
    expect(q.get('q')).toBe('boom');
    expect(q.get('start_time')).toBeTruthy();
  });
});

// --- in-flight invalidation -------------------------------------------------

describe('useLogsData in-flight invalidation', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not write back audit rows that arrived after a reset', async () => {
    let release;
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    globalThis.fetch.mockImplementation(async () => {
      await gate;
      return logResponse([{ id: 1, model: 'previous-account-row' }], 1);
    });

    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: vi.fn(), isZh: true }));

    let pending;
    act(() => {
      pending = result.current.fetchLogs();
    });
    act(() => {
      result.current.reset();
    });

    release();
    await act(async () => {
      await pending;
    });

    expect(result.current.logs).toEqual([]);
    expect(result.current.logTotal).toBe(0);
  });

  it('stops an export that spans a session change', async () => {
    // The export walks pages; a logout midway must not assemble a report from
    // another account's rows.
    let calls = 0;
    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: vi.fn(), isZh: true }));
    globalThis.fetch.mockImplementation(async () => {
      calls += 1;
      if (calls === 2) {
        // The session ends while the second page is in the air.
        act(() => {
          result.current.reset();
        });
      }
      return { ok: true, status: 200, json: async () => ({ code: 0, data: Array.from({ length: 200 }, (_, i) => ({ id: i })) }) };
    });

    let rows;
    await act(async () => {
      rows = await result.current.fetchAllLogsForExport();
    });

    // Only the first page was collected before the generation changed.
    expect(rows).toHaveLength(200);
  });

  it('still applies rows when no reset happened', async () => {
    globalThis.fetch.mockResolvedValue(logResponse([{ id: 5 }], 1));
    const { result } = renderHook(() => useLogsData({ filters: filters(), showToast: vi.fn(), isZh: true }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(result.current.logs).toEqual([{ id: 5 }]);
  });
});
