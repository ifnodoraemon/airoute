import { expect, it } from 'vitest';
import { applyThinkingMetadata } from './channelThinking';

it('applies explicit metadata without freezing catalog guesses', () => {
  const result = applyThinkingMetadata({}, {
    physical: { known: true, source: 'metadata', off: true, efforts: ['minimal', 'low', 'ultra'] },
    guessed: { known: true, source: 'catalog', off: false, efforts: ['high'] },
  });
  expect(result).toEqual({ physical: { thinking: { can_disable: true, efforts: ['low', 'ultra'] } } });
});

it('keeps manual per-model and wildcard declarations', () => {
  const detected = { physical: { known: true, source: 'metadata', off: true, efforts: ['ultra'] } };
  for (const config of [
    { physical: { thinking: { can_disable: false, efforts: [] } } },
    { '*': { thinking: { can_disable: false, efforts: ['high'] } } },
  ]) expect(applyThinkingMetadata(config, detected)).toEqual(config);
});
