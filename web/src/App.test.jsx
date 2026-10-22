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
