// One Escape key, one owner.
//
// Several layers can be open at once: a form modal, the command palette opened
// over it, a confirmation dialog inside the modal. Each of them listens for Escape.
// When every listener acted independently, a single Escape closed the palette *and*
// dismissed the form behind it, which surfaced an unsaved-changes prompt nobody
// asked for.
//
// Layers register here while they are open. The module installs one capture-phase
// listener that hands Escape to the topmost layer only, and consumes the event so
// the other listeners never see it. Capture phase matters: it runs before the
// bubble-phase listeners (the shell's global shortcut handler, React's root
// handler), so "consumed" really means consumed.

const layers = [];

let listening = false;

function onKeyDown(event) {
  if (event.key !== 'Escape' && event.key !== 'Esc') return;
  const top = layers[layers.length - 1];
  if (!top) return;
  event.preventDefault();
  // Stop the event from reaching the bubble-phase handlers on window/document.
  if (typeof event.stopPropagation === 'function') event.stopPropagation();
  top.handler();
}

function ensureListener() {
  if (listening || typeof window === 'undefined') return;
  window.addEventListener('keydown', onKeyDown, true);
  listening = true;
}

/**
 * Register an open layer. The most recently pushed layer owns Escape.
 *
 * @param {Function} handler called when this layer is the topmost one and Escape
 *   is pressed. It should close exactly one layer (the confirmation dialog, the
 *   palette, the modal…).
 * @returns {Function} unregister function; call it when the layer closes.
 */
export function pushEscapeLayer(handler) {
  if (typeof handler !== 'function') return () => {};
  const entry = { handler };
  layers.push(entry);
  ensureListener();
  return () => {
    const index = layers.lastIndexOf(entry);
    if (index >= 0) layers.splice(index, 1);
    if (layers.length === 0 && listening && typeof window !== 'undefined') {
      window.removeEventListener('keydown', onKeyDown, true);
      listening = false;
    }
  };
}

/** True when at least one layer is open and therefore owns Escape. */
export function hasEscapeLayers() {
  return layers.length > 0;
}

/** Test helper: drop every registered layer and its listener. */
export function resetEscapeLayers() {
  layers.length = 0;
  if (listening && typeof window !== 'undefined') {
    window.removeEventListener('keydown', onKeyDown, true);
    listening = false;
  }
}
