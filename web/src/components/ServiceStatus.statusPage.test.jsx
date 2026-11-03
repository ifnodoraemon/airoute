// Regression tests for the public status page's breaker state and latency column.
//
// Started as the BUG-WEB-02 / BUG-WEB-03 probes; now asserts the CORRECT behaviour
// (they fail on the pre-fix code).
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/ServiceStatus.statusPage.test.jsx

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import ServiceStatus from './ServiceStatus';

function jsonResponse(body, status = 200) {
  return { ok: status < 400, status, json: async () => body };
}

// An *enabled* channel whose breaker is OPEN: the gateway is routing around it,
// which is exactly what a status page has to show.
const TRIPPED_CHANNEL = {
  id: 7,
  name: 'upstream-primary',
  type: 'openai',
  status: 'active',
  breaker_status: 'OPEN',
};

const MODEL_ROUTES = [{ model: 'gpt-x', modality: 'chat', providers: [{ status: 'active' }] }];

function renderPage({ channels, probeLatencies = {} }) {
  return render(
    <ServiceStatus
      isStandalone={false}
      lang="zh"
      channels={channels}
      probeLatencies={probeLatencies}
      modelRoutes={MODEL_ROUTES}
    />,
  );
}

describe('ServiceStatus status page', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ code: 0, data: { status: 'operational', models: [] } })));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reports a breaker-OPEN channel as tripped and lists it as tripped in the table', async () => {
    renderPage({ channels: [TRIPPED_CHANNEL] });

    // Page-level summary.
    expect(await screen.findByText('存在已熔断渠道')).toBeInTheDocument();
    expect(screen.queryByText('未发现熔断')).toBeNull();
    expect(screen.queryByText('当前未发现熔断渠道')).toBeNull();

    // Per-channel row.
    expect(screen.getByText('已熔断跳闸')).toBeInTheDocument();
    expect(screen.queryByText('健康闭合')).toBeNull();
  });

  it('distinguishes a HALF-OPEN breaker from a closed one', async () => {
    renderPage({ channels: [{ ...TRIPPED_CHANNEL, breaker_status: 'HALF-OPEN' }] });

    expect(await screen.findByText('半开探测中')).toBeInTheDocument();
    expect(screen.queryByText('健康闭合')).toBeNull();
    expect(screen.getByText('存在已熔断渠道')).toBeInTheDocument();
  });

  it('shows a CLOSED breaker as healthy', async () => {
    renderPage({ channels: [{ ...TRIPPED_CHANNEL, breaker_status: 'CLOSED' }] });

    expect(await screen.findByText('健康闭合')).toBeInTheDocument();
    expect(screen.getByText('未发现熔断')).toBeInTheDocument();
    expect(screen.queryByText('已熔断跳闸')).toBeNull();
  });

  it('does not invent a latency for a channel that was never probed', async () => {
    renderPage({ channels: [{ ...TRIPPED_CHANNEL, breaker_status: 'CLOSED' }], probeLatencies: {} });

    expect(await screen.findByText('未探测')).toBeInTheDocument();
    expect(screen.queryByText('~35 ms (探针正常)')).toBeNull();
  });

  it('shows the measured latency when a probe has run', async () => {
    renderPage({
      channels: [{ ...TRIPPED_CHANNEL, breaker_status: 'CLOSED' }],
      probeLatencies: { 7: { latency_ms: 128, error: '' } },
    });

    expect(await screen.findByText('128 ms')).toBeInTheDocument();
    expect(screen.queryByText('未探测')).toBeNull();
  });
});
