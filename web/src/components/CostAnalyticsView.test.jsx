import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import CostAnalyticsView from './CostAnalyticsView';

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

const overview = {
  requests: 2, revenue: 4, cost: 2, margin: 2, margin_percent: 50,
  billed_units: 4000000000, cost_basis_units: 2000000000,
  known_cost_requests: 1, unknown_cost_requests: 1,
};
const groups = [
  { key: '1', label: '单位A', requests: 1, revenue: 3, cost: 2, margin: 1, margin_percent: 33.3, known_cost_requests: 1 },
];

function setup() {
  const adminFetch = vi.fn(async (url, options = {}) => {
    if (url === '/api/v1/admin/cost/overview') return envelope(overview);
    if (url.startsWith('/api/v1/admin/cost/breakdown')) return envelope(groups);
    if (url === '/api/v1/admin/supplier-costs' && options.method === 'POST') {
      return envelope({ id: 1, channel: 'ch-a', model: 'm-1', input_cost: 2, output_cost: 4, status: 'active' });
    }
    if (url === '/api/v1/admin/supplier-costs') return envelope([]);
    throw new Error(`unexpected ${url} ${options.method || 'GET'}`);
  });
  render(<CostAnalyticsView adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
  return { adminFetch };
}

describe('CostAnalyticsView', () => {
  it('renders revenue, cost, margin and the unpriced count', async () => {
    setup();
    expect(await screen.findByText('50.00%')).toBeTruthy();
    expect(screen.getByText('单位A')).toBeTruthy();
    expect(screen.getByText('未定价请求')).toBeTruthy();
    expect(screen.getAllByText('¥4.0000').length).toBeGreaterThan(0);
    expect(screen.getAllByText('¥2.0000').length).toBeGreaterThan(0);
  });

  it('switches the breakdown group and creates a supplier price', async () => {
    const { adminFetch } = setup();
    await screen.findByText('单位A');

    fireEvent.click(screen.getByRole('button', { name: '通道' }));
    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/cost/breakdown?group=channel');
    });

    fireEvent.change(screen.getByLabelText('通道名'), { target: { value: 'ch-a' } });
    fireEvent.change(screen.getByLabelText('模型'), { target: { value: 'm-1' } });
    fireEvent.change(screen.getByLabelText('输入成本'), { target: { value: '2' } });
    fireEvent.change(screen.getByLabelText('输出成本'), { target: { value: '4' } });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/supplier-costs', expect.objectContaining({ method: 'POST' }));
    });
    const call = adminFetch.mock.calls.find(([url, options]) => url === '/api/v1/admin/supplier-costs' && options?.method === 'POST');
    expect(JSON.parse(call[1].body)).toMatchObject({ channel: 'ch-a', model: 'm-1', input_cost: 2, output_cost: 4 });
  });
});
