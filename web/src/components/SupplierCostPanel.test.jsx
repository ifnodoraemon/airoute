import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import SupplierCostPanel from './SupplierCostPanel';
import PricingManager from './PricingManager';
const response = (data) => ({ ok: true, status: 200, json: async () => ({ code: 0, data }) });
beforeEach(() => window.history.replaceState(null, '', '/'));

it('does not expose or fetch procurement data for a customer even through a saved URL', async () => {
  window.history.replaceState(null, '', '/?pricing.view=suppliers');
  const adminFetch = vi.fn(async () => response([]));
  render(<PricingManager isAdmin={false} adminFetch={adminFetch} showToast={vi.fn()} />);
  await waitFor(() => expect(adminFetch).toHaveBeenCalled());
  expect(screen.queryByRole('button', { name: '渠道采购报价' })).not.toBeInTheDocument();
  expect(adminFetch.mock.calls.some(([url]) => url.includes('supplier-costs'))).toBe(false);
});

it('opens a routed physical model quote, preserves blank caches, then reloads after save', async () => {
  let saved;
  const adminFetch = vi.fn(async (url, options) => {
    if (options?.method === 'POST') {
      saved = JSON.parse(options.body);
      return response(saved);
    }
    return response(saved ? [saved] : []);
  });
  const clear = vi.fn();
  render(
    <SupplierCostPanel
      adminFetch={adminFetch}
      focusChannel="proxy"
      focusModel="physical"
      onClearTarget={clear}
      channels={[{ id: 1, name: 'proxy', upstream_models: ['physical'] }]}
      modelRoutes={[{ model: 'public', providers: [{ channel_id: 1, mapped_model: 'physical' }] }]}
    />
  );
  const model = await screen.findByLabelText('实际上游模型');
  expect(model).toHaveValue('physical');
  fireEvent.change(screen.getByLabelText('报价币种'), { target: { value: 'USD' } });
  fireEvent.change(screen.getByLabelText('输入采购价'), { target: { value: '0' } });
  fireEvent.change(screen.getByLabelText('输出采购价'), { target: { value: '2.5' } });
  fireEvent.change(screen.getByLabelText('缓存读取采购价'), { target: { value: '0' } });
  fireEvent.click(screen.getByRole('button', { name: '保存采购报价' }));
  await waitFor(() => expect(clear).toHaveBeenCalled());
  expect(saved).toEqual({
    channel: 'proxy',
    model: 'physical',
    currency: 'USD',
    billing_mode: 'token',
    input_cost: 0,
    output_cost: 2.5,
    cache_read_cost: 0,
    status: 'active',
  });
  expect(saved).not.toHaveProperty('cache_write_cost');
});

it('submits a request quote without carrying stale token prices', async () => {
  let saved;
  const adminFetch = vi.fn(async (_, options) => {
    if (options?.method === 'POST') saved = JSON.parse(options.body);
    return response([]);
  });
  render(<SupplierCostPanel adminFetch={adminFetch} focusChannel="proxy" focusModel="image" />);
  await screen.findByLabelText('采购计费方式');
  fireEvent.change(screen.getByLabelText('输入采购价'), { target: { value: '10' } });
  fireEvent.change(screen.getByLabelText('采购计费方式'), { target: { value: 'request' } });
  fireEvent.change(screen.getByLabelText('每次请求采购价'), { target: { value: '0.02' } });
  fireEvent.click(screen.getByRole('button', { name: '保存采购报价' }));
  await waitFor(() => expect(saved).toBeDefined());
  expect(saved.fixed_cost).toBe(0.02);
  expect(saved).not.toHaveProperty('input_cost');
});
