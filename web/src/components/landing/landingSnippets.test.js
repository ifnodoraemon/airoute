import { describe, expect, it } from 'vitest';
import { getLandingSnippets } from './landingSnippets';

describe('landing code examples', () => {
  it('localizes all comments and prompts, keeping English examples free of Chinese', () => {
    const en = getLandingSnippets('https://gateway.example', false);
    const zh = getLandingSnippets('https://gateway.example', true);
    for (const key of Object.keys(en)) {
      expect(en[key]).not.toMatch(/\p{Script=Han}/u);
      expect(zh[key]).toMatch(/\p{Script=Han}/u);
      expect(en[key]).not.toMatch(/gpt-6|claude-opus-5\.5|gpt-live-transcribe/);
    }
    expect(en.python).toContain('model="gpt-4.1"');
    expect(en.claude).toContain('model="claude-sonnet-5-5"');
    expect(en.multimodal).toContain('https://gateway.example/v1/images/generations');
  });
});
