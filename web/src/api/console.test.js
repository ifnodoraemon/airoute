import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { consoleFetch } from './console';
import { configureApiClient, resetApiClient } from './client';
import { SESSION_MARKER } from './session';

beforeEach(() => { resetApiClient(); document.cookie = 'airoute_csrf=test-csrf; path=/'; });
afterEach(() => { resetApiClient(); vi.unstubAllGlobals(); document.cookie = 'airoute_csrf=; max-age=0; path=/'; });

describe('console requests', () => {
  it('uses an HttpOnly session, CSRF protection and only a non-secret key identifier', async () => {
    configureApiClient({ token: () => SESSION_MARKER });
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })));
    const body = new FormData(); body.append('file', new Blob(['audio']), 'test.wav');
    await consoleFetch('/v1/audio/transcriptions', '42', { method: 'POST', body });
    expect(fetch).toHaveBeenCalledWith('/api/v1/playground/v1/audio/transcriptions', expect.objectContaining({
      method: 'POST', body, credentials: 'same-origin',
      headers: { 'X-Airoute-Key-ID': '42', 'X-CSRF-Token': 'test-csrf' },
    }));
    expect(fetch.mock.calls[0][1]).not.toHaveProperty('sessionAuthErrorsOnly');
  });

  it.each(['', '__none__', '__custom__', 'ak_1234', 'sk-secret', '-1', '1.5'])('refuses invalid selection %s before sending a request', id => {
    vi.stubGlobal('fetch', vi.fn());
    expect(() => consoleFetch('/v1/chat/completions', id)).toThrow('Select an API key');
    expect(fetch).not.toHaveBeenCalled();
  });

  it('keeps login credentials away from arbitrary URLs', () => {
    vi.stubGlobal('fetch', vi.fn());
    expect(() => consoleFetch('https://example.com/v1/chat/completions', '42')).toThrow('Invalid playground path');
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each([
    [{ code: 401, error: 'Session expired' }, true],
    [{ error: { message: 'Provider rejected its credential', code: 'invalid_api_key' } }, false],
  ])('distinguishes session expiry from an upstream 401 without consuming the response', async (body, expires) => {
    const unauthorized = vi.fn(); configureApiClient({ unauthorized });
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(body), { status: 401 })));
    const response = await consoleFetch('/v1/chat/completions', '42', { method: 'POST' });
    expect(unauthorized).toHaveBeenCalledTimes(expires ? 1 : 0);
    expect(await response.json()).toEqual(body);
  });
});
