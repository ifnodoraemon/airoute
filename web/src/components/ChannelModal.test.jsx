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
    mapping_rules: [],
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
  it('never renders a stored credential and explains how to keep it', () => {
    // The draft must not carry the stored secret, so the input is empty and the form
    // says the key is already configured instead of round-tripping it.
    const { container } = setup({
      editingChannelId: 7,
      newChannel: draft({ name: 'upstream', base_url: 'https://a.example', api_key: '', api_key_configured: true }),
    });

    const apiKeyInput = container.querySelector('input[type="password"]');
    expect(apiKeyInput).toBeTruthy();
    expect(apiKeyInput.value).toBe('');
    expect(screen.getByText('已配置')).toBeInTheDocument();
    // The field explains that blank means "keep the stored key".
    expect(apiKeyInput.placeholder).toContain('留空保持不变');
    expect(document.body.textContent).not.toContain('sk-upstream');
  });

  it('reports a typed replacement key through setNewChannel', () => {
    const { setNewChannel, container } = setup({
      editingChannelId: 7,
      newChannel: draft({ name: 'upstream', base_url: 'https://a.example', api_key: '', api_key_configured: true }),
    });

    fireEvent.change(container.querySelector('input[type="password"]'), { target: { value: 'sk-rotated' } });

    expect(setNewChannel).toHaveBeenCalled();
    const next = setNewChannel.mock.calls.at(-1)[0];
    expect(next.api_key).toBe('sk-rotated');
    expect(next.api_key_configured).toBe(true);
  });

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

describe('ChannelModal routing numbers', () => {
  // The form used to rewrite an explicit 0 into 1 (priority) / 10 (weight) before
  // the request was built, so the console silently disagreed with what the
  // operator typed and the gateway could never receive 0 from this screen.
  it('keeps an explicit 0 in the priority and weight fields', () => {
    const { setNewChannel } = setup();
    // The three numeric fields are priority, weight and timeout, in that order.
    const [priorityInput, weightInput] = screen.getAllByRole('spinbutton');

    fireEvent.change(priorityInput, { target: { value: '0' } });
    expect(setNewChannel.mock.calls.at(-1)[0].priority).toBe(0);

    fireEvent.change(weightInput, { target: { value: '0' } });
    expect(setNewChannel.mock.calls.at(-1)[0].weight).toBe(0);
  });

  it('maps an emptied field to 0 (the gateway default) instead of inventing a value', () => {
    const { setNewChannel } = setup();
    const [priorityInput] = screen.getAllByRole('spinbutton');

    fireEvent.change(priorityInput, { target: { value: '' } });

    expect(setNewChannel.mock.calls.at(-1)[0].priority).toBe(0);
  });
});
