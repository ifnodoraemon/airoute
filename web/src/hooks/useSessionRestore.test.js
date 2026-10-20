import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { useSessionRestore } from './useSessionRestore';
import { useConsoleSession } from './useConsoleSession';

// A fetch whose resolution the test controls, so the "session answers after the
// user has moved on" case can be exercised deterministically.
function deferredResponse() {
  let resolve;
  const promise = new Promise((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function okResponse(body) {
  return { ok: true, status: 200, json: async () => body };
}

describe('useSessionRestore', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reports a restored session with the profile', async () => {
    const onRestored = vi.fn();
    const onExpired = vi.fn();
    globalThis.fetch.mockResolvedValue(okResponse({ code: 0, data: { user: { username: 'alice' } } }));

    renderHook(() => useSessionRestore({ onRestored, onExpired }));

    await waitFor(() => expect(onRestored).toHaveBeenCalledTimes(1));
    expect(onRestored).toHaveBeenCalledWith({ username: 'alice' });
    expect(onExpired).not.toHaveBeenCalled();
  });

  it('accepts a payload that is the profile itself', async () => {
    const onRestored = vi.fn();
    globalThis.fetch.mockResolvedValue(okResponse({ code: 0, data: { username: 'bob' } }));

    renderHook(() => useSessionRestore({ onRestored, onExpired: vi.fn() }));

    await waitFor(() => expect(onRestored).toHaveBeenCalledWith({ username: 'bob' }));
  });

  it('reports an expired session when the gateway rejects the request', async () => {
    const onRestored = vi.fn();
    const onExpired = vi.fn();
    globalThis.fetch.mockResolvedValue({ ok: false, status: 401, json: async () => ({ code: 401 }) });

    renderHook(() => useSessionRestore({ onRestored, onExpired }));

    await waitFor(() => expect(onExpired).toHaveBeenCalledTimes(1));
    expect(onRestored).not.toHaveBeenCalled();
  });

  it('reports an expired session on a non-zero envelope code', async () => {
    const onExpired = vi.fn();
    globalThis.fetch.mockResolvedValue(okResponse({ code: 401 }));

    renderHook(() => useSessionRestore({ onRestored: vi.fn(), onExpired }));

    await waitFor(() => expect(onExpired).toHaveBeenCalledTimes(1));
  });

  it('reports an expired session when the body is unusable', async () => {
    const onExpired = vi.fn();
    globalThis.fetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => {
        throw new Error('not json');
      },
    });

    renderHook(() => useSessionRestore({ onRestored: vi.fn(), onExpired }));

    // A rejected .json() lands in the catch, which is deliberately silent.
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1));
    expect(onExpired).not.toHaveBeenCalled();
  });

  it('sends a stored legacy token as a bearer header', async () => {
    localStorage.setItem('airoute_admin_token', 'legacy-token');
    globalThis.fetch.mockResolvedValue(okResponse({ code: 401 }));

    renderHook(() => useSessionRestore({ onRestored: vi.fn(), onExpired: vi.fn() }));

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1));
    const [url, options] = globalThis.fetch.mock.calls[0];
    expect(url).toBe('/api/v1/auth/me');
    expect(options.headers.Authorization).toBe('Bearer legacy-token');
    expect(options.credentials).toBe('same-origin');
  });

  it('omits the bearer header when nothing is stored', async () => {
    globalThis.fetch.mockResolvedValue(okResponse({ code: 401 }));

    renderHook(() => useSessionRestore({ onRestored: vi.fn(), onExpired: vi.fn() }));

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1));
    expect(globalThis.fetch.mock.calls[0][1].headers.Authorization).toBeUndefined();
  });

  it('calls the latest handler when the session answers after a re-render', async () => {
    // The regression this guards: the effect used to capture the component's
    // view mode at mount, so a late answer was judged against a stale value.
    const first = vi.fn();
    const second = vi.fn();
    const deferred = deferredResponse();
    globalThis.fetch.mockReturnValue(deferred.promise);

    const { rerender } = renderHook(
      ({ onExpired }) => useSessionRestore({ onRestored: vi.fn(), onExpired }),
      { initialProps: { onExpired: first } }
    );

    // The user has navigated on before the gateway answers.
    rerender({ onExpired: second });
    deferred.resolve({ ok: false, status: 401, json: async () => ({ code: 401 }) });

    await waitFor(() => expect(second).toHaveBeenCalledTimes(1));
    expect(first).not.toHaveBeenCalled();
  });

  it('fetches exactly once even when the handlers change', async () => {
    globalThis.fetch.mockResolvedValue(okResponse({ code: 401 }));

    const { rerender } = renderHook(
      ({ onExpired }) => useSessionRestore({ onRestored: vi.fn(), onExpired }),
      { initialProps: { onExpired: vi.fn() } }
    );
    rerender({ onExpired: vi.fn() });
    rerender({ onExpired: vi.fn() });

    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalledTimes(1));
  });

  it.each(['logout', 'login'])('ignores a stale restore response after %s', async (transition) => {
    const pending = deferredResponse();
    globalThis.fetch.mockReturnValue(pending.promise);
    const { result } = renderHook(() => {
      const session = useConsoleSession({ initialRoute: { viewMode: 'console', authTab: 'login' } });
      useSessionRestore({
        onRestored: session.restoreSession, onExpired: session.endSession,
        getSessionGeneration: session.getSessionGeneration,
      });
      return session;
    });
    act(() => {
      if (transition === 'logout') result.current.signOutLocal();
      else result.current.signIn({ username: 'new-user', role: 'user' });
    });
    await act(async () => {
      pending.resolve(okResponse({ code: 0, data: { username: 'old-user', role: 'admin' } }));
      await pending.promise;
    });
    expect(result.current.adminUser?.username).toBe(transition === 'logout' ? undefined : 'new-user');
    expect(localStorage.getItem('airoute_user') || '').not.toContain('old-user');
  });
});
