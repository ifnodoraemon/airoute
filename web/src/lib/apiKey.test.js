import { describe, it, expect } from 'vitest';
import {
  API_KEY_PLACEHOLDER,
  apiKeyHeaders,
  credentialProblem,
  credentialProblemHint,
  credentialProblemLabel,
  isUsableAPIKey,
  keyDisplayRef,
} from './apiKey';

// The key list returns display metadata because credentials are stored as
// digests and cannot be recovered. These helpers decide what may be sent as a
// credential, so the guard itself is pinned: a regression here silently sends a
// key ID to the gateway, which surfaces as "invalid API key" instead of a code
// review comment.

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

  it('rejects every display-mask shape an audit row can carry', () => {
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
  it('prefers the public key id and never renders a credential', () => {
    expect(keyDisplayRef({ key_id: 'ak_0123456789abcdef0123', id: 42 })).toBe('ak_0123456789abcdef0123');
    expect(keyDisplayRef({ id: 42 })).toBe('#42');
    expect(keyDisplayRef({})).toBeTruthy();
  });
});

describe('API_KEY_PLACEHOLDER', () => {
  it('is obviously not a credential', () => {
    expect(isUsableAPIKey(API_KEY_PLACEHOLDER)).toBe(false);
  });
});

// The key list shows a key id (`ak_…`) next to a copy button. It is long enough
// to pass the length-only guard, so the request would reach the gateway, which
// answered "Incorrect or unauthorized API key". These cases pin the
// classification that now stops that round trip.
describe('credentialProblem', () => {
  it('classifies the key id the key list lets operators copy', () => {
    expect(credentialProblem('ak_0f0013dea24034ac1234')).toBe('key-id');
    expect(isUsableAPIKey('ak_0f0013dea24034ac1234')).toBe(false);
  });

  it('classifies every other value that cannot authenticate', () => {
    expect(credentialProblem('')).toBe('missing');
    expect(credentialProblem('   ')).toBe('missing');
    expect(credentialProblem(undefined)).toBe('missing');
    expect(credentialProblem('3a4b5c6d***9f8e')).toBe('mask');
    expect(credentialProblem(API_KEY_PLACEHOLDER)).toBe('placeholder');
    expect(credentialProblem('Bearer sk-airoute-0123456789abcdef')).toBe('wrapped');
    expect(credentialProblem('sk-airoute-0123456789abcdef\nnext-line')).toBe('wrapped');
    // A stored digest / whole config block is not a credential either.
    expect(credentialProblem('a1'.repeat(32))).toBe('digest');
    expect(credentialProblem('"sk-airoute-0123456789abcdef"')).toBe('invalid-characters');
    expect(credentialProblem('sk-short')).toBe('too-short');
    expect(credentialProblem(`sk-${'a'.repeat(400)}`)).toBe('too-long');
  });

  it('accepts the credential shapes the gateway issues', () => {
    expect(credentialProblem('sk-airoute-0123456789abcdef0123456789abcdef')).toBeNull();
    // Legacy/seeded keys may contain characters the generator never emits.
    expect(credentialProblem('sk-legacy.key:with/slashes+and=padding')).toBeNull();
  });

  it('explains a key id in the operator language', () => {
    expect(credentialProblemHint('key-id', true)).toContain('密钥 ID');
    expect(credentialProblemHint('key-id', false)).toContain('key ID');
    expect(credentialProblemHint(null, false)).toContain('plaintext');
  });

  it('labels the chip state without ever calling a key id ready', () => {
    expect(credentialProblemLabel(null, false)).toBe('ready');
    expect(credentialProblemLabel('missing', false)).toBe('missing');
    expect(credentialProblemLabel('key-id', false)).toBe('key id');
    expect(credentialProblemLabel('mask', false)).not.toBe('ready');
  });
});
