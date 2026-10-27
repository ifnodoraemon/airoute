import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import KeyModal from './KeyModal';

// Key creation UI. The gateway now owns the tenant of a self-service key and applies
// a default/capped quota policy, so the form must not offer a tenant field it cannot
// honour, must not describe 0 as "unlimited" for everyone, and must show the
// plaintext credential exactly once (at creation).

function draft(over = {}) {
  return {
    tenant_id: '',
    key: '',
    rpm: 60,
    tpm: 100000,
    budget: 0,
    allowed_models: [],
    group_name: 'default',
    ...over,
  };
}

function setup({ adminUser = { username: 'alice', role: 'user', group_name: 'default' }, newKey = draft(), ...props } = {}) {
  const handlers = {
    onClose: vi.fn(),
    setNewKey: vi.fn(),
    onSubmit: vi.fn((e) => e?.preventDefault?.()),
    onCopy: vi.fn(),
    submitting: false,
    lang: 'zh',
    t: {},
  };
  const utils = render(
    <KeyModal
      isOpen
      newKey={newKey}
      adminUser={adminUser}
      {...handlers}
      {...props}
    />,
  );
  return { ...utils, ...handlers };
}

describe('KeyModal tenant ownership', () => {
  it('shows a server-assigned tenant to a self-service user instead of an input', () => {
    setup({ adminUser: { username: 'alice', role: 'user', group_name: 'default' } });

    expect(screen.getByText('归属租户（由服务器分配）')).toBeInTheDocument();
    expect(screen.getByText('alice')).toBeInTheDocument();
    // No editable tenant field: the server derives it from the session.
    expect(screen.queryByPlaceholderText('如 个人开发 / Cursor / Claude Code')).not.toBeInTheDocument();
    expect(screen.getByText(/个人密钥自动归属您的账号/)).toBeInTheDocument();
  });

  it('keeps the tenant field for an administrator', () => {
    setup({ adminUser: { username: 'root', role: 'admin' } });

    const tenantInput = screen.getByPlaceholderText('如 个人开发 / Cursor / Claude Code');
    expect(tenantInput).toBeInTheDocument();
    expect(screen.getByText(/管理员可指定该密钥归属的租户标识/)).toBeInTheDocument();
  });

  it('never offers a manually typed credential to a self-service user', () => {
    setup({ adminUser: { username: 'alice', role: 'user' } });

    expect(screen.queryByPlaceholderText('留空自动生成')).not.toBeInTheDocument();
    expect(screen.getByText(/密钥明文仅在创建时显示一次/)).toBeInTheDocument();
  });
});

describe('KeyModal quota policy', () => {
  it('documents the self-service defaults and caps, not "0 = unlimited"', () => {
    setup({ adminUser: { username: 'alice', role: 'user' } });

    expect(screen.getByText(/默认 60 RPM，自助创建最高 6000 RPM/)).toBeInTheDocument();
    expect(screen.getByText(/默认 100,000 TPM，自助创建最高 10,000,000 TPM/)).toBeInTheDocument();
    // "Unlimited" is an administrator-only option for the rate/token quotas. Exactly
    // one 不限 control remains — the spend-budget preset, where 0 really does mean
    // "no cap" for every user.
    expect(screen.queryByText(/速率限制 \(RPM, 0 为不限\)/)).not.toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: '不限' })).toHaveLength(1);
  });

  it('shows the cap in the RPM/TPM labels and inputs for self-service', () => {
    setup({ adminUser: { username: 'alice', role: 'user' } });

    expect(screen.getByText(/速率上限 \(RPM\) \(60–6000\)/)).toBeInTheDocument();
    expect(screen.getByText(/Token 上限 \(TPM\) \(100000–10000000\)/)).toBeInTheDocument();
    // The spend-budget field is also a number input, so pick the last two.
    const numberInputs = document.querySelectorAll('input[type="number"]');
    const rpmInput = numberInputs[numberInputs.length - 2];
    const tpmInput = numberInputs[numberInputs.length - 1];
    expect(rpmInput.max).toBe('6000');
    expect(tpmInput.max).toBe('10000000');
  });

  it('keeps "0 = unlimited" available to an administrator', () => {
    setup({ adminUser: { username: 'root', role: 'admin' }, newKey: draft({ tenant_id: 'team-a', rpm: 0, tpm: 0 }) });

    expect(screen.getAllByText('不限').length).toBeGreaterThan(0);
    expect(screen.getByText(/0 表示不限速（仅管理员可创建不限速密钥）/)).toBeInTheDocument();
    expect(screen.getByText(/0 表示不限量（仅管理员可创建不限量密钥）/)).toBeInTheDocument();
  });
});

describe('KeyModal one-time credential reveal', () => {
  it('shows the created key, its fingerprint and the single-view warning', () => {
    setup({
      createdKey: { key: 'sk-airoute-created', fingerprint: 'kfp_abc123', tenant: 'alice', rpm: 60, tpm: 100000 },
    });

    expect(screen.getByText('密钥已创建')).toBeInTheDocument();
    expect(screen.getByTestId('created-key').textContent).toBe('sk-airoute-created');
    expect(screen.getByText('kfp_abc123')).toBeInTheDocument();
    expect(screen.getByText(/唯一一次显示明文/)).toBeInTheDocument();
  });

  it('copies the credential only from the reveal panel', () => {
    const { onCopy } = setup({
      createdKey: { key: 'sk-airoute-created', fingerprint: 'kfp_abc123', tenant: 'alice', rpm: 60, tpm: 100000 },
    });

    fireEvent.click(screen.getByRole('button', { name: '复制' }));

    expect(onCopy).toHaveBeenCalledWith('sk-airoute-created');
  });

  it('closes without a discard prompt once the key is created', () => {
    const { onClose } = setup({
      createdKey: { key: 'sk-airoute-created', fingerprint: 'kfp_abc123', tenant: 'alice', rpm: 60, tpm: 100000 },
    });

    fireEvent.click(screen.getByRole('button', { name: '我已保存，关闭' }));

    expect(onClose).toHaveBeenCalled();
    expect(screen.queryByText('放弃未保存的密钥配置？')).not.toBeInTheDocument();
  });

  it('notes when the requested quota was clamped to the self-service limit', () => {
    setup({
      createdKey: { key: 'sk-airoute-created', fingerprint: 'kfp_abc123', tenant: 'alice', rpm: 6000, tpm: 10000000, quotaAdjusted: true },
    });

    expect(screen.getByText(/已按自助密钥上限调整速率\/Token 配额/)).toBeInTheDocument();
  });
});
