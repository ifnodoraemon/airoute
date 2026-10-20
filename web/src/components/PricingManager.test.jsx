import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import PricingManager from './PricingManager';

// The regression this file exists for: the gateway used to send
// `is_current_off_peak` / `effective_discount` while this component read
// `is_currently_off_peak` / `current_discount` and two `effective_*_price`
// fields that did not exist. Every value therefore arrived undefined, so the
// "live effective rate" column rendered the standard price for every model,
// off-peak or not. Nothing failed, because no test rendered the component.
//
// The payloads below use the wire shape the Go handler actually produces; the
// contract test on the Go side pins the same names.

function pricingResponse(rows, extra = {}) {
  return {
    ok: true,
    status: 200,
    json: async () => ({
      code: 0,
      data: rows,
      server_time: '03:15:00',
      server_weekday: 'Monday',
      is_weekend: false,
      ...extra,
    }),
  };
}

function modelsResponse() {
  return { ok: true, status: 200, json: async () => ({ code: 0, data: ['gpt-x'] }) };
}

function renderPricing(rows, extra = {}) {
  const showToast = vi.fn();
  const adminFetch = vi.fn(async (url) => {
    if (String(url).includes('/admin/models')) return modelsResponse();
    return pricingResponse(rows, extra);
  });
  const utils = render(
    <PricingManager adminFetch={adminFetch} showToast={showToast} isAdmin lang="zh" />
  );
  return { ...utils, adminFetch, showToast };
}

const offPeakRow = {
  model: 'gpt-x',
  group_name: 'default',
  prompt_price: 10,
  completion_price: 20,
  currency: 'CNY',
  is_currently_off_peak: true,
  current_discount: 0.5,
  effective_prompt_price: 5,
  effective_completion_price: 10,
  off_peak_slots: '',
  off_peak_start: '00:00',
  off_peak_end: '08:30',
};

const standardRow = {
  ...offPeakRow,
  model: 'gpt-y',
  is_currently_off_peak: false,
  current_discount: 1,
  effective_prompt_price: 10,
  effective_completion_price: 20,
};

describe('PricingManager off-peak display', () => {
  it('preserves the complete model and group when deleting pricing', async () => {
    const model = 'vendor/team/模型 +%2F?#&';
    const group = 'vip +&/组';
    const { adminFetch, showToast } = renderPricing([{ ...offPeakRow, model, group_name: group }]);
    await screen.findByText(model);
    adminFetch.mockImplementation(async (_url, options) => (
      options?.method === 'DELETE'
        ? { ok: true, json: async () => ({ code: 0 }) }
        : pricingResponse([])
    ));
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    try {
      fireEvent.click(screen.getByRole('button', { name: '删除', exact: true }));
      await waitFor(() => expect(showToast).toHaveBeenCalledWith(`已删除 [${group}] 组模型 [${model}] 定价规则`, 'success'));
      const [target] = adminFetch.mock.calls.find(([, options]) => options?.method === 'DELETE');
      const url = new URL(target, window.location.origin);
      expect(url.pathname).toBe('/api/v1/admin/pricing');
      expect(url.searchParams.get('model')).toBe(model);
      expect(url.searchParams.get('group')).toBe(group);
      await waitFor(() => expect(screen.queryByText(model)).not.toBeInTheDocument());
    } finally {
      confirm.mockRestore();
    }
  });

  it('shows saved-but-not-applied warnings instead of a successful reload toast', async () => {
    const { adminFetch, showToast } = renderPricing([offPeakRow]);
    await screen.findByText('gpt-x');
    adminFetch.mockImplementation(async (url, options) => {
      if (options?.method === 'POST') return { ok: true, status: 202, json: async () => ({ code: 0, warning: '配置已保存，但热加载未完全生效' }) };
      return String(url).includes('/admin/models') ? modelsResponse() : pricingResponse([offPeakRow]);
    });
    fireEvent.click(screen.getByRole('button', { name: '编辑' }));
    fireEvent.click(screen.getByRole('button', { name: '保存定价' }));
    await waitFor(() => expect(showToast).toHaveBeenCalledWith('配置已保存，但热加载未完全生效', 'warning'));
    expect(showToast.mock.calls.some(([, tone]) => tone === 'success')).toBe(false);
  });
  // The effective-rate cell renders "输入 ¥5.00 / 输出 ¥10.00" as one string, so
  // matching the whole phrase identifies that column. Matching a bare number does
  // not: the table has other columns showing the list price.
  function rowText(model) {
    const cell = screen.getByText(model);
    const row = cell.closest('tr');
    if (!row) throw new Error(`no row for ${model}`);
    return row.textContent;
  }

  it('shows the discounted rate while off-peak is active', async () => {
    renderPricing([offPeakRow]);

    await waitFor(() => expect(screen.getByText('gpt-x')).toBeInTheDocument());

    // The effective rate, not the list price.
    expect(rowText('gpt-x')).toContain('输入 ¥5.00 / 输出 ¥10.00');
    // And the badge that says why.
    expect(screen.getByText(/生效中/)).toBeInTheDocument();
  });

  it('shows the standard rate when off-peak is not active', async () => {
    renderPricing([standardRow]);

    await waitFor(() => expect(screen.getByText('gpt-y')).toBeInTheDocument());

    expect(rowText('gpt-y')).toContain('输入 ¥10.00 / 输出 ¥20.00');
    expect(screen.getByText('基准单价')).toBeInTheDocument();
    expect(screen.queryByText(/生效中/)).not.toBeInTheDocument();
  });

  it('renders both rows correctly side by side', async () => {
    renderPricing([offPeakRow, standardRow]);

    await waitFor(() => expect(screen.getByText('gpt-x')).toBeInTheDocument());

    // The two rows disagree about whether the discount applies.
    expect(rowText('gpt-x')).toContain('输入 ¥5.00 / 输出 ¥10.00');
    expect(rowText('gpt-y')).toContain('输入 ¥10.00 / 输出 ¥20.00');
    // Exactly one row is marked as off-peak.
    expect(screen.getAllByText(/生效中/)).toHaveLength(1);
    expect(screen.getAllByText('基准单价')).toHaveLength(1);
  });

  it('shows the server clock so the off-peak state is interpretable', async () => {
    renderPricing([offPeakRow]);

    await waitFor(() => expect(screen.getByText('gpt-x')).toBeInTheDocument());

    expect(screen.getByText(/Monday/)).toBeInTheDocument();
    expect(screen.getByText(/03:15:00/)).toBeInTheDocument();
  });

  it('marks a weekend when the gateway reports one', async () => {
    renderPricing([offPeakRow], { server_weekday: 'Sunday', is_weekend: true });

    await waitFor(() => expect(screen.getByText('gpt-x')).toBeInTheDocument());

    expect(screen.getByText(/Sunday/)).toBeInTheDocument();
    expect(screen.getByText(/周末/)).toBeInTheDocument();
  });

  it('falls back to the list price when the gateway omits the effective rate', async () => {
    // An older gateway that does not send the effective fields must not crash the
    // column: the component falls back to the stored price.
    const legacy = { ...offPeakRow };
    delete legacy.effective_prompt_price;
    delete legacy.effective_completion_price;
    renderPricing([legacy]);

    await waitFor(() => expect(screen.getByText('gpt-x')).toBeInTheDocument());

    expect(rowText('gpt-x')).toContain('输入 ¥10.00 / 输出 ¥20.00');
  });
});
