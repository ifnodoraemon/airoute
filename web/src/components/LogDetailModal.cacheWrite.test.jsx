// Log inspector billing formula: it must reconstruct the charge, including the
// prompt-cache write premium, and the token split must show the cached writes.
//
// Regression for BUG-H07 (was: the formula omitted cache_write_price and counted
// written tokens as ordinary "uncached" input). These assertions fail on the pre-fix
// code.
//
// Runner:
//   cd web && npx vitest run --configLoader runner src/components/LogDetailModal.cacheWrite.test.jsx

import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import LogDetailModal from './LogDetailModal';

// prompt 1000 = 500 uncached + 200 cache-read + 300 cache-write, output 100.
// Charged at prompt 2/1M, cache read 0.2/1M, cache write 2.5/1M, output 8/1M:
//   500×2 + 200×0.2 + 300×2.5 + 100×8 = 1000 + 40 + 750 + 800 = 2590 → ¥0.00259
const CACHE_WRITE_LOG = {
  id: 1,
  trace_id: 'tr-1',
  session_id: 'sess-1',
  model: 'claude-x',
  channel: 'upstream-a',
  status_code: 200,
  duration_ms: 12,
  ttft_ms: 5,
  prompt_tokens: 1000,
  completion_tokens: 100,
  cached_tokens: 200,
  cache_creation_tokens: 300,
  total_tokens: 1100,
  cost: 0.00259,
  is_off_peak: false,
  usage_source: 'upstream',
  pricing_snapshot: {
    model: 'claude-x',
    group_name: 'default',
    price_group: 'default',
    currency: 'CNY',
    prompt_price: 2,
    completion_price: 8,
    cache_read_price: 0.2,
    cache_write_price: 2.5,
    cache_creation_tokens: 300,
    fixed_price: 0,
    group_multiplier: 1,
    off_peak_discount: 1,
    billable_units: 1,
  },
};

function renderLog(log) {
  return render(<LogDetailModal log={log} onClose={() => {}} onCopy={() => {}} lang="zh" />).container;
}

describe('LogDetailModal billing formula with a cache write', () => {
  it('includes the cache-write term so the formula reconstructs the charged amount', () => {
    const container = renderLog(CACHE_WRITE_LOG);
    const text = container.textContent;

    expect(text).toContain(
      '(未缓存 500 × ¥2/1M + 缓存命中 200 × ¥0.2/1M + 缓存写入 300 × ¥2.5/1M + 输出 100 × ¥8/1M) × 1 = ¥0.0026',
    );
  });

  it('splits the cache-write tokens out of the uncached input', () => {
    const container = renderLog(CACHE_WRITE_LOG);
    const text = container.textContent;

    expect(text).toContain('未缓存输入: 500');
    expect(text).toContain('缓存写入: 300');
    expect(text).toContain('缓存命中: 200');
  });

  it('keeps the old two-part formula for a request without cache writes', () => {
    const container = renderLog({
      ...CACHE_WRITE_LOG,
      cache_creation_tokens: 0,
      pricing_snapshot: { ...CACHE_WRITE_LOG.pricing_snapshot, cache_creation_tokens: 0 },
    });
    const text = container.textContent;

    expect(text).toContain('(未缓存 800 × ¥2/1M + 缓存命中 200 × ¥0.2/1M + 输出 100 × ¥8/1M) × 1 = ¥0.0026');
    expect(text).not.toContain('缓存写入');
  });
});
