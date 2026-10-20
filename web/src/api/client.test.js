import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiFetch, apiJSON, configureApiClient, resetApiClient, extractErrorMessage } from './client';
import { SESSION_MARKER } from './session';

function jsonResponse(body, { status = 200, ok } = {}) {
  return {
    ok: ok === undefined ? status >= 200 && status < 300 : ok,
    status,
    json: async () => body,
  };
}

// The options object handed to the most recent fetch() call.
function lastFetchOptions() {
  const calls = globalThis.fetch.mock.calls;
  return calls[calls.length - 1][1];
}

function clearCookies() {
  document.cookie = 'airoute_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/';
}

describe('apiFetch', () => {
  beforeEach(() => {
    resetApiClient();
    clearCookies();
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ code: 0, data: {} })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends the configured token as a bearer token', async () => {
    configureApiClient({ token: () => 'real-token' });

    await apiFetch('/api/v1/admin/keys');

    expect(lastFetchOptions().headers.Authorization).toBe('Bearer real-token');
  });

  it('never sends the session marker as a bearer token', async () => {
    // The marker only means "logged in"; the credential is the HttpOnly cookie.
    // Sending the marker would make the server reject the request before it ever
    // consults that cookie.
    configureApiClient({ token: () => SESSION_MARKER });

    await apiFetch('/api/v1/admin/keys');

    expect(lastFetchOptions().headers.Authorization).toBeUndefined();
  });

  it('sends no Authorization header when no token is configured', async () => {
    await apiFetch('/api/v1/admin/keys');

    expect(lastFetchOptions().headers.Authorization).toBeUndefined();
  });

  it('lets a per-call token override the configured token', async () => {
    configureApiClient({ token: () => 'configured' });

    await apiFetch('/api/v1/admin/keys', { token: 'explicit' });

    expect(lastFetchOptions().headers.Authorization).toBe('Bearer explicit');
  });

  it('reads the current token at call time rather than capturing it', async () => {
    let token = 'first';
    configureApiClient({ token: () => token });

    await apiFetch('/api/v1/admin/keys');
    token = 'second';
    await apiFetch('/api/v1/admin/keys');

    const calls = globalThis.fetch.mock.calls;
    expect(calls[0][1].headers.Authorization).toBe('Bearer first');
    expect(calls[1][1].headers.Authorization).toBe('Bearer second');
  });

  it('forwards the CSRF cookie as a header', async () => {
    document.cookie = 'airoute_csrf=abc123';

    await apiFetch('/api/v1/admin/keys');

    expect(lastFetchOptions().headers['X-CSRF-Token']).toBe('abc123');
  });

  it('omits the CSRF header when the cookie is absent', async () => {
    await apiFetch('/api/v1/admin/keys');

    expect(lastFetchOptions().headers['X-CSRF-Token']).toBeUndefined();
  });

  it('always sends same-origin credentials', async () => {
    await apiFetch('/api/v1/admin/keys', { credentials: 'omit' });

    expect(lastFetchOptions().credentials).toBe('same-origin');
  });

  it('preserves caller-supplied headers', async () => {
    await apiFetch('/api/v1/admin/keys', { headers: { 'Content-Type': 'application/json' } });

    expect(lastFetchOptions().headers['Content-Type']).toBe('application/json');
  });

  it('reports a 401 to the unauthorized handler with the response and token', async () => {
    const unauthorized = vi.fn();
    configureApiClient({ token: () => 'tok', unauthorized });
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 401 }, { status: 401 }));

    await apiFetch('/api/v1/admin/keys');

    expect(unauthorized).toHaveBeenCalledTimes(1);
    const ctx = unauthorized.mock.calls[0][0];
    expect(ctx.token).toBe('tok');
    expect(ctx.response.status).toBe(401);
  });

  it('does not report non-401 responses', async () => {
    const unauthorized = vi.fn();
    configureApiClient({ unauthorized });
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 0 }, { status: 200 }));

    await apiFetch('/api/v1/admin/keys');

    expect(unauthorized).not.toHaveBeenCalled();
  });

  it('still returns the response after reporting a 401', async () => {
    // Callers that inspect status themselves must keep working.
    configureApiClient({ unauthorized: vi.fn() });
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 401 }, { status: 401 }));

    const res = await apiFetch('/api/v1/admin/keys');

    expect(res.status).toBe(401);
  });
});

describe('apiJSON', () => {
  beforeEach(() => {
    resetApiClient();
    clearCookies();
    localStorage.clear();
    vi.stubGlobal('fetch', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('returns the parsed envelope on success', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 0, data: { id: 7 } }));

    await expect(apiJSON('/api/v1/admin/keys')).resolves.toEqual({ code: 0, data: { id: 7 } });
  });

  it('throws the server message when the status is not ok', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ error: 'boom' }, { status: 500 }));

    await expect(apiJSON('/api/v1/admin/keys')).rejects.toThrow('boom');
  });

  it('throws when the envelope code is non-zero', async () => {
    // A 200 response carrying a failure code must not be reported as success.
    globalThis.fetch.mockResolvedValue(jsonResponse({ code: 1001, message: 'quota exceeded' }));

    await expect(apiJSON('/api/v1/admin/keys')).rejects.toThrow('quota exceeded');
  });

  it('throws an HTTP status error when the body is not JSON', async () => {
    globalThis.fetch.mockResolvedValue({
      ok: false,
      status: 502,
      json: async () => {
        throw new Error('not json');
      },
    });

    await expect(apiJSON('/api/v1/admin/keys')).rejects.toThrow('HTTP 502');
  });

  it('treats a 401 as an error after notifying the handler', async () => {
    const unauthorized = vi.fn();
    configureApiClient({ unauthorized });
    globalThis.fetch.mockResolvedValue(jsonResponse({ error: 'unauthorized' }, { status: 401 }));

    await expect(apiJSON('/api/v1/admin/keys')).rejects.toThrow('unauthorized');
    expect(unauthorized).toHaveBeenCalledTimes(1);
  });

  it('surfaces the message from the data plane error shape', async () => {
    // Auth and rate-limit middleware answer with the OpenAI-style object. Reading
    // data.error as a string rendered "[object Object]" and hid the explanation.
    globalThis.fetch.mockResolvedValue(jsonResponse({
      error: { message: 'Incorrect or unauthorized API key provided.', type: 'authentication_error', code: 'invalid_api_key' },
    }, { status: 401 }));

    await expect(apiJSON('/v1/models')).rejects.toThrow('Incorrect or unauthorized API key provided.');
  });

  it('surfaces the data plane message for a non-zero envelope code too', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({
      code: 429,
      error: { message: 'Rate limit exceeded', type: 'rate_limit_error', code: 'rate_limited' },
    }));

    await expect(apiJSON('/v1/models')).rejects.toThrow('Rate limit exceeded');
  });

  it('falls back to the status when the body carries no usable message', async () => {
    globalThis.fetch.mockResolvedValue(jsonResponse({ error: { type: 'x' } }, { status: 502 }));

    await expect(apiJSON('/v1/models')).rejects.toThrow('HTTP 502');
  });
});

describe('extractErrorMessage', () => {
  it('reads the admin string shape', () => {
    expect(extractErrorMessage({ error: '保存失败' }, 'fallback')).toBe('保存失败');
  });

  it('reads the data plane object shape', () => {
    expect(extractErrorMessage({ error: { message: 'rate limited', type: 't', code: 'c' } }, 'fallback'))
      .toBe('rate limited');
  });

  it('reads a top-level message', () => {
    expect(extractErrorMessage({ message: 'plain' }, 'fallback')).toBe('plain');
  });

  it('never renders an object', () => {
    // The regression this guards: an object without a usable message used to be
    // stringified into "[object Object]".
    const got = extractErrorMessage({ error: { type: 'authentication_error' } }, 'HTTP 401');
    expect(got).toBe('HTTP 401');
    expect(got).not.toContain('[object');
  });

  it('falls back for a null body, an empty object and blank strings', () => {
    expect(extractErrorMessage(null, 'HTTP 500')).toBe('HTTP 500');
    expect(extractErrorMessage({}, 'HTTP 500')).toBe('HTTP 500');
    expect(extractErrorMessage({ error: '' }, 'HTTP 500')).toBe('HTTP 500');
    expect(extractErrorMessage({ error: { message: '' } }, 'HTTP 500')).toBe('HTTP 500');
  });
});
