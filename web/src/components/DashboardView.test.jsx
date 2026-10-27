import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, within, fireEvent, waitFor, act } from '@testing-library/react';
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

  it('renders real-time RPM, TPM, and req/s telemetry', () => {
    setup({
      stats: {
        total_requests: 120,
        current_rpm: 60,
        current_tpm: 12000,
        peak_rpm: 90,
        peak_tpm: 18000
      }
    });

    // 60 RPM / 60s = 1.0 req/s
    expect(screen.getByText('1.0')).toBeInTheDocument();
    expect(screen.getAllByText('req/s').length).toBeGreaterThan(0);
    expect(screen.getByText(/60 RPM/)).toBeInTheDocument();
    expect(screen.getByText(/12,000/)).toBeInTheDocument();
  });

  it('allows dismissing attention items', () => {
    localStorage.clear();
    setup({ channels: [{ id: 1, name: 'c1', status: 'active', breaker_status: 'OPEN' }] });
    expect(screen.getByText(/断路器已触发/)).toBeInTheDocument();

    const dismissBtn = screen.getByTitle('忽略此项关注');
    fireEvent.click(dismissBtn);

    // Item dismissed
    expect(screen.queryByText(/断路器已触发/)).not.toBeInTheDocument();
  });

  it('renders in English when asked', () => {
    setup({ lang: 'en' });

    expect(screen.getByText('Dashboard')).toBeInTheDocument();
  });

  it('does not invent latency or a success rate for a channel with no samples', () => {
    // A configured channel that has served nothing (and was never probed) used to
    // be reported as 160 ms with a 100.0% SLA, which also outranked measured
    // providers in the benchmark list.
    setup({ channels: [{ id: 7, name: 'never-used', status: 'active', breaker_status: 'CLOSED' }] });

    const panel = screen.getByText('服务商实时时延与可用性跑分').closest('.ui-card');
    expect(within(panel).getByText('never-used')).toBeInTheDocument();
    expect(within(panel).getByText('无样本')).toBeInTheDocument();
    expect(within(panel).getByText('SLA 未知')).toBeInTheDocument();
    expect(within(panel).queryByText('160 ms')).not.toBeInTheDocument();
    expect(within(panel).queryByText(/SLA 100\.0%/)).not.toBeInTheDocument();
  });

  it('computes the error budget against the 99.9% target its label names', () => {
    // 1000 requests with 15 failures is 1.5% — fifteen times the 0.1% a three-nines
    // target allows, so the budget must be exhausted instead of reading "25% left".
    setup({
      stats: {
        total_requests: 1000,
        success_requests: 985,
        client_error_requests: 10,
        server_error_requests: 5,
      },
    });

    const card = screen.getByText('剩余错误预算').closest('div');
    expect(within(card).getByText('0%')).toBeInTheDocument();
  });

  it('keeps the error budget at 100% while every request succeeds', () => {
    setup({ stats: { total_requests: 1000, success_requests: 1000 } });

    const card = screen.getByText('剩余错误预算').closest('div');
    expect(within(card).getByText('100%')).toBeInTheDocument();
  });

  it('shows the error budget as unknown when nothing has been served', () => {
    setup({ stats: { total_requests: 0 } });

    const card = screen.getByText('剩余错误预算').closest('div');
    expect(within(card).getByText('—')).toBeInTheDocument();
  });
});

// The live endpoint is additive: with it the cockpit reports in-flight work; without
// it nothing may change. Both directions are pinned below, because a silent fallback
// is otherwise easy to turn into a silent regression (or a toast on every poll).

const livePayload = (overrides = {}) => ({
  active_requests: 0,
  active_streams: 0,
  tokens_per_sec_1s: 0,
  tokens_per_sec_10s: 0,
  tokens_per_sec_60s: 0,
  tokens_last_60s: 0,
  last_token_at: null,
  idle_seconds: 0,
  nodes: 2,
  server_time: '2026-10-22T02:12:03Z',
  ...overrides
});

// Stubs the live endpoint (with an optional failure status); every other admin
// request succeeds with an empty payload so the rest of the dashboard keeps its
// ordinary empty-state behaviour.
function liveFetch(liveData, { status = 200 } = {}) {
  return vi.fn(async (url) => {
    const path = String(url).split('?')[0];
    if (path === '/api/v1/admin/stats/live') {
      if (status !== 200) {
        return { ok: false, status, json: async () => ({ code: status }) };
      }
      return { ok: true, status: 200, json: async () => ({ code: 0, data: liveData }) };
    }
    return { ok: true, status: 200, json: async () => ({ code: 0, data: [] }) };
  });
}

const liveCallCount = (adminFetch) => adminFetch.mock.calls
  .filter(([url]) => String(url).split('?')[0] === '/api/v1/admin/stats/live').length;

describe('DashboardView live telemetry', () => {
  it('polls the live endpoint and renders the generating badge', async () => {
    const adminFetch = liveFetch(livePayload({
      active_requests: 3,
      active_streams: 2,
      tokens_per_sec_10s: 41.3,
      tokens_last_60s: 900
    }));
    setup({ adminFetch });

    await waitFor(() => expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/stats/live'));

    // Two streams in flight: the badge names them, and the TPM card switches from
    // the 60s mean to the 10s rate plus the in-flight count.
    expect(await screen.findByText('生成中 2')).toBeInTheDocument();
    expect(screen.getByText('当前 41 tok/s · 2 个生成中')).toBeInTheDocument();
    // Concurrency is reported next to the channel count, not instead of it.
    expect(screen.getByText('当前并发 3')).toBeInTheDocument();
    // The static badge must be gone once live data speaks.
    expect(screen.queryByText('LIVE 60s')).not.toBeInTheDocument();
  });

  it('reports idle instead of a live window when nothing is moving', async () => {
    const adminFetch = liveFetch(livePayload({ idle_seconds: 42 }));
    setup({ adminFetch });

    expect(await screen.findByText('空闲')).toBeInTheDocument();
    expect(screen.getByText('空闲 · 最近请求 42s 前')).toBeInTheDocument();
    expect(screen.queryByText('LIVE 60s')).not.toBeInTheDocument();
  });

  it('collapses a long idle period into "over an hour"', async () => {
    const adminFetch = liveFetch(livePayload({ idle_seconds: 7200 }));
    setup({ adminFetch });

    expect(await screen.findByText('空闲 · 最近请求超过 1 小时')).toBeInTheDocument();
  });

  it('keeps the 60s mean when there is recent traffic but no stream in flight', async () => {
    const adminFetch = liveFetch(livePayload({ tokens_last_60s: 2112, tokens_per_sec_10s: 3 }));
    setup({ adminFetch, stats: { current_tpm: 12000 } });

    expect(await screen.findByText('LIVE 60s')).toBeInTheDocument();
    // 12000 TPM / 60 = 200 tok/s, the pre-existing 60s mean.
    expect(screen.getByText('~200 tok/s')).toBeInTheDocument();
    expect(screen.queryByText(/个生成中/)).not.toBeInTheDocument();
  });

  it('uses the same live labels in English', async () => {
    const adminFetch = liveFetch(livePayload({ active_streams: 2, active_requests: 1, tokens_per_sec_10s: 41.3 }));
    setup({ adminFetch, lang: 'en' });

    expect(await screen.findByText('Generating 2')).toBeInTheDocument();
    expect(screen.getByText('Current 41 tok/s · 2 streaming')).toBeInTheDocument();
    expect(screen.getByText('Concurrency 1')).toBeInTheDocument();
  });

  it('degrades silently when the live endpoint is missing', async () => {
    // An older gateway answers 404. The dashboard must keep its previous 60s-window
    // behaviour: no error toast, no idle claim, no crash.
    const adminFetch = liveFetch(null, { status: 404 });
    const showToast = vi.fn();
    setup({ adminFetch, showToast });

    await waitFor(() => expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/stats/live'));

    expect(screen.getByText('运营总览')).toBeInTheDocument();
    expect(screen.getByText('LIVE 60s')).toBeInTheDocument();
    expect(screen.queryByText('空闲')).not.toBeInTheDocument();
    expect(screen.queryByText(/当前并发/)).not.toBeInTheDocument();
    expect(showToast).not.toHaveBeenCalled();
  });

  it('degrades silently when the live request throws', async () => {
    const adminFetch = vi.fn(async (url) => {
      if (String(url).endsWith('/stats/live')) throw new Error('network down');
      return { ok: true, status: 200, json: async () => ({ code: 0, data: [] }) };
    });
    setup({ adminFetch });

    await waitFor(() => expect(adminFetch).toHaveBeenCalledWith('/api/v1/admin/stats/live'));
    expect(screen.getByText('LIVE 60s')).toBeInTheDocument();
    expect(screen.queryByText(/空闲/)).not.toBeInTheDocument();
  });

  // Timers are faked here: the cadence itself is the contract, so real 2s/15s
  // waits would be both slow and flaky.
  describe('polling cadence', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
      delete document.hidden;
    });

    it('polls every 2s while mounted', async () => {
      const adminFetch = liveFetch(livePayload());
      setup({ adminFetch });

      await act(async () => {});
      expect(liveCallCount(adminFetch)).toBe(1);

      await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
      expect(liveCallCount(adminFetch)).toBe(2);

      await act(async () => { await vi.advanceTimersByTimeAsync(2000); });
      expect(liveCallCount(adminFetch)).toBe(3);
    });

    it('pauses while the tab is hidden and catches up on return', async () => {
      const adminFetch = liveFetch(livePayload());
      setup({ adminFetch });

      await act(async () => {});
      expect(liveCallCount(adminFetch)).toBe(1);

      Object.defineProperty(document, 'hidden', { configurable: true, get: () => true });
      await act(async () => { document.dispatchEvent(new Event('visibilitychange')); });
      const hiddenAt = liveCallCount(adminFetch);
      await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
      expect(liveCallCount(adminFetch)).toBe(hiddenAt);

      // Coming back visible fetches immediately rather than waiting for the tick.
      Object.defineProperty(document, 'hidden', { configurable: true, get: () => false });
      await act(async () => { document.dispatchEvent(new Event('visibilitychange')); });
      expect(liveCallCount(adminFetch)).toBe(hiddenAt + 1);
    });

    it('stops polling after unmount', async () => {
      const adminFetch = liveFetch(livePayload());
      const { unmount } = setup({ adminFetch });

      await act(async () => {});
      const before = liveCallCount(adminFetch);
      expect(before).toBe(1);

      unmount();
      await act(async () => { await vi.advanceTimersByTimeAsync(6000); });
      expect(liveCallCount(adminFetch)).toBe(before);
    });

    it('refreshes the time-series and the audit stream every 15s', async () => {
      const adminFetch = liveFetch(livePayload());
      setup({ adminFetch });

      const countPath = (suffix) => adminFetch.mock.calls
        .filter(([url]) => String(url).split('?')[0].endsWith(suffix)).length;

      await act(async () => {});
      const timeseriesBefore = countPath('/admin/stats/timeseries');
      const logsBefore = countPath('/admin/logs');
      expect(timeseriesBefore).toBeGreaterThan(0);
      expect(logsBefore).toBeGreaterThan(0);

      await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
      expect(countPath('/admin/stats/timeseries')).toBeGreaterThan(timeseriesBefore);
      expect(countPath('/admin/logs')).toBeGreaterThan(logsBefore);
    });
  });
});


// ---------------------------------------------------------------------------
// Key drill-down must filter by numeric id
// ---------------------------------------------------------------------------

// The key list only carries display masks, so the drill-down cannot send `key=`.
// The backend resolves `key_id` (logs already do; stats now accept it too), which
// makes the row id the only value that can scope the dashboard to one key.
const MASK = '3a4b5c6d***9f8e';

const drillKeys = [
  { id: 7, key: MASK, key_fingerprint: 'kfp_7', tenant_id: 'acme', name: 'acme-key', status: 'active', rpm: 60 },
  { id: 8, key: 'ffee0011***22aa', key_fingerprint: 'kfp_8', tenant_id: 'globex', status: 'active' },
];

function drillFetch() {
  return vi.fn(async (url) => {
    const path = String(url).split('?')[0];
    if (path === '/api/v1/admin/stats/live') {
      return { ok: true, status: 200, json: async () => ({ code: 0, data: livePayload() }) };
    }
    if (path === '/api/v1/admin/stats/overview') {
      return { ok: true, status: 200, json: async () => ({ code: 0, data: {} }) };
    }
    return { ok: true, status: 200, json: async () => ({ code: 0, data: [] }) };
  });
}

const queriedUrls = (adminFetch) => adminFetch.mock.calls.map(([url]) => String(url));

describe('DashboardView key drill-down', () => {
  it('offers key ids — never masked values — as drill-down options', () => {
    setup({ keys: drillKeys, adminFetch: drillFetch() });

    const select = screen.getByLabelText('下钻密钥');
    const values = Array.from(select.options).map(o => o.value);
    // The empty option is the platform-wide view; the rest are row ids.
    expect(values).toEqual(['', '7', '8']);
    expect(values.join(',')).not.toContain('***');
  });

  it('filters logs, timeseries and overview by key_id', async () => {
    const adminFetch = drillFetch();
    setup({ keys: drillKeys, adminFetch });

    fireEvent.change(screen.getByLabelText('下钻密钥'), { target: { value: '7' } });

    await waitFor(() => {
      const urls = queriedUrls(adminFetch);
      expect(urls.some(u => u.startsWith('/api/v1/admin/logs') && u.includes('key_id=7'))).toBe(true);
      expect(urls.some(u => u.startsWith('/api/v1/admin/stats/timeseries') && u.includes('key_id=7'))).toBe(true);
      expect(urls.some(u => u.startsWith('/api/v1/admin/stats/overview') && u.includes('key_id=7'))).toBe(true);
    });

    // The old `key=` parameter (which would have sent the mask) must be gone.
    const keyed = queriedUrls(adminFetch).filter(u => /[?&]key=/.test(u));
    expect(keyed).toEqual([]);
    expect(queriedUrls(adminFetch).join('\n')).not.toContain('***');
    expect(queriedUrls(adminFetch).join('\n')).not.toContain(MASK);
  });

  it('resolves the drill-down label from the row id', async () => {
    const adminFetch = drillFetch();
    setup({ keys: drillKeys, adminFetch });

    fireEvent.change(screen.getByLabelText('下钻密钥'), { target: { value: '7' } });

    // The telemetry card names the selected key's tenant, resolved by id.
    expect(await screen.findByText(/密钥实时速率遥测 · acme/)).toBeInTheDocument();
  });
});
