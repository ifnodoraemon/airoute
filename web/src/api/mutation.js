import { apiFetch } from './client';

export async function apiMutation(url, method, body, fetcher = apiFetch) {
  const options = { method };
  if (body !== undefined) {
    options.headers = { 'Content-Type': 'application/json' };
    options.body = JSON.stringify(body);
  }
  const response = await fetcher(url, options);
  let data;
  try {
    data = await response.json();
  } catch {
    data = null;
  }
  if (!data || typeof data !== 'object') {
    // Non-JSON body (proxy 502 HTML, 204 empty): fall back to the HTTP status so
    // callers surface a meaningful message instead of a JSON parse error.
    data = { code: response.ok ? 0 : response.status, message: response.statusText || `HTTP ${response.status}` };
  }
  return { response, data, ok: response.ok && data.code === 0 };
}
