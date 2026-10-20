import { useEffect, useState } from 'react';
import { X, RefreshCw, AlertTriangle } from 'lucide-react';

// TraceSnapshotModal shows the redacted request/response snapshot archived for a
// trace id, which is usually the fastest way to diagnose a failed call.
export default function TraceSnapshotModal({ isOpen, traceId, onClose, lang = 'zh', adminFetch }) {
  const isZh = lang === 'zh';
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  // Allow searching by any correlation id, not just the one that opened the modal.
  const [queryField, setQueryField] = useState('trace_id');
  const [queryValue, setQueryValue] = useState(traceId || '');

  const load = async (field = queryField, value = queryValue || traceId) => {
    if (!value) {
      setRows([]);
      return;
    }
    setLoading(true);
    setError('');
    try {
      const params = new URLSearchParams({ limit: '20' });
      params.set(field, value);
      const res = await adminFetch(`/api/v1/admin/traces?${params.toString()}`);
      const data = await res.json();
      if (!res.ok || data.code !== 0) {
        throw new Error(data.error || `HTTP ${res.status}`);
      }
      setRows(data.data || []);
    } catch (e) {
      setError(e.message || String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen) {
      setQueryValue(traceId || '');
      setQueryField('trace_id');
      load('trace_id', traceId);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, traceId]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4 backdrop-blur-sm">
      <div className="w-full max-w-3xl max-h-[85vh] overflow-hidden rounded-2xl bg-white dark:bg-slate-900 shadow-2xl border border-slate-200 dark:border-slate-800 flex flex-col">
        <div className="flex items-center justify-between px-5 py-3 border-b border-slate-200 dark:border-slate-800">
          <div>
            <div className="text-sm font-bold text-slate-900 dark:text-white">
              {isZh ? '链路快照' : 'Trace Snapshot'}
            </div>
            <div className="ui-meta font-mono">{traceId}</div>
          </div>
          <div className="flex items-center space-x-1">
            <button onClick={load} className="ui-icon-btn" title={isZh ? '刷新' : 'Refresh'}>
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
            </button>
            <button onClick={onClose} className="ui-icon-btn">
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 px-4 py-2 border-b border-slate-200 dark:border-slate-800">
          <select
            value={queryField}
            onChange={(e) => setQueryField(e.target.value)}
            className="ui-field ui-field-sm w-auto cursor-pointer"
          >
            <option value="trace_id">Trace ID</option>
            <option value="chat_id">Chat ID</option>
            <option value="session_id">Session ID</option>
          </select>
          <input
            value={queryValue}
            onChange={(e) => setQueryValue(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') load(); }}
            placeholder={isZh ? '输入关联 ID 后回车查询' : 'Enter a correlation id and press Enter'}
            className="ui-field ui-field-sm flex-1 min-w-[180px] font-mono"
          />
          <button
            onClick={() => load()}
            className="ui-btn ui-btn-sm bg-indigo-600 hover:bg-indigo-700 text-white font-semibold"
          >
            {isZh ? '查询' : 'Search'}
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-3 text-xs">
          {error && (
            <div className="rounded-xl border border-rose-200 dark:border-rose-900/60 bg-rose-50 dark:bg-rose-950/30 px-3 py-2 text-rose-700 dark:text-rose-300">
              {error}
            </div>
          )}
          {!error && !loading && rows.length === 0 && (
            <div className="text-slate-500 dark:text-slate-400">
              {isZh
                ? '该链路没有归档快照（仅失败请求默认归档，或已超过保留期）。'
                : 'No archived snapshot for this trace (only failures are archived by default, or it expired).'}
            </div>
          )}
          {rows.map((row) => (
            <div key={row.id} className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
              <div className="flex flex-wrap items-center gap-2 px-3 py-2 bg-slate-50 dark:bg-slate-800/60 text-xs">
                <span className={`px-1.5 py-0.5 rounded font-semibold ${row.status_code >= 400 ? 'bg-rose-100 text-rose-700 dark:bg-rose-950/60 dark:text-rose-300' : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'}`}>
                  HTTP {row.status_code}
                </span>
                <span className="font-mono text-slate-600 dark:text-slate-300">{row.model}</span>
                {row.channel && <span className="text-slate-500 dark:text-slate-400">@ {row.channel}</span>}
                <span className="text-slate-500 dark:text-slate-400">{row.duration_ms}ms</span>
                <span className="ml-auto text-slate-500 dark:text-slate-400">{row.created_at}</span>
              </div>
              {row.error_message && (
                <div className="flex items-start gap-2 px-3 py-2 border-t border-slate-100 dark:border-slate-800 text-rose-700 dark:text-rose-300">
                  <AlertTriangle className="w-3.5 h-3.5 mt-0.5 shrink-0" />
                  <span className="break-all">{row.error_message}</span>
                </div>
              )}
              {row.request_snippet && (
                <div className="border-t border-slate-100 dark:border-slate-800">
                  <div className="px-3 pt-2 ui-section-title">
                    {isZh ? '请求（已脱敏）' : 'Request (redacted)'}
                  </div>
                  <pre className="px-3 py-2 overflow-x-auto font-mono text-xs text-slate-700 dark:text-slate-300 whitespace-pre-wrap break-all">{row.request_snippet}</pre>
                </div>
              )}
              {row.response_snippet && (
                <div className="border-t border-slate-100 dark:border-slate-800">
                  <div className="px-3 pt-2 ui-section-title">
                    {isZh ? '响应（已脱敏）' : 'Response (redacted)'}
                  </div>
                  <pre className="px-3 py-2 overflow-x-auto font-mono text-xs text-slate-700 dark:text-slate-300 whitespace-pre-wrap break-all">{row.response_snippet}</pre>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
