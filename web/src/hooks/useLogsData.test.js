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

  it('uses the same typed trace and key filters for loading and exporting', async () => {
    const { result } = renderHook(() => useLogsData({ filters: filters({ session: 'trace-123', correlation_type:'trace_id', key_id:'42' }) }));
    await act(async () => { await result.current.fetchLogs(); });
    expect(lastQuery().get('trace_id')).toBe('trace-123');
    expect(lastQuery().get('session_id')).toBeNull();
    expect(lastQuery().get('key_id')).toBe('42');
    await act(async () => { await result.current.fetchAllLogsForExport(); });
    expect(lastQuery().get('trace_id')).toBe('trace-123');
    expect(lastQuery().get('key_id')).toBe('42');
    await act(async () => { await result.current.fetchLogs({offset:100}); await result.current.fetchLogs({sessionFilter:'chat-456',correlationType:'chat_id'}); });
    expect(lastQuery().get('chat_id')).toBe('chat-456');
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

  it('surfaces a load failure so the console does not render an empty trail', async () => {
    globalThis.fetch.mockResolvedValue({ ok: false, status: 503, json: async () => ({ error: 'database offline' }) });
    const { result } = renderHook(() => useLogsData({ filters: filters(), isZh: true }));

    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(result.current.logError).toContain('database offline');
    expect(result.current.logs).toEqual([]);
  });

  it('clears the failure once a later load succeeds', async () => {
    globalThis.fetch.mockResolvedValue({ ok: false, status: 503, json: async () => ({ error: 'database offline' }) });
    const { result } = renderHook(() => useLogsData({ filters: filters(), isZh: true }));
    await act(async () => {
      await result.current.fetchLogs();
    });

    globalThis.fetch.mockResolvedValue(logResponse([{ id: 1 }], 1));
    await act(async () => {
      await result.current.fetchLogs();
    });

    expect(result.current.logError).toBe('');
    expect(result.current.logs).toEqual([{ id: 1 }]);
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

    let outcome;
    await act(async () => {
      outcome = await result.current.fetchAllLogsForExport();
    });

    expect(outcome.rows).toHaveLength(405);
    expect(outcome.truncated).toBe(false);
    expect(globalThis.fetch).toHaveBeenCalledTimes(3);
  });

  it('stops the export at the requested ceiling', async () => {
    globalThis.fetch.mockImplementation(async () => ({
      ok: true, status: 200, json: async () => ({ code: 0, data: Array.from({ length: 200 }, (_, i) => ({ id: i })) }),
    }));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let outcome;
    await act(async () => {
      outcome = await result.current.fetchAllLogsForExport(300);
    });

    // The cap is a hard limit: the report must not grow to 400 rows, and the
    // caller has to be able to tell the report was shortened. The third request
    // is the one-row probe that confirms more rows really do exist.
    expect(outcome.rows).toHaveLength(300);
    expect(outcome.truncated).toBe(true);
    expect(globalThis.fetch).toHaveBeenCalledTimes(3);
    const probe = String(globalThis.fetch.mock.calls.at(-1)[0]);
    const probeQuery = new URLSearchParams(probe.split('?')[1] || '');
    expect(probeQuery.get('limit')).toBe('1');
    expect(probeQuery.get('offset')).toBe('400');
  });

  it('does not claim truncation when the result set is exactly the cap', async () => {
    // Regression: with exactly 5000 matching rows the loop reached the cap on a
    // page boundary and reported truncated=true without ever asking for the next
    // row, so the UI warned about a shortened report that was in fact complete.
    const total = 400;
    globalThis.fetch.mockImplementation(async (url) => {
      const q = new URLSearchParams(String(url).split('?')[1] || '');
      const limit = Number(q.get('limit'));
      const offset = Number(q.get('offset'));
      const count = Math.max(0, Math.min(limit, total - offset));
      return {
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: Array.from({ length: count }, (_, i) => ({ id: offset + i })) }),
      };
    });
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let outcome;
    await act(async () => {
      outcome = await result.current.fetchAllLogsForExport(400);
    });

    expect(outcome.rows).toHaveLength(400);
    expect(outcome.truncated).toBe(false);
  });

  it('reports truncation when the result set exceeds the cap by a single row', async () => {
    const total = 401;
    globalThis.fetch.mockImplementation(async (url) => {
      const q = new URLSearchParams(String(url).split('?')[1] || '');
      const limit = Number(q.get('limit'));
      const offset = Number(q.get('offset'));
      const count = Math.max(0, Math.min(limit, total - offset));
      return {
        ok: true,
        status: 200,
        json: async () => ({ code: 0, data: Array.from({ length: count }, (_, i) => ({ id: offset + i })) }),
      };
    });
    const { result } = renderHook(() => useLogsData({ filters: filters() }));

    let outcome;
    await act(async () => {
      outcome = await result.current.fetchAllLogsForExport(400);
    });

    expect(outcome.rows).toHaveLength(400);
    expect(outcome.truncated).toBe(true);
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

  it('reports export progress and freezes the relative time window', async () => {
    const pages = [Array.from({ length: 200 }, (_, i) => ({ id: i })), [{ id: 200 }]];
    let call = 0;
    globalThis.fetch.mockImplementation(async () => {
      const body = pages[Math.min(call, pages.length - 1)];
      call += 1;
      return { ok: true, status: 200, json: async () => ({ code: 0, data: body }) };
    });
    const progress = [];
    const { result } = renderHook(() => useLogsData({ filters: filters({ range: 'today' }) }));

    let outcome;
    await act(async () => {
      outcome = await result.current.fetchAllLogsForExport(5000, ({ collected }) => progress.push(collected));
    });

    expect(outcome.rows).toHaveLength(201);
    expect(outcome.truncated).toBe(false);
    expect(progress).toEqual([200, 201]);

    // Both boundaries are pinned once, so pages cannot drift while the export
    // is walking a relative range like "today".
    const q = lastQuery();
    expect(q.get('start_time')).toBeTruthy();
    expect(q.get('end_time')).toBeTruthy();
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

  it('keeps the newest page and its global facets when requests complete out of order', async () => {
    const resolve = [];
    globalThis.fetch.mockImplementation(() => new Promise((done) => resolve.push(done)));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));
    let older, newer;
    act(() => { older = result.current.fetchLogs({ offset: 0 }); });
    act(() => { newer = result.current.fetchLogs({ offset: 50 }); });
    await act(async () => {
      resolve[1]({ ok: true, json: async () => ({ code: 0, data: [{ id: 51 }], total: 100, facets: { models: ['m'], channels: ['historic'] } }) });
      await newer;
    });
    await act(async () => { resolve[0](logResponse([{ id: 1 }], 100)); await older; });
    expect(result.current.logOffset).toBe(50);
    expect(result.current.logs).toEqual([{ id: 51 }]);
    expect(result.current.logFacets.channels).toEqual(['historic']);
  });

  it('does not clear the newer request loading state when an older request finishes', async () => {
    const resolve = [];
    globalThis.fetch.mockImplementation(() => new Promise((done) => resolve.push(done)));
    const { result } = renderHook(() => useLogsData({ filters: filters() }));
    let older, newer;
    act(() => { older = result.current.fetchLogs(); newer = result.current.fetchLogs({ offset: 50 }); });
    await act(async () => { resolve[0](logResponse([], 0)); await older; });
    expect(result.current.logLoading).toBe(true);
    await act(async () => { resolve[1](logResponse([{ id: 51 }], 100)); await newer; });
    expect(result.current.logLoading).toBe(false);
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

    await act(async () => {
      await expect(result.current.fetchAllLogsForExport()).rejects.toThrow('Export cancelled: session changed');
    });
    expect(calls).toBe(2);
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
