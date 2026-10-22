import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, renderHook, act } from '@testing-library/react';
import App from './App.jsx';
import OnboardingModal from './components/OnboardingModal.jsx';
import WalletManagementView from './components/WalletManagementView.jsx';
import { useConsoleData } from './hooks/useConsoleData.js';

const user = { id: 77, username: 'audit-customer', role: 'user', balance: 100, group_name: 'default' };
const key = { id: 77, key: 'sk-audit-synthetic-key-only', tenant_id: 'audit-client', status: 'active', allowed_models: ['*'], rpm: 60, budget: 100 };
const response = (data) => ({ ok: true, status: 200, json: async () => data });

beforeEach(() => {
  localStorage.clear();
  window.history.replaceState(null, '', '/');
  vi.stubGlobal('fetch', vi.fn(async () => response({ code: 0, data: [] })));
});
afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); });

describe('Role journey audit: desired outcomes', () => {
  it('customer key telemetry should reach usage information rather than wallet', async () => {
    localStorage.setItem('airoute_admin_token', 'sk-airoute-test');
    localStorage.setItem('airoute_admin_user', JSON.stringify(user));
    window.history.replaceState(null, '', '/#/keys');
    fetch.mockImplementation(async (url) => {
      const path = String(url).split('?')[0];
      const data = {
        '/api/v1/auth/me': { user },
        '/api/v1/admin/keys': [key],
        '/api/v1/admin/stats/overview': {},
        '/api/v1/admin/stats/timeseries': {},
        '/api/v1/admin/models': ['audit-chat'],
        '/api/v1/user/wallet': { ...user, orders: [] },
      };
      return response({ code: 0, data: data[path] ?? [], total: 0 });
    });
    render(<App />);
    fireEvent.click(await screen.findByTitle('查看该密钥的调用记录与扣费', {}, { timeout: 8000 }));
    // Wait until the role guard's effect has settled.
    await waitFor(() => expect(window.location.hash).not.toBe('#/keys'), { timeout: 8000 });
    await new Promise(resolve => setTimeout(resolve, 50));
    expect(window.location.hash).toBe('#/logs');
    await waitFor(() => expect(fetch.mock.calls.some(([url]) => { const u = new URL(String(url), 'http://localhost'); return u.pathname.endsWith('/admin/logs') && u.searchParams.get('key_id') === '77' && !u.searchParams.has('q'); })).toBe(true));
    expect(window.location.search).not.toContain(key.key);
  });

  it('onboarding should use loaded credentials after initially mounting without data', () => {
    const props = { onClose: vi.fn(), showToast: vi.fn(), lang: 'zh' };
    const { rerender } = render(<OnboardingModal {...props} isOpen={false} keys={[]} models={[]} />);
    rerender(<OnboardingModal {...props} isOpen keys={[key]} models={['audit-chat']} />);
    expect(screen.queryByText('sk-airoute-demo-key')).not.toBeInTheDocument();
    expect(screen.getByText(key.key)).toBeInTheDocument();
  });

  it('removing the final available model should clear the model catalog', async () => {
    let models = ['audit-chat'];
    fetch.mockImplementation(async (url) => response({ code: 0, data: String(url).endsWith('/models') ? models : [] }));
    const { result } = renderHook(() => useConsoleData({ adminUser: user }));
    await act(async () => { await result.current.refresh(); });
    expect(result.current.models).toEqual(['audit-chat']);
    models = [];
    await act(async () => { await result.current.refresh(); });
    expect(result.current.models).toEqual([]);
  });

  it('cancelled recharge orders should not be described as still processing', async () => {
    const adminFetch = vi.fn(async () => response({ code: 0, data: { ...user, orders: [
      { id: 1, order_no: 'AUDIT-CANCELLED', amount: 50, channel: 'stripe', status: 'cancelled' },
    ] } }));
    render(<WalletManagementView adminUser={user} adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
    await screen.findByText('AUDIT-CANCELLED');
    expect(screen.queryByText('处理中')).not.toBeInTheDocument();
  });
});
