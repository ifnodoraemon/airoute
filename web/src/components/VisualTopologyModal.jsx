import { X, RefreshCw, Network, AlertTriangle, ArrowDown, ExternalLink } from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';
import { useRoutingTopology } from '../hooks/useRoutingTopology';
import RoutingTopologyStep from './RoutingTopologyStep';

export default function VisualTopologyModal({ isOpen, onClose, adminFetch, onViewLogs, onManageRoutes, lang = 'zh' }) {
  useModalA11y(isOpen, onClose);
  const state = useRoutingTopology(isOpen, adminFetch);
  if (!isOpen) return null;
  const isZh = lang === 'zh';
  const { snapshot, loading, error, refresh, autoRefresh, setAutoRefresh, selectModel } = state;
  const routing = snapshot?.routing;
  const activity = snapshot?.activity;
  const providers = routing?.steps.flatMap(step => step.providers) || [];
  const available = new Set(providers.filter(provider => provider.eligible).map(provider => provider.channel_id)).size;
  const openLogs = trace => { onViewLogs?.(routing.model, trace); onClose(); };
  const time = value => value ? new Date(value).toLocaleString() : '—';

  return (
    <div className="fixed inset-0 bg-slate-900/60 flex items-center justify-center p-3 sm:p-6 z-50 backdrop-blur-sm" onClick={event => { if (event.target === event.currentTarget) onClose(); }}>
      <section role="dialog" aria-modal="true" aria-labelledby="routing-topology-title" className="ui-card w-full max-w-6xl max-h-[92vh] overflow-y-auto shadow-2xl p-5 sm:p-7 space-y-5">
        <header className="flex items-start justify-between gap-4">
          <div>
            <h2 id="routing-topology-title" className="text-lg font-semibold flex items-center gap-2"><Network className="w-5 h-5 text-indigo-500" />{isZh ? '运行时容灾拓扑' : 'Live routing topology'}</h2>
            <p className="ui-meta mt-1">{isZh ? '映射、优先级与兜底，调用结果取自审计日志' : 'Mappings, priority tiers and fallbacks from audit logs'}</p>
          </div>
          <button type="button" className="ui-btn-ghost p-2" aria-label={isZh ? '关闭容灾拓扑' : 'Close topology'} onClick={onClose}><X className="w-5 h-5" /></button>
        </header>
        <div className="flex flex-wrap items-center gap-3">
          <label className="text-sm flex items-center gap-2">{isZh ? '请求模型' : 'Requested model'}
            <select aria-label={isZh ? '请求模型' : 'Requested model'} className="ui-field max-w-80" value={routing?.model || ''} onChange={event => selectModel(event.target.value)} disabled={!routing?.models.length}>
              {!routing?.models.length && <option value="">{isZh ? '暂无生效模型' : 'No active models'}</option>}
              {routing?.models.map(model => <option key={model} value={model}>{model}</option>)}
            </select>
          </label>
          <label className="ui-meta flex items-center gap-2"><input type="checkbox" checked={autoRefresh} onChange={event => setAutoRefresh(event.target.checked)} />{isZh ? '每 10 秒刷新' : 'Refresh every 10s'}</label>
          <button type="button" className="ui-btn ui-btn-secondary ui-btn-sm" disabled={loading} onClick={refresh}><RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />{isZh ? '刷新快照' : 'Refresh'}</button>
          {onViewLogs && routing?.model && <button type="button" className="ui-btn ui-btn-secondary ui-btn-sm" onClick={() => openLogs()}>{isZh ? '查看该模型日志' : 'Model logs'}<ExternalLink className="w-3.5 h-3.5" /></button>}
          {onManageRoutes && <button type="button" className="ui-btn ui-btn-secondary ui-btn-sm" onClick={() => { onManageRoutes(); onClose(); }}>{isZh ? '管理路由' : 'Manage routes'}</button>}
        </div>
        {error && <div role="alert" className="p-3 rounded-lg bg-rose-50 dark:bg-rose-950/30 text-rose-600 text-sm">{error}{snapshot && (isZh ? '；下方保留上次快照。' : '; showing previous snapshot.')}</div>}
        {loading && !snapshot && <p role="status" className="ui-meta">{isZh ? '正在读取运行时配置与调用记录…' : 'Loading routing state and audit records…'}</p>}
        {snapshot && <>
          <p className="ui-meta font-mono break-all">{isZh ? '观测实例' : 'Node'}: {snapshot.node || '—'} · {isZh ? '本地配置版本' : 'Local config version'}: {snapshot.config_version} · {time(snapshot.observed_at)}</p>
          <p className="ui-meta">{isZh ? '熔断状态属于本次响应实例，不代表其他副本；闭合仅表示允许尝试，不等于上游已通过验证。' : 'Breaker state belongs to the responding node. Closed means eligible, not a verified healthy upstream.'}</p>
          {snapshot.activity_error && <p role="alert" className="text-amber-600 text-sm">{snapshot.activity_error}</p>}
          {routing.model ? <>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              {[
                [isZh ? '24h 调用记录' : '24h requests', activity?.requests ?? '—'],
                [isZh ? '24h 异常记录' : '24h failures', activity?.errors ?? '—'],
                [isZh ? '24h 已记录渠道切换' : '24h recorded failovers', activity?.failovers ?? '—'],
                [isZh ? '当前可尝试渠道' : 'Eligible channels', available],
              ].map(([label, value]) => <div key={label} className="rounded-xl border border-slate-200 dark:border-slate-800 p-3"><div className="ui-meta">{label}</div><div className="font-mono text-xl mt-1">{value}</div></div>)}
            </div>
            {available < 2 && <p role="status" className="text-amber-700 dark:text-amber-400 text-sm flex items-center gap-2"><AlertTriangle className="w-4 h-4 shrink-0" />{available === 0 ? (isZh ? '此兜底链当前没有可尝试渠道。' : 'No eligible channel in this chain.') : (isZh ? '此兜底链当前仅有一个可尝试渠道，存在单点风险。' : 'Only one eligible channel: a single point of failure.')}</p>}
            <div className="space-y-3">
              {routing.steps.map((step, index) => <div key={step.model}>
                {index > 0 && <div className="flex items-center justify-center gap-2 ui-meta py-3"><ArrowDown className="w-4 h-4" />{isZh ? '发生可重试失败且尚未输出内容时，尝试跨模型兜底' : 'Cross-model fallback for retryable failures before output'}</div>}
                <RoutingTopologyStep step={step} activity={activity} state={state} isZh={isZh} />
              </div>)}
            </div>
            {routing.chain_error && <p role="alert" className="text-rose-600 text-sm">{routing.chain_error}</p>}
            <section className="space-y-2 border-t border-slate-200 dark:border-slate-800 pt-4">
              <h3 className="font-semibold text-sm">{isZh ? '最近异常与真实切换记录（24h）' : 'Recent failures and recorded failovers (24h)'}</h3>
              {!activity ? <p className="ui-meta">{isZh ? '调用统计不可用。' : 'Audit activity unavailable.'}</p> : activity.recent.length === 0 ? <p className="ui-meta">{isZh ? '没有已落盘的异常或渠道切换记录；尚不能据此确认备用渠道已经成功接管。' : 'No recorded failures or failovers. Backup takeover has not been demonstrated.'}</p> : <ul className="divide-y divide-slate-100 dark:divide-slate-800">
                {activity.recent.map(event => <li key={event.id} className="py-2 text-xs flex flex-wrap items-center justify-between gap-2">
                  <div className="min-w-0"><span className="font-mono break-all">{event.channel || '—'}</span><span className="ml-2">HTTP {event.status_code}</span><p className="ui-meta mt-1">{time(event.created_at)}{event.error && ` · ${event.error}`}</p></div>
                  {onViewLogs && <button className="ui-btn-ghost ui-btn-sm" onClick={() => openLogs(event.trace_id)}>{isZh ? '查看记录' : 'Inspect'}</button>}
                </li>)}
              </ul>}
            </section>
          </> : <p className="ui-meta py-6">{isZh ? '未发现生效模型路由，请先接入渠道并绑定模型。' : 'No active model routes. Add a channel and bind a model.'}</p>}
        </>}
      </section>
    </div>
  );
}
