import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import MyUsageView from './MyUsageView';

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

describe('MyUsageView', () => {
  it('shows only the caller-scoped usage, wallet and key waterlines', async () => {
    const adminFetch = vi.fn(async (url) => {
      if (url.endsWith('/stats/overview')) {
        return envelope({ total_requests: 1234, total_tokens: 5678, total_cost: 1.2345, saved_cost: 0.5 });
      }
      if (url.endsWith('/user/wallet')) {
        return envelope({ balance: 10.5 });
      }
      if (url.endsWith('/user/keys')) {
        return envelope([{ id: 1, key: 'sk-***mask', status: 'active', budget: 5, used_cost: 2.5, rpm: 60, tpm: 100000, used_tokens: 1000 }]);
      }
      throw new Error(`unexpected ${url}`);
    });

    render(<MyUsageView adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);

    expect(await screen.findByText('1,234')).toBeTruthy();
    expect(screen.getByText('5,678')).toBeTruthy();
    expect(screen.getByText('¥1.2345')).toBeTruthy();
    expect(screen.getByText('¥10.5000')).toBeTruthy();
    expect(screen.getByText('sk-***mask')).toBeTruthy();
    expect(screen.getByText('¥2.5000')).toBeTruthy();
    expect(screen.getByText('¥5.0000')).toBeTruthy();
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/stats/overview');
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/user/wallet');
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/user/keys');
  });
});
