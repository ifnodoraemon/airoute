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
    data: url.endsWith('/wallet') ? { ...user, group_name: 'default', orders: [] } : {},
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
  });

  it('renders the balance it was given, to two decimals', () => {
    setup();

    // A bare "100" would be ambiguous; the wallet shows a currency-formatted value.
    expect(screen.getByText(/¥\s*100\.00/)).toBeInTheDocument();
  });

  describe('recharge amount', () => {
    it('sends the selected preset amount', async () => {
      const { adminFetch } = setup();

      fireEvent.click(screen.getByRole('button', { name: /立即支付|Pay Now/ }));

      await waitFor(() => expect(adminFetch).toHaveBeenCalled());
      const call = adminFetch.mock.calls.find(([url]) => String(url).includes('recharge'));
      expect(JSON.parse(call[1].body).amount).toBe(50);
    });

    it('prefers a typed custom amount over the preset', async () => {
      const { adminFetch } = setup();

      fireEvent.change(screen.getByPlaceholderText(/输入金额|Enter amount/), { target: { value: '123.45' } });
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
      fireEvent.click(screen.getByRole('button', { name: /立即支付|Pay Now/ }));

      await waitFor(() => expect(adminFetch).toHaveBeenCalled());
      const call = adminFetch.mock.calls.find(([url]) => String(url).includes('recharge'));
      expect(JSON.parse(call[1].body).amount).toBe(50);
    });

    it('refuses a zero or negative amount', async () => {
      const { adminFetch, showToast } = setup();

      fireEvent.change(screen.getByPlaceholderText(/输入金额|Enter amount/), { target: { value: '-10' } });
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
