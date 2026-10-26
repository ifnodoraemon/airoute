import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { TextEncoder } from 'node:util';
import { useChannelActions } from './hooks/useChannelActions';
import { useLogsData } from './hooks/useLogsData';
import { resetApiClient } from './api/client';
import PlaygroundView from './components/PlaygroundView';
import DashboardView from './components/DashboardView';
import AnnouncementModal from './components/AnnouncementModal';
import ModelRouteModal from './components/ModelRouteModal';

const response = (data, status = 200) => ({ ok: status < 400, status, json: async () => data });
afterEach(() => { vi.unstubAllGlobals(); resetApiClient(); });

describe('Audit review probes against unchanged behavior', () => {
  it('preserves model tags in a channel mapping payload', async () => {
    const fetch = vi.fn().mockResolvedValue(response({ code: 0 }));
    vi.stubGlobal('fetch', fetch);
    const { result } = renderHook(() => useChannelActions({ isZh: true, showToast: vi.fn(), refresh: vi.fn() }));
    act(() => result.current.setNewChannel(previous => ({ ...previous, name: 'audit', base_url: 'https://example.invalid', mapping_str: 'public-model:qwen3:32b' })));
    await act(async () => { await result.current.handleCreateChannel({ preventDefault() {} }); });
    expect(JSON.parse(fetch.mock.calls[0][1].body).model_mapping['public-model']).toBe('qwen3:32b');
  });

  it('ignores a channel probe response after a different channel is opened', async () => {
    let finishProbe;
    vi.stubGlobal('fetch', vi.fn(() => new Promise(resolve => { finishProbe = resolve; })));
    const { result } = renderHook(() => useChannelActions({ isZh: true, showToast: vi.fn(), refresh: vi.fn() }));
    act(() => result.current.handleEditChannel({ id: 1, name: 'first', base_url: 'https://first.invalid', models: ['first-model'] }));
    act(() => result.current.handleProbeChannel());
    act(() => {
      result.current.closeChannelModal();
      result.current.handleEditChannel({ id: 2, name: 'second', base_url: 'https://second.invalid', models: ['second-model'] });
    });
    await act(async () => { finishProbe(response({ code: 0, data: { type: 'anthropic', models: ['from-first-probe'], protocols: ['anthropic_messages'] } })); });
    expect(result.current.newChannel.models_str).toBe('second-model');
  });

  it('reports a failed second export page instead of returning a complete export', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(response({ code: 0, data: Array.from({ length: 200 }, (_, id) => ({ id })) }))
      .mockResolvedValueOnce(response({ error: 'database temporarily unavailable' }, 503));
    vi.stubGlobal('fetch', fetch);
    const { result } = renderHook(() => useLogsData({ filters: { range: 'today' }, isZh: true, showToast: vi.fn() }));
    await expect(result.current.fetchAllLogsForExport()).rejects.toThrow();
  });

  it('surfaces a stream error even when its message does not contain stream error', async () => {
    const reader = { read: vi.fn()
      .mockResolvedValueOnce({ done: false, value: new TextEncoder().encode('data: {"error":{"message":"upstream overloaded","type":"upstream_error"}}\n\ndata: [DONE]\n\n') })
      .mockResolvedValueOnce({ done: true }) };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, body: { getReader: () => reader } }));
    const showToast = vi.fn();
    render(<PlaygroundView keys={[{ key: 'sk-audit', allowed_models: ['*'] }]} models={['gpt-audit']} initialModel="gpt-audit" showToast={showToast} />);
    fireEvent.change(screen.getByPlaceholderText('输入对话 Prompt，Enter 发送 / Shift+Enter 换行...'), { target: { value: 'hello' } });
    fireEvent.click(screen.getByRole('button', { name: '发送请求' }));
    await waitFor(() => expect(screen.getByRole('button', { name: '发送请求' })).toBeEnabled());
    expect(showToast).toHaveBeenCalledWith('upstream overloaded', 'error');
  });

  it('loads selected-key overview when returning to the dashboard', async () => {
    const adminFetch = vi.fn().mockResolvedValue(response({ code: 0, data: [] }));
    render(<DashboardView selectedKey="sk-selected" keys={[{key:'sk-selected'}]} stats={{total_requests:100}} adminUser={{role:'admin'}} adminFetch={adminFetch} />);
    await act(async () => {});
    expect(adminFetch.mock.calls.map(([url]) => url)).toContain('/api/v1/admin/stats/overview?key=sk-selected');
  });

  it('does not overwrite an announcement draft with a late initial load', async () => {
    let finishLoad;
    const adminFetch = vi.fn(() => new Promise(resolve => { finishLoad = resolve; }));
    render(<AnnouncementModal isOpen onClose={vi.fn()} showToast={vi.fn()} adminFetch={adminFetch} />);
    const title = screen.getByPlaceholderText('例如：双活集群网络升级与维护预告');
    fireEvent.change(title, { target: { value: 'My new draft' } });
    await act(async () => { finishLoad(response({ code: 0, data: { enabled: true, title: 'Old published title', content: 'Old content' } })); });
    expect(title).toHaveValue('My new draft');
  });

  it('guards unsaved model routing changes when Escape is pressed', () => {
    const onClose = vi.fn();
    render(<ModelRouteModal isOpen modelRoute={{ model: 'audit-model', providers: [], prompt_price: 2 }} onClose={onClose} adminFetch={vi.fn()} showToast={vi.fn()} />);
    fireEvent.change(screen.getByDisplayValue('2'), { target: { value: '7' } });
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();
  });
});
