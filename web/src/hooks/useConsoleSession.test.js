import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useConsoleSession } from './useConsoleSession';
import { SESSION_MARKER } from '../api/session';

// The session state machine. These transitions decide whether a visitor is in the
// console at all, so they are worth pinning harder than the average hook: a mistake
// either locks a signed-in user out or leaves a signed-out one in.

const route = (over = {}) => ({ viewMode: 'landing', tab: 'dashboard', authTab: 'login', ...over });

function setup({ initialRoute = route(), onEntered, onEnded } = {}) {
  const utils = renderHook(() => useConsoleSession({
    initialRoute,
    onEntered: onEntered || vi.fn(),
    onEnded: onEnded || vi.fn(),
  }));
  return utils;
}

describe('useConsoleSession', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
  });

  it('starts from the resolved route', () => {
    const { result } = setup({ initialRoute: route({ viewMode: 'console', authTab: 'register' }) });

    expect(result.current.viewMode).toBe('console');
    expect(result.current.authTab).toBe('register');
    expect(result.current.adminUser).toBeNull();
  });

  it('reads a legacy stored session so an upgrade does not sign anyone out', () => {
    localStorage.setItem('airoute_admin_token', 'legacy-token');
    localStorage.setItem('airoute_admin_user', JSON.stringify({ username: 'alice', role: 'user' }));

    const { result } = setup();

    expect(result.current.adminToken).toBe('legacy-token');
    expect(result.current.adminUser).toEqual({ username: 'alice', role: 'user' });
  });

  describe('signIn', () => {
    it('enters the console and keeps the credential the endpoint returned', () => {
      const onEntered = vi.fn();
      const { result } = setup({ onEntered });

      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' }, { token: 'sk-real', tab: 'dashboard' });
      });

      expect(result.current.viewMode).toBe('console');
      expect(result.current.adminToken).toBe('sk-real');
      expect(result.current.credentialToken).toBe('sk-real');
      expect(result.current.adminUser).toEqual({ username: 'root', role: 'admin' });
      expect(result.current.authTab).toBe('dashboard');
      expect(onEntered).toHaveBeenCalledTimes(1);
    });

    it('records a marker instead when the sign-in produced no credential', () => {
      const { result } = setup();

      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' });
      });

      expect(result.current.adminToken).toBe(SESSION_MARKER);
      // The marker must never be sent as a bearer token.
      expect(result.current.credentialToken).toBe('');
    });

    it('persists the profile but never the credential', () => {
      const { result } = setup();

      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' }, { token: 'sk-real' });
      });

      expect(localStorage.getItem('airoute_user')).toContain('root');
      expect(localStorage.getItem('airoute_token')).toBeNull();
      expect(localStorage.getItem('airoute_admin_token')).toBeNull();
    });

    it('reports the admin role', () => {
      const { result } = setup();

      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' });
      });

      expect(result.current.isAdmin).toBe(true);
      expect(result.current.isAuthenticated).toBe(true);
    });
  });

  describe('restoreSession', () => {
    it('leaves the auth page for the console', () => {
      const { result } = setup({ initialRoute: route({ viewMode: 'auth' }) });

      act(() => {
        result.current.restoreSession({ username: 'root', role: 'admin' });
      });

      expect(result.current.viewMode).toBe('console');
      expect(result.current.adminToken).toBe(SESSION_MARKER);
    });

    it('does not drag a visitor off the landing page', () => {
      // A valid session makes the auth page pointless, but it does not override
      // where someone deliberately is.
      const { result } = setup({ initialRoute: route({ viewMode: 'landing' }) });

      act(() => {
        result.current.restoreSession({ username: 'root', role: 'admin' });
      });

      expect(result.current.viewMode).toBe('landing');
      expect(result.current.adminUser).toEqual({ username: 'root', role: 'admin' });
    });
  });

  describe('endSession', () => {
    it('clears the credential and leaves the console', () => {
      const onEnded = vi.fn();
      const { result } = setup({ initialRoute: route({ viewMode: 'console' }), onEnded });
      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' }, { token: 'sk-real' });
      });

      act(() => {
        result.current.endSession({ notify: true });
      });

      expect(result.current.viewMode).toBe('landing');
      expect(result.current.adminToken).toBe('');
      expect(result.current.adminUser).toBeNull();
      expect(localStorage.getItem('airoute_user')).toBeNull();
      expect(onEnded).toHaveBeenCalledWith({ notify: true });
    });

    it('does not interrupt a visitor who is not in the console', () => {
      const onEnded = vi.fn();
      const { result } = setup({ initialRoute: route({ viewMode: 'landing' }), onEnded });

      act(() => {
        result.current.endSession({ notify: true });
      });

      expect(result.current.viewMode).toBe('landing');
      // No dialog: they were never in a session to lose.
      expect(onEnded).not.toHaveBeenCalled();
    });

    it('stays silent on a rejected restore', () => {
      const onEnded = vi.fn();
      const { result } = setup({ initialRoute: route({ viewMode: 'console' }), onEnded });

      act(() => {
        result.current.endSession({ notify: false });
      });

      expect(onEnded).toHaveBeenCalledWith({ notify: false });
    });
  });

  describe('signOutLocal', () => {
    it('clears the credential and returns to the landing page', () => {
      const { result } = setup();
      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' }, { token: 'sk-real' });
      });

      act(() => {
        result.current.signOutLocal();
      });

      expect(result.current.viewMode).toBe('landing');
      expect(result.current.adminToken).toBe('');
      expect(result.current.adminUser).toBeNull();
    });
  });

  describe('applyRoute', () => {
    it('adopts the route it is given', () => {
      const { result } = setup();

      act(() => {
        result.current.applyRoute({ viewMode: 'console', tab: 'logs', authTab: 'register' });
      });

      expect(result.current.viewMode).toBe('console');
      expect(result.current.authTab).toBe('register');
    });

    it('keeps the current value when the route agrees, so no needless render', () => {
      const { result } = setup({ initialRoute: route({ viewMode: 'console' }) });

      act(() => {
        result.current.applyRoute({ viewMode: 'console', tab: 'logs', authTab: 'login' });
      });

      expect(result.current.viewMode).toBe('console');
    });
  });

  describe('profile updates', () => {
    it('does not revive an ended session or replace a different signed-in user', () => {
      const { result } = setup();
      act(() => result.current.signIn({ username: 'old-user', role: 'admin' }));
      act(() => result.current.signOutLocal());
      act(() => result.current.refreshUser({ username: 'old-user', role: 'admin' }));
      expect(result.current.adminUser).toBeNull();
      act(() => result.current.signIn({ username: 'new-user', role: 'user' }));
      act(() => result.current.refreshUser({ username: 'old-user', role: 'admin' }));
      expect(result.current.adminUser.username).toBe('new-user');
    });
    it('refreshes the profile without touching the credential', () => {
      const { result } = setup();
      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' }, { token: 'sk-real' });
      });

      act(() => {
        result.current.refreshUser({ username: 'root', role: 'admin', balance: 42 });
      });

      expect(result.current.adminUser.balance).toBe(42);
      expect(result.current.adminToken).toBe('sk-real');
    });

    it('ignores an empty refresh', () => {
      const { result } = setup();
      act(() => {
        result.current.signIn({ username: 'root', role: 'admin' });
      });

      act(() => {
        result.current.refreshUser(null);
      });

      expect(result.current.adminUser).toEqual({ username: 'root', role: 'admin' });
    });

    it('merges a partial patch and persists it', () => {
      const { result } = setup();
      act(() => {
        result.current.signIn({ username: 'root', role: 'admin', balance: 10 });
      });

      act(() => {
        result.current.patchUser({ balance: 25 });
      });

      expect(result.current.adminUser).toEqual({ username: 'root', role: 'admin', balance: 25 });
      expect(localStorage.getItem('airoute_user')).toContain('25');
    });

    it('does not merge a patch into a signed-out session', () => {
      const { result } = setup();

      act(() => {
        result.current.patchUser({ balance: 99 });
      });

      expect(result.current.adminUser).toBeNull();
    });
  });
});
