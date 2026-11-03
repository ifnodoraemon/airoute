import { apiFetch } from './client';

export function consoleFetch(path, keyId, options = {}) {
  if (!/^\/v1(?:beta)?\//.test(path)) throw new Error('Invalid playground path');
  if (!/^[1-9]\d*$/.test(String(keyId || ''))) throw new Error('Select an API key first');
  return apiFetch(`/api/v1/playground${path}`, {
    ...options,
    headers: { ...(options.headers || {}), 'X-Airoute-Key-ID': String(keyId) },
    // An upstream 401 concerns the provider, not the console login session.
    sessionAuthErrorsOnly: true,
  });
}
