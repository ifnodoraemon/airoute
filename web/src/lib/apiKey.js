// API-key presentation helpers shared by every console surface that can send a
// credential somewhere.
//
// The key list endpoint intentionally returns display masks (the stored value is
// a digest of the credential, so the plaintext is unrecoverable). A mask is an
// identifier, never a credential: code that builds an Authorization header, a
// request body, or a copyable snippet from `keys[i].key` is sending a value that
// can only ever fail authentication. These helpers make "is this actually a
// credential?" an explicit, testable question.

/** Placeholder shown in copyable snippets instead of a real credential. */
export const API_KEY_PLACEHOLDER = '<YOUR_API_KEY>';

/** Sentinel used by the consoles for "call without any credential". */
export const UNAUTHENTICATED_KEY = '__none__';

// Real credentials are much longer than this; the list masks are 8+3+4 chars.
const MIN_PLAINTEXT_LENGTH = 16;
const MASK_MARKER = '***';

/**
 * Whether a value can be used as a credential in an outbound call.
 *
 * Rejects empty values, display masks (any value containing `***`) and anything
 * shorter than 16 non-space characters. The last rule is what stops a truncated
 * or half-copied key from being shipped as if it were usable; it also rejects the
 * `sk-***` placeholder the backend uses for short legacy rows.
 *
 * @param {unknown} value Candidate credential.
 * @returns {boolean}
 */
export function isUsableAPIKey(value) {
  if (typeof value !== 'string') return false;
  const trimmed = value.trim();
  if (!trimmed) return false;
  if (trimmed.includes(MASK_MARKER)) return false;
  return trimmed.replace(/\s+/g, '').length >= MIN_PLAINTEXT_LENGTH;
}

/**
 * Renders the reference to a listed key for UI display: fingerprint when the API
 * provides one, otherwise a fixed opaque mask. Never returns the stored value.
 *
 * @param {{ key_fingerprint?: string }} key Listed key.
 * @returns {string}
 */
export function keyDisplayRef(key) {
  return key?.key_fingerprint || 'sk-airoute-••••••••';
}

/**
 * Builds the auth headers for a data-plane request. A missing key yields no auth
 * header (the gateway may be running in public mode); an unusable value throws so
 * masking regressions fail loudly instead of shipping `Bearer sk-ab***cd` to the
 * gateway.
 *
 * @param {string} key Plaintext credential or '' for unauthenticated calls.
 * @param {string} [headerName] Header carrying the credential.
 * @param {string} [prefix] Value prefix (e.g. 'Bearer ').
 * @returns {Record<string, string>}
 */
export function apiKeyHeaders(key, headerName = 'Authorization', prefix = 'Bearer ') {
  if (!key) return {};
  if (!isUsableAPIKey(key)) {
    throw new Error('Refusing to send a masked or truncated API key');
  }
  return { [headerName]: `${prefix}${key.trim()}` };
}
