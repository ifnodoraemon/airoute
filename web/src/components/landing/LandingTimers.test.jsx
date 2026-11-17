import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import LandingPage from '../LandingPage';

vi.mock('../../config', () => ({ getGatewayOrigin: () => 'https://gateway.example', copyToClipboard: vi.fn().mockResolvedValue(true) }));

afterEach(() => {
  vi.useRealTimers();
  window.history.replaceState(null, '', '/');
});

describe('landing timers', () => {
  it('cancels deferred anchor scrolling on unmount', () => {
    vi.useFakeTimers();
    window.history.replaceState(null, '', '/#features');
    const { unmount } = render(<LandingPage />);
    expect(vi.getTimerCount()).toBe(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('restarts copy feedback on repeated clicks and clears it on unmount', async () => {
    vi.useFakeTimers();
    const { unmount } = render(<LandingPage lang="en" />);
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Copy Code' })));
    expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(2000));
    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Copied' })));
    act(() => vi.advanceTimersByTime(600));
    expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument();
    expect(vi.getTimerCount()).toBe(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
