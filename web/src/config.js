// Centralized runtime configuration - adhering strictly to "万物皆可配置" principle.
// Zero hardcoded host, ports, or protocols.

let _publicGatewayUrl = '';

/**
 * Update the public gateway URL dynamically (e.g. from /api/v1/public/status or server settings).
 */
export function setPublicGatewayUrl(url) {
  if (url && typeof url === 'string') {
    _publicGatewayUrl = url.trim().replace(/\/+$/, '');
  }
}

/**
 * Dynamically resolves the current gateway origin without hardcoding any host or port.
 * Priority:
 * 1. Backend configured public_url (from PUBLIC_URL / GATEWAY_PUBLIC_URL or reverse proxy headers)
 * 2. Active browser window.location.origin (automatically matches the exact host, domain, and port user accessed)
 * 3. Empty string fallback (allowing relative paths like /v1)
 */
export function getGatewayOrigin() {
  if (_publicGatewayUrl) {
    return _publicGatewayUrl;
  }
  if (typeof window !== 'undefined' && window.location && window.location.origin && window.location.origin !== 'null') {
    return window.location.origin;
  }
  return '';
}

/**
 * Dynamically resolves the API base URL (e.g. "${origin}/v1" or "/v1").
 */
export function getGatewayBaseUrl() {
  const origin = getGatewayOrigin();
  return origin ? `${origin}/v1` : '/v1';
}

/**
 * Resolves any relative or partial endpoint to the dynamically determined gateway origin.
 */
export function resolveGatewayUrl(path) {
  if (!path) return getGatewayOrigin();
  if (path.startsWith('http://') || path.startsWith('https://')) return path;
  const origin = getGatewayOrigin();
  const cleanPath = path.startsWith('/') ? path : `/${path}`;
  return origin ? `${origin}${cleanPath}` : cleanPath;
}
