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

  it('propagates network failures for the original caller to handle', async () => {
    const error = new Error('network');
    const fetcher = vi.fn(async () => { throw error; });
    await expect(apiMutation('/keys', 'POST', null, fetcher)).rejects.toBe(error);
  });

  it('falls back to the HTTP status on a non-JSON body instead of throwing', async () => {
    const response = { ok: false, status: 502, statusText: 'Bad Gateway', json: async () => { throw new Error('json'); } };
    const fetcher = vi.fn(async () => response);
    const result = await apiMutation('/keys', 'POST', null, fetcher);
    expect(result.ok).toBe(false);
    expect(result.data.code).toBe(502);
    expect(result.data.message).toBe('Bad Gateway');
  });

  it('treats an empty success body (204) as a successful mutation', async () => {
    const response = { ok: true, status: 204, statusText: 'No Content', json: async () => { throw new Error('json'); } };
    const fetcher = vi.fn(async () => response);
    const result = await apiMutation('/keys/1', 'DELETE', undefined, fetcher);
    expect(result.ok).toBe(true);
    expect(result.data.code).toBe(0);
  });
});
