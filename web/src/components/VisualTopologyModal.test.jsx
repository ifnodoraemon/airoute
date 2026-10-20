import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import VisualTopologyModal from './VisualTopologyModal';

const provider = (name, priority, id) => ({
  channel_id: id, name, type: 'gpustack', priority, weight: 10, mapped_model: `${name}-mapped`,
  protocols: ['openai_chat'], provider_supported: true, eligible: true,
  breaker: { state: 'CLOSED', consecutive_failures: 0, canary_in_flight: 0, recovery_probe: false, cooldown_remaining_ms: 0 },
});
function data(overrides = {}) {
  return {
    node: 'runtime-node-2', config_version: 4, observed_at: '2026-10-20T05:00:00Z',
    routing: { model: 'alias', models: ['alias', 'dr'], steps: [
      { model: 'alias', modality: 'chat', providers: [provider('primary', 2, 1), provider('backup', 8, 2)] },
      { model: 'dr', modality: 'chat', providers: [provider('disaster-recovery', 7, 3)] },
    ] },
    activity: { requests: 100, errors: 3, failovers: 2, channels: {}, recent: [] },
    ...overrides,
  };
}
const response = value => ({ ok: true, json: async () => ({ code: 0, data: value }) });

function setup(props = {}) {
  const adminFetch = vi.fn(async () => response(data()));
  const onViewLogs = vi.fn();
  const utils = render(<VisualTopologyModal isOpen onClose={vi.fn()} adminFetch={adminFetch} onViewLogs={onViewLogs} {...props} />);
  return { ...utils, adminFetch, onViewLogs };
}

describe('runtime routing topology', () => {
  it('renders the actual model chain and priorities rather than three global slots', async () => {
    setup();
    expect(await screen.findByText('primary')).toBeInTheDocument();
    expect(screen.getByText('Priority 2')).toBeInTheDocument();
    expect(screen.getByText('Priority 8')).toBeInTheDocument();
    expect(screen.getByText('Priority 7')).toBeInTheDocument();
    expect(screen.getByText(/runtime-node-2/)).toBeInTheDocument();
    expect(screen.queryByText(/故障推演|自动接管流量中|24 ms/)).not.toBeInTheDocument();
  });

  it('has no invented primary/backup/healthy channels when runtime configuration is empty', async () => {
    setup({ adminFetch: vi.fn(async () => response(data({ routing: { model: '', models: [], steps: [] }, activity: null }))) });
    expect(await screen.findByText(/未发现生效模型路由/)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: '验证模型' })).not.toBeInTheDocument();
  });

  it('sends an explicit bounded-model validation request and displays the real result', async () => {
    const adminFetch = vi.fn(async (url, options) => options?.method === 'POST'
      ? response({ model: 'alias', channel_id: 1, success: false, error: '上游返回 HTTP 503' }) : response(data()));
    setup({ adminFetch });
    await screen.findByText('primary');
    fireEvent.click(screen.getAllByRole('button', { name: '验证模型' })[0]);
    expect(await screen.findByText('上游返回 HTTP 503')).toBeInTheDocument();
    const call = adminFetch.mock.calls.find(([, options]) => options?.method === 'POST');
    expect(call[0]).toBe('/api/v1/admin/models/topology/probe');
    expect(JSON.parse(call[1].body)).toEqual({ model: 'alias', channel_id: 1 });
  });

  it('shows the observed OPEN breaker and recovery cooldown', async () => {
    const payload = data();
    payload.routing.steps[0].providers[0] = { ...provider('primary', 2, 1), eligible: false,
      breaker: { state: 'OPEN', consecutive_failures: 3, canary_in_flight: 0, cooldown_remaining_ms: 29000 } };
    setup({ adminFetch: vi.fn(async () => response(payload)) });
    expect(await screen.findByText('当前跳过 · OPEN')).toBeInTheDocument();
    expect(screen.getByText(/剩余冷却 29s/)).toBeInTheDocument();
  });

  it('keeps the last observed snapshot visibly stale on refresh failure', async () => {
    const { adminFetch } = setup();
    await screen.findByText('primary');
    adminFetch.mockRejectedValue(new Error('offline'));
    fireEvent.click(screen.getByRole('button', { name: '刷新快照' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('下方保留上次快照');
    expect(screen.getByText('primary')).toBeInTheDocument();
  });

  it('links to the selected model logs and does not call upstream validation automatically', async () => {
    const { adminFetch, onViewLogs } = setup();
    await screen.findByText('primary');
    fireEvent.click(screen.getByRole('button', { name: '查看该模型日志' }));
    expect(onViewLogs).toHaveBeenCalledWith('alias', undefined);
    expect(adminFetch.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false);
  });

  it('does not fetch runtime topology while closed', async () => {
    const { adminFetch } = setup({ isOpen: false });
    await waitFor(() => expect(adminFetch).not.toHaveBeenCalled());
  });
});
