import { useEffect, useRef, useState } from 'react';
import { apiJSON } from '../api/client';
import { effortName } from '../lib/modelPresentation';

export default function ModelThinkingProbe({ channel, channelId, model, thinking, onResult, isZh }) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');
  const pending = useRef(null);
  const resultHandler = useRef(onResult);
  useEffect(() => { resultHandler.current = onResult; }, [onResult]);
  useEffect(() => () => pending.current?.abort(), []);
  const probe = async () => {
    pending.current?.abort();
    const controller = new AbortController();
    pending.current = controller;
    setBusy(true);
    setError('');
    setResult(null);
    try {
      const response = await apiJSON('/api/v1/admin/channels/thinking/probe', {
        signal: controller.signal,
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ channel_id: channelId || undefined, base_url: channel.base_url, api_key: channel.api_key || '', type: channel.type, protocols: channel.protocols, model, thinking }),
      });
      if (controller.signal.aborted || pending.current !== controller) return;
      setResult(response.data);
      resultHandler.current(response.data);
    } catch (failure) {
      if (!controller.signal.aborted) setError(failure.message);
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  };
  const statusLabel = (status) => {
    if (status === 'supported') return isZh ? '已确认支持' : 'Confirmed';
    if (status === 'unsupported') return isZh ? '不支持' : 'Unsupported';
    return isZh ? '待确认' : 'Unconfirmed';
  };
  return (
    <div className="space-y-4">
      <button type="button" className="ui-btn-secondary" title={isZh ? '使用少量短请求逐档探测，保存后生效' : 'Tests each level with short requests. Save to apply.'} disabled={busy || !channel.base_url || model === '*'} onClick={probe}>
        {busy ? (isZh ? '探测中…' : 'Probing…') : (isZh ? '自动探测' : 'Auto-detect')}
      </button>
      {result && <details className="text-slate-500"><summary className="cursor-pointer">{isZh ? '探测结果' : 'Results'}</summary><ul className="space-y-3 pt-4" aria-label={isZh ? '思考能力探测结果' : 'Thinking probe results'}>
        {result.message && <li>{result.message}</li>}
        {Object.entries(result.checks || {}).map(([level, check]) => (
          <li key={level} title={check.reason}>{level === 'off' ? (isZh ? '关闭思考' : 'Off') : effortName(level, isZh)}：{statusLabel(check.status)}</li>
        ))}
      </ul></details>}
      {error && <p role="alert" className="text-rose-600">{error}</p>}
    </div>
  );
}
