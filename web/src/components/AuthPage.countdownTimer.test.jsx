// Verification-code countdown: the interval belongs to the component lifetime.
//
// Regression for BUG-H04 (was: the countdown started a setInterval inside an async
// handler and never cleared it on unmount). These assertions fail on the pre-fix
// code (the timer count stays > 0 after unmount).
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/AuthPage.countdownTimer.test.jsx

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import AuthPage from './AuthPage';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

describe('AuthPage verification-code countdown', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('keeps ticking while mounted and releases its timer on unmount', async () => {
    vi.useFakeTimers();
    const fetchMock = vi.fn(async (url) => {
      if (String(url).includes('/api/v1/public/status')) {
        return jsonResponse({ code: 0, data: { allow_registration: true, require_email_verification: true } });
      }
      if (String(url).includes('/api/v1/auth/send-verification-code')) {
        return jsonResponse({ code: 0, message: 'sent' });
      }
      return jsonResponse({ code: 0, data: {} });
    });
    vi.stubGlobal('fetch', fetchMock);

    const { unmount } = render(<AuthPage initialTab="register" onLoginSuccess={() => {}} lang="zh" toggleLang={() => {}} />);

    fireEvent.change(screen.getByPlaceholderText('name@example.com'), { target: { value: 'alice@example.com' } });
    // The code field only exists once /public/status says verification is required.
    await act(async () => {});
    fireEvent.click(screen.getByRole('button', { name: '获取验证码' }));
    await act(async () => {});

    expect(screen.getByRole('button', { name: /后重发/ })).toBeInTheDocument();
    expect(vi.getTimerCount()).toBe(1);

    // The countdown advances while the page is open.
    act(() => { vi.advanceTimersByTime(1000); });
    expect(screen.getByRole('button', { name: /59s 后重发/ })).toBeInTheDocument();

    // Leaving the page releases the interval instead of leaving it running.
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
