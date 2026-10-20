import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
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
    window.location.hash = '';
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
    window.location.hash = '';
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
    window.location.hash = '#/console';
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);

    await waitForConsoleLoad();
  });

  it('does not request the super-admin endpoints for a regular user', async () => {
    signIn('user', 'alice');
    window.location.hash = '#/console';
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
    window.location.hash = '#/console';
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    const urls = requestedUrls();
    expect(urls.some((u) => u.includes('/api/v1/admin/channels'))).toBe(true);
    expect(urls.some((u) => u.includes('/api/v1/admin/models/routes'))).toBe(true);
  });

  it('sends the audit-log query when the logs tab is routed to', async () => {
    signIn();
    window.location.hash = '#/logs';
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);

    await waitFor(() => {
      expect(requestedUrls().some((u) => u.includes('/api/v1/admin/logs'))).toBe(true);
    }, { timeout: 8000 });
  });
});
