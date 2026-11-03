// Listed keys expose non-secret identifiers. Console calls authenticate through
// the login session; external SDK examples always use an explicit placeholder.
export const API_KEY_PLACEHOLDER = '<YOUR_API_KEY>';

export function keyDisplayRef(key) {
  if (key?.key_id) return key.key_id;
  if (key?.id != null && key.id !== '') return `#${key.id}`;
  return '—';
}
