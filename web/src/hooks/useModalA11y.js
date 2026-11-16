import { useEffect, useRef } from 'react';
import { pushEscapeLayer } from '../lib/escapeLayer';

// Body scroll lock, reference counted.
//
// Each modal used to save `document.body.style.overflow` in its own effect and
// restore it in its own cleanup, which only works when cleanups happen in strict
// LIFO order. Any other order (closing the lower modal first, or unmounting several
// modals in one commit — logging out, session expiry, leaving the view) wrote the
// wrong value back: the page was scrollable while a modal was still open, and then
// stayed `overflow:hidden` forever after the last one closed, so the next screen
// could not be scrolled at all.
//
// A single counter owns the lock instead: the first opener records the original
// value, the last closer restores it, and the order in which modals open or close
// stops mattering.
let lockCount = 0;
let savedOverflow = '';

function lockBodyScroll() {
  if (typeof document === 'undefined') return;
  if (lockCount === 0) {
    savedOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
  }
  lockCount += 1;
}

function unlockBodyScroll() {
  if (typeof document === 'undefined') return;
  lockCount = Math.max(0, lockCount - 1);
  if (lockCount === 0) {
    document.body.style.overflow = savedOverflow;
    savedOverflow = '';
  }
}

/**
 * Test helper: drop the lock and its listener state.
 *
 * Mirrors `resetEscapeLayers()` in lib/escapeLayer.js — both pieces of modal state
 * are module-level, so a test that fails mid-way must be able to reset them.
 */
export function resetBodyScrollLock() {
  lockCount = 0;
  savedOverflow = '';
  if (typeof document !== 'undefined') document.body.style.overflow = '';
}

// The topmost open dialog/alertdialog in the DOM. Focus management is scoped to
// it so a stacked confirmation dialog owns the focus trap, not the modal behind.
function topDialog() {
  if (typeof document === 'undefined') return null;
  const dialogs = document.querySelectorAll('[role="dialog"],[role="alertdialog"]');
  return dialogs.length ? dialogs[dialogs.length - 1] : null;
}

function focusableIn(el) {
  const nodes = el.querySelectorAll(
    'a[href],button:not([disabled]),textarea:not([disabled]),input:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])'
  );
  // getClientRects is empty under jsdom (no layout), so the trap stays inert in
  // unit tests while working in a real browser.
  return Array.from(nodes).filter((node) => node.getClientRects().length > 0);
}

/**
 * Custom hook to handle Modal accessibility & interactions:
 * - Escape closes *this* modal only when no newer layer is open (Jakob Nielsen
 *   Heuristic #3: User control and freedom). Layers stack: a palette or a
 *   confirmation dialog opened on top owns Escape until it closes.
 * - Prevents background page scrolling while any modal is open (reference counted,
 *   so any open/close order leaves the page in the right state).
 * - Restores background page scrolling when the last modal unmounts or closes.
 * - Traps Tab focus inside the topmost dialog and restores focus to the element
 *   that opened the modal on close (WCAG 2.4.3 Focus Order).
 *
 * The close callback is read through a ref so callers can pass an inline
 * function (for example an unsaved-changes guard) without re-registering the
 * layer or re-locking body scroll on every render.
 *
 * @param {boolean} isOpen - Whether the modal is currently open.
 * @param {function} onClose - Callback when the modal should close.
 */
export function useModalA11y(isOpen, onClose) {
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!isOpen) return undefined;

    // This modal is an Escape layer; the shared stack decides who handles the
    // key, so a modal behind another layer no longer reacts to it.
    const unregister = pushEscapeLayer(() => closeRef.current?.());
    lockBodyScroll();

    const previouslyFocused = typeof document !== 'undefined' ? document.activeElement : null;
    const dialog = topDialog();
    if (dialog) {
      const focusable = focusableIn(dialog);
      (focusable[0] || dialog).focus?.();
    }
    const onKeyDown = (event) => {
      if (event.key !== 'Tab' || typeof document === 'undefined') return;
      const top = topDialog();
      if (!top) return;
      const focusable = focusableIn(top);
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    if (typeof document !== 'undefined') document.addEventListener('keydown', onKeyDown);

    return () => {
      unregister();
      unlockBodyScroll();
      if (typeof document !== 'undefined') document.removeEventListener('keydown', onKeyDown);
      previouslyFocused?.focus?.();
    };
  }, [isOpen]);
}

export default useModalA11y;
