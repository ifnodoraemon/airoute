import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import AuthPage from './AuthPage';

// Same contract as LoginModal: the sign-in response hands the credential to the
// caller (memory / HttpOnly cookie) and never writes it to web storage, where an
// XSS could read a long-lived session token.

function jsonResponse(body, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

describe('AuthPage credential storage', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      if (String(url).includes('/api/v1/public/status')) {
        return jsonResponse({ code: 0, data: { allow_registration: true } });
      }
      if (String(url).includes('/api/v1/auth/login')) {
        return jsonResponse({ code: 0, data: { token: 'session-credential', user: { username: 'root', role: 'admin' } } });
      }
      return jsonResponse({ code: 0, data: {} });
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it('signs in without persisting the session token', async () => {
    const onLoginSuccess = vi.fn();
    const { container } = render(
      <AuthPage initialTab="login" onLoginSuccess={onLoginSuccess} onBackHome={() => {}} lang="en" t={{}} />,
    );

    fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'root' } });
    fireEvent.change(container.querySelector('input[type="password"]'), { target: { value: 'secret123' } });
    fireEvent.submit(container.querySelector('form'));

    await waitFor(() => expect(onLoginSuccess).toHaveBeenCalledWith('session-credential', expect.objectContaining({ username: 'root' })));
    expect(localStorage.getItem('airoute_token')).toBeNull();
    expect(localStorage.getItem('airoute_admin_token')).toBeNull();
  });

  it('surfaces the gateway message when the sign-in is rejected', async () => {
    globalThis.fetch.mockImplementation(async (url) => {
      if (String(url).includes('/api/v1/auth/login')) {
        return jsonResponse({ code: 401, error: '用户名或密码错误' }, 401);
      }
      if (String(url).includes('/api/v1/public/status')) {
        return jsonResponse({ code: 0, data: {} });
      }
      return jsonResponse({ code: 0, data: {} });
    });
    const onLoginSuccess = vi.fn();
    const { container } = render(
      <AuthPage initialTab="login" onLoginSuccess={onLoginSuccess} onBackHome={() => {}} lang="en" t={{}} />,
    );

    fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'root' } });
    fireEvent.change(container.querySelector('input[type="password"]'), { target: { value: 'wrong' } });
    fireEvent.submit(container.querySelector('form'));

    expect(await screen.findByText('用户名或密码错误')).toBeInTheDocument();
    expect(onLoginSuccess).not.toHaveBeenCalled();
  });
});
