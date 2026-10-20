import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useApiSession } from './useApiSession';
import { apiFetch, resetApiClient } from '../api/client';

function lastFetchOptions() {
  const calls = globalThis.fetch.mock.calls;
  return calls[calls.length - 1][1];
}

describe('useApiSession', () => {
  beforeEach(() => {
    resetApiClient();
    localStorage.clear();
    document.cookie = 'airoute_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/';
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ code: 0, data: {} }),
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('hands the current token to the request layer', async () => {
    renderHook(() => useApiSession({ token: 'tok-1', onUnauthorized: () => {} }));

    await apiFetch('/api/v1/admin/keys');

    expect(lastFetchOptions().headers.Authorization).toBe('Bearer tok-1');
  });

  it('reflects a token change without re-registering the client', async () => {
    // This is the stale-closure hazard: a client that captured the first token
    // would keep sending it after the session changed.
    const { rerender } = renderHook(
      ({ token }) => useApiSession({ token, onUnauthorized: () => {} }),
      { initialProps: { token: 'tok-1' } }
    );

    await apiFetch('/api/v1/admin/keys');
    rerender({ token: 'tok-2' });
    await apiFetch('/api/v1/admin/keys');

    const calls = globalThis.fetch.mock.calls;
    expect(calls[0][1].headers.Authorization).toBe('Bearer tok-1');
    expect(calls[1][1].headers.Authorization).toBe('Bearer tok-2');
  });

  it('routes a 401 to the most recently supplied handler', async () => {
    const stale = vi.fn();
    const current = vi.fn();

    const { rerender } = renderHook(
      ({ onUnauthorized }) => useApiSession({ token: 'tok', onUnauthorized }),
      { initialProps: { onUnauthorized: stale } }
    );
    rerender({ onUnauthorized: current });

    globalThis.fetch.mockResolvedValue({ ok: false, status: 401, json: async () => ({}) });
    await apiFetch('/api/v1/admin/keys');

    expect(stale).not.toHaveBeenCalled();
    expect(current).toHaveBeenCalledTimes(1);
  });

  it('passes the token that failed to the handler', async () => {
    const onUnauthorized = vi.fn();
    renderHook(() => useApiSession({ token: 'tok-9', onUnauthorized }));

    globalThis.fetch.mockResolvedValue({ ok: false, status: 401, json: async () => ({}) });
    await apiFetch('/api/v1/admin/keys');

    expect(onUnauthorized.mock.calls[0][0].token).toBe('tok-9');
  });
});
