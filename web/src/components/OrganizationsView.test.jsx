import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import OrganizationsView from './OrganizationsView';

const orgs = [
  { id: 1, parent_id: 0, level: 'unit', name: '默认单位', code: 'default', path: '/1/', depth: 0, status: 'active' },
  { id: 2, parent_id: 1, level: 'department', name: '算力运营', code: 'compute', path: '/1/2/', depth: 1, status: 'active' },
];

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

function setup() {
  const adminFetch = vi.fn(async (url, options = {}) => {
    if (url.endsWith('/members')) {
      if (options.method === 'POST') {
        return envelope([{ org_id: 2, user_id: 9, username: 'alice', role: 'admin' }]);
      }
      return envelope([]);
    }
    if (options.method === 'POST') {
      return envelope({ id: 3, parent_id: 0, level: 'unit', name: '新单位', code: 'new-unit', path: '/3/', depth: 0, status: 'active' });
    }
    return envelope(orgs);
  });
  const showToast = vi.fn();
  render(<OrganizationsView adminFetch={adminFetch} showToast={showToast} lang="zh" />);
  return { adminFetch, showToast };
}

describe('OrganizationsView', () => {
  it('renders the tree and shows the selected organization details', async () => {
    setup();

    expect(await screen.findByText('默认单位')).toBeTruthy();
    expect(screen.getByText('算力运营')).toBeTruthy();

    fireEvent.click(screen.getByText('算力运营'));

    await waitFor(() => {
      expect(screen.getByDisplayValue('算力运营')).toBeTruthy();
    });
    expect(screen.getByText('/1/2/')).toBeTruthy();
  });

  it('creates an organization through the admin API', async () => {
    const { adminFetch } = setup();
    await screen.findByText('默认单位');

    fireEvent.change(screen.getByLabelText('名称'), { target: { value: '新单位' } });
    fireEvent.change(screen.getByLabelText('编码'), { target: { value: 'new-unit' } });
    fireEvent.click(screen.getByRole('button', { name: /创建/ }));

    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/orgs', expect.objectContaining({ method: 'POST' }));
    });
    const call = adminFetch.mock.calls.find(([url, options]) => url === '/api/v1/admin/orgs' && options?.method === 'POST');
    const body = JSON.parse(call[1].body);
    expect(body).toMatchObject({ level: 'unit', name: '新单位', code: 'new-unit', parent_id: 0 });
  });

  it('adds a member to the selected organization', async () => {
    const { adminFetch } = setup();
    await screen.findByText('默认单位');
    fireEvent.click(screen.getByText('算力运营'));

    await waitFor(() => expect(screen.getByLabelText('用户 ID')).toBeTruthy());
    fireEvent.change(screen.getByLabelText('用户 ID'), { target: { value: '9' } });
    fireEvent.change(screen.getByLabelText('角色'), { target: { value: 'admin' } });
    fireEvent.click(screen.getByRole('button', { name: '加入' }));

    await waitFor(() => {
      expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/orgs/2/members', expect.objectContaining({ method: 'POST' }));
    });
    expect(await screen.findByText('alice')).toBeTruthy();
  });
});
