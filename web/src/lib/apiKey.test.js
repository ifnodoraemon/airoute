import { describe, expect, it } from 'vitest';
import { API_KEY_PLACEHOLDER, keyDisplayRef } from './apiKey';

describe('key presentation', () => {
  it('shows a public reference without reading any credential fields', () => {
    expect(keyDisplayRef({ key_id: 'ak_0123456789abcdef0123', id: 42, key: 'secret' })).toBe('ak_0123456789abcdef0123');
    expect(keyDisplayRef({ id: 42, key: 'secret' })).toBe('#42');
    expect(keyDisplayRef({ key: 'secret' })).toBe('—');
    expect(API_KEY_PLACEHOLDER).toBe('<YOUR_API_KEY>');
  });
});
