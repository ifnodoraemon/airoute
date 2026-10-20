import { useEffect, useRef } from 'react';

/**
 * Custom hook to handle Modal accessibility & interactions:
 * - Listens for 'Escape' key press to trigger onClose (Jakob Nielsen Heuristic #3: User control and freedom).
 * - Prevents background page scrolling while modal is open.
 * - Restores background page scrolling when modal unmounts or closes.
 *
 * The close callback is read through a ref so callers can pass an inline
 * function (for example an unsaved-changes guard) without re-registering the
 * listener and re-locking body scroll on every render.
 *
 * @param {boolean} isOpen - Whether the modal is currently open.
 * @param {function} onClose - Callback when the modal should close.
 */
export function useModalA11y(isOpen, onClose) {
  const closeRef = useRef(onClose);
  closeRef.current = onClose;

  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e) => {
      if (e.key === 'Escape' || e.key === 'Esc') {
        e.stopPropagation();
        closeRef.current?.();
      }
    };

    window.addEventListener('keydown', handleKeyDown);

    // Save previous overflow style and lock background scroll
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = prevOverflow;
    };
  }, [isOpen]);
}

export default useModalA11y;
