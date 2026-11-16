import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import MyUsageView from './MyUsageView';

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

describe('MyUsageView', () => {
  it('shows only the caller-scoped usage, wallet and key waterlines', async () => {
    const adminFetch = vi.fn(async (url) => {
      if (url.includes('/stats/timeseries')) return envelope({ buckets: [{ requests: 12, errors: 2, label: '12:00' }] });
      if (url.endsWith('/stats/overview')) {
        return envelope({ total_requests: 1234, total_tokens: 5678, total_cost: 1.2345, saved_cost: 0.5 });
      }
      if (url.endsWith('/user/wallet')) {
        return envelope({ balance: 10.5 });
      }
      if (url.endsWith('/user/keys')) {
        return envelope([{ id: 1, key_id: 'ak_1f0013dea24034ac0001', status: 'active', budget: 5, used_cost: 2.5, rpm: 60, tpm: 100000, used_tokens: 1000 }]);
      }
      throw new Error(`unexpected ${url}`);
    });

    render(<MyUsageView adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);

    expect(await screen.findByText('1,234')).toBeTruthy();
    expect(screen.getByText('5,678')).toBeTruthy();
    expect(screen.getByText('¥1.2345')).toBeTruthy();
    expect(screen.getByText('¥10.5000')).toBeTruthy();
    expect(screen.getByText('ak_1f0013dea24034ac0001')).toBeTruthy();
    expect(screen.getByText('¥2.5000')).toBeTruthy();
    expect(screen.getByText('¥5.0000')).toBeTruthy();
    expect(screen.getByText('累计 Tokens')).toBeInTheDocument();
    expect(screen.getByText('TPM 上限')).toBeInTheDocument();
    expect(screen.getByText('1,000')).toBeInTheDocument();
    expect(screen.getByText('100,000')).toBeInTheDocument();
    expect(screen.queryByText('1,000 / 100,000')).not.toBeInTheDocument();
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/stats/overview');
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/user/wallet');
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/user/keys');
  });

  function fetchWithKeys(keys) {
    return vi.fn(async (url) => {
      if (url.includes('/stats/timeseries')) return envelope({ buckets: [] });
      if (url.endsWith('/stats/overview')) return envelope({ total_requests: 1, total_tokens: 1, total_cost: 0, saved_cost: 0 });
      if (url.endsWith('/user/wallet')) return envelope({ balance: 10 });
      if (url.endsWith('/user/keys')) return envelope(keys);
      throw new Error(`unexpected ${url}`);
    });
  }

  it('warns when a key is exhausted or near its budget cap', async () => {
    const adminFetch = fetchWithKeys([
      { id: 1, key_id: 'ak_over', status: 'active', budget: 5, used_cost: 5, rpm: 60, tpm: 1000, used_tokens: 1 },
      { id: 2, key_id: 'ak_near', status: 'active', budget: 10, used_cost: 8.5, rpm: 60, tpm: 1000, used_tokens: 1 },
    ]);
    render(<MyUsageView adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
    expect(await screen.findByText(/预算提醒：1 个密钥已耗尽预算，1 个密钥接近上限/)).toBeInTheDocument();
  });

  it('shows no budget warning when keys are well under the cap', async () => {
    const adminFetch = fetchWithKeys([
      { id: 1, key_id: 'ak_ok', status: 'active', budget: 10, used_cost: 1, rpm: 60, tpm: 1000, used_tokens: 1 },
      { id: 2, key_id: 'ak_unlimited', status: 'active', budget: 0, used_cost: 99, rpm: 60, tpm: 1000, used_tokens: 1 },
    ]);
    render(<MyUsageView adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
    await screen.findByText('ak_ok');
    expect(screen.queryByText(/预算提醒/)).not.toBeInTheDocument();
  });
});
