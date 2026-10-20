import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ChannelModal from './ChannelModal';

// ChannelModal is a controlled form: the channel draft lives in the console, and
// this component edits it through setNewChannel. What is worth pinning is that the
// wiring is right — a field that does not reach the draft, or a submit that does not
// fire, is invisible until someone's provider config silently reverts.

function draft(over = {}) {
  return {
    name: '',
    type: 'openai',
    base_url: '',
    api_key: '',
    priority: 1,
    weight: 10,
    timeout_seconds: 60,
    models_str: '',
    mapping_str: '',
    protocols: ['openai_chat'],
    ...over,
  };
}

function setup({ isOpen = true, editingChannelId = null, newChannel = draft(), ...props } = {}) {
  const setNewChannel = vi.fn();
  const handlers = {
    onClose: vi.fn(),
    setNewChannel,
    applyPreset: vi.fn(),
    handleProbeChannel: vi.fn(),
    toggleProtocol: vi.fn(),
    onSubmit: vi.fn((e) => e?.preventDefault?.()),
    probing: false,
    submitting: false,
  };
  const utils = render(
    <ChannelModal
      isOpen={isOpen}
      editingChannelId={editingChannelId}
      newChannel={newChannel}
      probeAlert={null}
      lang="zh"
      {...handlers}
      {...props}
    />
  );
  return { ...utils, setNewChannel, ...handlers };
}

describe('ChannelModal', () => {
  it('renders nothing when closed', () => {
    const { container } = setup({ isOpen: false });

    expect(container).toBeEmptyDOMElement();
  });

  it('shows the draft values', () => {
    setup({ newChannel: draft({ name: 'my-provider', base_url: 'https://api.example/v1' }) });

    expect(screen.getByDisplayValue('my-provider')).toBeInTheDocument();
    expect(screen.getByDisplayValue('https://api.example/v1')).toBeInTheDocument();
  });

  describe('editing the draft', () => {
    it('reports a name change', () => {
      const { setNewChannel } = setup({ newChannel: draft({ name: 'before' }) });

      fireEvent.change(screen.getByPlaceholderText('gpustack-primary'), { target: { value: 'renamed' } });

      expect(setNewChannel).toHaveBeenCalledWith(expect.objectContaining({ name: 'renamed' }));
    });

    it('carries the rest of the draft along with the change', () => {
      // The handler spreads the draft it was given rather than sending only the
      // edited field, so the model list and protocols survive a rename.
      const { setNewChannel } = setup({
        newChannel: draft({ name: 'keep-me', models_str: 'a,b', protocols: ['openai_chat', 'images'] }),
      });

      fireEvent.change(screen.getByPlaceholderText('gpustack-primary'), { target: { value: 'new-name' } });

      const sent = setNewChannel.mock.calls.at(-1)[0];
      expect(sent.name).toBe('new-name');
      expect(sent.models_str).toBe('a,b');
      expect(sent.protocols).toEqual(['openai_chat', 'images']);
    });
  });

  describe('submitting', () => {
    it('reports a create', () => {
      const { onSubmit } = setup({ editingChannelId: null });

      fireEvent.submit(screen.getByRole('button', { name: /确认接入/ }).closest('form'));

      expect(onSubmit).toHaveBeenCalledTimes(1);
    });

    it('labels the button for an edit and reports it', () => {
      const { onSubmit } = setup({ editingChannelId: 7 });

      const button = screen.getByRole('button', { name: /保存配置/ });
      fireEvent.submit(button.closest('form'));

      expect(onSubmit).toHaveBeenCalledTimes(1);
    });

    it('disables the submit while saving', () => {
      // Double-submitting a provider create would create two channels.
      setup({ submitting: true });

      expect(screen.getByRole('button', { name: /保存中/ })).toBeDisabled();
    });

    it('reports the cancel', () => {
      const { onClose } = setup();

      fireEvent.click(screen.getByRole('button', { name: /取消/ }));

      expect(onClose).toHaveBeenCalledTimes(1);
    });
  });

  it('reports the probe request', () => {
    const { handleProbeChannel } = setup();

    fireEvent.click(screen.getByRole('button', { name: /测试连通性|Test Ping/ }));

    expect(handleProbeChannel).toHaveBeenCalledTimes(1);
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en' });

    expect(screen.getByRole('button', { name: /Connect Provider/ })).toBeInTheDocument();
  });
});
