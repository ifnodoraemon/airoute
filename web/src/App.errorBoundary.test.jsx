// Error boundary: a crashed module shows a recovery UI instead of a blank page.
//
// Regression for BUG-H03 (was: any render-time throw unmounted the whole tree, so
// the console went blank with no way to recover except a refresh). These assertions
// fail on the pre-fix code.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/App.errorBoundary.test.jsx

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';

// A view that explodes while the flag is set — the same shape as a lazily imported
// chunk that failed to evaluate after a deploy (and as a row a panel cannot format).
vi.mock('./components/LogsView', () => ({
  default: function PossiblyExplodingView() {
    if (globalThis.__crashLogsView) throw new Error('Failed to fetch dynamically imported module');
    return <div>审计日志已加载</div>;
  },
}));

import App from './App';
import ErrorBoundary from './components/ErrorBoundary';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function stubGateway() {
  const routes = {
    '/api/v1/auth/me': () => jsonResponse({ code: 0, data: { user: { username: 'root', role: 'admin' } } }),
    '/api/v1/public/status': () => jsonResponse({ code: 0, data: {} }),
    '/api/v1/admin/stats/overview': () => jsonResponse({ code: 0, data: {} }),
    '/api/v1/admin/keys': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/models': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/models/routes': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/pricing': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/channels': () => jsonResponse({ code: 0, data: [] }),
    '/api/v1/admin/logs': () => jsonResponse({ code: 0, data: [], total: 0 }),
    '/api/v1/announcement': () => jsonResponse({ code: 0, data: null }),
  };
  globalThis.fetch.mockImplementation(async (url) => {
    const path = String(url);
    const hit = Object.keys(routes).find((fragment) => path.includes(fragment));
    return hit ? routes[hit]() : jsonResponse({ code: 0, data: [] });
  });
}

describe('console error boundary', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('airoute_admin_token', 'sk-airoute-test');
    localStorage.setItem('airoute_admin_user', JSON.stringify({ username: 'root', role: 'admin' }));
    window.history.replaceState(null, '', '/#/logs');
    vi.stubGlobal('fetch', vi.fn());
    globalThis.__crashLogsView = true;
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    globalThis.__crashLogsView = false;
    localStorage.clear();
    window.history.replaceState(null, '', '/');
  });

  it('shows a recovery panel instead of blanking the console, and retry recovers', async () => {
    stubGateway();

    const { container } = render(<App />);

    // A crashed view no longer empties the page…
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument(), { timeout: 8000 });
    expect(container.textContent).not.toBe('');
    expect(screen.getByText('控制台加载失败')).toBeInTheDocument();
    // …and it explains the likely cause for a chunk failure.
    expect(screen.getByText(/新版本可能已经发布/)).toBeInTheDocument();
    // The shell stays: the console navigation is still there.
    expect(container.querySelector('aside nav')).not.toBeNull();

    // The cause is gone (new chunk deployed / reloaded): retry renders the view.
    globalThis.__crashLogsView = false;
    fireEvent.click(screen.getByRole('button', { name: '重试' }));

    expect(await screen.findByText('审计日志已加载', {}, { timeout: 8000 })).toBeInTheDocument();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('renders a generic message for a non-chunk render error', () => {
    function Boom() {
      throw new Error('Cannot read properties of undefined');
    }
    render(
      <ErrorBoundary lang="zh">
        <Boom />
      </ErrorBoundary>,
    );

    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText(/这个模块在渲染时出错了/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '刷新页面' })).toBeInTheDocument();
  });

  it('localises the fallback for an English console', () => {
    function Boom() {
      throw new Error('boom');
    }
    render(
      <ErrorBoundary lang="en">
        <Boom />
      </ErrorBoundary>,
    );

    expect(screen.getByText('The console failed to load')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });
});
