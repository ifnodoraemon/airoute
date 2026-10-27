import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import KeysView from './KeysView';

// The console must never render a stored API key: the gateway exposes a stable,
// non-reversible `key_fingerprint` (kfp_…) for audit and support, and the plaintext
// exists only in the creation response.

function key(over = {}) {
  return {
    id: 7,
    tenant_id: 'alice',
    key: 'sk-airoute-0123456789abcdef0123456789abcdef',
    key_fingerprint: 'kfp_9f8e7d6c5b4a3928',
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
  it('identifies a key by its fingerprint', () => {
    setup();

    expect(screen.getByText('kfp_9f8e7d6c5b4a3928')).toBeInTheDocument();
  });

  it('never puts the stored plaintext key in the DOM', () => {
    const { container } = setup();

    expect(container.textContent).not.toContain('sk-airoute-0123456789abcdef');
    // No control offers to reveal it either.
    expect(screen.queryByTitle('显示明文密钥')).not.toBeInTheDocument();
    expect(screen.queryByText('显示明文')).not.toBeInTheDocument();
  });

  it('falls back to an opaque mask for legacy rows without a fingerprint', () => {
    const { container } = setup({ keys: [key({ key_fingerprint: undefined })] });

    expect(screen.getByText('sk-airoute-••••••••••••')).toBeInTheDocument();
    expect(container.textContent).not.toContain('0123456789abcdef');
  });

  it('copies the fingerprint rather than the credential', () => {
    const { copyToClipboard } = setup();

    const copyButton = screen.getByTitle('复制密钥指纹');
    copyButton.click();

    expect(copyToClipboard).toHaveBeenCalledWith('kfp_9f8e7d6c5b4a3928');
    expect(copyToClipboard).not.toHaveBeenCalledWith('sk-airoute-0123456789abcdef0123456789abcdef');
  });
});
