const hasCount = value => typeof value === 'number' && Number.isFinite(value) && value >= 0;
const failedLog = log => log && (log.status_code >= 400 || (log.status_code ?? 200) < 200 || log.error_message);

export default function dashboardAttentionSummary({ channels = [], logs = [], stats = {}, timeseriesData, timeframe }) {
  const series = timeseriesData?.timeframe === timeframe ? timeseriesData : null;
  const summary = series?.attention;
  const errorLogs = logs.filter(failedLog);
  const windowCounts = hasCount(summary?.failed_requests) && hasCount(summary?.total_requests);
  const overviewCounts = hasCount(stats.total_requests) && hasCount(stats.client_error_requests) && hasCount(stats.server_error_requests);
  const scope = windowCounts ? 'window' : overviewCounts ? 'overview' : 'page';
  const aggregateCodes = summary?.error_count_by_code;
  const codeScope = aggregateCodes && typeof aggregateCodes === 'object' ? 'window' : 'page';
  const errorCountByCode = codeScope === 'window' ? aggregateCodes : errorLogs.reduce((counts, log) => {
    const code = log.status_code ?? 'unknown';
    counts[code] = (counts[code] || 0) + 1;
    return counts;
  }, {});
  return {
    scope, codeScope, timeframe, errorCountByCode,
    tripped: hasCount(series?.tripped_channels) ? series.tripped_channels : channels.filter(ch => ['OPEN', 'HALF-OPEN'].includes(String(ch.breaker_status || '').toUpperCase())).length,
    disabled: hasCount(series?.disabled_channels) ? series.disabled_channels : channels.filter(ch => ch.status !== 'active').length,
    failed: windowCounts ? summary.failed_requests : overviewCounts ? stats.client_error_requests + stats.server_error_requests : errorLogs.length,
    recentTotal: windowCounts ? summary.total_requests : overviewCounts ? stats.total_requests : logs.length,
    rateLimited: hasCount(summary?.rate_limited_requests) ? summary.rate_limited_requests : errorCountByCode['429'] || 0,
    rateLimitScope: hasCount(summary?.rate_limited_requests) ? 'window' : codeScope,
  };
}

export function attentionScopeLabel(scope, timeframe, isZh) {
  if (scope === 'window') return isZh ? `最近 ${timeframe} 的全量请求` : `all requests in the last ${timeframe}`;
  if (scope === 'overview') return isZh ? '全部调用日志' : 'all invocation logs';
  return isZh ? '当前页日志样本' : 'the current log page sample';
}
