// AI Copilot harness reads the console data with the right field names.
//
// Regression for BUG-H05 (breaker casing / cache-hit rate / currency / invented TTFT)
// and BUG-H06 (route provider field name). These assertions fail on the pre-fix code.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/copilotHarness.contract.test.jsx

import { describe, it, expect } from 'vitest';
import { executeHarnessTool } from './copilotHarness';

const TRIPPED_CHANNEL = {
  id: 1,
  name: 'volc-primary',
  type: 'openai',
  status: 'active',
  breaker_status: 'OPEN',
};

const HEALTHY_CHANNEL = {
  id: 2,
  name: 'aliyun-backup',
  type: 'openai',
  status: 'active',
  breaker_status: 'CLOSED',
};

// Exactly the shape /api/v1/admin/stats/overview returns (storage.StatsOverview).
const STATS = {
  total_requests: 1000,
  total_tokens: 500000,
  prompt_tokens: 400000,
  completion_tokens: 100000,
  total_cached_tokens: 200000,
  total_cost: 88.5,
  avg_ttft_ms: 0,
  ttft_samples: 0,
};

function context(overrides = {}) {
  return {
    channels: [],
    models: ['gpt-x'],
    modelRoutes: [],
    logs: [],
    stats: {},
    prices: [],
    origin: 'https://gateway.example',
    baseUrl: 'https://gateway.example/v1',
    isZh: true,
    ...overrides,
  };
}

describe('get_cluster_telemetry reflects the real gateway payload', () => {
  it('counts a breaker-OPEN channel as tripped and lowers the health score', async () => {
    const res = await executeHarnessTool('get_cluster_telemetry', {}, context({
      channels: [TRIPPED_CHANNEL, HEALTHY_CHANNEL],
      stats: STATS,
    }));

    expect(res.result.cluster_summary.tripped_channels).toBe(1);
    expect(res.result.cluster_summary.health_score).toBe(50);
  });

  it('treats HALF-OPEN as degraded too', async () => {
    const res = await executeHarnessTool('get_cluster_telemetry', {}, context({
      channels: [{ ...TRIPPED_CHANNEL, breaker_status: 'HALF-OPEN' }, HEALTHY_CHANNEL],
      stats: STATS,
    }));
    expect(res.result.cluster_summary.tripped_channels).toBe(1);
  });

  it('derives the cache-hit rate from the cached prompt tokens', async () => {
    const res = await executeHarnessTool('get_cluster_telemetry', {}, context({ stats: STATS }));
    // 200000 cached of 400000 prompt tokens.
    expect(res.result.performance_metrics.cache_hit_rate).toBe('50.0%');

    const noSamples = await executeHarnessTool('get_cluster_telemetry', {}, context({
      stats: { ...STATS, prompt_tokens: 0, total_cached_tokens: 0 },
    }));
    expect(noSamples.result.performance_metrics.cache_hit_rate).toBe('n/a');
  });

  it('labels the spend as CNY and does not invent a TTFT baseline', async () => {
    const res = await executeHarnessTool('get_cluster_telemetry', {}, context({ stats: STATS }));
    expect(res.result.performance_metrics.currency).toBe('CNY');
    expect(res.result.performance_metrics.total_cost).toBe(88.5);
    expect(res.result.performance_metrics.total_cost_usd).toBeUndefined();
    // No samples (ttft_samples 0 / avg 0) must stay "no data", not 120 ms.
    expect(res.result.performance_metrics.avg_ttft_ms).toBeNull();

    const measured = await executeHarnessTool('get_cluster_telemetry', {}, context({
      stats: { ...STATS, avg_ttft_ms: 240, ttft_samples: 30 },
    }));
    expect(measured.result.performance_metrics.avg_ttft_ms).toBe(240);
  });

  it('reports the real breaker state in the per-channel list', async () => {
    const res = await executeHarnessTool('get_cluster_telemetry', {}, context({ channels: [TRIPPED_CHANNEL] }));
    expect(res.result.channels[0].breaker_status).toBe('OPEN');
  });
});

describe('audit_routing_spof names the real provider channels', () => {
  it('uses channel_name for the vulnerable route primary channel', async () => {
    const route = {
      model: 'gpt-x',
      modality: 'chat',
      providers: [{ channel_id: 7, channel_name: 'volc-primary', priority: 1, weight: 80, status: 'active' }],
      fallback_model: '',
    };

    const res = await executeHarnessTool('audit_routing_spof', {}, context({
      modelRoutes: [route],
      prices: [{ model: 'gpt-x', group_name: 'default', prompt_price: 2, completion_price: 8 }],
    }));

    expect(res.result.spof_vulnerabilities[0].primary_channel).toBe('volc-primary');
  });

  it('lists the real channels of a redundant route', async () => {
    const route = {
      model: 'gpt-y',
      providers: [
        { channel_id: 7, channel_name: 'volc-primary', priority: 1, weight: 80 },
        { channel_id: 8, channel_name: 'aliyun-backup', priority: 2, weight: 20 },
      ],
    };

    const res = await executeHarnessTool('audit_routing_spof', {}, context({
      modelRoutes: [route],
      prices: [{ model: 'gpt-y' }],
    }));

    expect(res.result.safe_routes_sample[0].providers.map((p) => p.channel))
      .toEqual(['volc-primary', 'aliyun-backup']);
  });

  it('falls back to channel_id when a provider has no name', async () => {
    const res = await executeHarnessTool('audit_routing_spof', {}, context({
      modelRoutes: [{ model: 'gpt-z', providers: [{ channel_id: 42, priority: 1, weight: 100 }] }],
      prices: [{ model: 'gpt-z' }],
    }));
    expect(res.result.spof_vulnerabilities[0].primary_channel).toBe('channel#42');
  });
});
