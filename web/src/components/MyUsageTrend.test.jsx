import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import MyUsageTrend from './MyUsageTrend';

describe('MyUsageTrend', () => {
  it('shows scoped traffic, failures and a forecast based on measured spend', async () => {
    const adminFetch = vi.fn(async () => ({ ok: true, json: async () => ({ code: 0, data: { buckets: [{ label: '12:00', requests: 7, errors: 2 }] } }) }));
    render(<MyUsageTrend adminFetch={adminFetch} balance={10} dailyCost={2} isZh />);
    expect(await screen.findByText('7 次调用 · 2 次失败')).toBeInTheDocument();
    expect(screen.getByText(/余额约可用 5.0 天/)).toBeInTheDocument();
    expect(screen.getByRole('img', { name: '每时段调用量' })).toBeInTheDocument();
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/stats/timeseries?timeframe=24h');
  });

  it('keeps a failed load retryable and avoids a forecast with no spend', async () => {
    const adminFetch = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 503, json: async () => ({ error: { message: 'unavailable' } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ code: 0, data: { buckets: [] } }) });
    render(<MyUsageTrend adminFetch={adminFetch} balance={10} dailyCost={0} isZh={false} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('unavailable');
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('0 requests · 0 failures')).toBeInTheDocument();
    expect(screen.queryByText(/balance covers/)).not.toBeInTheDocument();
  });
});
