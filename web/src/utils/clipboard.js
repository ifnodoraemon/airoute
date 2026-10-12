/**
 * Cross-platform clipboard copy helper with fallback for non-secure contexts.
 *
 * Modern navigator.clipboard.writeText is only available in Secure Contexts (HTTPS / localhost).
 * When accessed via LAN IP over plain HTTP (e.g. http://192.168.1.100:8080), window.isSecureContext
 * is false and navigator.clipboard is undefined.
 * This helper falls back gracefully to a temporary readonly textarea with document.execCommand('copy').
 *
 * @param {string} text - The text string to copy.
 * @returns {Promise<boolean>} - Resolves to true if copy succeeded, false otherwise.
 */
export async function copyToClipboard(text) {
  if (text == null) return false;
  const content = String(text);

  if (typeof window !== 'undefined' && window.isSecureContext && navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(content);
      return true;
    } catch {
      // Fall through to textarea execCommand fallback
    }
  }

  if (typeof document === 'undefined') return false;

  try {
    const textArea = document.createElement('textarea');
    textArea.value = content;
    textArea.setAttribute('readonly', '');
    textArea.style.position = 'fixed';
    textArea.style.top = '-9999px';
    textArea.style.left = '-9999px';
    textArea.style.opacity = '0';
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();
    const success = document.execCommand('copy');
    document.body.removeChild(textArea);
    return !!success;
  } catch (err) {
    console.warn('Clipboard copy fallback failed:', err);
    return false;
  }
}
