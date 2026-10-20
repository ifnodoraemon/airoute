import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useKeyActions } from './useKeyActions';

// Covers the key domain extracted from App.jsx. The property worth pinning hardest
// is that the browser never chooses a credential: the form sends a name and a
// quota, and the gateway mints the secret.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function setup(overrides = {}) {
  const showToast = vi.fn();
  const refresh = vi.fn();
  const props = {
    keys: [{ id: 1 }, { id: 2 }],
    adminUser: { username: 'root', role: 'admin' },
    showToast,
    isZh: true,
    refresh,
    ...overrides,
  };
  const utils = renderHook(() => useKeyActions(props));
  return { ...utils, showToast, refresh };
}

describe('useKeyActions', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ code: 0, data: {} })));
    vi.stubGlobal('confirm', vi.fn(() => true));
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('opens the create form with a suggested name and no credential', () => {
    const { result } = setup();

    act(() => {
      result.current.handleOpenCreateKey();
    });

    expect(result.current.showKeyModal).toBe(true);
    // The browser must never invent the secret.
    expect(result.current.newKey.key).toBe('');
    expect(result.current.newKey.tenant_id).toBe('密钥-3');
  });

  it('defaults an administrator to the default group', () => {
    const { result } = setup();

    act(() => {
      result.current.handleOpenCreateKey();
    });

    expect(result.current.newKey.group_name).toBe('default');
  });

  it("defaults a regular user to their own group", () => {
    const { result } = setup({ adminUser: { username: 'alice', role: 'user', group_name: 'vip' } });

    act(() => {
      result.current.handleOpenCreateKey();
    });

    expect(result.current.newKey.group_name).toBe('vip');
  });

  it('refuses to create without a name', async () => {
    const { result, showToast, refresh } = setup();
    act(() => {
      result.current.setNewKey(prev => ({ ...prev, tenant_id: '   ' }));
    });

    await act(async () => {
      await result.current.handleCreateKey();
    });

    expect(showToast).toHaveBeenCalledWith(expect.any(String), 'warning');
    expect(refresh).not.toHaveBeenCalled();
  });

  it('creates a key, closes the form and refreshes the console', async () => {
    const { result, refresh } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
    });

    await act(async () => {
      await result.current.handleCreateKey();
    });

    const [url, opts] = globalThis.fetch.mock.calls.at(-1);
    expect(String(url)).toBe('/api/v1/admin/keys');
    expect(opts.method).toBe('POST');
    const sent = JSON.parse(opts.body);
    // No credential is sent: the gateway mints it.
    expect(sent.key).toBe('');
    expect(sent.tenant_id).toBe('密钥-3');

    expect(result.current.showKeyModal).toBe(false);
    expect(refresh).toHaveBeenCalled();
  });

  it('keeps the form open when creation fails', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 1, error: 'nope' }));
    const { result, refresh } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
    });

    await act(async () => {
      await result.current.handleCreateKey();
    });

    expect(result.current.showKeyModal).toBe(true);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('reports a network failure without throwing', async () => {
    globalThis.fetch.mockRejectedValue(new Error('offline'));
    const { result, showToast } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
    });

    await act(async () => {
      await result.current.handleCreateKey();
    });

    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('offline'), 'error');
    expect(result.current.submittingKey).toBe(false);
  });

  it('toggles a key from active to disabled', async () => {
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleToggleKeyStatus({ id: 1, key: 'sk-a', status: 'active' });
    });

    const [url, opts] = globalThis.fetch.mock.calls.at(-1);
    expect(String(url)).toBe('/api/v1/admin/keys/1');
    expect(JSON.parse(opts.body).status).toBe('disabled');
    expect(refresh).toHaveBeenCalled();
  });

  it('toggles a disabled key back to active', async () => {
    const { result } = setup();

    await act(async () => {
      await result.current.handleToggleKeyStatus({ id: 1, key: 'sk-a', status: 'disabled' });
    });

    expect(JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body).status).toBe('active');
  });

  it('updates the billing group', async () => {
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleUpdateKeyGroup(5, 'vip');
    });

    expect(String(globalThis.fetch.mock.calls.at(-1)[0])).toBe('/api/v1/admin/keys/5');
    expect(JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body).group_name).toBe('vip');
    expect(refresh).toHaveBeenCalled();
  });

  it('asks before revoking, and does nothing when declined', async () => {
    globalThis.confirm = vi.fn(() => false);
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleDeleteKey(1);
    });

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
  });

  it('revokes and refreshes when confirmed', async () => {
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleDeleteKey(1);
    });

    expect(globalThis.fetch.mock.calls.at(-1)[1].method).toBe('DELETE');
    expect(refresh).toHaveBeenCalled();
  });

  it('batch-revokes the selection and clears it', async () => {
    const { result, refresh } = setup();
    act(() => {
      result.current.setSelectedKeyIds([1, 2]);
    });

    await act(async () => {
      await result.current.handleBatchDeleteKeys();
    });

    expect(JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body).ids).toEqual([1, 2]);
    expect(result.current.selectedKeyIds).toEqual([]);
    expect(refresh).toHaveBeenCalled();
  });

  it('does nothing when the batch selection is empty', async () => {
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleBatchDeleteKeys();
    });

    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
  });

  it('batch-updates the status of the selection', async () => {
    const { result } = setup();
    act(() => {
      result.current.setSelectedKeyIds([1, 2]);
    });

    await act(async () => {
      await result.current.handleBatchStatusKeys('disabled');
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent.ids).toEqual([1, 2]);
    expect(sent.status).toBe('disabled');
    expect(result.current.selectedKeyIds).toEqual([]);
  });

  it('records the copied text and clears it again', async () => {
    const { result, showToast } = setup();

    await act(async () => {
      await result.current.copyToClipboard('sk-copied');
    });

    expect(result.current.copiedKey).toBe('sk-copied');
    expect(showToast).toHaveBeenCalledWith(expect.any(String), 'success');

    // The indicator is transient.
    await act(async () => {
      vi.advanceTimersByTime(3000);
    });
    expect(result.current.copiedKey).toBe('');
  });

  it('clears the tenant-scoped key state on reset', () => {
    const { result } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
      result.current.setSelectedKeyIds([1]);
    });

    act(() => {
      result.current.reset();
    });

    expect(result.current.showKeyModal).toBe(false);
    expect(result.current.selectedKeyIds).toEqual([]);
    expect(result.current.copiedKey).toBe('');
  });
});
