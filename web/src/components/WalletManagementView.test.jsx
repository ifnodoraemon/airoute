import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import WalletManagementView from './WalletManagementView';

// The wallet moves money, so the paths worth pinning are the ones that decide how
// much is charged and what is sent to the payment endpoint. A form that accepts
// "abc" as an amount, or sends NaN, is worse than one that refuses it.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function setup({ user = { username: 'alice', role: 'user', balance: 100 }, ...props } = {}) {
  const adminFetch = vi.fn(async (url) => jsonResponse({
    code: 0,
    data: url.endsWith('/wallet') ? { ...user, group_name: 'default', stripe_enabled: true, orders: [] } : {},
  }));
  const handlers = {
    adminFetch,
    showToast: vi.fn(),
    onUserUpdated: vi.fn(),
    onBalanceUpdate: vi.fn(),
  };
  const utils = render(
    <WalletManagementView adminUser={user} lang="zh" {...handlers} {...props} />
  );
  return { ...utils, adminFetch, ...handlers };
}

describe('WalletManagementView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState(null, '', '/');
  });

  it('renders the balance it was given, to two decimals', () => {
    setup();

    // A bare "100" would be ambiguous; the wallet shows a currency-formatted value.
    expect(screen.getByText(/¥\s*100\.00/)).toBeInTheDocument();
  });

  it('shows a failed wallet fetch and lets the customer retry', async () => {
    const adminFetch = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ error: 'unavailable' }, 503))
      .mockResolvedValue(jsonResponse({ code: 0, data: { balance: 130, orders: [], stripe_enabled: true } }));
    setup({ adminFetch });
    expect(await screen.findByRole('alert')).toHaveTextContent('刷新失败');
    expect(screen.queryByText('暂无充值流水记录')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '重试' }));
    await screen.findByText(/¥\s*130\.00/);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('keeps a confirmed payment distinct from a failed balance refresh', async () => {
    window.history.replaceState(null, '', '/?recharge=success&order_no=REC-refresh');
    const adminFetch = vi.fn(async url => String(url).includes('/orders/')
      ? jsonResponse({ code: 0, data: { status: 'paid' } })
      : jsonResponse({ error: 'unavailable' }, 503));
    setup({ adminFetch });
    expect(await screen.findByRole('status')).toHaveTextContent('充值已到账，余额暂未刷新');
    expect(screen.queryByText('充值已到账，余额已更新。')).not.toBeInTheDocument();
  });

  it('retains redeemed gift cards and prevents deleting their evidence', async () => {
    const adminFetch = vi.fn(async url => jsonResponse({ code: 0, data: String(url).endsWith('/redemptions')
      ? [{ id: 1, code: 'CARD-used', status: 'used', amount: 20, used_by: 'alice' }]
      : { balance: 100, orders: [] } }));
    setup({ user: { username: 'operator', role: 'admin' }, adminFetch });
    fireEvent.click(screen.getByRole('button', { name: /兑换卡管理/ }));
    await screen.findByText('CARD-used');
    expect(screen.getByTitle('已兑换记录保留用于核账')).toBeDisabled();
  });

  it('does not describe a failed gift-card query as an empty list', async () => {
    const adminFetch = vi.fn(async url => String(url).endsWith('/redemptions')
      ? jsonResponse({ error: 'unavailable' }, 503)
      : jsonResponse({ code: 0, data: { balance: 100, orders: [] } }));
    setup({ user: { username: 'operator', role: 'admin' }, adminFetch });
    fireEvent.click(screen.getByRole('button', { name: /兑换卡管理/ }));
    expect(await screen.findByRole('alert')).toHaveTextContent('兑换卡列表加载失败');
    expect(screen.queryByText(/暂无兑换卡记录/)).not.toBeInTheDocument();
  });

  it('rejects fractional gift-card counts instead of silently rounding them', async () => {
    const adminFetch = vi.fn(async url => jsonResponse({ code: 0, data: String(url).endsWith('/redemptions') ? [] : { balance: 100, orders: [] } }));
    const showToast = vi.fn();
    setup({ user: { username: 'operator', role: 'admin' }, adminFetch, showToast });
    fireEvent.click(screen.getByRole('button', { name: /兑换卡管理/ }));
    fireEvent.click(screen.getByRole('button', { name: '批量生成兑换卡' }));
    fireEvent.change(screen.getByPlaceholderText('5'), { target: { value: '1.5' } });
    fireEvent.submit(screen.getByRole('button', { name: '确认生成' }).closest('form'));
    expect(showToast).toHaveBeenCalledWith('每批须生成 1 至 100 张兑换卡', 'warning');
    expect(adminFetch.mock.calls.some(([url]) => String(url).endsWith('/generate'))).toBe(false);
  });

  describe('recharge amount', () => {
    it('sends the selected preset amount', async () => {
      const { adminFetch } = setup();

      await waitFor(() => expect(screen.getByRole('button', { name: /立即支付|Pay Now/ })).toBeEnabled());
      fireEvent.click(screen.getByRole('button', { name: /立即支付|Pay Now/ }));

      await waitFor(() => expect(adminFetch).toHaveBeenCalled());
      const call = adminFetch.mock.calls.find(([url]) => String(url).includes('recharge'));
      expect(JSON.parse(call[1].body).amount).toBe(50);
    });

    it('prefers a typed custom amount over the preset', async () => {
      const { adminFetch } = setup();

      fireEvent.change(screen.getByPlaceholderText(/输入金额|Enter amount/), { target: { value: '123.45' } });
      await waitFor(() => expect(screen.getByRole('button', { name: /立即支付|Pay Now/ })).toBeEnabled());
      fireEvent.click(screen.getByRole('button', { name: /立即支付|Pay Now/ }));

      await waitFor(() => expect(adminFetch).toHaveBeenCalled());
      const call = adminFetch.mock.calls.find(([url]) => String(url).includes('recharge'));
      expect(JSON.parse(call[1].body).amount).toBe(123.45);
    });

    it('falls back to the preset when the custom field is empty', async () => {
      // The field is type="number", so a browser will not let "abc" into it at all;
      // what can actually reach the handler is an empty value, which must not be
      // treated as a zero charge.
      const { adminFetch } = setup();

      const field = screen.getByPlaceholderText(/输入金额|Enter amount/);
      fireEvent.change(field, { target: { value: '99' } });
      fireEvent.change(field, { target: { value: '' } });
      await waitFor(() => expect(screen.getByRole('button', { name: /立即支付|Pay Now/ })).toBeEnabled());
      fireEvent.click(screen.getByRole('button', { name: /立即支付|Pay Now/ }));

      await waitFor(() => expect(adminFetch).toHaveBeenCalled());
      const call = adminFetch.mock.calls.find(([url]) => String(url).includes('recharge'));
      expect(JSON.parse(call[1].body).amount).toBe(50);
    });

    it('refuses a zero or negative amount', async () => {
      const { adminFetch, showToast } = setup();

      fireEvent.change(screen.getByPlaceholderText(/输入金额|Enter amount/), { target: { value: '-10' } });
      await waitFor(() => expect(screen.getByRole('button', { name: /立即支付|Pay Now/ })).toBeEnabled());
      fireEvent.click(screen.getByRole('button', { name: /立即支付|Pay Now/ }));

      await waitFor(() => expect(showToast).toHaveBeenCalledWith(expect.any(String), 'warning'));
      expect(adminFetch.mock.calls.some(([url]) => String(url).includes('recharge'))).toBe(false);
    });
  });

  describe('redemption', () => {
    it('refuses an empty code without calling the gateway', async () => {
      const { adminFetch, showToast } = setup();

      fireEvent.submit(screen.getByRole('button', { name: /兑换|Redeem/ }).closest('form'));

      await waitFor(() => expect(showToast).toHaveBeenCalledWith(expect.any(String), 'warning'));
      expect(adminFetch.mock.calls.some(([url]) => String(url).includes('redeem'))).toBe(false);
    });

    it('refuses a whitespace-only code', async () => {
      const { adminFetch, showToast } = setup();

      fireEvent.change(screen.getByPlaceholderText(/CARD-/), { target: { value: '   ' } });
      fireEvent.submit(screen.getByRole('button', { name: /兑换|Redeem/ }).closest('form'));

      await waitFor(() => expect(showToast).toHaveBeenCalledWith(expect.any(String), 'warning'));
      expect(adminFetch.mock.calls.some(([url]) => String(url).includes('redeem'))).toBe(false);
    });

    it('trims the code before sending it', async () => {
      const { adminFetch } = setup();

      fireEvent.change(screen.getByPlaceholderText(/CARD-/), { target: { value: '  CARD-1234-5678  ' } });
      fireEvent.submit(screen.getByRole('button', { name: /兑换|Redeem/ }).closest('form'));

      await waitFor(() => expect(adminFetch).toHaveBeenCalled());
      const call = adminFetch.mock.calls.find(([url]) => String(url).includes('redeem'));
      expect(JSON.parse(call[1].body).code).toBe('CARD-1234-5678');
    });
  });

  describe('payment method', () => {
    it('does not offer simulated sandbox channel to an administrator', () => {
      setup({ user: { username: 'root', role: 'admin', balance: 0 } });

      expect(screen.queryByText(/沙箱|Sandbox/)).not.toBeInTheDocument();
      expect(screen.getByText(/Stripe/)).toBeInTheDocument();
    });
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en' });

    // The redemption field is the only placeholder that differs by language and is
    // always visible on the wallet tab.
    expect(screen.getByPlaceholderText(/Enter code CARD-/)).toBeInTheDocument();
  });
});
