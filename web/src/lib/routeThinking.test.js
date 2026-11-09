import { expect, it } from 'vitest';
import { routeThinkingAvailability } from './routeThinking';

it('reads the actual model after a mapping change and prefers current server facts', () => {
  const before = { known: true, on: true, off: true, efforts: ['low', 'high', 'xhigh', 'max'] };
  const after = { known: true, on: true, off: true, efforts: ['high'] };
  const channels = [{ id: 1, status: 'active', thinking_capabilities: { before, after } }];
  const original = routeThinkingAvailability(
    [{ channel_id: 1, mapped_model: 'before', thinking: before }],
    channels,
    'public'
  );
  expect(original.efforts).toEqual(['low', 'high', 'xhigh', 'max']);
  const changed = routeThinkingAvailability(
    [{ channel_id: 1, mapped_model: 'after', thinking: before }],
    channels,
    'public'
  );
  expect(changed.efforts).toEqual(['high']);
});

it('does not advertise minimal returned by an upstream', () => {
  const result = routeThinkingAvailability(
    [{ channel_id: 1, mapped_model: 'actual', thinking: { known: true, on: true, efforts: ['minimal', 'low', 'ultra'] } }],
    [{ id: 1, status: 'active' }],
    'public'
  );
  expect(result.efforts).toEqual(['low', 'ultra']);
});
