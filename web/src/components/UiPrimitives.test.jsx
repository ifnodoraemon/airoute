import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import Toast from './Toast';
import DiscardChangesDialog from './DiscardChangesDialog';
import CurlExportModal from './CurlExportModal';

// Three small UI primitives. They are grouped in one file because each is a few
// lines of behaviour: what is rendered, what closes it, and what it reports back.

describe('Toast', () => {
  it('renders nothing when it is not showing', () => {
    const { container } = render(<Toast toast={{ show: false, message: 'x', type: 'info' }} onClose={vi.fn()} />);

    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when there is no toast at all', () => {
    const { container } = render(<Toast toast={null} onClose={vi.fn()} />);

    expect(container).toBeEmptyDOMElement();
  });

  it('shows the message', () => {
    render(<Toast toast={{ show: true, message: '已保存', type: 'success' }} onClose={vi.fn()} />);

    expect(screen.getByText('已保存')).toBeInTheDocument();
  });

  it('reports the close when dismissed', () => {
    const onClose = vi.fn();
    render(<Toast toast={{ show: true, message: 'x', type: 'info' }} onClose={onClose} />);

    fireEvent.click(screen.getByRole('button'));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('renders every type without crashing', () => {
    for (const type of ['success', 'error', 'warning', 'info', 'something-else']) {
      const { unmount } = render(<Toast toast={{ show: true, message: `msg-${type}`, type }} onClose={vi.fn()} />);
      expect(screen.getByText(`msg-${type}`)).toBeInTheDocument();
      unmount();
    }
  });
});

describe('DiscardChangesDialog', () => {
  it('renders nothing when closed', () => {
    const { container } = render(<DiscardChangesDialog open={false} onKeepEditing={vi.fn()} onDiscard={vi.fn()} />);

    expect(container).toBeEmptyDOMElement();
  });

  it('renders an alert dialog with the default copy', () => {
    render(<DiscardChangesDialog open onKeepEditing={vi.fn()} onDiscard={vi.fn()} />);

    // role="alertdialog" is what a screen reader announces as an interrupting prompt.
    const dialog = screen.getByRole('alertdialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(screen.getByText(/放弃未保存的修改/)).toBeInTheDocument();
  });

  it('accepts custom copy', () => {
    render(
      <DiscardChangesDialog open onKeepEditing={vi.fn()} onDiscard={vi.fn()} title="Leave?" description="Edits will be lost." />
    );

    expect(screen.getByText('Leave?')).toBeInTheDocument();
    expect(screen.getByText('Edits will be lost.')).toBeInTheDocument();
  });

  it('reports keeping the edits', () => {
    const onKeepEditing = vi.fn();
    render(<DiscardChangesDialog open onKeepEditing={onKeepEditing} onDiscard={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: /继续编辑/ }));

    expect(onKeepEditing).toHaveBeenCalledTimes(1);
  });

  it('reports discarding', () => {
    const onDiscard = vi.fn();
    render(<DiscardChangesDialog open onKeepEditing={vi.fn()} onDiscard={onDiscard} />);

    fireEvent.click(screen.getByRole('button', { name: /放弃并关闭/ }));

    expect(onDiscard).toHaveBeenCalledTimes(1);
  });

  it('keeps editing when the backdrop is clicked', () => {
    // Clicking away is the safe outcome: it must not discard.
    const onKeepEditing = vi.fn();
    const onDiscard = vi.fn();
    const { container } = render(<DiscardChangesDialog open onKeepEditing={onKeepEditing} onDiscard={onDiscard} />);

    fireEvent.click(container.firstElementChild);

    expect(onKeepEditing).toHaveBeenCalledTimes(1);
    expect(onDiscard).not.toHaveBeenCalled();
  });

  it('focuses the safe button when it opens', () => {
    // The focused default should not be the destructive action.
    render(<DiscardChangesDialog open onKeepEditing={vi.fn()} onDiscard={vi.fn()} />);

    expect(screen.getByRole('button', { name: /继续编辑/ })).toHaveFocus();
  });

  it('renders in English when asked', () => {
    render(<DiscardChangesDialog open lang="en" onKeepEditing={vi.fn()} onDiscard={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Keep editing' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Discard & close' })).toBeInTheDocument();
  });
});

describe('CurlExportModal', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders nothing without a command', () => {
    const { container } = render(<CurlExportModal curlCmd="" onClose={vi.fn()} onCopy={vi.fn()} />);

    expect(container).toBeEmptyDOMElement();
  });

  it('shows the command', () => {
    render(<CurlExportModal curlCmd="curl https://gw.example/v1/chat" onClose={vi.fn()} onCopy={vi.fn()} />);

    expect(screen.getByText(/curl https:\/\/gw\.example/)).toBeInTheDocument();
  });

  it('copies the command and acknowledges it', async () => {
    const onCopy = vi.fn();
    render(<CurlExportModal curlCmd="curl x" onClose={vi.fn()} onCopy={onCopy} />);

    fireEvent.click(screen.getByRole('button', { name: /复制命令|Copy Command/ }));

    expect(onCopy).toHaveBeenCalledWith('curl x');
    expect(screen.getByRole('button', { name: /已复制！|Copied!/ })).toBeInTheDocument();
  });

  it('clears the acknowledgement again', async () => {
    render(<CurlExportModal curlCmd="curl x" onClose={vi.fn()} onCopy={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: /复制命令|Copy Command/ }));
    // The state update comes from a timer callback, so it has to be flushed inside
    // act before the assertion sees it.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });

    expect(screen.getByRole('button', { name: /复制命令|Copy Command/ })).toBeInTheDocument();
  });

  it('closes on the close button', () => {
    const onClose = vi.fn();
    render(<CurlExportModal curlCmd="curl x" onClose={onClose} onCopy={vi.fn()} />);

    // The header button is the one with an accessible name from its title/aria.
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]);

    expect(onClose).toHaveBeenCalled();
  });

  it('renders in English when asked', () => {
    render(<CurlExportModal curlCmd="curl x" lang="en" onClose={vi.fn()} onCopy={vi.fn()} />);

    expect(screen.getByText(/Export cURL Command/)).toBeInTheDocument();
  });
});
