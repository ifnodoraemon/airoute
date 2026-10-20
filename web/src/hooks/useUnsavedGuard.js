import { useCallback, useEffect, useState } from 'react';

/**
 * Route every dismissal of a form modal through one guard.
 *
 * Backdrop click, the close button and Escape all call `requestClose`. When the
 * form has unsaved edits the guard surfaces a confirmation instead of silently
 * dropping the draft; otherwise it closes immediately.
 *
 * @param {object}   options
 * @param {boolean}  options.isOpen
 * @param {boolean}  options.isDirty
 * @param {function} options.onClose  Actually closes the modal.
 */
export default function useUnsavedGuard({ isOpen, isDirty, onClose }) {
  const [confirming, setConfirming] = useState(false);

  useEffect(() => {
    if (!isOpen) setConfirming(false);
  }, [isOpen]);

  const requestClose = useCallback(() => {
    if (confirming) {
      setConfirming(false);
      return;
    }
    if (isDirty) {
      setConfirming(true);
      return;
    }
    onClose?.();
  }, [confirming, isDirty, onClose]);

  const keepEditing = useCallback(() => setConfirming(false), []);

  const discard = useCallback(() => {
    setConfirming(false);
    onClose?.();
  }, [onClose]);

  return { confirming, requestClose, keepEditing, discard };
}
