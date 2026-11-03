// Wallet refresh on tab return: one gesture, one request, newest answer wins.
//
// Regression for BUG-H02 (was: `focus` + `visibilitychange` each fired a wallet
// load and an old response could overwrite a newer balance). These assertions fail
// on the pre-fix code.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/WalletManagementView.refreshRace.test.jsx

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import WalletManagementView from './WalletManagementView';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function deferred() {
  let resolve;
  const promise = new Promise((res) => { resolve = res; });
  return { promise, resolve };
}

function renderView(adminFetch) {
  return render(
    <WalletManagementView
      adminUser={{ username: 'alice', role: 'user', balance: 0 }}
      adminFetch={adminFetch}
      showToast={() => {}}
      lang="zh"
    />,
  );
}

describe('WalletManagementView tab-return refresh', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('issues exactly one wallet request for a focus+visibilitychange pair', async () => {
    const walletCalls = [];
    const adminFetch = vi.fn((url) => {
      if (String(url).includes('/api/v1/user/wallet')) {
        const call = deferred();
        walletCalls.push(call);
        return call.promise;
      }
      return Promise.resolve(jsonResponse({ code: 0, data: [] }));
    });

    renderView(adminFetch);
    await waitFor(() => expect(walletCalls.length).toBe(1));
    walletCalls[0].resolve(jsonResponse({ code: 0, data: { balance: 100, orders: [] } }));
    await waitFor(() => expect(screen.getByText('¥ 100.00')).toBeInTheDocument());

    // One tab-return gesture: the browser fires both events.
    act(() => {
      window.dispatchEvent(new Event('focus'));
      document.dispatchEvent(new Event('visibilitychange'));
    });

    await waitFor(() => expect(walletCalls.length).toBe(2));
    // Give a second event the chance to wrongly schedule another load.
    await act(async () => {});
    expect(walletCalls.length).toBe(2);
  });

  it('shows the newest balance when responses would otherwise arrive out of order', async () => {
    const walletCalls = [];
    const adminFetch = vi.fn((url) => {
      if (String(url).includes('/api/v1/user/wallet')) {
        const call = deferred();
        walletCalls.push(call);
        return call.promise;
      }
      return Promise.resolve(jsonResponse({ code: 0, data: [] }));
    });

    renderView(adminFetch);
    await waitFor(() => expect(walletCalls.length).toBe(1));
    walletCalls[0].resolve(jsonResponse({ code: 0, data: { balance: 100, orders: [] } }));
    await waitFor(() => expect(screen.getByText('¥ 100.00')).toBeInTheDocument());

    // A later gesture starts a second load; the first one is superseded.
    act(() => { window.dispatchEvent(new Event('focus')); });
    await waitFor(() => expect(walletCalls.length).toBe(2));

    // The newer answer lands first...
    walletCalls[1].resolve(jsonResponse({ code: 0, data: { balance: 300, orders: [] } }));
    await waitFor(() => expect(screen.getByText('¥ 300.00')).toBeInTheDocument());

    // ...and the older in-flight answer resolves afterwards. It must be discarded.
    walletCalls[0].resolve(jsonResponse({ code: 0, data: { balance: 200, orders: [] } }));
    await act(async () => {});
    expect(screen.getByText('¥ 300.00')).toBeInTheDocument();
    expect(screen.queryByText('¥ 200.00')).toBeNull();
  });
});
