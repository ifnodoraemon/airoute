import { useState } from 'react';
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import useModalA11y from './useModalA11y';

function DialogExample() {
  const [open, setOpen] = useState(false);
  useModalA11y(open, () => setOpen(false));
  return <>
    <button onClick={() => setOpen(true)}>Open dialog</button>
    {open && <div role="dialog" aria-modal="true" aria-label="Example">
      <button>First action</button><button>Last action</button>
    </div>}
  </>;
}

describe('modal keyboard focus', () => {
  it('wraps Tab in both directions and restores the opener after Escape', () => {
    // jsdom has no layout; visible controls get rectangles for this keyboard test.
    const rectangles = vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{}]);
    try {
      render(<DialogExample />);
      const opener = screen.getByRole('button', { name: 'Open dialog' });
      opener.focus();
      fireEvent.click(opener);
      expect(screen.getByRole('button', { name: 'First action' })).toHaveFocus();
      fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
      expect(screen.getByRole('button', { name: 'Last action' })).toHaveFocus();
      fireEvent.keyDown(document, { key: 'Tab' });
      expect(screen.getByRole('button', { name: 'First action' })).toHaveFocus();
      fireEvent.keyDown(document, { key: 'Escape' });
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(opener).toHaveFocus();
    } finally {
      rectangles.mockRestore();
    }
  });
});
