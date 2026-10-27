import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import LoginModal from './LoginModal';

// The console moved to an HttpOnly cookie session: the browser may hold a
// "logged in" marker and profile hints, but never the credential itself. The
// login form used to write the raw session token to localStorage, where an XSS
// could read a long-lived credential.

function jsonResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

describe('LoginModal credential storage', () => {
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
    Element.prototype.scrollIntoView = vi.fn();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    delete Element.prototype.scrollIntoView;
    localStorage.clear();
  });

  it('hands the token to the caller without persisting it in web storage', async () => {
    const onLoginSuccess = vi.fn();
    const { container } = render(
      <LoginModal isOpen onClose={() => {}} onLoginSuccess={onLoginSuccess} lang="en" t={{}} />,
    );

    const username = container.querySelector('input[type="text"]');
    const password = container.querySelector('input[type="password"]');
    fireEvent.change(username, { target: { value: 'root' } });
    fireEvent.change(password, { target: { value: 'secret123' } });
    fireEvent.submit(username.closest('form'));

    await waitFor(() => expect(onLoginSuccess).toHaveBeenCalledWith('session-credential', expect.objectContaining({ username: 'root' })));
    // The credential stays in memory only; the HttpOnly cookie carries the session.
    expect(localStorage.getItem('airoute_token')).toBeNull();
    expect(localStorage.getItem('airoute_admin_token')).toBeNull();
  });

  it('reports a rejected login without touching storage', async () => {
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
      <LoginModal isOpen onClose={() => {}} onLoginSuccess={onLoginSuccess} lang="en" t={{}} />,
    );

    fireEvent.change(container.querySelector('input[type="text"]'), { target: { value: 'root' } });
    fireEvent.change(container.querySelector('input[type="password"]'), { target: { value: 'wrong' } });
    fireEvent.submit(container.querySelector('input[type="text"]').closest('form'));

    expect(await screen.findByText('用户名或密码错误')).toBeInTheDocument();
    expect(onLoginSuccess).not.toHaveBeenCalled();
    expect(localStorage.getItem('airoute_token')).toBeNull();
  });
});
