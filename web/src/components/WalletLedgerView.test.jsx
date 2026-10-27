import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import WalletLedgerView from './WalletLedgerView';

const rows = [
  {
    id: 1, user_id: 7, username: 'alice', type: 'recharge',
    delta: 50, delta_units: 50000000000, balance_after: 150, balance_after_units: 150000000000,
    reason: 'top up', ref_id: 'order:A1', operator: 'stripe', created_at: '2026-10-27T12:00:00Z',
  },
  {
    id: 2, user_id: 7, username: 'alice', type: 'charge',
    delta: -3, delta_units: -3000000000, balance_after: 147, balance_after_units: 147000000000,
    reason: 'usage charge', ref_id: '', operator: '', created_at: '2026-10-27T12:05:00Z',
  },
];

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

function setup() {
  const adminFetch = vi.fn(async () => envelope(rows));
  render(<WalletLedgerView adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
  return { adminFetch };
}

describe('WalletLedgerView', () => {
  it('renders ledger rows with type and signed amounts', async () => {
    setup();
    expect(await screen.findAllByText('alice')).toHaveLength(2);
    expect(screen.getByText('充值')).toBeTruthy();
    expect(screen.getByText('扣费')).toBeTruthy();
    expect(screen.getByText('+¥50.0000')).toBeTruthy();
    expect(screen.getByText('-¥3.0000')).toBeTruthy();
    expect(screen.getByText('¥150.0000')).toBeTruthy();
    expect(screen.getByText('order:A1')).toBeTruthy();
  });

  it('filters by user id', async () => {
    const { adminFetch } = setup();
    await screen.findAllByText('alice');

    fireEvent.change(screen.getByLabelText('用户 ID'), { target: { value: '7' } });
    fireEvent.click(screen.getByRole('button', { name: '查询' }));

    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith(expect.stringContaining('user_id=7'));
    });
  });
});
