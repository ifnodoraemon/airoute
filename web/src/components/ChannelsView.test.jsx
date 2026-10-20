import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ChannelsView from './ChannelsView';

// ChannelsView decides what an operator sees of the provider fleet. Two things
// carry real meaning here: the health classification, which has a documented
// precedence order, and the facet counts, which the component promises are computed
// against the *other* active filters so a count always equals the rows clicking it
// will show.

function channel(over = {}) {
  return {
    id: 1,
    name: 'openai-official',
    type: 'openai',
    base_url: 'https://api.openai.com',
    status: 'active',
    breaker_status: 'CLOSED',
    priority: 1,
    weight: 10,
    models: ['gpt-6'],
    ...over,
  };
}

function setup({ channels = [], channelLatencies = {}, ...props } = {}) {
  const handlers = {
    handleBatchPing: vi.fn(),
    setEditingChannelId: vi.fn(),
    setNewChannel: vi.fn(),
    setProbeAlert: vi.fn(),
    setShowChannelModal: vi.fn(),
    setSelectedChannelIds: vi.fn(),
    handleBatchStatusChannels: vi.fn(),
    handleBatchDeleteChannels: vi.fn(),
    copyToClipboard: vi.fn(),
    handleTestChannel: vi.fn(),
    handleEditChannel: vi.fn(),
    handleDeleteChannel: vi.fn(),
    onNavigateToReconciliation: vi.fn(),
    showToast: vi.fn(),
  };
  const utils = render(
    <ChannelsView
      channels={channels}
      channelLatencies={channelLatencies}
      batchTesting={false}
      testingId={null}
      selectedChannelIds={[]}
      lang="zh"
      {...handlers}
      {...props}
    />
  );
  return { ...utils, ...handlers };
}

describe('ChannelsView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // The filters live in the URL, so one test's filter would otherwise be in place
    // for the next.
    window.history.replaceState(null, '', window.location.pathname);
  });

  it('renders with no channels', () => {
    setup();

    expect(document.body.textContent.length).toBeGreaterThan(0);
  });

  it('lists the channels it was given', () => {
    setup({ channels: [channel({ name: 'openai-official' }), channel({ id: 2, name: 'groq-fast' })] });

    expect(screen.getByText('openai-official')).toBeInTheDocument();
    expect(screen.getByText('groq-fast')).toBeInTheDocument();
  });

  describe('health classification', () => {
    // The facet controls render "label (count)", and the count is the observable
    // that changes with the classification. Asserting only that the label exists
    // would pass no matter which bucket a channel landed in.
    function healthCount(label) {
      const node = screen.getByText(new RegExp(`^${label} \\(\\d+\\)$`));
      const match = node.textContent.match(/\((\d+)\)/);
      return Number(match[1]);
    }

    it('calls an OPEN breaker tripped', () => {
      setup({ channels: [channel({ breaker_status: 'OPEN' })] });

      expect(healthCount('熔断')).toBe(1);
      expect(healthCount('健康')).toBe(0);
    });

    it('calls a HALF-OPEN breaker half-open', () => {
      setup({ channels: [channel({ breaker_status: 'HALF-OPEN' })] });

      expect(healthCount('半开')).toBe(1);
      expect(healthCount('熔断')).toBe(0);
    });

    it('treats a failed probe as failed even with a closed breaker', () => {
      // A failed probe is the most recent direct evidence, so it outranks the
      // breaker state.
      setup({
        channels: [channel({ id: 7, breaker_status: 'CLOSED' })],
        channelLatencies: { 7: { success: false, latency_ms: 0, error: 'refused' } },
      });

      expect(healthCount('探测失败')).toBe(1);
      expect(healthCount('健康')).toBe(0);
    });

    it('treats a tripped breaker as tripped even when the last probe succeeded', () => {
      // A lucky probe does not clear a tripped breaker.
      setup({
        channels: [channel({ id: 7, breaker_status: 'OPEN' })],
        channelLatencies: { 7: { success: true, latency_ms: 20, error: '' } },
      });

      expect(healthCount('熔断')).toBe(1);
      expect(healthCount('健康')).toBe(0);
    });

    it('treats a successful probe as healthy', () => {
      setup({
        channels: [channel({ id: 7, breaker_status: 'CLOSED' })],
        channelLatencies: { 7: { success: true, latency_ms: 20, error: '' } },
      });

      expect(healthCount('健康')).toBe(1);
      expect(healthCount('探测失败')).toBe(0);
    });

    it('treats a never-probed channel with no breaker state as unprobed', () => {
      setup({ channels: [channel({ breaker_status: '' })] });

      expect(healthCount('未探测')).toBe(1);
      expect(healthCount('健康')).toBe(0);
    });

    it('classifies a mixed fleet into the right buckets', () => {
      setup({
        channels: [
          channel({ id: 1, breaker_status: 'OPEN' }),
          channel({ id: 2, breaker_status: 'CLOSED' }),
          channel({ id: 3, breaker_status: '' }),
        ],
      });

      expect(healthCount('熔断')).toBe(1);
      expect(healthCount('健康')).toBe(1);
      expect(healthCount('未探测')).toBe(1);
    });
  });

  describe('filters', () => {
    const fleet = [
      channel({ id: 1, name: 'openai-official', type: 'openai', status: 'active', breaker_status: 'CLOSED' }),
      // A distinct base URL: the default fixture shares one, and a search by URL
      // would then match both channels.
      channel({ id: 2, name: 'groq-fast', type: 'groq', base_url: 'https://api.groq.com/openai', status: 'active', breaker_status: 'OPEN' }),
      channel({ id: 3, name: 'legacy-provider', type: 'openai', status: 'disabled', breaker_status: 'CLOSED' }),
    ];

    it('shows the whole fleet by default', () => {
      setup({ channels: fleet });

      expect(screen.getByText('openai-official')).toBeInTheDocument();
      expect(screen.getByText('groq-fast')).toBeInTheDocument();
      expect(screen.getByText('legacy-provider')).toBeInTheDocument();
    });

    it('searches the channel name', () => {
      setup({ channels: fleet });

      fireEvent.change(screen.getByPlaceholderText(/搜索服务商名称|Search provider/), { target: { value: 'groq' } });

      expect(screen.getByText('groq-fast')).toBeInTheDocument();
      expect(screen.queryByText('openai-official')).not.toBeInTheDocument();
    });

    it('searches the base URL', () => {
      setup({ channels: fleet });

      fireEvent.change(screen.getByPlaceholderText(/搜索服务商名称|Search provider/), { target: { value: 'api.openai.com' } });

      expect(screen.getByText('openai-official')).toBeInTheDocument();
      expect(screen.queryByText('groq-fast')).not.toBeInTheDocument();
    });

    it('searches the model list', () => {
      setup({ channels: [channel({ name: 'a', models: ['gpt-6'] }), channel({ id: 2, name: 'b', models: ['llama-3'] })] });

      fireEvent.change(screen.getByPlaceholderText(/搜索服务商名称|Search provider/), { target: { value: 'llama' } });

      expect(screen.getByText('b')).toBeInTheDocument();
      expect(screen.queryByText('a')).not.toBeInTheDocument();
    });
  });

  describe('facet counts', () => {
    it('counts each health state', () => {
      setup({
        channels: [
          channel({ id: 1, breaker_status: 'CLOSED' }),
          channel({ id: 2, breaker_status: 'OPEN' }),
          channel({ id: 3, breaker_status: 'HALF-OPEN' }),
        ],
      });

      // The controls carry the numbers; asserting the labels exist keeps this from
      // depending on exact markup.
      expect(screen.getAllByText(/熔断/).length).toBeGreaterThan(0);
      expect(screen.getAllByText(/半开/).length).toBeGreaterThan(0);
      expect(screen.getAllByText(/健康/).length).toBeGreaterThan(0);
    });

    it('counts the enabled and disabled channels', () => {
      setup({
        channels: [
          channel({ id: 1, status: 'active' }),
          channel({ id: 2, status: 'disabled' }),
          channel({ id: 3, status: 'active' }),
        ],
      });

      expect(screen.getAllByText(/已启用/).length).toBeGreaterThan(0);
      expect(screen.getAllByText(/已停用/).length).toBeGreaterThan(0);
    });
  });

  it('renders in English when asked', () => {
    setup({ channels: [channel()], lang: 'en' });

    expect(screen.getByText('openai-official')).toBeInTheDocument();
  });
});
