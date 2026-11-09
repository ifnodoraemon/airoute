import { apiFetch } from './client';

export async function apiMutation(url, method, body, fetcher = apiFetch) {
  const options = { method };
  if (body !== undefined) {
    options.headers = { 'Content-Type': 'application/json' };
    options.body = JSON.stringify(body);
  }
  const response = await fetcher(url, options);
  const data = await response.json();
  return { response, data, ok: response.ok && data.code === 0 };
}
