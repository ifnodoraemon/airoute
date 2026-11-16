import { AlertCircle, RefreshCw } from 'lucide-react';
import { translations } from '../i18n';

// Persistent inline error state for primary data loads. A auto-dismissing toast
// alone leaves the operator with stale/empty content and no way to retry once it
// fades; this keeps the failure visible and actionable.
export default function ErrorState({ message, onRetry, isZh }) {
  const copy = translations[isZh ? 'zh' : 'en'].common;
  return (
    <div
      role="alert"
      className="ui-card p-6 flex flex-col items-center justify-center text-center gap-3 border border-rose-200 dark:border-rose-900/60 bg-rose-50/50 dark:bg-rose-950/20"
    >
      <AlertCircle className="w-6 h-6 text-rose-500" />
      <p className="text-sm text-rose-700 dark:text-rose-300">{message || copy.loadFailed}</p>
      {onRetry && (
        <button type="button" onClick={onRetry} className="ui-btn-secondary flex items-center gap-1.5">
          <RefreshCw className="w-3.5 h-3.5" />{copy.retry}
        </button>
      )}
    </div>
  );
}
