// Shell regression tests: role-based tab routing, the module loader's language,
// and the workspace teardown on logout.
//
// These started as the BUG-WEB-01 / BUG-WEB-06 / BUG-WEB-07 probes and now assert
// the CORRECT behaviour (they fail on the pre-fix code).
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/App.roleTabs.test.jsx

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent, within } from '@testing-library/react';
import App, { ViewLoader } from './App';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function stubGateway(overrides = {}) {
  const routes = {
    '/api/v1/auth/me': () => jsonResponse({ code: 401 }),
    '/api/v1/public/status': () => jsonResponse({ code: 0, data: {} }),
    '/api/v1/admin/stats/overview': () => jsonResponse({ code: 0, data: {} }),
    '/api/v1/admin/stats/live': () => jsonResponse({ code: 0, data: null }),
    '/api/v1/admin/stats/timeseries': () => jsonResponse({ code: 0, data: null }),
    '/api/v1/admin/keys': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/models': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/models/routes': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/pricing': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/channels': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/logs': () => jsonResponse({ code: 0, data: [], total: 0 }),
    '/api/v1/admin/announcement/config': () =>
      jsonResponse({ code: 0, data: { enabled: true, title: '维护通知', content: '今晚 22:00 维护', level: 'info' } }),
    '/api/v1/announcement': () => jsonResponse({ code: 0, data: null }),
    '/api/v1/user/wallet': () => jsonResponse({ code: 0, data: { balance: 12, orders: [] } }),
    '/api/v1/user/keys': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/auth/login': () =>
      jsonResponse({ code: 0, data: { token: 'token-root', user: { username: 'root', role: 'admin' } } }),
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

async function waitForConsoleLoad(since = 0) {
  await waitFor(() => {
    expect(requestedUrls().slice(since).some((u) => u.includes('/api/v1/admin/stats/overview'))).toBe(true);
  }, { timeout: 8000 });
}

describe('role-based tab routing', () => {
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

  it('keeps an administrator on #/dashboard', async () => {
    signIn('admin', 'root');
    window.history.replaceState(null, '', '/#/dashboard');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    await waitFor(() => expect(window.location.hash).toBe('#/dashboard'), { timeout: 8000 });
  });

  it('renders the self-service usage page for a regular user deep-linking #/dashboard', async () => {
    // Regression: the role guard kept its own list of allowed tabs, omitted
    // `dashboard`, and rewrote the tab to the wallet — while the sidebar, the route
    // resolver and <MyUsageView> all offered it.
    signIn('user', 'alice');
    window.history.replaceState(null, '', '/#/dashboard');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'alice', role: 'user' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    expect(await screen.findByText('我的用量', {}, { timeout: 8000 })).toBeInTheDocument();
    await waitFor(() => expect(window.location.hash).toBe('#/dashboard'), { timeout: 8000 });
  });

  it('lets a regular user open the sidebar "概览" entry', async () => {
    signIn('user', 'alice');
    window.history.replaceState(null, '', '/#/wallet');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'alice', role: 'user' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    const sidebar = within(document.querySelector('aside nav'));
    fireEvent.click(sidebar.getByText('概览'));

    expect(await screen.findByText('我的用量', {}, { timeout: 8000 })).toBeInTheDocument();
    await waitFor(() => expect(window.location.hash).toBe('#/dashboard'), { timeout: 8000 });
  });

  it('sends a regular user to the role\'s first allowed tab for a forbidden deep link', async () => {
    // `channels` is admin-only. The fallback is derived from the same allowed-tab
    // list, so it is the role's first tab (dashboard) rather than a hard-coded page.
    signIn('user', 'alice');
    window.history.replaceState(null, '', '/#/channels');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'alice', role: 'user' } } }),
    });

    render(<App />);
    await waitForConsoleLoad();

    expect(await screen.findByText('我的用量', {}, { timeout: 8000 })).toBeInTheDocument();
    await waitFor(() => expect(window.location.hash).toBe('#/dashboard'), { timeout: 8000 });
    // The admin-only channel view must never be mounted for a regular user.
    expect(requestedUrls().some((u) => u.includes('/api/v1/admin/channels'))).toBe(false);
  });
});

describe('module loader language', () => {
  it('renders the loader in the console language', () => {
    const { unmount } = render(<ViewLoader lang="zh" />);
    expect(screen.getByText('正在加载模块…')).toBeInTheDocument();
    unmount();

    render(<ViewLoader lang="en" />);
    expect(screen.getByText('Loading module…')).toBeInTheDocument();
    expect(screen.queryByText('正在加载模块…')).toBeNull();
  });
});

describe('workspace teardown on logout', () => {
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

  it('does not inherit the previous session\'s key drill-down or announcement dialog', async () => {
    signIn('admin', 'root');
    window.history.replaceState(null, '', '/#/dashboard');
    stubGateway({
      '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
      '/api/v1/admin/keys': () => jsonResponse({
        code: 0,
        data: [{ id: 11, key_id: 'ak_0123456789abcdef0123', tenant_id: 'tenant-11', status: 'active', rpm: 60, tpm: 100, budget: 0, used_cost: 0 }],
      }),
    });

    render(<App />);
    await waitForConsoleLoad();

    // Drill the dashboard into one key, then open the announcement dialog.
    const selector = await screen.findByLabelText('下钻密钥', {}, { timeout: 8000 });
    fireEvent.change(selector, { target: { value: '11' } });
    await waitFor(() => {
      expect(requestedUrls().some((u) => u.includes('/stats/overview?key_id=11'))).toBe(true);
    }, { timeout: 8000 });
    expect(await screen.findByText(/密钥实时速率遥测/, {}, { timeout: 8000 })).toBeInTheDocument();

    fireEvent.click(screen.getByTitle('发布或编辑系统级全员公告'));
    expect(await screen.findByText('发布公告并置顶', {}, { timeout: 8000 })).toBeInTheDocument();

    // Log out. The teardown must drop both pieces of shell state (the dialog covers
    // the header in a browser; the state reset must not depend on that).
    const beforeLogout = requestedUrls().length;
    fireEvent.click(screen.getByTitle('退出登录'));
    await waitFor(() => expect(screen.queryByText('发布公告并置顶')).toBeNull(), { timeout: 8000 });

    // Sign back in through the auth page.
    window.history.replaceState(null, '', '/#/login');
    fireEvent(window, new Event('popstate'));
    const username = await screen.findByPlaceholderText('输入用户名或登录邮箱', {}, { timeout: 8000 });
    fireEvent.change(username, { target: { value: 'root' } });
    fireEvent.change(screen.getByPlaceholderText('输入账户密码'), { target: { value: 'secret123' } });
    fireEvent.submit(username.closest('form'));

    await waitFor(() => {
      expect(requestedUrls().slice(beforeLogout).some((u) => u.includes('/api/v1/admin/stats/overview'))).toBe(true);
    }, { timeout: 8000 });

    // No stale drill-down: the dashboard loads platform-wide, and the announcement
    // dialog does not re-open by itself.
    expect(requestedUrls().slice(beforeLogout).some((u) => u.includes('/stats/overview?key_id='))).toBe(false);
    expect(screen.queryByText(/密钥实时速率遥测/)).toBeNull();
    expect(screen.queryByText('发布公告并置顶')).toBeNull();
  }, 30000);
});
