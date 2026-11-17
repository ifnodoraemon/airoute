import { describe, expect, it } from 'vitest';
import summarize from './dashboardAttentionSummary';
import buildDashboardAttention from './dashboardAttention';

const channels = [{ status: 'active', breaker_status: 'OPEN' }];
const logs = [{ status_code: 503 }, { status_code: 429 }];
const series = attention => ({ timeframe: '1h', tripped_channels: 0, disabled_channels: 0, attention });

describe('dashboard attention sources', () => {
  it('uses the complete window summary regardless of log pagination', () => {
    const summary = { total_requests: 5000, failed_requests: 200, rate_limited_requests: 150, error_count_by_code: { 429: 150, 502: 50 } };
    const attention = summarize({ channels, logs, stats: { total_requests: 9000 }, timeseriesData: series(summary), timeframe: '1h' });
    expect(attention).toMatchObject({ scope: 'window', codeScope: 'window', failed: 200, recentTotal: 5000, tripped: 0, rateLimited: 150, errorCountByCode: { 429: 150, 502: 50 } });
    expect(summarize({ channels, logs: [], timeseriesData: series(summary), timeframe: '1h' })).toEqual(attention);
    const { attentionItems } = buildDashboardAttention({ attention, channels, isZh: false });
    expect(attentionItems.find(item => item.key === 'error-codes').detail).toContain('all requests in the last 1h');
    expect(attentionItems.find(item => item.key === 'rate-limits').title).toContain('150');
  });

  it('preserves authoritative zeros and an empty histogram', () => {
    const attention = summarize({ channels, logs, timeseriesData: series({ total_requests: 100, failed_requests: 0, rate_limited_requests: 0, error_count_by_code: {} }), timeframe: '1h' });
    expect(attention).toMatchObject({ failed: 0, tripped: 0, rateLimited: 0, errorCountByCode: {} });
  });

  it('uses overview totals when window counts are absent', () => {
    const attention = summarize({ channels, logs, stats: { total_requests: 500, client_error_requests: 10, server_error_requests: 20 } });
    expect(attention).toMatchObject({ scope: 'overview', failed: 30, recentTotal: 500, codeScope: 'page', tripped: 1 });
  });

  it('labels the page sample when aggregate fields are missing or stale', () => {
    const attention = summarize({ channels, logs, timeframe: '24h', timeseriesData: series({ total_requests: 0, failed_requests: 0, error_count_by_code: {} }) });
    expect(attention).toMatchObject({ scope: 'page', failed: 2, recentTotal: 2, errorCountByCode: { 429: 1, 503: 1 }, tripped: 1 });
  });
});
