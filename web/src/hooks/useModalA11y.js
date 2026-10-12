import { useEffect } from 'react';

/**
 * Custom hook to handle Modal accessibility & interactions:
 * - Listens for 'Escape' key press to trigger onClose (Jakob Nielsen Heuristic #3: User control and freedom).
 * - Prevents background page scrolling while modal is open.
 * - Restores background page scrolling when modal unmounts or closes.
 *
 * @param {boolean} isOpen - Whether the modal is currently open.
 * @param {function} onClose - Callback when the modal should close.
 */
export function useModalA11y(isOpen, onClose) {
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e) => {
      if (e.key === 'Escape' || e.key === 'Esc') {
        e.stopPropagation();
        onClose?.();
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
  }, [isOpen, onClose]);
}

export default useModalA11y;
