import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import KeysView from './KeysView';

// The console must never render a stored API key: the gateway exposes a public,
// non-secret `key_id` (ak_…) for audit and support, and the plaintext exists
// only in the creation response.

const KEYS = { keyId: 'ak_9f8e7d6c5b4a39281357' };
const KEY_ID_TOOLTIP = '密钥 ID（仅用于日志与工单识别，不能用于鉴权）';

function key(over = {}) {
  return {
    id: 7,
    tenant_id: 'alice',
    key_id: KEYS.keyId,
    group_name: 'default',
    rpm: 60,
    tpm: 100000,
    budget: 0,
    used_cost: 1.5,
    allowed_models: ['*'],
    status: 'active',
    created_at: '2026-10-27T00:00:00Z',
    ...over,
  };
}

// Filters live in the URL (`keys.q`), so a search in one test would otherwise
// leak into the next render of the same file.
beforeEach(() => {
  window.history.replaceState(null, '', '/');
});

function setup(overrides = {}) {
  const handlers = {
    handleOpenCreateKey: vi.fn(),
    setSelectedKeyIds: vi.fn(),
    handleBatchStatusKeys: vi.fn(),
    handleBatchDeleteKeys: vi.fn(),
    copyToClipboard: vi.fn(),
    handleToggleKeyStatus: vi.fn(),
    setActiveQuickKey: vi.fn(),
    handleDeleteKey: vi.fn(),
    handleUpdateKeyGroup: vi.fn(),
    adminUser: { username: 'alice', role: 'user' },
    pricingGroups: ['default'],
    groupMultipliers: { default: 1 },
    showToast: vi.fn(),
    lang: 'zh',
    t: {},
    keys: [key()],
    ...overrides,
  };
  const utils = render(<KeysView {...handlers} />);
  return { ...utils, ...handlers };
}

describe('KeysView credential display', () => {
  it('identifies a key by its key ID', () => {
    setup();

    expect(screen.getByText(KEYS.keyId)).toBeInTheDocument();
  });

  it('labels the identifier column as the key ID and explains what it is for', () => {
    setup();

    const header = screen.getByRole('columnheader', { name: '密钥 ID' });
    expect(header).toHaveAttribute('title', KEY_ID_TOOLTIP);
  });

  it('never puts the stored plaintext key in the DOM', () => {
    // A legacy/mocked response may still carry `key`; the row must ignore it.
    const { container } = setup({ keys: [key({ key: 'sk-airoute-0123456789abcdef0123456789abcdef' })] });

    expect(container.textContent).not.toContain('sk-airoute-0123456789abcdef');
    // No control offers to reveal it either.
    expect(screen.queryByTitle('显示明文密钥')).not.toBeInTheDocument();
    expect(screen.queryByText('显示明文')).not.toBeInTheDocument();
  });

  it('falls back to the row number for rows without a key ID', () => {
    const { container } = setup({ keys: [key({ key_id: undefined })] });

    expect(screen.getByText('#7')).toBeInTheDocument();
    expect(container.textContent).not.toContain('***');
  });

  it('copies the key ID rather than a credential', () => {
    const { copyToClipboard } = setup();

    const copyButton = screen.getAllByTitle(KEY_ID_TOOLTIP).find((el) => el.tagName === 'BUTTON');
    copyButton.click();

    expect(copyToClipboard).toHaveBeenCalledWith(KEYS.keyId);
    expect(copyToClipboard).not.toHaveBeenCalledWith('sk-airoute-0123456789abcdef0123456789abcdef');
  });

  it('matches the search on tenant, key ID and pricing group', () => {
    setup({ keys: [key(), key({ id: 8, tenant_id: 'bob', key_id: 'ak_11112222333344445555', group_name: 'vip' })] });

    // All three fields are searchable; the old masked `key` no longer exists.
    const input = screen.getByPlaceholderText('搜索密钥别名 / 密钥 ID / 计费分组...');
    fireEvent.change(input, { target: { value: 'ak_1111' } });

    expect(screen.getByText('bob')).toBeInTheDocument();
    expect(screen.queryByText('alice')).not.toBeInTheDocument();
  });
});


// #16: the copy helper reports whether the write actually happened. Both keys-page
// call sites used to discard that boolean, so a blocked clipboard showed a "copied"
// toast while nothing was on the clipboard.
describe('KeysView copy feedback', () => {
  const chatModels = ['gpt-4o'];

  it('reports an error instead of success when the cURL sample cannot be copied', async () => {
    const copyToClipboard = vi.fn(async () => false);
    const { showToast } = setup({ copyToClipboard, models: chatModels });

    fireEvent.click(screen.getByTitle('复制 cURL 快速接入命令'));

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalled());
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('复制失败'), 'error');
    expect(showToast).not.toHaveBeenCalledWith(expect.stringContaining('已复制到剪贴板'), 'success');
  });

  it('confirms the cURL sample once the copy succeeds', async () => {
    const copyToClipboard = vi.fn(async () => true);
    const { showToast } = setup({ copyToClipboard, models: chatModels });

    fireEvent.click(screen.getByTitle('复制 cURL 快速接入命令'));

    await waitFor(() => expect(showToast).toHaveBeenCalledWith(expect.stringContaining('已复制到剪贴板'), 'success'));
  });

  it('reports an error when the key ID cannot be copied', async () => {
    const copyToClipboard = vi.fn(async () => false);
    const { showToast } = setup({ copyToClipboard });

    fireEvent.click(screen.getAllByTitle(KEY_ID_TOOLTIP).find((el) => el.tagName === 'BUTTON'));

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalledWith(KEYS.keyId));
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('密钥 ID'), 'error');
  });

  it('does not claim a copy when the clipboard helper fails on the key ID', async () => {
    const copyToClipboard = vi.fn(async () => false);
    const { showToast } = setup({ copyToClipboard });

    fireEvent.click(screen.getAllByTitle(KEY_ID_TOOLTIP).find((el) => el.tagName === 'BUTTON'));

    await waitFor(() => expect(showToast).toHaveBeenCalled());
    expect(showToast).not.toHaveBeenCalledWith(expect.any(String), 'success');
  });
});

// The copied snippet names the key by its key ID and still asks the operator for
// the plaintext: the list can only identify a key, never authenticate with it.
describe('KeysView cURL snippet', () => {
  it('annotates the command with the key ID and a plaintext placeholder', async () => {
    const copyToClipboard = vi.fn(async () => true);
    setup({ copyToClipboard, models: ['gpt-4o'] });

    fireEvent.click(screen.getByTitle('复制 cURL 快速接入命令'));

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalled());
    const snippet = copyToClipboard.mock.calls[0][0];
    expect(snippet).toContain(`# key id: ${KEYS.keyId}`);
    expect(snippet).toContain('Authorization: Bearer <YOUR_API_KEY>');
  });
});

// A CSV leaves the browser, so it must carry the identifier only — never a mask
// that looks like a credential (and, by contract, the API returns neither).
describe('KeysView CSV export', () => {
  it('exports the key ID column', async () => {
    const createObjectURL = vi.fn(() => 'blob:keys');
    const revokeObjectURL = vi.fn();
    const originalCreate = URL.createObjectURL;
    const originalRevoke = URL.revokeObjectURL;
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = revokeObjectURL;
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    try {
      const { showToast } = setup();

      fireEvent.click(screen.getByTitle('导出当前密钥列表为 CSV'));

      expect(createObjectURL).toHaveBeenCalled();
      const blob = createObjectURL.mock.calls[0][0];
      const csv = await blob.text();
      expect(csv).toContain('密钥 ID');
      expect(csv).toContain(KEYS.keyId);
      expect(csv).not.toContain('Key Fingerprint');
      expect(csv).not.toContain('***');
      expect(showToast).toHaveBeenCalledWith(expect.stringContaining('成功导出'), 'success');
    } finally {
      clickSpy.mockRestore();
      if (originalCreate) URL.createObjectURL = originalCreate; else delete URL.createObjectURL;
      if (originalRevoke) URL.revokeObjectURL = originalRevoke; else delete URL.revokeObjectURL;
    }
  });
});
