import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import BudgetsView from './BudgetsView';

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

const budgets = [
  { id: 1, scope_type: 'org', scope_id: 2, period: 'monthly', amount: 1000, used: 250, percent: 25, action: 'disable', warn_percent: 80, status: 'active' },
];
const orgs = [{ id: 2, name: '预算单位', code: 'bu', level: 'unit' }];

function setup() {
  const adminFetch = vi.fn(async (url, options = {}) => {
    if (url === '/api/v1/admin/budgets' && options.method === 'POST') {
      return envelope({ id: 2, scope_type: 'org', scope_id: 3, period: 'monthly', amount: 500, used: 0, percent: 0, action: 'notify', warn_percent: 80, status: 'active' });
    }
    if (url.startsWith('/api/v1/admin/budgets/') && options.method === 'PUT') {
      return envelope({ ...budgets[0], amount: 2000 });
    }
    if (url.startsWith('/api/v1/admin/budgets/') && options.method === 'DELETE') {
      return envelope({ id: 1 });
    }
    if (url === '/api/v1/admin/budgets') return envelope(budgets);
    if (url === '/api/v1/admin/orgs') return envelope(orgs);
    if (url === '/api/v1/admin/users') return envelope([{ id: 7, username: 'alice' }]);
    if (url === '/api/v1/admin/keys') return envelope([{ id: 9, key: 'sk-***mask', key_fingerprint: 'kfp_9' }]);
    throw new Error(`unexpected ${url}`);
  });
  const showToast = vi.fn();
  render(<BudgetsView adminFetch={adminFetch} showToast={showToast} lang="zh" />);
  return { adminFetch, showToast };
}

describe('BudgetsView', () => {
  it('renders existing budgets with scope names and waterlines', async () => {
    setup();
    expect(await screen.findByText('预算单位')).toBeTruthy();
    expect(screen.getByText('25.0%')).toBeTruthy();
    expect(screen.getByText('¥250.0000 / ¥1000.0000')).toBeTruthy();
  });

  it('creates a budget for the selected scope', async () => {
    const { adminFetch } = setup();
    await screen.findByText('预算单位');

    fireEvent.change(screen.getByLabelText('范围'), { target: { value: '2' } });
    fireEvent.change(screen.getByLabelText('金额'), { target: { value: '500' } });
    fireEvent.click(screen.getByRole('button', { name: /创建/ }));

    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/budgets', expect.objectContaining({ method: 'POST' }));
    });
    const call = adminFetch.mock.calls.find(([url, options]) => url === '/api/v1/admin/budgets' && options?.method === 'POST');
    expect(JSON.parse(call[1].body)).toMatchObject({ scope_type: 'org', scope_id: 2, period: 'monthly', amount: 500, action: 'notify' });
  });

  it('edits and deletes an existing budget', async () => {
    const { adminFetch } = setup();
    await screen.findByText('预算单位');

    fireEvent.click(screen.getByTitle('编辑'));
    fireEvent.change(screen.getByLabelText('金额'), { target: { value: '2000' } });
    fireEvent.click(screen.getByRole('button', { name: /保存/ }));
    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/budgets/1', expect.objectContaining({ method: 'PUT' }));
    });

    fireEvent.click(screen.getByTitle('删除'));
    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/budgets/1', expect.objectContaining({ method: 'DELETE' }));
    });
  });
});
