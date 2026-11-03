import { describe, it, expect } from 'vitest';
import { assistantContent, textContent } from './assistantContent';

describe('assistant content presentation', () => {
  it('separates Gemma 4 thought channels from the final answer', () => {
    expect(assistantContent('<|channel>thought\nCheck the arithmetic.\n<channel|>\nThe answer is **4**.'))
      .toEqual({ content: 'The answer is **4**.', reasoning: 'Check the arithmetic.' });
  });

  it('handles empty Gemma thought channels when thinking is disabled', () => {
    expect(assistantContent('<|channel>thought\n<channel|>Hello')).toEqual({ content: 'Hello', reasoning: '' });
  });

  it('does not display partial Gemma control tokens during streaming', () => {
    const wire = '<|channel>thought\nreason<channel|>answer';
    for (let i = 1; i <= wire.length; i += 1) {
      const parsed = assistantContent(wire.slice(0, i));
      expect(parsed.content).not.toContain('<|channel>');
      expect(parsed.reasoning).not.toContain('<channel');
    }
    expect(assistantContent(wire)).toEqual({ content: 'answer', reasoning: 'reason' });
  });

  it('supports native chat think tags without changing the wire protocol', () => {
    expect(assistantContent('<think>reason</think>answer')).toEqual({ content: 'answer', reasoning: 'reason' });
  });

  it('leaves code examples and ordinary text intact', () => {
    const content = 'Use the token `<|channel>thought` in a template.';
    expect(assistantContent(content)).toEqual({ content, reasoning: '' });
    expect(assistantContent('<', '', true)).toEqual({ content: '<', reasoning: '' });
  });

  it('reads structured text parts without rendering object coercions', () => {
    expect(textContent([{ type: 'text', text: 'hello' }, { type: 'image_url', image_url: 'x' }, { type: 'text', text: ' world' }])).toBe('hello world');
  });
});
