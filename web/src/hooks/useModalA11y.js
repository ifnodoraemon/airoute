import { useEffect, useRef } from 'react';
import { pushEscapeLayer } from '../lib/escapeLayer';

/**
 * Custom hook to handle Modal accessibility & interactions:
 * - Escape closes *this* modal only when no newer layer is open (Jakob Nielsen
 *   Heuristic #3: User control and freedom). Layers stack: a palette or a
 *   confirmation dialog opened on top owns Escape until it closes.
 * - Prevents background page scrolling while modal is open.
 * - Restores background page scrolling when the modal unmounts or closes.
 *
 * The close callback is read through a ref so callers can pass an inline
 * function (for example an unsaved-changes guard) without re-registering the
 * layer and re-locking body scroll on every render.
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

    // Save previous overflow style and lock background scroll
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    return () => {
      unregister();
      document.body.style.overflow = prevOverflow;
    };
  }, [isOpen]);
}

export default useModalA11y;
