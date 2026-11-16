import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import InfraHealthPanel from './InfraHealthPanel';

function envelope(data) {
  return { ok: true, status: 200, json: async () => ({ code: 0, data }) };
}

const components = [
  { id: 'database', name: '持久化存储引擎 (PostgreSQL)', category: '数据存储', status: 'operational', mode: 'PostgreSQL 关系引擎', latency_ms: 1.25, description: '可靠存储。' },
  { id: 'notification', name: '用户通知与安全验证系统', category: '通知与认证', status: 'disabled', mode: '未配置 SMTP', description: '邮箱验证码。' },
];

describe('InfraHealthPanel', () => {
  it('renders component health from the admin middlewares endpoint', async () => {
    const adminFetch = vi.fn(async () => envelope(components));
    render(<InfraHealthPanel adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
    expect(await screen.findByText('持久化存储引擎 (PostgreSQL)')).toBeInTheDocument();
    expect(screen.getByText('运行正常')).toBeInTheDocument();
    expect(screen.getByText('未启用')).toBeInTheDocument();
    expect(screen.getByText(/1.25 ms/)).toBeInTheDocument();
    expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/system/middlewares', { signal: expect.any(AbortSignal) });
  });

  it('warns when a component is degraded', async () => {
    const adminFetch = vi.fn(async () => envelope([
      { id: 'database', name: 'DB', category: '数据存储', status: 'degraded', mode: 'PG', description: 'x' },
    ]));
    render(<InfraHealthPanel adminFetch={adminFetch} showToast={vi.fn()} lang="zh" />);
    expect(await screen.findByText(/1 个组件性能降级/)).toBeInTheDocument();
    expect(screen.getByText('性能降级')).toBeInTheDocument();
  });

  it('surfaces a load failure', async () => {
    const adminFetch = vi.fn(async () => ({ ok: false, status: 500, json: async () => ({ code: 1, error: 'boom' }) }));
    const showToast = vi.fn();
    render(<InfraHealthPanel adminFetch={adminFetch} showToast={showToast} lang="zh" />);
    expect(await screen.findByText(/健康状态加载失败：boom/)).toBeInTheDocument();
    expect(showToast).toHaveBeenCalledWith('boom', 'error');
  });

  it('uses shared English retry copy and recovers from a structured failure', async () => {
    const adminFetch = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 503, json: async () => ({ error: { message: 'database unavailable' } }) })
      .mockResolvedValueOnce(envelope(components));
    render(<InfraHealthPanel adminFetch={adminFetch} showToast={vi.fn()} lang="en" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Failed to load health: database unavailable');
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('Operational')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByText(/Last updated/)).toBeInTheDocument();
  });

  it('discards a late response from the previous account', async () => {
    let finishOld;
    const oldFetch = vi.fn(() => new Promise(resolve => { finishOld = resolve; }));
    const showToast = vi.fn();
    const { rerender } = render(<InfraHealthPanel adminFetch={oldFetch} showToast={showToast} />);
    rerender(<InfraHealthPanel adminFetch={async () => envelope(components)} showToast={showToast} />);
    await screen.findByText('持久化存储引擎 (PostgreSQL)');
    await act(async () => finishOld(envelope([{ id: 'old', name: 'Previous account' }])));
    expect(screen.queryByText('Previous account')).not.toBeInTheDocument();
    expect(oldFetch.mock.calls[0][1].signal.aborted).toBe(true);
  });
});
