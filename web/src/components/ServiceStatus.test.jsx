import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';
import ServiceStatus from './ServiceStatus';

// The public status page is reachable without an account. It used to drive the
// console's tenant loader (`onRefresh`) from its 30-second poll, which issued four
// unauthorized /admin/* requests per anonymous visitor every 30 seconds, and it
// performed the requests inside a setState updater (an updater may be replayed).

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function publicData(over = {}) {
  return {
    status: 'operational',
    models: [{ model: 'deepseek-v4-flash', name: 'DeepSeek V4 Flash', modality: 'chat' }],
    ...over,
  };
}

function renderStatus(props = {}) {
  const onRefresh = vi.fn();
  const adminFetch = vi.fn(async () => jsonResponse({ code: 0, data: {} }));
  const utils = render(
    <ServiceStatus
      lang="zh"
      onRefresh={onRefresh}
      adminFetch={adminFetch}
      {...props}
    />,
  );
  return { ...utils, onRefresh, adminFetch };
}

function requestedPaths() {
  return globalThis.fetch.mock.calls.map((call) => String(call[0]));
}

describe('ServiceStatus public polling', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ code: 0, data: publicData() })));
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('only calls the public endpoint for an anonymous visitor', async () => {
    const { onRefresh } = renderStatus({ isLoggedIn: false });

    await act(async () => {});
    expect(requestedPaths().every((p) => p.includes('/api/v1/public/status'))).toBe(true);

    // Two full refresh cycles: still public-only, and the console loader is untouched.
    await act(async () => { await vi.advanceTimersByTimeAsync(61000); });
    expect(onRefresh).not.toHaveBeenCalled();
    expect(requestedPaths().every((p) => p.includes('/api/v1/public/status'))).toBe(true);
  });

  it('fires exactly one public request per 30s cycle, not one per tick', async () => {
    renderStatus({ isLoggedIn: false });

    await act(async () => {});
    const initial = requestedPaths().length;

    // 29 one-second ticks must not re-issue the request.
    await act(async () => { await vi.advanceTimersByTimeAsync(29000); });
    expect(requestedPaths().length).toBe(initial);

    // The 30th tick does.
    await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
    expect(requestedPaths().length).toBe(initial + 1);
  });

  it('refreshes the console data on the same cadence for a signed-in session', async () => {
    const { onRefresh } = renderStatus({ isLoggedIn: true });

    await act(async () => {});
    expect(onRefresh).toHaveBeenCalledTimes(1);

    await act(async () => { await vi.advanceTimersByTimeAsync(30000); });
    expect(onRefresh).toHaveBeenCalledTimes(2);
  });

  it('does not poll the console loader from the manual refresh button when anonymous', async () => {
    const { onRefresh } = renderStatus({ isLoggedIn: false });

    await act(async () => {});
    fireEvent.click(screen.getByTitle('刷新状态'));

    await act(async () => {});
    expect(onRefresh).not.toHaveBeenCalled();
    expect(requestedPaths().filter((p) => p.includes('/api/v1/public/status')).length).toBeGreaterThanOrEqual(2);
  });
});
