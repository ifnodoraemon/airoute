import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useConsoleData } from './useConsoleData';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

// Routes a stubbed fetch by URL so a test can describe the gateway's answers
// instead of the order in which they happen to be requested.
function stubGateway(handlers) {
  globalThis.fetch.mockImplementation(async (url) => {
    const path = String(url);
    const handler = Object.entries(handlers).find(([fragment]) => path.includes(fragment));
    if (!handler) return jsonResponse({ code: 0, data: [] });
    return handler[1]();
  });
}

describe('useConsoleData', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('skips the super-admin endpoints for a regular user', async () => {
    stubGateway({});
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'user' } }));

    await act(async () => {
      await result.current.refresh();
    });

    const requested = globalThis.fetch.mock.calls.map((c) => String(c[0]));
    expect(requested.some((u) => u.includes('/admin/channels'))).toBe(false);
    expect(requested.some((u) => u.includes('/admin/models/routes'))).toBe(false);
    // The endpoints a regular user may call are still requested.
    expect(requested.some((u) => u.includes('/admin/keys'))).toBe(true);
    expect(requested.some((u) => u.includes('/admin/stats/overview'))).toBe(true);
    expect(requested.some((u) => u.includes('/admin/models'))).toBe(true);
    expect(requested.some((u) => u.includes('/admin/pricing'))).toBe(true);
  });

  it('requests the super-admin endpoints for an administrator', async () => {
    stubGateway({});
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    await act(async () => {
      await result.current.refresh();
    });

    const requested = globalThis.fetch.mock.calls.map((c) => String(c[0]));
    expect(requested.some((u) => u.includes('/admin/channels'))).toBe(true);
    expect(requested.some((u) => u.includes('/admin/models/routes'))).toBe(true);
  });

  it('falls back to the stored role when the session state is not visible yet', async () => {
    // The loader can run in the same tick as the login state update.
    localStorage.setItem('airoute_admin_user', JSON.stringify({ role: 'admin', username: 'root' }));
    stubGateway({});
    const { result } = renderHook(() => useConsoleData({ adminUser: null }));

    await act(async () => {
      await result.current.refresh();
    });

    const requested = globalThis.fetch.mock.calls.map((c) => String(c[0]));
    expect(requested.some((u) => u.includes('/admin/channels'))).toBe(true);
  });

  it('fills the slices from the answers', async () => {
    stubGateway({
      '/admin/channels': () => jsonResponse({ code: 0, data: [{ id: 1, name: 'c1' }] }),
      '/admin/keys': () => jsonResponse({ code: 0, data: [{ id: 2, key: 'sk-1' }] }),
      '/admin/stats/overview': () => jsonResponse({ code: 0, data: { total_requests: 7 } }),
      '/admin/models/routes': () => jsonResponse({ code: 0, data: [] }),
      '/admin/models': () => jsonResponse({ code: 0, data: ['gpt-x'] }),
    });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.channels).toEqual([{ id: 1, name: 'c1' }]);
    expect(result.current.keys).toEqual([{ id: 2, key: 'sk-1' }]);
    expect(result.current.stats).toEqual({ total_requests: 7 });
    expect(result.current.models).toEqual(['gpt-x']);
  });

  it('reports failure on a 5xx', async () => {
    stubGateway({ '/admin/stats/overview': () => jsonResponse({ error: 'boom' }, 500) });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    let outcome;
    await act(async () => {
      outcome = await result.current.refresh();
    });

    expect(outcome).toBe(false);
  });

  it('does not treat a permission error as a failure', async () => {
    // A limited account must not see a spurious "refresh failed" toast.
    stubGateway({ '/admin/keys': () => jsonResponse({ error: 'forbidden' }, 403) });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'user' } }));

    let outcome;
    await act(async () => {
      outcome = await result.current.refresh();
    });

    expect(outcome).toBe(true);
  });

  it('reports failure when the request itself rejects', async () => {
    globalThis.fetch.mockRejectedValue(new Error('offline'));
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    let outcome;
    await act(async () => {
      outcome = await result.current.refresh();
    });

    expect(outcome).toBe(false);
  });

  it('derives the pricing groups from the prices and the account group', async () => {
    stubGateway({
      '/admin/pricing': () => jsonResponse({
        code: 0,
        data: [
          { model: 'a', group_name: 'VIP' },
          { model: 'b', group_name: 'default' },
          { model: 'c' },
        ],
      }),
    });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin', group_name: 'Team' } }));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.pricingGroups).toContain('default');
    expect(result.current.pricingGroups).toContain('vip');
    expect(result.current.pricingGroups).toContain('team');
  });

  it('selects a chat model for the playground from the route table', async () => {
    stubGateway({
      '/admin/models/routes': () => jsonResponse({ code: 0, data: [{ modality: 'chat', model: 'routed-chat' }] }),
      '/admin/models': () => jsonResponse({ code: 0, data: ['embed-x'] }),
    });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    await act(async () => {
      await result.current.refresh();
    });

    // The route table wins over name inference.
    expect(result.current.playModel).toBe('routed-chat');
  });

  it('infers a chat model when the route table has none', async () => {
    stubGateway({
      '/admin/models/routes': () => jsonResponse({ code: 0, data: [] }),
      '/admin/models': () => jsonResponse({ code: 0, data: ['text-embedding-3', 'gpt-4o'] }),
    });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.playModel).toBe('gpt-4o');
  });

  it('keeps a playground model that is still offered', async () => {
    stubGateway({
      '/admin/models/routes': () => jsonResponse({ code: 0, data: [{ modality: 'chat', model: 'a' }, { modality: 'chat', model: 'b' }] }),
      '/admin/models': () => jsonResponse({ code: 0, data: ['a', 'b'] }),
    });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    act(() => {
      result.current.setPlayModel('b');
    });
    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.playModel).toBe('b');
  });

  it('clears every tenant-scoped slice on reset', async () => {
    // The regression: stats, prices and pricingGroups used to survive a logout,
    // so the next account could observe the previous one's usage and pricing.
    stubGateway({
      '/admin/channels': () => jsonResponse({ code: 0, data: [{ id: 1 }] }),
      '/admin/keys': () => jsonResponse({ code: 0, data: [{ id: 2 }] }),
      '/admin/stats/overview': () => jsonResponse({ code: 0, data: { total_requests: 7 } }),
      '/admin/models/routes': () => jsonResponse({ code: 0, data: [{ modality: 'chat', model: 'a' }] }),
      '/admin/models': () => jsonResponse({ code: 0, data: ['a'] }),
      '/admin/pricing': () => jsonResponse({ code: 0, data: [{ model: 'a', group_name: 'vip' }] }),
    });
    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    await act(async () => {
      await result.current.refresh();
    });
    // Everything is populated before the reset, so the assertions below are real.
    expect(result.current.channels).toHaveLength(1);
    expect(result.current.keys).toHaveLength(1);
    expect(result.current.models).toHaveLength(1);
    expect(result.current.modelRoutes).toHaveLength(1);
    expect(result.current.prices).toHaveLength(1);
    expect(result.current.stats).toEqual({ total_requests: 7 });
    expect(result.current.pricingGroups).toContain('vip');
    expect(result.current.playModel).toBe('a');

    act(() => {
      result.current.reset();
    });

    expect(result.current.channels).toEqual([]);
    expect(result.current.keys).toEqual([]);
    expect(result.current.models).toEqual([]);
    expect(result.current.modelRoutes).toEqual([]);
    expect(result.current.playModel).toBe('');
    // The three that used to be missed.
    expect(result.current.stats).toEqual({});
    expect(result.current.prices).toEqual([]);
    expect(result.current.pricingGroups).toEqual(['default']);
  });
});

// --- in-flight invalidation -------------------------------------------------
//
// The reported leak: clearing state on logout is not enough, because a request
// already in the air still resolves and writes the previous account's data back
// into the cleared state. These reproduce that ordering exactly.

describe('useConsoleData in-flight invalidation', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('does not write back data that arrived after a reset', async () => {
    let release;
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    globalThis.fetch.mockImplementation(async (url) => {
      await gate;
      if (String(url).includes('/admin/keys')) {
        return jsonResponse({ code: 0, data: [{ id: 1, key: 'sk-previous-account' }] });
      }
      if (String(url).includes('/admin/stats/overview')) {
        return jsonResponse({ code: 0, data: { total_requests: 999 } });
      }
      return jsonResponse({ code: 0, data: [] });
    });

    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    // Start the load, then log out while it is still in the air.
    let pending;
    act(() => {
      pending = result.current.refresh();
    });
    act(() => {
      result.current.reset();
    });

    // Now let the previous session's response land.
    release();
    await act(async () => {
      await pending;
    });

    expect(result.current.keys).toEqual([]);
    expect(result.current.stats).toEqual({});
  });

  it('reports the discarded load as unsuccessful', async () => {
    let release;
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    globalThis.fetch.mockImplementation(async () => {
      await gate;
      return jsonResponse({ code: 0, data: [] });
    });

    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));
    let pending;
    act(() => {
      pending = result.current.refresh();
    });
    act(() => {
      result.current.reset();
    });
    release();

    let outcome;
    await act(async () => {
      outcome = await pending;
    });

    // A caller refreshing the dashboard must not be told the refresh succeeded.
    expect(outcome).toBe(false);
  });

  it('still applies data when no reset happened', async () => {
    // The control: the guard must not break the ordinary path.
    globalThis.fetch.mockImplementation(async (url) => {
      if (String(url).includes('/admin/keys')) {
        return jsonResponse({ code: 0, data: [{ id: 7 }] });
      }
      return jsonResponse({ code: 0, data: [] });
    });

    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.keys).toEqual([{ id: 7 }]);
  });

  it('applies data loaded after a reset in the new session', async () => {
    globalThis.fetch.mockImplementation(async (url) => {
      if (String(url).includes('/admin/keys')) {
        return jsonResponse({ code: 0, data: [{ id: 2, key: 'sk-new-account' }] });
      }
      return jsonResponse({ code: 0, data: [] });
    });

    const { result } = renderHook(() => useConsoleData({ adminUser: { role: 'admin' } }));
    act(() => {
      result.current.reset();
    });

    await act(async () => {
      await result.current.refresh();
    });

    expect(result.current.keys).toEqual([{ id: 2, key: 'sk-new-account' }]);
  });
});
