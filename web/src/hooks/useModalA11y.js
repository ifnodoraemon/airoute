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

/**
 * Custom hook to handle Modal accessibility & interactions:
 * - Escape closes *this* modal only when no newer layer is open (Jakob Nielsen
 *   Heuristic #3: User control and freedom). Layers stack: a palette or a
 *   confirmation dialog opened on top owns Escape until it closes.
 * - Prevents background page scrolling while any modal is open (reference counted,
 *   so any open/close order leaves the page in the right state).
 * - Restores background page scrolling when the last modal unmounts or closes.
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

    return () => {
      unregister();
      unlockBodyScroll();
    };
  }, [isOpen]);
}

export default useModalA11y;
