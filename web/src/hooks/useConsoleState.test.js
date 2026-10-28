import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useToast } from './useToast';
import { useUiPreferences } from './useUiPreferences';
import { useConsoleOverlays } from './useConsoleOverlays';
import { useConsoleSelections } from './useConsoleSelections';
import { useConsoleChrome } from './useConsoleChrome';

const translations = { zh: { hello: '你好' }, en: { hello: 'hello' } };

describe('useToast', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows a message and hides it after the timeout', () => {
    const { result } = renderHook(() => useToast());

    act(() => {
      result.current.showToast('saved', 'success');
    });
    expect(result.current.toast).toEqual({ show: true, message: 'saved', type: 'success' });

    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(result.current.toast.show).toBe(false);
  });

  it('defaults to the info type', () => {
    const { result } = renderHook(() => useToast());

    act(() => {
      result.current.showToast('note');
    });

    expect(result.current.toast.type).toBe('info');
  });

  it('restarts the timer when a second message arrives', () => {
    // Otherwise the first message's timer would hide the second one early.
    const { result } = renderHook(() => useToast());

    act(() => {
      result.current.showToast('first');
    });
    act(() => {
      vi.advanceTimersByTime(3000);
    });
    act(() => {
      result.current.showToast('second');
    });
    act(() => {
      vi.advanceTimersByTime(1000);
    });

    expect(result.current.toast.show).toBe(true);
    expect(result.current.toast.message).toBe('second');
  });

  it('dismisses immediately', () => {
    const { result } = renderHook(() => useToast());
    act(() => {
      result.current.showToast('bye');
    });

    act(() => {
      result.current.dismissToast();
    });

    expect(result.current.toast.show).toBe(false);
  });

  it('keeps showToast stable across renders', () => {
    // The request layer registers it in an effect; a new identity each render would
    // re-register the API client every render.
    const { result, rerender } = renderHook(() => useToast());
    const first = result.current.showToast;

    rerender();

    expect(result.current.showToast).toBe(first);
  });
});

describe('useUiPreferences', () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.className = '';
  });

  afterEach(() => {
    localStorage.clear();
    document.documentElement.className = '';
  });

  it('defaults to Chinese and light', () => {
    const { result } = renderHook(() => useUiPreferences({ translations }));

    expect(result.current.lang).toBe('zh');
    expect(result.current.isZh).toBe(true);
    expect(result.current.theme).toBe('light');
    expect(result.current.t).toEqual(translations.zh);
  });

  it('reads a stored preference', () => {
    localStorage.setItem('airoute_lang', 'en');
    localStorage.setItem('airoute_theme', 'dark');

    const { result } = renderHook(() => useUiPreferences({ translations }));

    expect(result.current.lang).toBe('en');
    expect(result.current.theme).toBe('dark');
  });

  it('toggles the language and persists it', () => {
    const { result } = renderHook(() => useUiPreferences({ translations }));

    act(() => {
      result.current.toggleLang();
    });

    expect(result.current.lang).toBe('en');
    expect(result.current.t).toEqual(translations.en);
    expect(localStorage.getItem('airoute_lang')).toBe('en');
  });

  it('toggles back to Chinese', () => {
    localStorage.setItem('airoute_lang', 'en');
    const { result } = renderHook(() => useUiPreferences({ translations }));

    act(() => {
      result.current.toggleLang();
    });

    expect(result.current.lang).toBe('zh');
  });

  it('applies the dark class to the document', () => {
    localStorage.setItem('airoute_theme', 'dark');

    renderHook(() => useUiPreferences({ translations }));

    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.classList.contains('light')).toBe(false);
  });

  it('removes the dark class when switching to light', () => {
    localStorage.setItem('airoute_theme', 'dark');
    const { result } = renderHook(() => useUiPreferences({ translations }));

    act(() => {
      result.current.toggleTheme();
    });

    expect(result.current.theme).toBe('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    expect(localStorage.getItem('airoute_theme')).toBe('light');
  });

  it('localises the document title', () => {
    const { result } = renderHook(() => useUiPreferences({ translations }));
    expect(document.title).toContain('AI路由器');

    act(() => {
      result.current.toggleLang();
    });

    expect(document.title).toBe('Airoute');
  });

  it('falls back to Chinese for an unknown language', () => {
    localStorage.setItem('airoute_lang', 'klingon');

    const { result } = renderHook(() => useUiPreferences({ translations }));

    expect(result.current.t).toEqual(translations.zh);
  });
});

describe('useConsoleOverlays', () => {
  it('starts with every overlay closed', () => {
    const { result } = renderHook(() => useConsoleOverlays());

    expect(result.current.showCommandPalette).toBe(false);
    expect(result.current.showOnboardingModal).toBe(false);
    expect(result.current.showTopologyModal).toBe(false);
    expect(result.current.showAiCopilot).toBe(false);
    expect(result.current.showLoginModal).toBe(false);
    expect(result.current.showAccountModal).toBe(false);
    expect(result.current.copilotInitialQuery).toBe('');
    expect(result.current.accountModalTab).toBe('profile');
  });

  it('clears every overlay on reset so the next session starts clean', () => {
    const { result } = renderHook(() => useConsoleOverlays());
    act(() => {
      result.current.setShowCommandPalette(true);
      result.current.setShowOnboardingModal(true);
      result.current.setShowTopologyModal(true);
      result.current.setCopilotInitialQuery('staged');
      result.current.setShowAiCopilot(true);
      result.current.setShowLoginModal(true);
      result.current.setShowAccountModal(true);
      result.current.setAccountModalTab('security');
    });

    act(() => {
      result.current.reset();
    });

    expect(result.current.showCommandPalette).toBe(false);
    expect(result.current.showOnboardingModal).toBe(false);
    expect(result.current.showTopologyModal).toBe(false);
    expect(result.current.showAiCopilot).toBe(false);
    expect(result.current.copilotInitialQuery).toBe('');
    expect(result.current.showLoginModal).toBe(false);
    expect(result.current.showAccountModal).toBe(false);
    expect(result.current.accountModalTab).toBe('profile');
  });
});

describe('useConsoleSelections', () => {
  it('starts empty', () => {
    const { result } = renderHook(() => useConsoleSelections());

    expect(result.current.activeQuickKey).toBeNull();
    expect(result.current.activeLogDetail).toBeNull();
    expect(result.current.traceSnapshotId).toBe('');
    expect(result.current.curlExportCmd).toBe('');
  });

  it('clears every tenant-scoped selection on reset', () => {
    // These are the values a following session must not inherit.
    const { result } = renderHook(() => useConsoleSelections());
    act(() => {
      result.current.setActiveQuickKey('sk-secret');
      result.current.setActiveLogDetail({ id: 9, error_message: 'boom' });
      result.current.setTraceSnapshotId('trace-1');
      result.current.setCurlExportCmd('curl -H "Authorization: Bearer sk-secret"');
    });

    act(() => {
      result.current.reset();
    });

    expect(result.current.activeQuickKey).toBeNull();
    expect(result.current.activeLogDetail).toBeNull();
    expect(result.current.traceSnapshotId).toBe('');
    expect(result.current.curlExportCmd).toBe('');
  });
});

describe('useConsoleChrome', () => {
  it('starts with the sidebar open, the mobile menu closed and the route tab', () => {
    const { result } = renderHook(() => useConsoleChrome({ initialTab: 'logs' }));

    expect(result.current.sidebarCollapsed).toBe(false);
    expect(result.current.mobileMenuOpen).toBe(false);
    expect(result.current.currentTab).toBe('logs');
    expect(result.current.playModality).toBe('chat');
  });

  it('collapses and expands the sidebar', () => {
    const { result } = renderHook(() => useConsoleChrome());

    act(() => {
      result.current.collapseSidebar();
    });
    expect(result.current.sidebarCollapsed).toBe(true);

    act(() => {
      result.current.expandSidebar();
    });
    expect(result.current.sidebarCollapsed).toBe(false);
  });

  it('opens and closes the mobile menu', () => {
    const { result } = renderHook(() => useConsoleChrome());

    act(() => {
      result.current.setMobileMenuOpen(true);
    });
    expect(result.current.mobileMenuOpen).toBe(true);

    act(() => {
      result.current.closeMobileMenu();
    });
    expect(result.current.mobileMenuOpen).toBe(false);
  });

  it('carries the playground modality another view selects', () => {
    const { result } = renderHook(() => useConsoleChrome());

    act(() => {
      result.current.setPlayModality('images');
    });

    expect(result.current.playModality).toBe('images');
  });
});
