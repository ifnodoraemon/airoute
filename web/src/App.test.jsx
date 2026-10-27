import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import App from './App';

// The first test that renders the whole console. Everything before this exercised
// hooks and leaf components in isolation, which left the wiring between them — the
// thing that actually breaks in production — unverified.
//
// It is deliberately a smoke net rather than an exhaustive suite: it pins that App
// mounts, resolves its session, routes by hash, and reaches the console with the
// data loaders running. Per-domain behaviour lives in the hook tests.
//
// Two behaviours of the real app shape these tests:
//   - The console is reached by route, not merely by holding a token. An anonymous
//     visitor on "#/" stays on the public landing page even with a valid session.
//   - The console loader only runs when the route resolved to "console".

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function stubGateway(overrides = {}) {
  const routes = {
    '/api/v1/auth/me': () => jsonResponse({ code: 401 }),
    '/api/v1/public/status': () => jsonResponse({ code: 0, data: {} }),
    '/api/v1/admin/stats/overview': () => jsonResponse({ code: 0, data: {} }),
    '/api/v1/admin/keys': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/models': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/models/routes': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/pricing': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/channels': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/logs': () => jsonResponse({ code: 0, data: [], total: 0 }),
    ...overrides,
  };
  globalThis.fetch.mockImplementation(async (url) => {
    const path = String(url);
    const hit = Object.keys(routes).find((fragment) => path.includes(fragment));
    return hit ? routes[hit]() : jsonResponse({ code: 0, data: [] });
  });
}

function signIn(role = 'admin', username = 'root') {
  localStorage.setItem('airoute_admin_token', 'sk-airoute-test');
  localStorage.setItem('airoute_admin_user', JSON.stringify({ username, role }));
}

function requestedUrls() {
  return globalThis.fetch.mock.calls.map((c) => String(c[0]));
}

async function waitForConsoleLoad() {
  await waitFor(() => {
    expect(requestedUrls().some((u) => u.includes('/api/v1/admin/stats/overview'))).toBe(true);
  }, { timeout: 8000 });
}

describe('App integration', () => {
  beforeEach(() => {
    localStorage.clear();
    window.history.replaceState(null, '', '/');
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    window.history.replaceState(null, '', '/');
  });

  it('renders the public landing page for an anonymous visitor', async () => {
    stubGateway();

    render(<App />);

    // The landing view is lazily imported, so this also proves the code-split chunk
    // resolves and the Suspense boundary settles.
    expect(await screen.findByText('Airoute', {}, { timeout: 8000 })).toBeInTheDocument();
  });

  it('keeps a signed-in visitor on the landing page until they choose the console', async () => {
    signIn();
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);

    // The session is restored...
    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/auth/me',
        expect.objectContaining({ credentials: 'same-origin' })
      );
    }, { timeout: 8000 });

    // ...but the console is a route, so its loader must not have run.
    expect(requestedUrls().some((u) => u.includes('/api/v1/admin/stats/overview'))).toBe(false);
  });

  it('loads the console data when the route asks for the console', async () => {
    signIn();
    window.history.replaceState(null, '', '/#/console');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);

    await waitForConsoleLoad();
  });

  it('restores the audit page named in the URL', async () => {
    signIn();
    // Filters/page live in the query string while the route lives in the hash.
    window.history.replaceState(null, '', '/?logs.page=3#/logs');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);

    // Page 3 of a 50-row page is offset 100; a refresh or shared link must land
    // there instead of silently jumping back to the first page.
    await waitFor(() => {
      expect(requestedUrls().some((u) => u.includes('/api/v1/admin/logs') && u.includes('offset=100'))).toBe(true);
    }, { timeout: 8000 });
  });

  it('restores the dashboard deep link from a cookie without any legacy token or profile hint', async () => {
    window.history.replaceState(null, '', '/#/dashboard');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });
    render(<App />);
    await waitForConsoleLoad();
    await waitFor(() => expect(window.location.hash).toBe('#/dashboard'));
    expect(localStorage.getItem('airoute_token')).toBeNull();
    expect(await screen.findByText('需要关注')).toBeInTheDocument();
  });

  it('opens all failed logs from the overview and clears unrelated saved filters', async () => {
    window.history.replaceState(null, '', '/?logs.channel=old&logs.range=1h&logs.q=other#/dashboard');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
      '/api/v1/admin/stats/overview': () => jsonResponse({ code: 0, data: { total_requests: 100, server_error_requests: 2 } }),
    });
    render(<App />);
    fireEvent.click(await screen.findByRole('button', { name: /查看日志/ }, { timeout: 8000 }));
    await waitFor(() => {
      const urls = requestedUrls().filter((url) => url.includes('/admin/logs?'));
      const query = new URL(urls.at(-1), 'http://test').searchParams;
      expect(query.get('status')).toBe('error');
      expect(query.has('channel')).toBe(false);
      expect(query.has('q')).toBe(false);
      expect(query.has('start_time')).toBe(false);
      expect(query.get('offset')).toBe('0');
    });
  }, 12000);

  it('does not request the super-admin endpoints for a regular user', async () => {
    signIn('user', 'alice');
    window.history.replaceState(null, '', '/#/console');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'alice', role: 'user' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    const urls = requestedUrls();
    // The role gate that used to produce a permanent 403 storm for regular users.
    expect(urls.some((u) => u.includes('/api/v1/admin/channels'))).toBe(false);
    expect(urls.some((u) => u.includes('/api/v1/admin/models/routes'))).toBe(false);
  });

  it('requests the super-admin endpoints for an administrator', async () => {
    signIn();
    window.history.replaceState(null, '', '/#/console');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    const urls = requestedUrls();
    expect(urls.some((u) => u.includes('/api/v1/admin/channels'))).toBe(true);
    expect(urls.some((u) => u.includes('/api/v1/admin/models/routes'))).toBe(true);
  });

  it('leaves the console when the stored session is rejected', async () => {
    // The expiry transition. The console is entered optimistically — the stored
    // marker is trusted until the gateway answers, which is what makes a refresh
    // feel instant — so a load may already have started. What must happen is that a
    // 401 moves the visitor back out to the landing page instead of leaving them in
    // a console whose every panel fails.
    signIn();
    window.history.replaceState(null, '', '/#/console');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 401 }, 401),
    });

    render(<App />);

    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalledWith(
        '/api/v1/auth/me',
        expect.objectContaining({ credentials: 'same-origin' })
      );
    }, { timeout: 8000 });

    // The landing page is rendered again, which only happens on the transition out
    // of the console.
    expect(await screen.findByText('Developer Guide', {}, { timeout: 8000 })).toBeInTheDocument();
  });

  it('clears the stored session when the gateway rejects it', async () => {
    signIn();
    window.history.replaceState(null, '', '/#/console');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 401 }, 401),
    });

    render(<App />);

    await waitFor(() => {
      expect(localStorage.getItem('airoute_admin_token')).toBeFalsy();
    }, { timeout: 8000 });
  });

  it('keeps the log filters in the URL when the visitor stops by the public pages', async () => {
    // logs.*/channels.* live in the query string so a filtered audit view can be
    // shared and reloaded. Navigating to the public status page rebuilt the URL from
    // the pathname, which silently dropped them.
    signIn();
    window.history.replaceState(null, '', '/?logs.q=boom&logs.status=error#/dashboard');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    fireEvent.click(await screen.findByTitle('查看公开服务状态', {}, { timeout: 8000 }));

    await waitFor(() => expect(window.location.hash).toBe('#/status'), { timeout: 8000 });
    const params = new URLSearchParams(window.location.search);
    expect(params.get('logs.q')).toBe('boom');
    expect(params.get('logs.status')).toBe('error');
  });

  it('does not show the previous account data when the next account signs in on the auth page', async () => {
    // Tenant teardown. Every login entry point must start from an empty workspace;
    // the auth page (#/login) skips the reset, so an account that signed in there
    // inherited the previous session's keys. That only becomes visible when the new
    // account's own load does not replace the slice — here the key list fails, which
    // must leave an empty list rather than the previous tenant's keys on screen.
    signIn('admin', 'alice');
    window.history.replaceState(null, '', '/#/keys');
    let signedInAs = 'alice';
    let expired = false;
    let keyListFails = false;
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: signedInAs, role: 'admin' } } }),
      '/api/v1/auth/login': () => {
        signedInAs = 'bob';
        return jsonResponse({ code: 0, data: { token: 'token-bob', user: { username: 'bob', role: 'admin' } } });
      },
      '/api/v1/admin/keys': () => {
        if (expired && !keyListFails) return jsonResponse({ code: 401, error: 'expired' }, 401);
        if (keyListFails) return jsonResponse({ error: 'keys unavailable' }, 500);
        return jsonResponse({
          code: 0,
          data: [{
            id: signedInAs === 'alice' ? 11 : 22,
            key: signedInAs === 'alice' ? 'sk-alice-key' : 'sk-bob-key',
            tenant_id: signedInAs === 'alice' ? 'tenant-alice' : 'tenant-bob',
            status: 'active',
            rpm: 60,
          }],
        });
      },
      '/api/v1/admin/stats/overview': () => (expired
        ? jsonResponse({ code: 401, error: 'expired' }, 401)
        : jsonResponse({ code: 0, data: {} })),
      '/api/v1/admin/logs': () => (expired
        ? jsonResponse({ code: 401, error: 'expired' }, 401)
        : jsonResponse({ code: 0, data: [], total: 0 })),
    });

    render(<App />);

    // Account A is looking at its own key.
    expect(await screen.findByText('tenant-alice', {}, { timeout: 8000 })).toBeInTheDocument();

    // Expire the session: the dashboard refresh issues tenant-scoped requests, which
    // now answer 401, and the shell moves the visitor out of the console.
    fireEvent.click(await screen.findByTitle('概览', {}, { timeout: 8000 }));
    const refreshButton = await screen.findByTitle('重新拉取统计、渠道与日志数据', {}, { timeout: 8000 });
    expired = true;
    fireEvent.click(refreshButton);
    await waitFor(() => expect(screen.queryByTitle('重新拉取统计、渠道与日志数据')).not.toBeInTheDocument(), { timeout: 8000 });

    // With no session left, the auth route renders the sign-in page.
    window.history.replaceState(null, '', '/#/login');
    fireEvent(window, new Event('popstate'));

    const username = await screen.findByPlaceholderText('输入用户名或登录邮箱', {}, { timeout: 8000 });
    fireEvent.change(username, { target: { value: 'bob' } });
    fireEvent.change(screen.getByPlaceholderText('输入账户密码'), { target: { value: 'secret123' } });
    // The new account's key list is unavailable, so nothing may replace the old one.
    keyListFails = true;
    expired = false;
    fireEvent.submit(username.closest('form'));

    // Account B's console must not show account A's key.
    fireEvent.click(await screen.findByTitle('API 密钥', {}, { timeout: 8000 }));
    await waitFor(() => expect(screen.queryByText('tenant-alice')).not.toBeInTheDocument(), { timeout: 8000 });
  }, 30000);

  it('sends the audit-log query when the logs tab is routed to', async () => {
    signIn();
    window.history.replaceState(null, '', '/#/logs');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);

    await waitFor(() => {
      expect(requestedUrls().some((u) => u.includes('/api/v1/admin/logs'))).toBe(true);
    }, { timeout: 8000 });
  });
});
