import { describe, it, expect, vi } from 'vitest';
import { apiMutation } from './mutation';

describe('apiMutation', () => {
  it('preserves the full saved-but-not-reloaded envelope and one-time key', async () => {
    const data = { code: 0, warning: 'reload pending', data: { key: 'test-once' } };
    const response = { ok: true, status: 202, json: async () => data };
    const fetcher = vi.fn(async () => response);
    expect(await apiMutation('/keys', 'POST', { name: 'test' }, fetcher)).toEqual({ response, data, ok: true });
    expect(fetcher).toHaveBeenCalledWith('/keys', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{"name":"test"}',
    });
  });

  it.each([{ ok: false, code: 0 }, { ok: true, code: 7 }, { ok: true }, { ok: true, code: '0' }])('requires HTTP success and numeric code zero: %j', async ({ ok, code }) => {
    const data = { code, message: 'business result' };
    const response = { ok, json: async () => data };
    const fetcher = vi.fn(async () => response);
    expect(await apiMutation('/keys/1', 'DELETE', undefined, fetcher)).toEqual({ response, data, ok: false });
    expect(fetcher).toHaveBeenCalledWith('/keys/1', { method: 'DELETE' });
  });

  it.each(['network', 'json'])('propagates %s failures for the original caller to handle', async (kind) => {
    const error = new Error(kind);
    const fetcher = kind === 'network'
      ? vi.fn(async () => { throw error; })
      : vi.fn(async () => ({ json: async () => { throw error; } }));
    await expect(apiMutation('/keys', 'POST', null, fetcher)).rejects.toBe(error);
  });
});
