import { useCallback, useEffect, useRef, useState } from 'react';
import { extractErrorMessage } from '../api/client';
import { translations } from '../i18n';
import ErrorState from './ErrorState';

export default function MyUsageTrend({ adminFetch, balance, dailyCost, isZh }) {
  const copy = translations[isZh ? 'zh' : 'en'].common;
  const [data, setData] = useState(null);
  const [error, setError] = useState('');
  const sequence = useRef(0);
  const load = useCallback(async () => {
    const current = ++sequence.current;
    setError('');
    try {
      const response = await adminFetch('/api/v1/admin/stats/timeseries?timeframe=24h');
      const body = await response.json().catch(() => null);
      if (!response.ok || body?.code !== 0 || !Array.isArray(body.data?.buckets)) {
        throw new Error(extractErrorMessage(body, `HTTP ${response.status}`));
      }
      if (sequence.current === current) setData(body.data);
    } catch (err) {
      if (sequence.current === current) setError(err.message);
    }
  }, [adminFetch]);
  const cancel = useCallback(() => { sequence.current++; }, []);
  useEffect(() => {
    load();
    return cancel;
  }, [load, cancel]);

  const buckets = data?.buckets || [];
  const max = Math.max(0, ...buckets.map(bucket => bucket.requests || 0));
  const failures = buckets.reduce((sum, bucket) => sum + (bucket.errors || 0), 0);
  const requests = buckets.reduce((sum, bucket) => sum + (bucket.requests || 0), 0);
  const days = dailyCost > 0 && balance > 0 ? balance / dailyCost : null;
  return (
    <section aria-label={copy.personalTrend} className="ui-card p-4 space-y-3">
      <h3 className="ui-section-title">{copy.personalTrend}</h3>
      {error && <ErrorState message={error} onRetry={load} isZh={isZh} />}
      {!data && !error && <p role="status" className="ui-meta">{copy.loading}</p>}
      {data && <>
        <p className="ui-meta">{copy.requestSummary.replace('{requests}', requests).replace('{failures}', failures)}</p>
        <svg viewBox="0 0 600 80" role="img" aria-label={copy.requestsPerInterval} className="w-full h-20 text-[#355CFF] dark:text-[#91A7FF]">
          {buckets.map((bucket, index) => {
            const height = max > 0 ? 72 * (bucket.requests || 0) / max : 0;
            return <rect key={index} x={600 * index / buckets.length} y={80 - height} width={Math.max(1, 600 / buckets.length - 2)} height={height} fill="currentColor"><title>{bucket.label}: {bucket.requests || 0}</title></rect>;
          })}
        </svg>
      </>}
      {days != null && <p className="ui-meta">{copy.balanceForecast.replace('{days}', days.toFixed(1))}</p>}
    </section>
  );
}
