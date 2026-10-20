import { describe, it, expect, beforeEach } from 'vitest';
import {
  SESSION_MARKER,
  getStoredToken,
  getStoredUser,
  setStoredAuth,
  clearStoredAuth,
  readCSRFCookie,
} from './session';

describe('session storage', () => {
  beforeEach(() => {
    localStorage.clear();
    document.cookie = 'airoute_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/';
  });

  it('still reads a token left by the pre-cookie implementation', () => {
    localStorage.setItem('airoute_admin_token', 'legacy-token');

    expect(getStoredToken()).toBe('legacy-token');
  });

  it('returns an empty token when nothing is stored', () => {
    expect(getStoredToken()).toBe('');
  });

  it('stores only the profile and drops the legacy credential keys', () => {
    localStorage.setItem('airoute_admin_token', 'legacy-token');

    setStoredAuth(SESSION_MARKER, { username: 'admin', role: 'admin' });

    expect(localStorage.getItem('airoute_admin_token')).toBeNull();
    expect(localStorage.getItem('airoute_token')).toBeNull();
    expect(getStoredUser()).toEqual({ username: 'admin', role: 'admin' });
  });

  it('accepts a pre-serialized profile', () => {
    setStoredAuth(SESSION_MARKER, '{"username":"bob"}');

    expect(getStoredUser()).toEqual({ username: 'bob' });
  });

  it('tolerates a corrupt stored profile', () => {
    localStorage.setItem('airoute_user', '{not json');

    expect(getStoredUser()).toBeNull();
  });

  it('returns null when no profile is stored', () => {
    expect(getStoredUser()).toBeNull();
  });

  it('clears every session key', () => {
    localStorage.setItem('airoute_token', 't');
    localStorage.setItem('airoute_admin_token', 't');
    localStorage.setItem('airoute_user', '{}');
    localStorage.setItem('airoute_admin_user', '{}');

    clearStoredAuth();

    for (const key of ['airoute_token', 'airoute_admin_token', 'airoute_user', 'airoute_admin_user']) {
      expect(localStorage.getItem(key)).toBeNull();
    }
  });

  it('reads the CSRF cookie', () => {
    document.cookie = 'airoute_csrf=token%2Bwith%2Bplus';

    expect(readCSRFCookie()).toBe('token+with+plus');
  });

  it('returns an empty CSRF token when the cookie is absent', () => {
    expect(readCSRFCookie()).toBe('');
  });

  it('does not confuse a differently named cookie for the CSRF token', () => {
    document.cookie = 'not_airoute_csrf=nope';

    expect(readCSRFCookie()).toBe('');
  });
});
