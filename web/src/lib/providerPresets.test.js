import { describe, it, expect } from 'vitest';
import { buildPresetChannel, emptyChannel, PROVIDER_PRESETS, ALL_PROTOCOLS } from './providerPresets';

// The preset table used to be a 198-line switch inside App.jsx. The property that
// matters is that a preset fills in the model list and protocol set for a known
// provider WITHOUT discarding the endpoint and credential the operator has already
// typed — a preset is onboarding help, not a connection reset.

describe('providerPresets', () => {
  it('offers a preset for every provider the form lists', () => {
    const expected = [
      'gpustack', 'deepseek', 'zhipu', 'doubao', 'moonshot', 'openai',
      'anthropic', 'gemini', 'ollama', 'vllm', 'sub2api', 'custom',
    ];
    expected.forEach((key) => expect(PROVIDER_PRESETS[key]).toBeTruthy());
  });

  it('starts a channel with every protocol selected', () => {
    expect(emptyChannel().protocols).toEqual(ALL_PROTOCOLS);
  });

  it('gives each call to emptyChannel its own protocol array', () => {
    // A shared array would let one form's edit leak into the next form.
    const a = emptyChannel();
    a.protocols.push('bogus');
    expect(emptyChannel().protocols).not.toContain('bogus');
  });

  it('fills the model list for a known provider', () => {
    const built = buildPresetChannel('deepseek', emptyChannel());

    expect(built.channel.models_str).toContain('deepseek-chat');
    expect(built.channel.type).toBe('deepseek');
    expect(built.channel.base_url).toBe('https://api.deepseek.com');
  });

  it('keeps the endpoint the operator already typed', () => {
    const previous = { ...emptyChannel(), base_url: 'https://my-proxy.internal/v1' };

    const built = buildPresetChannel('deepseek', previous);

    expect(built.channel.base_url).toBe('https://my-proxy.internal/v1');
  });

  it('keeps the credential the operator already typed', () => {
    const previous = { ...emptyChannel(), api_key: 'sk-typed-by-hand' };

    const built = buildPresetChannel('openai', previous);

    expect(built.channel.api_key).toBe('sk-typed-by-hand');
  });

  it('supplies a credential for a provider that needs a placeholder', () => {
    const built = buildPresetChannel('ollama', emptyChannel());

    expect(built.channel.api_key).toBe('ollama');
  });

  it('does not invent an endpoint for a self-hosted provider', () => {
    // Ollama and vLLM run wherever the operator put them.
    for (const key of ['ollama', 'vllm', 'custom', 'sub2api']) {
      const built = buildPresetChannel(key, emptyChannel());
      expect(built.channel.base_url).toBe('');
    }
  });

  it('keeps a model list the operator already typed when the preset has none', () => {
    const previous = { ...emptyChannel(), models_str: 'my-local-model' };

    const built = buildPresetChannel('gpustack', previous);

    expect(built.channel.models_str).toBe('my-local-model');
  });

  it('always clears a stale mapping string', () => {
    const previous = { ...emptyChannel(), mapping_str: 'a:b' };

    const built = buildPresetChannel('deepseek', previous);

    expect(built.channel.mapping_str).toBe('');
  });

  it('reports a message in both languages', () => {
    const built = buildPresetChannel('openai', emptyChannel());

    expect(built.messageZh).toBeTruthy();
    expect(built.messageEn).toBeTruthy();
    expect(built.messageZh).not.toBe(built.messageEn);
  });

  it('returns null for an unknown preset instead of guessing', () => {
    expect(buildPresetChannel('not-a-provider', emptyChannel())).toBeNull();
  });

  it('never invents a model for a provider whose list is empty', () => {
    // A fabricated model name would be routed to and fail at request time.
    for (const key of ['sub2api', 'custom']) {
      const built = buildPresetChannel(key, emptyChannel());
      expect(built.channel.models_str).toBe('');
    }
  });
});
