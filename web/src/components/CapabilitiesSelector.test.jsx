import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import CapabilitiesSelector from './CapabilitiesSelector';

// Every capability the gateway can expose. Pinned here because the "Chat Only"
// bulk action hardcodes a subset of these ids: if the list changes, that subset
// has to change with it, and nothing else would notice.
// `anthropic_native` is a marker (the upstream speaks Claude natively), not an
// extra route, but it is a selectable protocol like the rest.
const ALL_IDS = [
  'openai_chat', 'openai_response', 'openai_text', 'anthropic_messages', 'anthropic_native',
  'images', 'audio_speech', 'audio_transcription', 'embeddings', 'rerank', 'videos',
];
const CHAT_IDS = ['openai_chat', 'openai_response', 'openai_text', 'anthropic_messages'];

function setup(props = {}) {
  const handlers = { onToggle: vi.fn(), onSetProtocols: vi.fn() };
  const utils = render(
    <CapabilitiesSelector protocols={[]} lang="zh" {...handlers} {...props} />
  );
  return { ...utils, ...handlers };
}

describe('CapabilitiesSelector', () => {
  it('renders one entry per capability with its endpoint path', () => {
    setup();

    expect(screen.getByText('/v1/chat/completions')).toBeInTheDocument();
    expect(screen.getByText('/v1/responses')).toBeInTheDocument();
    expect(screen.getByText('/v1/embeddings')).toBeInTheDocument();
    expect(screen.getByText('/v1/videos/generations')).toBeInTheDocument();
  });

  it('counts the selected protocols', () => {
    setup({ protocols: ['openai_chat', 'images'] });

    expect(screen.getByText('协议能力 (2/11)')).toBeInTheDocument();
  });

  it('counts zero when nothing is selected', () => {
    setup();

    expect(screen.getByText('协议能力 (0/11)')).toBeInTheDocument();
  });

  it('reports the toggle for the capability that was clicked', () => {
    const { onToggle } = setup();

    fireEvent.click(screen.getByText('/v1/rerank'));

    expect(onToggle).toHaveBeenCalledWith('rerank');
  });

  it('selects every capability', () => {
    const { onSetProtocols } = setup();

    fireEvent.click(screen.getByRole('button', { name: '全选' }));

    expect(onSetProtocols).toHaveBeenCalledTimes(1);
    const sent = onSetProtocols.mock.calls[0][0];
    expect([...sent].sort()).toEqual([...ALL_IDS].sort());
  });

  it('selects exactly the chat protocols', () => {
    const { onSetProtocols } = setup();

    fireEvent.click(screen.getByRole('button', { name: '仅对话' }));

    // The hardcoded subset must stay inside the capability list.
    const sent = onSetProtocols.mock.calls[0][0];
    expect([...sent].sort()).toEqual([...CHAT_IDS].sort());
    sent.forEach((id) => expect(ALL_IDS).toContain(id));
  });

  it('clears the selection', () => {
    const { onSetProtocols } = setup({ protocols: ALL_IDS });

    fireEvent.click(screen.getByRole('button', { name: '清空' }));

    expect(onSetProtocols).toHaveBeenCalledWith([]);
  });

  it('marks only the selected capabilities as checked', () => {
    setup({ protocols: ['images'] });

    // Each entry shows a tick when selected; the rest render an empty box.
    const ticks = screen.getAllByText('✓');
    expect(ticks).toHaveLength(1);
  });

  it('offers the Anthropic-native marker with zh and en labels', () => {
    const { rerender, onToggle } = setup({ lang: 'zh' });
    expect(screen.getByText('Anthropic 原生')).toBeInTheDocument();

    rerender(<CapabilitiesSelector protocols={[]} lang="en" onToggle={onToggle} onSetProtocols={vi.fn()} />);
    expect(screen.getByText('Anthropic Native')).toBeInTheDocument();
  });

  it('keeps protocol ids it does not know about when using a bulk action', () => {
    // A newer gateway may report a capability this console has never heard of.
    // Selecting "all" must not silently drop it, which would rewrite the channel on
    // the next save.
    const { onSetProtocols } = setup({ protocols: ['openai_chat', 'future_protocol'] });

    fireEvent.click(screen.getByRole('button', { name: '全选' }));
    expect(onSetProtocols.mock.calls[0][0]).toContain('future_protocol');

    fireEvent.click(screen.getByRole('button', { name: '仅对话' }));
    expect(onSetProtocols.mock.calls[1][0]).toContain('future_protocol');

    fireEvent.click(screen.getByRole('button', { name: '清空' }));
    expect(onSetProtocols.mock.calls[2][0]).toEqual(['future_protocol']);
  });

  it('renders English labels when asked', () => {
    setup({ lang: 'en', protocols: ['openai_chat'] });

    expect(screen.getByText('Protocol Capabilities (1/11)')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Select All' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Chat Only' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear' })).toBeInTheDocument();
  });
});
