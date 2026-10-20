import { describe, it, expect, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { useRoutingTopology } from './useRoutingTopology';

const response = model => ({ ok: true, json: async () => ({ code: 0, data: { routing: { model, models: ['a', 'b'], steps: [] } } }) });

describe('useRoutingTopology lifecycle', () => {
  it('discards the previous model snapshot after selection changes', async () => {
    const release = [];
    const fetcher = vi.fn(() => new Promise(resolve => release.push(resolve)));
    const { result } = renderHook(() => useRoutingTopology(true, fetcher));
    act(() => result.current.selectModel('b'));
    await act(async () => { release[1](response('b')); });
    await act(async () => { release[0](response('a')); });
    expect(result.current.snapshot.routing.model).toBe('b');
    expect(fetcher.mock.calls[0][1].signal.aborted).toBe(true);
  });

  it('cancels both loading and manual validation when closed', async () => {
    let finishProbe;
    const fetcher = vi.fn((url, options) => options?.method === 'POST'
      ? new Promise(resolve => { finishProbe = resolve; }) : Promise.resolve(response('a')));
    const { result, rerender } = renderHook(({ open }) => useRoutingTopology(open, fetcher), { initialProps: { open: true } });
    await waitFor(() => expect(result.current.snapshot?.routing.model).toBe('a'));
    let pending;
    act(() => { pending = result.current.probe('a', 1); });
    rerender({ open: false });
    await act(async () => { finishProbe({ ok: true, json: async () => ({ code: 0, data: { success: true } }) }); await pending; });
    expect(result.current.probes).toEqual({});
    expect(result.current.snapshot).toBeNull();
    expect(result.current.probing).toBe('');
    expect(fetcher.mock.calls.at(-1)[1].signal.aborted).toBe(true);
  });
});
