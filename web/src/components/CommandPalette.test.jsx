import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import CommandPalette from './CommandPalette';
import ChannelModal from './ChannelModal';
import { resetEscapeLayers } from '../lib/escapeLayer';

// An administrator can open the command palette on top of a form modal. One Escape
// used to close the palette *and* dismiss the form behind it, which surfaced an
// unsaved-changes prompt for a dialog the operator never asked to close. Escape now
// goes to the topmost layer only.

function commandPalette(overrides = {}) {
  return (
    <CommandPalette
      isOpen
      onClose={overrides.onClose ?? (() => {})}
      currentTab="dashboard"
      setCurrentTab={() => {}}
      setViewMode={() => {}}
      models={[]}
      channels={[]}
      keys={[]}
      adminUser={{ username: 'root', role: 'admin' }}
      onOpenNewKey={() => {}}
      onOpenNewChannel={() => {}}
      toggleLang={() => {}}
      lang="zh"
      onLogout={() => {}}
      showToast={() => {}}
      onSelectModel={() => {}}
      onOpenCopilotWithQuery={() => {}}
    />
  );
}

function channelDraft(overrides = {}) {
  return {
    name: 'gpustack-primary',
    type: 'gpustack',
    base_url: 'https://gw.example.com/v1',
    api_key: '',
    priority: 1,
    weight: 10,
    timeout_seconds: 60,
    models_str: '',
    mapping_rules: [],
    protocols: ['openai_chat'],
    ...overrides,
  };
}

function channelModal(props) {
  return (
    <ChannelModal
      isOpen
      onClose={props.onClose}
      editingChannelId={null}
      newChannel={props.newChannel}
      setNewChannel={() => {}}
      probeAlert={null}
      probing={false}
      applyPreset={() => {}}
      handleProbeChannel={() => {}}
      toggleProtocol={() => {}}
      onSubmit={() => {}}
      submitting={false}
      lang="zh"
    />
  );
}

describe('Escape routing across stacked overlays', () => {
  afterEach(() => {
    resetEscapeLayers();
  });

  it('closes the command palette without touching a dirty modal underneath', () => {
    const paletteClose = vi.fn();
    const modalClose = vi.fn();
    const { rerender } = render(
      <div>
        {channelModal({ onClose: modalClose, newChannel: channelDraft() })}
        {commandPalette({ onClose: paletteClose })}
      </div>,
    );

    // Make the channel form dirty after it opened: the unsaved guard must be armed,
    // otherwise the test would pass for the wrong reason.
    rerender(
      <div>
        {channelModal({ onClose: modalClose, newChannel: channelDraft({ name: 'renamed-provider' }) })}
        {commandPalette({ onClose: paletteClose })}
      </div>,
    );
    expect(screen.getByText('有未保存的修改')).toBeInTheDocument();

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(paletteClose).toHaveBeenCalledTimes(1);
    expect(modalClose).not.toHaveBeenCalled();
    // No discard prompt: the form was never asked to close.
    expect(screen.queryByText('放弃未保存的服务商配置？')).not.toBeInTheDocument();
  });

  it('lets the next Escape reach the modal once the palette is closed', () => {
    const paletteClose = vi.fn();
    const modalClose = vi.fn();
    const view = (paletteOpen) => (
      <div>
        {channelModal({ onClose: modalClose, newChannel: channelDraft() })}
        {paletteOpen ? commandPalette({ onClose: paletteClose }) : null}
      </div>
    );
    const { rerender } = render(view(true));
    rerender(view(false));

    fireEvent.keyDown(window, { key: 'Escape' });

    // The modal owns Escape now; it is clean, so it closes.
    expect(modalClose).toHaveBeenCalledTimes(1);
    expect(paletteClose).not.toHaveBeenCalled();
  });
});
