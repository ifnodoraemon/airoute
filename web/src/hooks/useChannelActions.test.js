import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useChannelActions } from './useChannelActions';

// Covers the channel domain that used to be ~470 lines inside App.jsx. The App
// integration test proves the console mounts and routes; these prove the actions
// themselves, which the integration test does not reach.

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

function setup(overrides = {}) {
  const showToast = vi.fn();
  const refresh = vi.fn();
  const props = {
    channels: [{ id: 1, name: 'c1' }, { id: 2, name: 'c2' }],
    showToast,
    isZh: true,
    refresh,
    ...overrides,
  };
  const utils = renderHook(() => useChannelActions(props));
  return { ...utils, showToast, refresh };
}

describe('useChannelActions', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ code: 0, data: {} })));
    vi.stubGlobal('confirm', vi.fn(() => true));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows a committed channel reload warning and refreshes the saved record', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 0, warning: '渠道已保存，但未生效' }, 202));
    const { result, showToast, refresh } = setup();
    act(() => result.current.setNewChannel(previous => ({ ...previous, name: 'gpu', base_url: 'https://gpu.example' })));
    await act(async () => { await result.current.handleCreateChannel({ preventDefault() {} }); });
    expect(showToast).toHaveBeenCalledWith('渠道已保存，但未生效', 'warning');
    expect(refresh).toHaveBeenCalledOnce();
  });

  it('opens the create form empty', () => {
    const { result } = setup();

    act(() => {
      result.current.openCreateChannel();
    });

    expect(result.current.showChannelModal).toBe(true);
    expect(result.current.editingChannelId).toBeNull();
    expect(result.current.newChannel.name).toBe('');
    expect(result.current.newChannel.models_str).toBe('');
  });

  it('loads an existing channel into the form for editing', () => {
    const { result } = setup();

    act(() => {
      result.current.handleEditChannel({
        id: 7, name: 'upstream', type: 'anthropic', base_url: 'https://a.example',
        api_key: 'k', priority: 3, weight: 42, timeout_seconds: 90,
        models: ['m1', 'm2'], model_mapping: { a: 'b' }, protocols: ['openai_chat'],
      });
    });

    const form = result.current.newChannel;
    expect(result.current.editingChannelId).toBe(7);
    expect(form.name).toBe('upstream');
    expect(form.models_str).toBe('m1, m2');
    expect(form.mapping_str).toBe('a:b');
    expect(form.weight).toBe(42);
  });

  it('applies a preset without losing the typed endpoint', () => {
    const { result } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({ ...prev, base_url: 'https://mine.example/v1' }));
    });

    act(() => {
      result.current.applyPreset('deepseek');
    });

    expect(result.current.newChannel.base_url).toBe('https://mine.example/v1');
    expect(result.current.newChannel.models_str).toContain('deepseek-chat');
  });

  it('toggles a protocol on and off', () => {
    const { result } = setup();
    const before = result.current.newChannel.protocols.includes('images');

    act(() => {
      result.current.toggleProtocol('images');
    });
    expect(result.current.newChannel.protocols.includes('images')).toBe(!before);

    act(() => {
      result.current.toggleProtocol('images');
    });
    expect(result.current.newChannel.protocols.includes('images')).toBe(before);
  });

  it('refuses a base URL without a scheme', async () => {
    const { result, showToast, refresh } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({ ...prev, base_url: 'api.example.com', name: 'x' }));
    });

    await act(async () => {
      await result.current.handleCreateChannel({ preventDefault() {} });
    });

    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('http'), 'warning');
    expect(refresh).not.toHaveBeenCalled();
  });

  it('creates a channel, closes the form and refreshes the console', async () => {
    const { result, refresh } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({
        ...prev, name: 'new', base_url: 'https://api.example/v1', models_str: 'a, b',
      }));
    });

    await act(async () => {
      await result.current.handleCreateChannel({ preventDefault() {} });
    });

    const [url, opts] = globalThis.fetch.mock.calls.at(-1);
    expect(String(url)).toBe('/api/v1/admin/channels');
    expect(opts.method).toBe('POST');
    const sent = JSON.parse(opts.body);
    expect(sent.models).toEqual(['a', 'b']);
    // The form-only fields must not reach the API.
    expect(sent.models_str).toBeUndefined();
    expect(sent.mapping_str).toBeUndefined();

    expect(result.current.showChannelModal).toBe(false);
    expect(refresh).toHaveBeenCalled();
  });

  it('updates rather than creates when a channel is being edited', async () => {
    const { result } = setup();
    act(() => {
      result.current.handleEditChannel({ id: 7, name: 'x', base_url: 'https://a.example', models: [] });
    });

    await act(async () => {
      await result.current.handleCreateChannel({ preventDefault() {} });
    });

    const [url, opts] = globalThis.fetch.mock.calls.at(-1);
    expect(String(url)).toBe('/api/v1/admin/channels/7');
    expect(opts.method).toBe('PUT');
  });

  it('parses the model mapping into an object', async () => {
    const { result } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({
        ...prev, name: 'x', base_url: 'https://a.example', mapping_str: 'gpt-4: gpt-4o, claude: sonnet',
      }));
    });

    await act(async () => {
      await result.current.handleCreateChannel({ preventDefault() {} });
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent.model_mapping).toEqual({ 'gpt-4': 'gpt-4o', claude: 'sonnet' });
  });

  it('keeps the form open when the save fails', async () => {
    globalThis.fetch.mockRejectedValue(new Error('boom'));
    const { result, refresh } = setup();
    act(() => {
      result.current.openCreateChannel();
      result.current.setNewChannel(prev => ({ ...prev, name: 'x', base_url: 'https://a.example' }));
    });

    await act(async () => {
      await result.current.handleCreateChannel({ preventDefault() {} });
    });

    expect(result.current.showChannelModal).toBe(true);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('asks before deleting, and does nothing when declined', async () => {
    globalThis.confirm = vi.fn(() => false);
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleDeleteChannel(1);
    });

    expect(refresh).not.toHaveBeenCalled();
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it('deletes and refreshes when confirmed', async () => {
    const { result, refresh } = setup();

    await act(async () => {
      await result.current.handleDeleteChannel(1);
    });

    const [url, opts] = globalThis.fetch.mock.calls.at(-1);
    expect(String(url)).toBe('/api/v1/admin/channels/1');
    expect(opts.method).toBe('DELETE');
    expect(refresh).toHaveBeenCalled();
  });

  it('records a successful latency test against the channel', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 0, latency_ms: 42 }));
    const { result } = setup();

    await act(async () => {
      await result.current.handleTestChannel({ id: 1, name: 'c1' });
    });

    expect(result.current.channelLatencies[1]).toEqual({ latency_ms: 42, success: true, error: '' });
    expect(result.current.testingId).toBeNull();
  });

  it('records a failed latency test without throwing', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 1, error: 'refused' }));
    const { result } = setup();

    await act(async () => {
      await result.current.handleTestChannel({ id: 1, name: 'c1' });
    });

    expect(result.current.channelLatencies[1].success).toBe(false);
    expect(result.current.channelLatencies[1].error).toBe('refused');
  });

  it('health-checks every channel and reports the count', async () => {
    let call = 0;
    globalThis.fetch.mockImplementation(async () => {
      call += 1;
      return jsonResponse({ code: call === 1 ? 0 : 1, latency_ms: 10 });
    });
    const { result, showToast } = setup();

    await act(async () => {
      await result.current.handleBatchPing();
    });

    expect(Object.keys(result.current.channelLatencies)).toHaveLength(2);
    expect(result.current.batchTesting).toBe(false);
    expect(showToast).toHaveBeenCalledWith(expect.stringContaining('1/2'), 'warning');
  });

  it('does nothing when health-checking an empty fleet', async () => {
    const { result, showToast } = setup({ channels: [] });

    await act(async () => {
      await result.current.handleBatchPing();
    });

    expect(showToast).not.toHaveBeenCalled();
  });

  it('probes the form endpoint and applies what the upstream reported', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({
      code: 0,
      data: {
        type: 'anthropic', latency_ms: 12, suggested_name: 'probed-name',
        suggested_base_url: 'https://probed.example/v1',
        models: ['m1', 'm2'], protocols: ['anthropic_messages'],
      },
    }));
    const { result } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({ ...prev, base_url: 'https://probed.example', api_key: 'k', type: 'openai' }));
    });

    await act(async () => {
      await result.current.handleProbeChannel();
    });

    expect(result.current.probeAlert.type).toBe('success');
    expect(result.current.newChannel.models_str).toBe('m1, m2');
    expect(result.current.newChannel.type).toBe('anthropic');
    expect(result.current.probing).toBe(false);
  });

  it('reports a probe failure as an error alert', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 1, error: 'nope' }));
    const { result } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({ ...prev, base_url: 'https://bad.example' }));
    });

    await act(async () => {
      await result.current.handleProbeChannel();
    });

    expect(result.current.probeAlert.type).toBe('error');
  });

  it('refuses to probe without an endpoint', async () => {
    const { result, showToast } = setup();
    act(() => {
      result.current.setNewChannel(prev => ({ ...prev, base_url: '   ' }));
    });

    await act(async () => {
      await result.current.handleProbeChannel();
    });

    expect(showToast).toHaveBeenCalledWith(expect.any(String), 'warning');
    expect(globalThis.fetch).not.toHaveBeenCalled();
  });

  it('batch-deletes the selected channels and clears the selection', async () => {
    const { result, refresh } = setup();
    act(() => {
      result.current.setSelectedChannelIds([1, 2]);
    });

    await act(async () => {
      await result.current.handleBatchDeleteChannels();
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent.ids).toEqual([1, 2]);
    expect(result.current.selectedChannelIds).toEqual([]);
    expect(refresh).toHaveBeenCalled();
  });

  it('batch-status can act on an explicit row instead of the selection', async () => {
    // ChannelsView passes [ch.id] for a single-row toggle. Reading the selection
    // there either did nothing or changed unrelated channels.
    const { result, refresh } = setup();
    act(() => {
      result.current.setSelectedChannelIds([9]);
    });

    await act(async () => {
      await result.current.handleBatchStatusChannels('disabled', [1]);
    });

    const sent = JSON.parse(globalThis.fetch.mock.calls.at(-1)[1].body);
    expect(sent.ids).toEqual([1]);
    expect(sent.status).toBe('disabled');
    // The checkbox selection is untouched by a single-row action.
    expect(result.current.selectedChannelIds).toEqual([9]);
    expect(refresh).toHaveBeenCalled();
  });

  it('clears the selection after a checkbox-driven batch status', async () => {
    const { result } = setup();
    act(() => {
      result.current.setSelectedChannelIds([1, 2]);
    });

    await act(async () => {
      await result.current.handleBatchStatusChannels('active');
    });

    expect(result.current.selectedChannelIds).toEqual([]);
  });

  it('clears the tenant-scoped channel state on reset', () => {
    const { result } = setup();
    act(() => {
      result.current.setSelectedChannelIds([1]);
      result.current.openCreateChannel();
    });

    act(() => {
      result.current.reset();
    });

    expect(result.current.showChannelModal).toBe(false);
    expect(result.current.selectedChannelIds).toEqual([]);
    expect(result.current.channelLatencies).toEqual({});
    expect(result.current.newChannel.name).toBe('');
  });
});
