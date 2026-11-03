import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import UserManagementView from './UserManagementView';

// User administration creates accounts and decides who is an administrator, so the
// validation and the guards here are security-relevant rather than cosmetic: a
// blank password or a lockable system account is a real problem, not a rough edge.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function user(over = {}) {
  return {
    id: 1,
    username: 'alice',
    email: 'alice@example.com',
    role: 'user',
    status: 'active',
    balance: 10,
    group_name: 'default',
    ...over,
  };
}

function setup({ users = [user()], ...props } = {}) {
  const adminFetch = vi.fn(async (url) => {
    if (String(url).includes('/admin/users') && !String(url).includes('/users/')) {
      return jsonResponse({ code: 0, data: users });
    }
    return jsonResponse({ code: 0, data: {} });
  });
  const handlers = { adminFetch, showToast: vi.fn() };
  const utils = render(
    <UserManagementView adminUser={{ username: 'root', role: 'admin' }} lang="zh" {...handlers} {...props} />
  );
  return { ...utils, adminFetch, ...handlers };
}

async function openCreateForm() {
  const button = await screen.findByRole('button', { name: /新建用户|Add User|新建账号/ });
  fireEvent.click(button);
  return screen.findByPlaceholderText(/用户名|Username/);
}

function fillCreateForm({ username, password, email = '', balance = '' }) {
  if (username !== undefined) {
    fireEvent.change(screen.getByPlaceholderText(/至少 3 个字符|At least 3 characters/), { target: { value: username } });
  }
  if (password !== undefined) {
    fireEvent.change(screen.getByPlaceholderText(/至少 6 个字符|At least 6 characters/), { target: { value: password } });
  }
  if (email) fireEvent.change(screen.getByPlaceholderText('user@example.com'), { target: { value: email } });
  if (balance) fireEvent.change(screen.getByPlaceholderText('10.0'), { target: { value: balance } });
}

function submitCreateForm() {
  const form = screen.getByRole('button', { name: /确认创建|Create|创建账号/ }).closest('form');
  fireEvent.submit(form);
}

function createCalls(adminFetch) {
  return adminFetch.mock.calls.filter(([url, opts]) => String(url) === '/api/v1/admin/users' && opts?.method === 'POST');
}

describe('UserManagementView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('lists the users it loaded', async () => {
    setup({ users: [user({ username: 'alice' }), user({ id: 2, username: 'bob' })] });

    expect(await screen.findByText('alice')).toBeInTheDocument();
    expect(screen.getByText('bob')).toBeInTheDocument();
  });

  describe('creating a user', () => {
    it('refuses an empty username', async () => {
      const { adminFetch, showToast } = setup();
      await openCreateForm();

      fillCreateForm({ username: '   ', password: 'secret123' });
      submitCreateForm();

      await waitFor(() => expect(screen.getByText(/请输入登录用户名/)).toBeInTheDocument());
      expect(createCalls(adminFetch)).toHaveLength(0);
      expect(showToast).not.toHaveBeenCalledWith(expect.any(String), 'success');
    });

    it('refuses a username shorter than three characters', async () => {
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: 'ab', password: 'secret123' });
      submitCreateForm();

      await waitFor(() => expect(screen.getByText(/用户名长度至少需要 3 个字符/)).toBeInTheDocument());
      expect(createCalls(adminFetch)).toHaveLength(0);
    });

    it('refuses an empty password', async () => {
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: 'alice2', password: '   ' });
      submitCreateForm();

      await waitFor(() => expect(screen.getByText(/请输入初始登录密码/)).toBeInTheDocument());
      expect(createCalls(adminFetch)).toHaveLength(0);
    });

    it('refuses a password shorter than six characters', async () => {
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: 'alice2', password: '12345' });
      submitCreateForm();

      await waitFor(() => expect(screen.getByText(/密码长度至少需要 6 个字符/)).toBeInTheDocument());
      expect(createCalls(adminFetch)).toHaveLength(0);
    });

    it('trims the username but not the password', async () => {
      // Leading and trailing spaces are part of a password, and stripping them
      // would silently change the credential the operator set.
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: '  alice2  ', password: '  secret123  ' });
      submitCreateForm();

      await waitFor(() => expect(createCalls(adminFetch)).toHaveLength(1));
      const body = JSON.parse(createCalls(adminFetch)[0][1].body);
      expect(body.username).toBe('alice2');
      expect(body.password).toBe('  secret123  ');
    });

    it('sends the role and status that were chosen', async () => {
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: 'alice2', password: 'secret123' });
      // Both the create form and each row offer an admin option, so scope to the
      // select that sits inside the create form.
      const form = screen.getByRole('button', { name: /确认创建|Create|创建账号/ }).closest('form');
      const roleSelect = within(form).getByRole('option', { name: /超级管理员 \(admin\)|Administrator \(admin\)/ }).closest('select');
      fireEvent.change(roleSelect, { target: { value: 'admin' } });
      submitCreateForm();

      await waitFor(() => expect(createCalls(adminFetch)).toHaveLength(1));
      expect(JSON.parse(createCalls(adminFetch)[0][1].body).role).toBe('admin');
    });

    it('defaults an unparseable balance to zero instead of sending NaN', async () => {
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: 'alice2', password: 'secret123' });
      submitCreateForm();

      await waitFor(() => expect(createCalls(adminFetch)).toHaveLength(1));
      const body = JSON.parse(createCalls(adminFetch)[0][1].body);
      expect(Number.isFinite(body.balance)).toBe(true);
    });

    it('defaults a blank group to the default group', async () => {
      const { adminFetch } = setup();
      await openCreateForm();

      fillCreateForm({ username: 'alice2', password: 'secret123' });
      submitCreateForm();

      await waitFor(() => expect(createCalls(adminFetch)).toHaveLength(1));
      expect(JSON.parse(createCalls(adminFetch)[0][1].body).group_name).toBe('default');
    });

    it('surfaces a gateway error without closing the form', async () => {
      const { adminFetch, showToast } = setup();
      adminFetch.mockImplementation(async (url, opts) => {
        if (String(url) === '/api/v1/admin/users' && opts?.method === 'POST') {
          return jsonResponse({ code: 1, error: 'username already taken' }, 400);
        }
        return jsonResponse({ code: 0, data: [user()] });
      });
      await openCreateForm();

      fillCreateForm({ username: 'alice', password: 'secret123' });
      submitCreateForm();

      await waitFor(() => expect(screen.getByText('username already taken')).toBeInTheDocument());
      expect(showToast).toHaveBeenCalledWith('username already taken', 'error');
    });
  });

  describe('the system administrator account', () => {
    it('cannot be locked', async () => {
      // Locking "admin" would leave nobody able to administer the gateway. The
      // control is rendered disabled for that account.
      const { adminFetch } = setup({ users: [user({ username: 'admin', role: 'admin' })] });
      await screen.findByText('admin');

      const lockButton = screen.getByTitle('点击锁定账号');
      expect(lockButton).toBeDisabled();

      fireEvent.click(lockButton);

      // Nothing was sent.
      expect(adminFetch.mock.calls.some(([, opts]) => opts?.method === 'PUT')).toBe(false);

      // NOT COVERED: handleToggleStatus also refuses the system account in its own
      // body. That guard is unreachable from here — the control is disabled, and a
      // disabled button does not dispatch a click — so removing the handler guard
      // does not fail this suite. It is a second line of defence behind the disabled
      // state, not the enforced one.
    });

    it('can be locked when the account is an ordinary administrator', async () => {
      // The control: a non-system admin is lockable, so the guard is specific to
      // the system account rather than disabling the feature.
      setup({ users: [user({ username: 'ops', role: 'admin' })] });
      await screen.findByText('ops');

      expect(screen.getByTitle('点击锁定账号')).not.toBeDisabled();
    });
  });

  it('renders in English when asked', async () => {
    setup({ lang: 'en' });

    await screen.findByText('alice');
    expect(screen.getByRole('button', { name: /New User/ })).toBeInTheDocument();
  });
});

// The gateway treats a reset of your own password as a credential rotation: it
// rejects the request unless old_password matches. The dialog used to send only
// new_password, so self-reset was impossible from the console (always a 400).
describe('UserManagementView self password reset', () => {
  function setupWithRoot() {
    return setup({ users: [user({ username: 'root', role: 'admin' })] });
  }

  async function openResetDialog() {
    await screen.findByText('root');
    fireEvent.click(screen.getAllByTitle('重置登录密码')[0]);
    return screen.findByText('重置用户密码');
  }

  function submitReset() {
    const form = screen.getByRole('button', { name: /确认重置/ }).closest('form');
    fireEvent.submit(form);
  }

  function passwordCalls(adminFetch) {
    return adminFetch.mock.calls.filter(
      ([url, opts]) => String(url).endsWith('/password') && opts?.method === 'POST'
    );
  }

  it('asks for the current password when resetting your own account', async () => {
    setupWithRoot();
    await openResetDialog();

    expect(screen.getByText(/这是您自己的账号/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText('请输入当前登录密码')).toBeInTheDocument();
  });

  it('does not require the current password when resetting another user', async () => {
    setup({ users: [user({ username: 'alice' })] });
    await screen.findByText('alice');
    fireEvent.click(screen.getAllByTitle('重置登录密码')[0]);
    await screen.findByText('重置用户密码');

    expect(screen.queryByPlaceholderText('请输入当前登录密码')).not.toBeInTheDocument();
    expect(screen.queryByText(/这是您自己的账号/)).not.toBeInTheDocument();
  });

  it('refuses to reset your own password without the current one', async () => {
    const { adminFetch } = setupWithRoot();
    await openResetDialog();

    submitReset();

    await waitFor(() => expect(screen.getByText(/重置本人密码需要输入当前登录密码/)).toBeInTheDocument());
    expect(passwordCalls(adminFetch)).toHaveLength(0);
  });

  it('sends old_password along with the new one for a self reset', async () => {
    const { adminFetch } = setupWithRoot();
    await openResetDialog();

    const newPassword = screen.getByPlaceholderText(/至少 6 个字符/).value;
    fireEvent.change(screen.getByPlaceholderText('请输入当前登录密码'), { target: { value: 'current-pass-1' } });
    submitReset();

    await waitFor(() => expect(passwordCalls(adminFetch)).toHaveLength(1));
    const [url, opts] = passwordCalls(adminFetch)[0];
    expect(String(url)).toBe('/api/v1/admin/users/root/password');
    const body = JSON.parse(opts.body);
    expect(body.old_password).toBe('current-pass-1');
    expect(body.new_password).toBe(newPassword);
    // A successful reset closes the dialog.
    await waitFor(() => expect(screen.queryByText('重置用户密码')).not.toBeInTheDocument());
  });

  it('sends no old_password when resetting another user', async () => {
    const { adminFetch } = setup({ users: [user({ username: 'alice' })] });
    await screen.findByText('alice');
    fireEvent.click(screen.getAllByTitle('重置登录密码')[0]);
    await screen.findByText('重置用户密码');

    submitReset();

    await waitFor(() => expect(passwordCalls(adminFetch)).toHaveLength(1));
    const body = JSON.parse(passwordCalls(adminFetch)[0][1].body);
    expect(body).not.toHaveProperty('old_password');
    expect(body.new_password).toEqual(expect.any(String));
  });

  it('clears the old password when the dialog is closed and reopened', async () => {
    setupWithRoot();
    await openResetDialog();

    fireEvent.change(screen.getByPlaceholderText('请输入当前登录密码'), { target: { value: 'current-pass-1' } });
    // Dismissing a dirty dialog asks first.
    fireEvent.click(screen.getByRole('button', { name: '取消' }));
    fireEvent.click(await screen.findByRole('button', { name: '放弃并关闭' }));
    await waitFor(() => expect(screen.queryByText('重置用户密码')).not.toBeInTheDocument());

    fireEvent.click(screen.getAllByTitle('重置登录密码')[0]);
    await screen.findByText('重置用户密码');

    expect(screen.getByPlaceholderText('请输入当前登录密码').value).toBe('');
  });
});
