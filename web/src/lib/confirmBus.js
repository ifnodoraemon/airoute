// Global confirmation bus. Lets any hook or component request a branded
// confirmation dialog (rendered once by ConfirmDialogHost at the app root)
// without threading dialog state through every view. requestConfirm resolves
// true on confirm, false on cancel/Escape/backdrop.
//
// When no host is mounted (unit tests rendering a component in isolation) it
// falls back to the native window.confirm so callers still get a decision
// instead of a promise that never settles.

let listener = null;

export function requestConfirm(options) {
  return new Promise((resolve) => {
    if (!listener) {
      const message = options?.description || options?.title || '';
      resolve(typeof window !== 'undefined' && typeof window.confirm === 'function' ? window.confirm(message) : false);
      return;
    }
    listener({ options: options || {}, resolve });
  });
}

export function subscribeConfirm(fn) {
  listener = fn;
  return () => {
    if (listener === fn) listener = null;
  };
}