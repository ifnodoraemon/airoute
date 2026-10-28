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

it('does not apply the VIP multiplier twice to a group-specific price', async () => {
  renderPricing([
    {model:'gpt-x',group_name:'default',prompt_price:20,completion_price:40,currency:'CNY'},
    {model:'gpt-x',group_name:'vip',prompt_price:10,completion_price:20,currency:'CNY'},
  ], {group_multipliers:{default:1,vip:0.8}});
  await screen.findAllByText('gpt-x');
  const preview=screen.getAllByRole('combobox').find(select=>Array.from(select.options).some(option=>option.value==='vip'));
  fireEvent.change(preview,{target:{value:'vip'}});
  expect(screen.getByText(/输入 ¥10\.00 \/ 输出 ¥20\.00/)).toBeInTheDocument();
  expect(screen.queryByText(/输入 ¥8\.00 \/ 输出 ¥16\.00/)).not.toBeInTheDocument();
  expect(screen.getByText('使用该用户组专属定价')).toBeInTheDocument();
});

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

  it('renders the group discount tiers tab and displays group multipliers', async () => {
    renderPricing([standardRow], {
      group_multipliers: { default: 1.0, vip: 0.8, enterprise: 1.5 }
    });

    await waitFor(() => expect(screen.getByText('gpt-y')).toBeInTheDocument());

    // Click on the Group Discount Tiers sub-tab
    const groupTabBtn = screen.getByRole('button', { name: /分组折扣定价/ });
    expect(groupTabBtn).toBeInTheDocument();
    fireEvent.click(groupTabBtn);

    // Should render the group discount header and group cards
    await waitFor(() => expect(screen.getByText('业务分组折扣定价体系')).toBeInTheDocument());
    expect(screen.getByText('DEFAULT')).toBeInTheDocument();
    expect(screen.getByText('VIP')).toBeInTheDocument();
    expect(screen.getByText('ENTERPRISE')).toBeInTheDocument();
    expect(screen.getByText(/8折优惠/)).toBeInTheDocument();

    // Switch back to model rates
    const modelTabBtn = screen.getByRole('button', { name: /模型官方基准定价/ });
    fireEvent.click(modelTabBtn);
    await waitFor(() => expect(screen.getByText('gpt-y')).toBeInTheDocument());
  });
});

// The add/edit rule sheet holds a whole pricing configuration (base rates plus
// off-peak slots). Dismissing it has to be guarded like every other form modal:
// backdrop, X, Cancel and Escape all route through the unsaved-changes guard.
describe('PricingManager add/edit modal unsaved-changes guard', () => {
  const MODAL_TITLE = '新建模型定价与分时规则';
  const DISCARD_TITLE = '放弃未保存的定价修改？';

  async function openDirtyAddModal() {
    const utils = renderPricing([offPeakRow]);
    await screen.findByText('gpt-x');
    fireEvent.click(screen.getByRole('button', { name: '新建定价规则' }));
    await screen.findByText(MODAL_TITLE);
    // Typing a model makes the draft differ from the values it opened with.
    fireEvent.change(screen.getByPlaceholderText(/如 deepseek-chat 或点击下方一键选择/), {
      target: { value: 'gpt-new' },
    });
    return utils;
  }

  it('prompts before discarding typed pricing edits via the close (X) button', async () => {
    await openDirtyAddModal();

    fireEvent.click(screen.getByRole('button', { name: '关闭' }));

    expect(await screen.findByText(DISCARD_TITLE)).toBeInTheDocument();

    // Keep editing returns to the same draft, nothing lost.
    fireEvent.click(screen.getByRole('button', { name: '继续编辑' }));
    expect(screen.queryByText(DISCARD_TITLE)).not.toBeInTheDocument();
    expect(screen.getByDisplayValue('gpt-new')).toBeInTheDocument();

    // Discard actually closes.
    fireEvent.click(screen.getByRole('button', { name: '关闭' }));
    fireEvent.click(await screen.findByRole('button', { name: '放弃并关闭' }));
    await waitFor(() => expect(screen.queryByText(MODAL_TITLE)).not.toBeInTheDocument());
  });

  it('prompts before discarding via Escape', async () => {
    await openDirtyAddModal();

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(await screen.findByText(DISCARD_TITLE)).toBeInTheDocument();
  });

  it('prompts before discarding via the backdrop', async () => {
    await openDirtyAddModal();

    fireEvent.click(screen.getByTestId('pricing-modal-overlay'));

    expect(await screen.findByText(DISCARD_TITLE)).toBeInTheDocument();
  });

  it('closes an untouched modal immediately, without a prompt', async () => {
    renderPricing([offPeakRow]);
    await screen.findByText('gpt-x');
    fireEvent.click(screen.getByRole('button', { name: '新建定价规则' }));
    await screen.findByText(MODAL_TITLE);

    // The prefilled defaults match the values the sheet opened with.
    fireEvent.click(screen.getByRole('button', { name: '关闭' }));

    await waitFor(() => expect(screen.queryByText(MODAL_TITLE)).not.toBeInTheDocument());
    expect(screen.queryByText(DISCARD_TITLE)).not.toBeInTheDocument();
  });

  it('closes without a prompt after a successful save', async () => {
    const { adminFetch, showToast } = await openDirtyAddModal();
    adminFetch.mockImplementation(async (url, options) => {
      if (options?.method === 'POST') return { ok: true, status: 200, json: async () => ({ code: 0 }) };
      return String(url).includes('/admin/models') ? modelsResponse() : pricingResponse([offPeakRow]);
    });

    fireEvent.click(screen.getByRole('button', { name: '保存定价' }));

    await waitFor(() => expect(screen.queryByText(MODAL_TITLE)).not.toBeInTheDocument());
    expect(screen.queryByText(DISCARD_TITLE)).not.toBeInTheDocument();
    expect(showToast.mock.calls.some(([, tone]) => tone === 'success')).toBe(true);
    // The POST carried the edited model.
    const post = adminFetch.mock.calls.find(([, options]) => options?.method === 'POST');
    expect(JSON.parse(post[1].body).model).toBe('gpt-new');
  });
});

// The supplier price list used to live on the removed Cost & Margin page. It now
// shares the pricing page as a third sub-tab, where our selling price and the
// supplier price are shown side by side and the spread (ours - supplier) is the
// only derived value: the two price lists stay separate, nothing is written back.
describe('PricingManager supplier prices sub-tab', () => {
  const supplierRows = [
    { id: 1, channel: 'ch-a', model: 'gpt-x', input_cost: 12.5, output_cost: 25, status: 'active' },
    { id: 2, channel: 'ch-b', model: 'gpt-unpriced', input_cost: 1, output_cost: 2, status: 'active' },
    { id: 3, channel: 'ch-c', model: 'gpt-x', input_cost: 25, output_cost: 40, status: 'active' },
  ];

  function supplierPricingResponse() {
    return {
      ok: true,
      status: 200,
      json: async () => ({ code: 0, data: supplierRows }),
    };
  }

  async function openSupplierTab() {
    const showToast = vi.fn();
    const adminFetch = vi.fn(async (url) => {
      if (String(url).includes('/admin/models')) return modelsResponse();
      if (String(url).includes('/admin/supplier-costs')) return supplierPricingResponse();
      // gpt-x carries a default tier and a VIP tier; the default one is our
      // reference price.
      return pricingResponse([
        { model: 'gpt-x', group_name: 'vip', prompt_price: 30, completion_price: 60, currency: 'CNY' },
        { model: 'gpt-x', group_name: 'default', prompt_price: 20, completion_price: 40, currency: 'CNY' },
      ]);
    });
    render(<PricingManager adminFetch={adminFetch} showToast={showToast} isAdmin lang="zh" />);
    await screen.findAllByText('gpt-x');
    fireEvent.click(screen.getByRole('button', { name: /供应商单价/ }));
    await screen.findByText('ch-a');
    return { adminFetch, showToast };
  }

  function supplierRowText(channel) {
    const row = screen.getByText(channel).closest('tr');
    if (!row) throw new Error(`no row for ${channel}`);
    return row.textContent;
  }

  it('shows our selling price, the supplier price and the spread side by side', async () => {
    await openSupplierTab();

    const row = supplierRowText('ch-a');
    // Our price: the default tier (20/40), not the VIP tier (30/60).
    expect(row).toContain('¥20.0000');
    expect(row).toContain('¥40.0000');
    // Supplier price as contracted.
    expect(row).toContain('¥12.5000');
    expect(row).toContain('¥25.0000');
    // Spread: 20 - 12.5 = 7.5 and 40 - 25 = 15.
    expect(row).toContain('¥7.5000');
    expect(row).toContain('¥15.0000');
  });

  it('marks a model without a selling price as unpriced instead of assuming zero', async () => {
    await openSupplierTab();

    expect(supplierRowText('ch-b')).toContain('未定价');
  });

  it('renders a negative spread in red', async () => {
    await openSupplierTab();

    // ch-c sells gpt-x at 25/40 against our 20/40: the prompt spread is -5.
    expect(supplierRowText('ch-c')).toContain('¥-5.0000');
    expect(screen.getByText('¥-5.0000').className).toContain('text-rose-600');
  });

  it('keeps the supplier price add form posting and reloading', async () => {
    const { adminFetch, showToast } = await openSupplierTab();

    fireEvent.change(screen.getByLabelText('通道名'), { target: { value: 'ch-new' } });
    fireEvent.change(screen.getByLabelText('模型'), { target: { value: 'gpt-z' } });
    fireEvent.change(screen.getByLabelText('供应商输入价'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('供应商输出价'), { target: { value: '6' } });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    await waitFor(() => expect(showToast).toHaveBeenCalledWith('供应商单价已保存', 'success'));
    const post = adminFetch.mock.calls.find(([, options]) => options?.method === 'POST');
    expect(JSON.parse(post[1].body)).toEqual({
      channel: 'ch-new',
      model: 'gpt-z',
      input_cost: 3,
      output_cost: 6,
    });
  });
});
