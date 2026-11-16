import { WifiOff, RefreshCw } from 'lucide-react';
import { useConsoleContext } from './ConsoleContext';
import { translations } from '../i18n';

// Global stale-data banner. The 8s poll already tracks success/failure; this
// surfaces it so operators are not misled by silently stale channels/keys/stats
// during a gateway outage, and offers a one-click retry.
export default function ConnectionBanner() {
  const { connection, fetchData, isZh } = useConsoleContext();
  const copy = translations[isZh ? 'zh' : 'en'].common;
  if (!connection || (connection.ok && !connection.offline)) return null;
  const updated = connection.lastUpdated
    ? new Date(connection.lastUpdated).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : null;
  const label = connection.offline
    ? copy.offline
    : copy.gatewayLost;
  return (
    <div
      role="alert"
      className="flex items-center justify-between gap-3 px-4 sm:px-6 py-2 bg-amber-50 dark:bg-amber-950/40 border-b border-amber-200 dark:border-amber-900/60 text-amber-800 dark:text-amber-200 text-xs"
    >
      <span className="flex items-center gap-2 min-w-0">
        <WifiOff className="w-3.5 h-3.5 shrink-0" />
        <span className="truncate">
          {label}{updated ? ` · ${copy.lastUpdated} ${updated}` : ''}
        </span>
      </span>
      <button
        type="button"
        onClick={() => fetchData?.()}
        className="flex items-center gap-1 font-medium hover:underline shrink-0"
      >
        <RefreshCw className="w-3 h-3" />{copy.retry}
      </button>
    </div>
  );
}
