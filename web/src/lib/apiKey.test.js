import { describe, it, expect } from 'vitest';
import {
  API_KEY_PLACEHOLDER,
  apiKeyHeaders,
  isUsableAPIKey,
  keyDisplayRef,
} from './apiKey';

// The key list returns display masks because credentials are stored as digests
// and cannot be recovered. These helpers decide what may be sent as a credential,
// so the guard itself is pinned: a regression here silently sends masks to the
// gateway, which surfaces as "invalid API key" instead of a code review comment.

describe('isUsableAPIKey', () => {
  it('accepts a plaintext credential', () => {
    expect(isUsableAPIKey('sk-airoute-0123456789abcdef')).toBe(true);
  });

  it('trims surrounding whitespace before judging', () => {
    expect(isUsableAPIKey('  sk-airoute-0123456789abcdef  ')).toBe(true);
  });

  it('rejects empty and whitespace-only values', () => {
    expect(isUsableAPIKey('')).toBe(false);
    expect(isUsableAPIKey('   ')).toBe(false);
    expect(isUsableAPIKey(undefined)).toBe(false);
    expect(isUsableAPIKey(null)).toBe(false);
    expect(isUsableAPIKey(42)).toBe(false);
  });

  it('rejects every display mask the list endpoint can produce', () => {
    // Digest-derived mask, sk- mask, and the short-key placeholder.
    expect(isUsableAPIKey('3a4b5c6d***9f8e')).toBe(false);
    expect(isUsableAPIKey('sk-12ab***cd34')).toBe(false);
    expect(isUsableAPIKey('sk-***')).toBe(false);
  });

  it('rejects values shorter than 16 non-space characters', () => {
    expect(isUsableAPIKey('sk-short-key')).toBe(false);
    // 15 characters is still too short; 16 is the threshold.
    expect(isUsableAPIKey('a'.repeat(15))).toBe(false);
    expect(isUsableAPIKey('a'.repeat(16))).toBe(true);
  });

  it('counts non-space characters, so padding cannot inflate a short key', () => {
    // 15 real characters plus arbitrary padding is still too short.
    expect(isUsableAPIKey(`${'a'.repeat(15)}${' '.repeat(10)}`)).toBe(false);
    // 'a '.repeat(8) is 16 characters but only 8 real ones.
    expect(isUsableAPIKey('a '.repeat(8))).toBe(false);
  });
});

describe('apiKeyHeaders', () => {
  it('builds a bearer header from a usable key', () => {
    expect(apiKeyHeaders(' sk-airoute-0123456789abcdef ')).toEqual({
      Authorization: 'Bearer sk-airoute-0123456789abcdef',
    });
  });

  it('omits the header for unauthenticated calls', () => {
    expect(apiKeyHeaders('')).toEqual({});
  });

  it('refuses to build headers from a display mask', () => {
    expect(() => apiKeyHeaders('sk-12ab***cd34')).toThrow();
  });

  it('supports non-bearer protocols', () => {
    expect(apiKeyHeaders('sk-airoute-0123456789abcdef', 'x-api-key', '')).toEqual({
      'x-api-key': 'sk-airoute-0123456789abcdef',
    });
  });
});

describe('keyDisplayRef', () => {
  it('prefers the fingerprint and never the stored value', () => {
    expect(keyDisplayRef({ key: 'sk-12ab***cd34', key_fingerprint: 'kfp_abc' })).toBe('kfp_abc');
    expect(keyDisplayRef({ key: 'sk-12ab***cd34' })).not.toContain('cd34');
    expect(keyDisplayRef({})).toBeTruthy();
  });
});

describe('API_KEY_PLACEHOLDER', () => {
  it('is obviously not a credential', () => {
    expect(isUsableAPIKey(API_KEY_PLACEHOLDER)).toBe(false);
  });
});
