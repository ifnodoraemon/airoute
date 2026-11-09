import { it, expect, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useChannelActions } from './useChannelActions';

it('opens the shared create action with its GPUStack defaults after an edit', () => {
  const { result } = renderHook(() => useChannelActions({ showToast: vi.fn(), refresh: vi.fn(), isZh: true }));
  act(() => result.current.handleEditChannel({ id: 17, type: 'openai', name: 'edited', models: ['old'], protocols: ['openai_chat'] }));
  expect(result.current.editingChannelId).toBe(17);
  act(() => result.current.openCreateChannel());
  expect(result.current.showChannelModal).toBe(true);
  expect(result.current.editingChannelId).toBeNull();
  expect(result.current.probeAlert).toBeNull();
  expect(result.current.newChannel).toEqual({
    name: '', type: 'gpustack', base_url: '', api_key: '', priority: 1, weight: 10,
    timeout_seconds: 60, max_concurrency: 0, queue_limit: 0, queue_wait_seconds: 0,
    models_str: '', mapping_rules: [],
    protocols: ['openai_chat', 'openai_response', 'embeddings', 'rerank', 'images'],
  });
});
