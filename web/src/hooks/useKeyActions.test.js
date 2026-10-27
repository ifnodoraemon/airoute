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
    // The clipboard helper falls back to document.execCommand in jsdom; stub it
    // per-test and drop the stub afterwards.
    delete document.execCommand;
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

  it('creates a key and keeps the dialog open to show the credential once', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({
      code: 0,
      data: { key: 'sk-airoute-created', key_id: 'ak_abc123abc123abc12345', tenant_id: 'root', rpm: 60, tpm: 100000 },
    }));
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

    // The dialog stays open because the plaintext exists only in this response.
    expect(result.current.showKeyModal).toBe(true);
    expect(result.current.createdKey).toEqual(expect.objectContaining({
      key: 'sk-airoute-created',
      keyId: 'ak_abc123abc123abc12345',
    }));
    expect(refresh).toHaveBeenCalled();
  });

  it('closing the dialog drops the one-time credential', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({
      code: 0,
      data: { key: 'sk-airoute-created', key_id: 'ak_abc123abc123abc12345' },
    }));
    const { result } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
    });
    await act(async () => {
      await result.current.handleCreateKey();
    });

    act(() => {
      result.current.closeKeyModal();
    });

    expect(result.current.showKeyModal).toBe(false);
    expect(result.current.createdKey).toBeNull();
  });

  it('does not send a tenant for a self-service key', async () => {
    // The gateway derives the tenant from the session; sending one is ignored at
    // best, so the console must not pretend it can choose.
    const { result } = setup({ adminUser: { username: 'alice', role: 'user', group_name: 'default' } });
    act(() => {
      result.current.handleOpenCreateKey();
    });

    await act(async () => {
      await result.current.handleCreateKey();
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent).not.toHaveProperty('tenant_id');
    expect(sent).not.toHaveProperty('key');
  });

  it('applies the self-service quota defaults and caps instead of unlimited', async () => {
    // A self-service form used to default to 0, which the gateway read as
    // "unlimited". The documented policy is 60/100k by default with 6000/10M caps.
    const { result } = setup({ adminUser: { username: 'alice', role: 'user', group_name: 'default' } });
    act(() => {
      result.current.handleOpenCreateKey();
    });
    expect(result.current.newKey.rpm).toBe(60);
    expect(result.current.newKey.tpm).toBe(100000);

    act(() => {
      result.current.setNewKey(prev => ({ ...prev, rpm: 999999, tpm: 999999999 }));
    });
    await act(async () => {
      await result.current.handleCreateKey();
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent.rpm).toBe(6000);
    expect(sent.tpm).toBe(10000000);
  });

  it('still lets an administrator create an unlimited key', async () => {
    const { result } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
    });
    expect(result.current.newKey.rpm).toBe(0);
    expect(result.current.newKey.tpm).toBe(0);

    await act(async () => {
      await result.current.handleCreateKey();
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent.rpm).toBe(0);
    expect(sent.tpm).toBe(0);
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
      await result.current.handleToggleKeyStatus({ id: 1, key_id: 'ak_0000aaaa1111bbbb2222', status: 'active' });
    });

    const [url, opts] = globalThis.fetch.mock.calls.at(-1);
    expect(String(url)).toBe('/api/v1/admin/keys/1');
    expect(JSON.parse(opts.body).status).toBe('disabled');
    expect(refresh).toHaveBeenCalled();
  });

  it('toggles a disabled key back to active', async () => {
    const { result } = setup();

    await act(async () => {
      await result.current.handleToggleKeyStatus({ id: 1, key_id: 'ak_0000aaaa1111bbbb2222', status: 'disabled' });
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
    document.execCommand = vi.fn(() => true);
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

  it('reports a failed copy instead of a false success', async () => {
    // The clipboard helper returns false when both the async API and the
    // execCommand fallback fail. That must not read as "copied": the user may
    // close the one-time-credential dialog and lose the key for good.
    document.execCommand = vi.fn(() => false);
    const { result, showToast } = setup();

    let copied;
    await act(async () => {
      copied = await result.current.copyToClipboard('sk-copied');
    });

    expect(copied).toBe(false);
    expect(result.current.copiedKey).toBe('');
    expect(showToast).toHaveBeenCalledWith(expect.stringMatching(/复制失败|Copy failed/), 'error');
    expect(showToast).not.toHaveBeenCalledWith(expect.any(String), 'success');
  });

  it('shows the one-time credential even when the dialog was closed mid-request', async () => {
    // Regression: dismissals are blocked while the request is in flight, but a
    // close from elsewhere (logout/reset, a stale callback) must not swallow the
    // response-only plaintext either. The success path re-opens the dialog.
    let resolveFetch;
    globalThis.fetch.mockReturnValue(new Promise((resolve) => { resolveFetch = resolve; }));
    const { result } = setup();
    act(() => {
      result.current.handleOpenCreateKey();
    });

    let pending;
    act(() => {
      pending = result.current.handleCreateKey();
    });
    expect(result.current.submittingKey).toBe(true);

    act(() => {
      result.current.closeKeyModal();
    });
    expect(result.current.showKeyModal).toBe(false);

    resolveFetch(jsonResponse({
      code: 0,
      data: { key: 'sk-airoute-created', key_id: 'ak_abc123abc123abc12345', tenant_id: 'root', rpm: 60, tpm: 100000 },
    }));
    await act(async () => {
      await pending;
    });

    // The dialog is open again and holds the only copy of the plaintext.
    expect(result.current.showKeyModal).toBe(true);
    expect(result.current.createdKey).toEqual(expect.objectContaining({
      key: 'sk-airoute-created',
      keyId: 'ak_abc123abc123abc12345',
    }));
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
