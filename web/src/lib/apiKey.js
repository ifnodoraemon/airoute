// API-key presentation helpers shared by every console surface that can send a
// credential somewhere.
//
// The key list endpoint intentionally returns display metadata only (the stored
// value is a digest of the credential, so the plaintext is unrecoverable). The
// `key_id` it exposes is an identifier, never a credential: code that builds an
// Authorization header, a request body, or a copyable snippet from it is sending
// a value that can only ever fail authentication. These helpers make "is this
// actually a credential?" an explicit, testable question.

/** Placeholder shown in copyable snippets instead of a real credential. */
export const API_KEY_PLACEHOLDER = '<YOUR_API_KEY>';

/** Sentinel used by the consoles for "call without any credential". */
export const UNAUTHENTICATED_KEY = '__none__';

// Real credentials are much longer than this; a listed key ID (`ak_` + 20 hex)
// is 23 characters and therefore has to be classified explicitly below.
const MIN_PLAINTEXT_LENGTH = 16;
// A credential is one token. The gateway issues 43-character keys; the upper
// bound only keeps a whole config block or shell line from being shipped as one.
const MAX_PLAINTEXT_LENGTH = 256;
const MASK_MARKER = '***';
// `ak_…` is the public identifier the key list shows (ak_ + 20 lowercase hex).
// It sits next to a copy button and is long enough to pass the length check, so
// it has to be classified explicitly: it identifies a key but can never
// authenticate a call.
const KEY_ID_PREFIX = 'ak_';
// Credentials the gateway issues and commonly imported ones use this alphabet.
// Rejecting everything else catches a key pasted together with quotes,
// backticks or other markup; it deliberately stays wide enough for base64-ish
// and legacy `sk-…` keys that were seeded outside the console.
const CREDENTIAL_ALPHABET = /^[A-Za-z0-9._~+/=:-]+$/;
// api_keys.key holds a SHA-256 digest now. No surface shows it, so a pasted
// 64-character lowercase hex value is a digest (or a digest-shaped accident),
// never a credential.
const DIGEST_PATTERN = /^[0-9a-f]{64}$/;

/**
 * Explains why a pasted value cannot authenticate a data-plane call.
 *
 * Returning a reason instead of a bare boolean lets a panel tell the operator
 * what is wrong before the request is sent: "that is the key ID, paste the
 * plaintext" is actionable, while the gateway's generic 401 is not.
 *
 * @param {unknown} value Candidate credential.
 * @returns {null | 'missing' | 'key-id' | 'mask' | 'placeholder' | 'wrapped'
 *   | 'digest' | 'invalid-characters' | 'too-short' | 'too-long'} null when the
 *   value looks like a real credential.
 */
export function credentialProblem(value) {
  if (typeof value !== 'string') return 'missing';
  const trimmed = value.trim();
  if (!trimmed) return 'missing';
  if (trimmed.startsWith(KEY_ID_PREFIX)) return 'key-id';
  if (trimmed.includes(MASK_MARKER)) return 'mask';
  if (trimmed.startsWith('<') && trimmed.endsWith('>')) return 'placeholder';
  if (/^bearer\s+/i.test(trimmed)) return 'wrapped';
  if (/\s/.test(trimmed)) return 'wrapped';
  if (DIGEST_PATTERN.test(trimmed)) return 'digest';
  if (!CREDENTIAL_ALPHABET.test(trimmed)) return 'invalid-characters';
  if (trimmed.length < MIN_PLAINTEXT_LENGTH) return 'too-short';
  if (trimmed.length > MAX_PLAINTEXT_LENGTH) return 'too-long';
  return null;
}

/**
 * Whether a value can be used as a credential in an outbound call.
 *
 * Rejects empty values, display masks (`***`), key IDs (`ak_…`), snippet
 * placeholders, values copied with surrounding text, stored digests and
 * anything outside the credential alphabet or length range. Every one of these
 * would otherwise be sent to the gateway and come back as
 * "Incorrect or unauthorized API key provided", which looks like a gateway fault.
 *
 * @param {unknown} value Candidate credential.
 * @returns {boolean}
 */
export function isUsableAPIKey(value) {
  return credentialProblem(value) === null;
}

/**
 * Human explanation for a `credentialProblem` result, used by the panels before
 * an outbound call (and in the toast the guard raises).
 *
 * @param {string | null} problem Result of {@link credentialProblem}.
 * @param {boolean} [isZh] Render Chinese copy when true.
 * @returns {string}
 */
export function credentialProblemHint(problem, isZh = false) {
  switch (problem) {
    case 'missing':
      return isZh
        ? '请先粘贴本次会话的 API Key 明文。密钥列表只显示密钥 ID，它不能用于鉴权。'
        : 'Paste the plaintext API key for this session first. The key list only shows key IDs, which cannot authenticate a call.';
    case 'key-id':
      return isZh
        ? '粘贴的是密钥 ID（ak_…）：它只用于在日志、账单和工单中识别密钥，不能用于鉴权。请改成创建密钥时显示的完整明文（sk-airoute-…）。'
        : 'That value is a key ID (ak_…): it identifies a key in logs, billing and support tickets but cannot authenticate a call. Paste the plaintext shown when the key was created (sk-airoute-…).';
    case 'mask':
      return isZh
        ? '粘贴的是掩码（含 ***），掩码不能用于鉴权。请粘贴创建密钥时显示的完整明文。'
        : 'That value is a display mask (it contains ***), which cannot authenticate. Paste the plaintext shown when the key was created.';
    case 'placeholder':
      return isZh
        ? '粘贴的是占位符，不是密钥。请粘贴创建密钥时显示的完整明文。'
        : 'That is a placeholder, not a credential. Paste the plaintext shown when the key was created.';
    case 'wrapped':
      return isZh
        ? '粘贴内容里混入了空格、换行或 “Bearer” 之类的前缀。请只粘贴密钥本身。'
        : 'The pasted value contains whitespace or a prefix such as “Bearer”. Paste only the key itself.';
    case 'digest':
      return isZh
        ? '粘贴的是密钥摘要，不是可用凭证。请粘贴创建密钥时显示的完整明文。'
        : 'That is a key digest, not a usable credential. Paste the plaintext shown when the key was created.';
    case 'invalid-characters':
      return isZh
        ? '粘贴内容含有密钥不可能的字符（如引号、反引号或中文标点）。请只粘贴密钥本身。'
        : 'The pasted value contains characters a key cannot contain (quotes, backticks, non-ASCII punctuation). Paste only the key itself.';
    case 'too-short':
      return isZh
        ? '粘贴内容太短，像是被截断了。请粘贴完整的 API Key 明文。'
        : 'The pasted value is too short, so it looks truncated. Paste the complete API key plaintext.';
    case 'too-long':
      return isZh
        ? '粘贴内容过长，不像单个密钥。请只粘贴密钥本身。'
        : 'The pasted value is too long to be a single key. Paste only the key itself.';
    default:
      return isZh
        ? '请粘贴创建密钥时显示的完整明文。'
        : 'Paste the complete plaintext shown when the key was created.';
  }
}

/**
 * Short label for a panel chip, e.g. “key id” instead of “ready”. Kept
 * beside the classifier so every panel words the same state the same way.
 *
 * @param {string | null} problem Result of {@link credentialProblem}.
 * @param {boolean} [isZh] Render Chinese copy when true.
 * @returns {string}
 */
export function credentialProblemLabel(problem, isZh = false) {
  if (problem === null) return isZh ? '已就绪' : 'ready';
  switch (problem) {
    case 'missing':
      return isZh ? '待粘贴' : 'missing';
    case 'key-id':
      return isZh ? '是密钥 ID' : 'key id';
    case 'mask':
      return isZh ? '是掩码' : 'mask';
    case 'placeholder':
      return isZh ? '是占位符' : 'placeholder';
    case 'wrapped':
      return isZh ? '含多余字符' : 'wrapped';
    case 'digest':
      return isZh ? '是摘要' : 'digest';
    case 'too-short':
      return isZh ? '过短' : 'too short';
    case 'too-long':
      return isZh ? '过长' : 'too long';
    default:
      return isZh ? '格式异常' : 'invalid';
  }
}

/**
 * Renders the public identifier of a listed key: its `key_id` when the API
 * provides one, otherwise its row number as `#<id>`. Never returns a
 * credential — the list does not carry one.
 *
 * @param {{ key_id?: string, id?: number | string }} key Listed key.
 * @returns {string}
 */
export function keyDisplayRef(key) {
  if (key?.key_id) return key.key_id;
  if (key?.id != null && key.id !== '') return `#${key.id}`;
  return '—';
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
