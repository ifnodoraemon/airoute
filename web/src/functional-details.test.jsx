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
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); resetApiClient(); });

describe('Functional details regressions', () => {
  it('preserves model tags in a channel mapping payload', async () => {
    const fetch = vi.fn().mockResolvedValue(response({ code: 0 }));
    vi.stubGlobal('fetch', fetch);
    const { result } = renderHook(() => useChannelActions({ isZh: true, showToast: vi.fn(), refresh: vi.fn() }));
    act(() => result.current.setNewChannel(previous => ({ ...previous, name: 'audit', base_url: 'https://example.invalid', mapping_rules: [{from: 'public-model:tag', to: 'qwen3:32b'}] })));
    await act(async () => { await result.current.handleCreateChannel({ preventDefault() {} }); });
    expect(JSON.parse(fetch.mock.calls[0][1].body).model_mapping['public-model:tag']).toBe('qwen3:32b');
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

  it('prevents editing and saving before the announcement has loaded', async () => {
    let finishLoad;
    const adminFetch = vi.fn(() => new Promise(resolve => { finishLoad = resolve; }));
    render(<AnnouncementModal isOpen onClose={vi.fn()} showToast={vi.fn()} adminFetch={adminFetch} />);
    const title = screen.getByPlaceholderText('例如：双活集群网络升级与维护预告');
    expect(title).toBeDisabled();
    await act(async () => { finishLoad(response({ code: 0, data: { enabled: true, title: 'Old published title', content: 'Old content' } })); });
    expect(title).toBeEnabled();
    expect(title).toHaveValue('Old published title');
    fireEvent.change(title, { target: { value: 'My new draft' } });
    expect(title).toHaveValue('My new draft');
  });

  it('guards unsaved model routing changes when Escape is pressed', () => {
    const onClose = vi.fn();
    render(<ModelRouteModal isOpen modelRoute={{ model: 'audit-model', providers: [], prompt_price: 2 }} onClose={onClose} adminFetch={vi.fn()} showToast={vi.fn()} />);
    fireEvent.change(screen.getByDisplayValue('2'), { target: { value: '7' } });
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();
  });

  it('includes the selected video model in the task polling request', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    const fetch = vi.fn().mockResolvedValueOnce(response({ id: 'video-task', status: 'processing' }))
      .mockResolvedValue(response({ id: 'video-task', status: 'succeeded' }));
    vi.stubGlobal('fetch', fetch);
    render(<PlaygroundView initialModality="videos" initialModel="sora-audit" models={['sora-audit']} modelRoutes={[{model:'sora-audit',modality:'videos'}]} showToast={vi.fn()} />);
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '生成视频' })); });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    const pollURL = new URL(fetch.mock.calls[1][0], 'https://audit.invalid');
    expect(pollURL.searchParams.get('model')).toBe('sora-audit');
  });

  it('stops video polling after the user presses Stop', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    const fetch = vi.fn().mockResolvedValueOnce(response({ id: 'video-task', status: 'processing' }))
      .mockResolvedValue(response({ id: 'video-task', status: 'succeeded' }));
    vi.stubGlobal('fetch', fetch);
    render(<PlaygroundView initialModality="videos" initialModel="sora-audit" models={['sora-audit']} modelRoutes={[{model:'sora-audit',modality:'videos'}]} showToast={vi.fn()} />);
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '生成视频' })); });
    fireEvent.click(screen.getByRole('button', { name: '停止生成' }));
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});

it('uses only the selected key for every summary card and ignores late prior-key data', async () => {
  let finishFirst;
  const adminFetch = vi.fn(async (url) => {
    if (url.includes('/overview?key=first')) return new Promise(resolve => { finishFirst = resolve; });
    if (url.includes('/overview?key=second')) return response({code:0,data:{total_requests:222,total_cost:7}});
    return response({code:0,data:[]});
  });
  const props = { keys:[{key:'first'},{key:'second'}], stats:{total_requests:999777,total_cost:9999}, adminUser:{role:'admin'}, adminFetch };
  const view = render(<DashboardView {...props} selectedKey="first" />);
  expect(screen.queryByText('999,777')).not.toBeInTheDocument();
  view.rerender(<DashboardView {...props} selectedKey="second" />);
  await waitFor(() => expect(screen.getByText('222')).toBeInTheDocument());
  await act(async () => { finishFirst(response({code:0,data:{total_requests:111,total_cost:9}})); });
  expect(screen.getByText('222')).toBeInTheDocument();
  expect(screen.queryByText('111')).not.toBeInTheDocument();
  expect(screen.queryByText('999,777')).not.toBeInTheDocument();
});

it('refreshes the selected key overview on the dashboard timer', async () => {
  vi.useFakeTimers({toFake:['setInterval','clearInterval']});
  const adminFetch = vi.fn().mockResolvedValue(response({code:0,data:{total_requests:321}}));
  render(<DashboardView selectedKey="selected" stats={{}} adminFetch={adminFetch} />);
  await act(async () => {});
  const overviewCalls = () => adminFetch.mock.calls.filter(([url]) => url === '/api/v1/admin/stats/overview?key=selected').length;
  expect(overviewCalls()).toBe(1);
  await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
  expect(overviewCalls()).toBe(2);
});

it('keeps announcement saving disabled after a failed load and supports retry', async () => {
  const adminFetch = vi.fn().mockResolvedValueOnce(response({error:'configuration unavailable'},503))
    .mockResolvedValueOnce(response({code:0,data:{title:'Recovered',content:'Content',enabled:true}}));
  render(<AnnouncementModal isOpen onClose={vi.fn()} showToast={vi.fn()} adminFetch={adminFetch} />);
  await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('configuration unavailable'));
  expect(screen.getByPlaceholderText('例如：双活集群网络升级与维护预告')).toBeDisabled();
  fireEvent.click(screen.getByRole('button',{name:'重试'}));
  await waitFor(() => expect(screen.getByDisplayValue('Recovered')).toBeEnabled());
});

it('does not poll again after a video query HTTP error', async () => {
  vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});
  const fetch = vi.fn().mockResolvedValueOnce(response({id:'task',status:'processing'}))
    .mockResolvedValue(response({error:{message:'task unavailable'}},404));
  vi.stubGlobal('fetch',fetch);
  const showToast=vi.fn();
  render(<PlaygroundView initialModality="videos" initialModel="sora-audit" models={['sora-audit']} modelRoutes={[{model:'sora-audit',modality:'videos'}]} showToast={showToast} />);
  await act(async () => {fireEvent.click(screen.getByRole('button',{name:'生成视频'}));});
  await act(async () => {await vi.advanceTimersByTimeAsync(12000);});
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(showToast).toHaveBeenCalledWith('task unavailable','error');
  expect(screen.getByRole('button',{name:'生成视频'})).toBeEnabled();
});

it('cancels pending video polling when the view unmounts', async () => {
  vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});
  const fetch=vi.fn().mockResolvedValue(response({id:'task',status:'processing'}));
  vi.stubGlobal('fetch',fetch);
  const view=render(<PlaygroundView initialModality="videos" initialModel="sora-audit" models={['sora-audit']} modelRoutes={[{model:'sora-audit',modality:'videos'}]} showToast={vi.fn()} />);
  await act(async () => {fireEvent.click(screen.getByRole('button',{name:'生成视频'}));});
  view.unmount();
  await act(async () => {await vi.advanceTimersByTimeAsync(6000);});
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][1].signal.aborted).toBe(true);
});
