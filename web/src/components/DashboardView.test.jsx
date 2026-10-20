import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import DashboardView from './DashboardView';

// DashboardView is props-driven and computes every number it shows: the SLA
// percentage, the latency percentiles, the modality split, the token split. Those
// derivations are where a dashboard goes quietly wrong — a wrong percentile still
// renders, it just misleads — so they are pinned here rather than left to review.

function setup(props = {}) {
  const handlers = {
    handleBatchTest: vi.fn(),
    handleTestChannel: vi.fn(),
    fetchData: vi.fn(),
    fetchLogs: vi.fn(),
    showToast: vi.fn(),
    setCurrentTab: vi.fn(),
    setShowChannelModal: vi.fn(),
    setActiveLogDetail: vi.fn(),
    onOpenTopology: vi.fn(),
  };
  const utils = render(
    <DashboardView
      stats={{}}
      channels={[]}
      logs={[]}
      adminUser={{ username: 'root', role: 'admin' }}
      lang="zh"
      {...handlers}
      {...props}
    />
  );
  return { ...utils, ...handlers };
}

describe('DashboardView', () => {
  it('renders without data instead of crashing', () => {
    setup();

    expect(screen.getByText('运营总览')).toBeInTheDocument();
  });

  it('reports the SLA as unavailable when no request has been made', () => {
    // A dash, not 0.00%: a fresh gateway has no availability to report, and 0%
    // would read as an outage.
    setup({ stats: { total_requests: 0 } });

    // The card holds the label and the value; the value is a dash rather than a
    // percentage, because a gateway with no traffic has no availability to report.
    const card = screen.getByText('服务可用率 (SLA)').closest('div');
    expect(within(card).getByText('—')).toBeInTheDocument();
  });

  it('does not print a percentage sign after the unavailable marker', () => {
    // The three render sites appended "%" unconditionally, so a gateway with no
    // traffic showed "—%".
    setup({ stats: { total_requests: 0 } });

    expect(screen.queryByText('—%')).not.toBeInTheDocument();
  });

  it('computes the SLA from successful requests', () => {
    setup({ stats: { total_requests: 200, success_requests: 198, client_error_requests: 1, server_error_requests: 1 } });

    const card = screen.getByText('服务可用率 (SLA)').closest('div');
    expect(within(card).getByText('99.00%')).toBeInTheDocument();
  });

  it('says so when there are no latency samples to compute percentiles from', () => {
    setup({ logs: [{ id: 1, ttft_ms: 0 }, { id: 2 }] });

    // Zero and missing samples are not measurements; the panel must not invent one.
    expect(screen.getAllByText('无样本').length).toBeGreaterThan(0);
  });

  it('computes the latency percentiles from the samples it has', () => {
    // Ten samples, 10..100ms. The index is floor(n*q) clamped to the last sample,
    // so p50 is index 5 (60ms) and p90/p99 both land on index 9 (100ms).
    const logs = Array.from({ length: 10 }, (_, i) => ({ id: i + 1, ttft_ms: (i + 1) * 10 }));
    setup({ logs });

    expect(screen.getByText('60ms')).toBeInTheDocument();
    expect(screen.getAllByText('100ms').length).toBeGreaterThan(0);
  });

  it('ignores non-positive latencies when computing percentiles', () => {
    // A zero sample would be counted as an instant response and drag the
    // percentiles down, making the gateway look faster than it is. Here it is
    // dropped, leaving [80, 90], so every percentile is one of those two.
    const logs = [{ id: 1, ttft_ms: 0 }, { id: 2, ttft_ms: 80 }, { id: 3, ttft_ms: 90 }];
    setup({ logs });

    expect(screen.queryByText('0ms')).not.toBeInTheDocument();
    // Pin the value, not just its presence: keeping the zero would shift the sample
    // set from [80,90] to [0,80,90] and move p50 from 90ms to 80ms.
    const p50Card = screen.getByText('P50 中位时延').closest('div');
    expect(within(p50Card).getByText('90ms')).toBeInTheDocument();
  });

  it('splits the token totals between prompt and completion', () => {
    setup({ logs: [{ id: 1, prompt_tokens: 750, completion_tokens: 250 }] });

    // 750/1000 prompt, 250/1000 completion.
    expect(screen.getAllByText(/75%/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/25%/).length).toBeGreaterThan(0);
  });

  it('classifies model names into modalities', () => {
    setup({
      logs: [
        { id: 1, model: 'gpt-6', total_tokens: 10 },
        { id: 2, model: 'text-embedding-3-small', total_tokens: 10 },
        { id: 3, model: 'flux-pro', total_tokens: 10 },
        { id: 4, model: 'whisper-large-v3', total_tokens: 10 },
      ],
    });

    // The breakdown renders one row per modality it found.
    expect(screen.getByText('文本对话')).toBeInTheDocument();
    expect(screen.getByText('向量嵌入')).toBeInTheDocument();
    expect(screen.getByText('图像生成')).toBeInTheDocument();
    expect(screen.getByText('语音合成/识别')).toBeInTheDocument();
  });

  it('reports a tripped breaker as needing attention', () => {
    setup({ channels: [{ id: 1, name: 'c1', status: 'active', breaker_status: 'OPEN' }] });

    expect(screen.getByText(/断路器已触发/)).toBeInTheDocument();
  });

  it('reports a disabled channel as needing attention', () => {
    setup({ channels: [{ id: 1, name: 'c1', status: 'disabled', breaker_status: 'CLOSED' }] });

    expect(screen.getByText(/已停用/)).toBeInTheDocument();
  });

  it('clusters failed rows by status code', () => {
    // The histogram was computed and never rendered before; this pins that it now
    // reaches the operator.
    setup({
      logs: [
        { id: 1, status_code: 429, model: 'gpt-6' },
        { id: 2, status_code: 429, model: 'gpt-6' },
        { id: 3, status_code: 503, model: 'gpt-6' },
        { id: 4, status_code: 200, model: 'gpt-6' },
      ],
    });

    expect(screen.getByText(/429×2/)).toBeInTheDocument();
    expect(screen.getByText(/503×1/)).toBeInTheDocument();
  });

  it('does not report error codes when every request succeeded', () => {
    setup({ logs: [{ id: 1, status_code: 200, model: 'gpt-6' }] });

    expect(screen.queryByText(/失败状态码分布/)).not.toBeInTheDocument();
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en' });

    expect(screen.getByText('Dashboard')).toBeInTheDocument();
  });
});
