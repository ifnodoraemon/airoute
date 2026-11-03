import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import AuthPage from './AuthPage';
import LoginModal from './LoginModal';
import UserManagementView from './UserManagementView';

const key = 'sk-airoute-once-0123456789abcdef';
const user = { username: 'new-account', role: 'user' };
const response = data => ({ ok: true, status: 200, json: async () => ({ code: 0, data }) });

afterEach(() => { vi.unstubAllGlobals(); localStorage.clear(); });

describe('initial credential delivery', () => {
  it.each(['page', 'modal'])('keeps the registration key visible before entering the console from %s', async kind => {
    const onLoginSuccess = vi.fn();
    vi.stubGlobal('fetch', vi.fn(async url => response(String(url).endsWith('/register')
      ? { token: 'session-token', user, api_key: key }
      : { allow_registration: true })));
    const { container } = render(kind === 'page'
      ? <AuthPage initialTab="register" onLoginSuccess={onLoginSuccess} />
      : <LoginModal isOpen onClose={vi.fn()} onLoginSuccess={onLoginSuccess} />);
    if (kind === 'modal') fireEvent.click(screen.getByRole('button', { name: /注册/ }));
    const username = kind === 'page' ? '至少 3 个字符 (字母、数字)' : '字母/数字，至少 3 字符';
    fireEvent.change(screen.getByPlaceholderText(username), { target: { value: user.username } });
    fireEvent.change(screen.getByPlaceholderText(kind === 'page' ? 'name@example.com' : 'your-name@company.com'), { target: { value: 'new@example.com' } });
    fireEvent.change(screen.getByPlaceholderText(kind === 'page' ? '至少 6 个字符' : '至少 6 位字符'), { target: { value: 'new-password' } });
    fireEvent.submit(container.querySelector('form'));
    expect(await screen.findByTestId('initial-api-key')).toHaveTextContent(key);
    expect(onLoginSuccess).not.toHaveBeenCalled();
    expect(JSON.stringify(localStorage)).not.toContain(key);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.getByTestId('initial-api-key')).toHaveTextContent(key);
    fireEvent.click(screen.getByRole('button', { name: '我已保存，继续' }));
    expect(onLoginSuccess).toHaveBeenCalledWith('session-token', user);
    expect(screen.queryByTestId('initial-api-key')).not.toBeInTheDocument();
  });

  it('delivers the administrator-created account key after the form closes', async () => {
    const adminFetch = vi.fn(async (url, opts) => response(opts?.method === 'POST'
      ? { ...user, api_key: key }
      : String(url).endsWith('/users') ? [] : []));
    render(<UserManagementView adminUser={{ username: 'admin', role: 'admin' }} adminFetch={adminFetch} />);
    fireEvent.click(await screen.findByRole('button', { name: /新建用户|新建账号/ }));
    fireEvent.change(screen.getByPlaceholderText('至少 3 个字符'), { target: { value: user.username } });
    fireEvent.change(screen.getByPlaceholderText('至少 6 个字符'), { target: { value: 'new-password' } });
    fireEvent.submit(screen.getByPlaceholderText('至少 3 个字符').closest('form'));
    expect(await screen.findByTestId('initial-api-key')).toHaveTextContent(key);
    await waitFor(() => expect(screen.queryByPlaceholderText('至少 3 个字符')).not.toBeInTheDocument());
    expect(JSON.stringify(localStorage)).not.toContain(key);
  });
});
