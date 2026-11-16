import { Activity, RefreshCw } from 'lucide-react';
import { translations } from '../i18n';
import useInfraHealth from '../hooks/useInfraHealth';
import ErrorState from './ErrorState';

// Status presentation for the infrastructure components reported by
// /api/v1/admin/system/middlewares. Unknown statuses fall back to a neutral badge.
const STATUS_STYLES = {
  operational: { dot: 'bg-emerald-500', badge: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' },
  degraded: { dot: 'bg-amber-500', badge: 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300' },
  configured: { dot: 'bg-[#355CFF]', badge: 'bg-[#EEF2FF] text-[#355CFF] dark:bg-[#242D48] dark:text-[#91A7FF]' },
  disabled: { dot: 'bg-[#A1A7B0]', badge: 'bg-[#F0F1F2] text-[#656B75] dark:bg-[#22252B] dark:text-[#A1A7B0]' },
};

function statusStyle(status) {
  return STATUS_STYLES[status] || { dot: 'bg-[#A1A7B0]', badge: 'bg-[#F0F1F2] text-[#656B75] dark:bg-[#22252B] dark:text-[#A1A7B0]' };
}

// InfraHealthPanel surfaces the real-time health of the gateway's underlying
// infrastructure (database, cache, queue, breaker, billing, notification). It
// reads the admin-only /system/middlewares endpoint; no admin data is exposed to
// regular users because the tab itself is admin-gated in the navigation config.
export default function InfraHealthPanel({ adminFetch, showToast, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const copy = translations[isZh ? 'zh' : 'en'].common;
  const { components, loading, error, lastUpdated, load } = useInfraHealth(adminFetch, showToast);

  const degraded = components.filter((c) => c.status === 'degraded').length;
  const cardClass = 'rounded-xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-4';

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{copy.infraTitle}</h2>
          <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0]">
            {copy.infraDescription}
          </p>
        </div>
        <button type="button" onClick={load} disabled={loading} className="inline-flex items-center space-x-1.5 px-2.5 py-1.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs text-[#656B75] dark:text-[#A1A7B0] disabled:opacity-50">
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>{copy.refresh}</span>
        </button>
      </div>

      {lastUpdated && <p className="ui-meta">{copy.lastUpdated} {new Date(lastUpdated).toLocaleTimeString()}</p>}
      {loading && components.length === 0 && <p role="status" className="ui-meta">{copy.loading}</p>}
      {error && <ErrorState message={copy.infraLoadFailed + error} onRetry={load} isZh={isZh} />}

      {!error && degraded > 0 && (
        <div className="rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-900/40 dark:bg-amber-950/30 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          {copy.infraDegraded.replace('{count}', degraded)}
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {components.map((component) => {
          const style = statusStyle(component.status);
          return (
            <div key={component.id} className={cardClass}>
              <div className="flex items-center justify-between">
                <div className="flex items-center space-x-2 min-w-0">
                  <span className={`w-2 h-2 rounded-full shrink-0 ${style.dot}`} />
                  <span className="text-xs font-semibold text-[#202124] dark:text-[#F1F2F4] truncate">{component.name}</span>
                </div>
                <span className={`px-1.5 py-0.5 rounded text-[10px] shrink-0 ${style.badge}`}>{copy[component.status] || component.status || copy.unknown}</span>
              </div>
              <div className="mt-1.5 text-[10px] uppercase tracking-wide text-[#656B75] dark:text-[#A1A7B0]">{component.category}</div>
              <div className="mt-1 text-[11px] font-mono text-[#355CFF] dark:text-[#91A7FF]">{component.mode}</div>
              {typeof component.latency_ms === 'number' && component.latency_ms > 0 && (
                <div className="mt-1 text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{copy.probeLatency} {component.latency_ms.toFixed(2)} ms</div>
              )}
              <p className="mt-1.5 text-[11px] text-[#656B75] dark:text-[#A1A7B0] leading-relaxed">{component.description}</p>
            </div>
          );
        })}
        {!loading && components.length === 0 && !error && (
          <div className={`${cardClass} md:col-span-2 flex items-center justify-center text-[11px] text-[#656B75] dark:text-[#A1A7B0]`}>
            <Activity className="w-3.5 h-3.5 mr-1.5" />
            {copy.infraEmpty}
          </div>
        )}
      </div>
    </div>
  );
}
