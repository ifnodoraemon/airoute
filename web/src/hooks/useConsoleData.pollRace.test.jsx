// Console data loader: the newest refresh wins, even when an older poll answers last.
//
// Regression for BUG-H08 (was: a poll that started before a create/edit could answer
// after the mutation's own refresh and put the pre-mutation snapshot back). These
// assertions fail on the pre-fix code.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/hooks/useConsoleData.pollRace.test.jsx

import { describe, it, expect, vi, afterEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useConsoleData } from './useConsoleData';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function deferred() {
  let resolve;
  const promise = new Promise((res) => { resolve = res; });
  return { promise, resolve };
}

const EMPTY = () => jsonResponse({ code: 0, data: [] });

describe('useConsoleData refresh ordering', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('drops a stale poll response that resolves after a newer refresh', async () => {
    const slowPoll = deferred();
    let channelCalls = 0;

    vi.stubGlobal('fetch', vi.fn((url) => {
      const path = String(url);
      if (path.includes('/api/v1/admin/channels')) {
        channelCalls += 1;
        if (channelCalls === 1) return slowPoll.promise;
        return Promise.resolve(jsonResponse({ code: 0, data: [{ id: 2, name: 'just-created' }] }));
      }
      return Promise.resolve(EMPTY());
    }));

    const { result } = renderHook(() => useConsoleData({ adminUser: { username: 'root', role: 'admin' } }));

    // The 8s poll starts and will answer late.
    let pollRefresh;
    act(() => { pollRefresh = result.current.refresh(); });
    await waitFor(() => expect(channelCalls).toBe(1));

    // The operator creates a channel; the action-triggered refresh returns the new list.
    await act(async () => { await result.current.refresh(); });
    expect(result.current.channels.map((c) => c.name)).toEqual(['just-created']);

    // The older poll finally answers with the pre-create list.
    await act(async () => {
      slowPoll.resolve(jsonResponse({ code: 0, data: [{ id: 1, name: 'only-before-create' }] }));
      await pollRefresh;
    });

    // The newer result is still on screen: the superseded response was discarded.
    expect(result.current.channels.map((c) => c.name)).toEqual(['just-created']);
  });

  it('still applies a refresh when no newer one raced it', async () => {
    vi.stubGlobal('fetch', vi.fn((url) => {
      if (String(url).includes('/api/v1/admin/channels')) {
        return Promise.resolve(jsonResponse({ code: 0, data: [{ id: 1, name: 'ch-a' }] }));
      }
      return Promise.resolve(EMPTY());
    }));

    const { result } = renderHook(() => useConsoleData({ adminUser: { username: 'root', role: 'admin' } }));
    await act(async () => { await result.current.refresh(); });
    expect(result.current.channels.map((c) => c.name)).toEqual(['ch-a']);
  });
});
