// Regression test for the modal body-scroll lock.
//
// Started as the BUG-WEB-05 probe; now asserts the CORRECT behaviour (it fails on
// the pre-fix code, where each modal restored its own saved overflow value).
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/hooks/useModalA11y.scrollLock.test.js

import { describe, it, expect, afterEach } from 'vitest';
import { render, act } from '@testing-library/react';
import { useState } from 'react';
import useModalA11y, { resetBodyScrollLock } from './useModalA11y';

function Modal({ open, label }) {
  useModalA11y(open, () => {});
  return open ? <div data-testid={label} /> : null;
}

function TwoModals() {
  const [a, setA] = useState(true);
  const [b, setB] = useState(true);
  return (
    <div>
      <Modal open={a} label="modal-a" />
      <Modal open={b} label="modal-b" />
      <button type="button" onClick={() => setA(false)}>close-a</button>
      <button type="button" onClick={() => setB(false)}>close-b</button>
    </div>
  );
}

describe('useModalA11y body scroll lock', () => {
  afterEach(() => {
    resetBodyScrollLock();
  });

  it('keeps the page locked while any modal is open, whatever the close order', () => {
    const { getByText } = render(<TwoModals />);
    expect(document.body.style.overflow).toBe('hidden');

    // Close the modal that was opened FIRST while the second is still open.
    act(() => { getByText('close-a').click(); });
    expect(document.body.style.overflow).toBe('hidden');

    // Closing the last modal releases the page.
    act(() => { getByText('close-b').click(); });
    expect(document.body.style.overflow).toBe('');
  });

  it('releases the page when several modals unmount in one commit', () => {
    function ManyModals({ show }) {
      return (
        <div>
          <Modal open={show} label="m1" />
          <Modal open={show} label="m2" />
          <Modal open={show} label="m3" />
        </div>
      );
    }

    const { rerender } = render(<ManyModals show />);
    expect(document.body.style.overflow).toBe('hidden');

    // Logging out / session expiry unmounts every overlay in a single render.
    rerender(<ManyModals show={false} />);
    expect(document.body.style.overflow).toBe('');
  });

  it('restores the previous inline value, not an empty one', () => {
    document.body.style.overflow = 'scroll';
    const { unmount } = render(<Modal open label="only" />);
    expect(document.body.style.overflow).toBe('hidden');

    unmount();
    expect(document.body.style.overflow).toBe('scroll');
  });
});
