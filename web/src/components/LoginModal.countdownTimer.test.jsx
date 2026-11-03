// Login dialog verification-code countdown: the interval belongs to the component.
//
// Regression for BUG-H04 in the second place it occurred (LoginModal mirrored the
// AuthPage countdown). Fails on the pre-fix code: the timer survives unmount.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/LoginModal.countdownTimer.test.jsx

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import LoginModal from './LoginModal';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

describe('LoginModal verification-code countdown', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('releases the countdown timer when the dialog closes', async () => {
    vi.useFakeTimers();
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      if (String(url).includes('/api/v1/public/status')) {
        return jsonResponse({ code: 0, data: { allow_registration: true, require_email_verification: true } });
      }
      if (String(url).includes('/api/v1/auth/send-verification-code')) {
        return jsonResponse({ code: 0, message: 'sent' });
      }
      return jsonResponse({ code: 0, data: {} });
    }));

    const { rerender } = render(
      <LoginModal isOpen onClose={() => {}} onLoginSuccess={() => {}} lang="zh" />,
    );

    // Switch the dialog to the register form, which owns the email verification.
    fireEvent.click(screen.getByRole('button', { name: '注册账号' }));
    await act(async () => {});

    fireEvent.change(screen.getByPlaceholderText('your-name@company.com'), { target: { value: 'alice@example.com' } });
    await act(async () => {});
    fireEvent.click(screen.getByRole('button', { name: '获取验证码' }));
    await act(async () => {});

    expect(vi.getTimerCount()).toBe(1);

    // Closing the dialog must remove the interval (it used to keep ticking for 60s).
    rerender(<LoginModal isOpen={false} onClose={() => {}} onLoginSuccess={() => {}} lang="zh" />);
    expect(vi.getTimerCount()).toBe(0);
  });
});
