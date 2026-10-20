export default function RoutingTopologyStep({ step, activity, state, isZh }) {
  const priorities = [...new Set(step.providers.map(provider => provider.priority))].sort((a, b) => a - b);
  const firstEligible = Math.min(...step.providers.filter(provider => provider.eligible).map(provider => provider.priority));
  return (
    <section className="rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
      <h3 className="bg-slate-50 dark:bg-slate-900/50 px-4 py-3 font-mono font-semibold text-sm break-all">{step.model}</h3>
      {step.providers.length === 0 && <p className="p-4 text-sm text-rose-600">{isZh ? '此模型没有已加载的承载渠道。' : 'No active runtime channel serves this model.'}</p>}
      {priorities.map(priority => <div key={priority} className="border-t border-slate-100 dark:border-slate-800 p-4 space-y-2">
        <div className="flex flex-wrap items-center gap-2 text-xs"><strong>Priority {priority}</strong><span className="ui-meta">{priority === firstEligible ? (isZh ? '当前首选候选层' : 'First eligible tier') : (isZh ? '后续候选层 / 不可用层' : 'Later or unavailable tier')}</span><span className="ui-meta">{isZh ? '同层按配置权重选路，会话亲和可能固定渠道' : 'Weighted selection within a tier; session affinity may pin a channel'}</span></div>
        {step.providers.filter(provider => provider.priority === priority).map(provider => {
          const key = `${step.model}:${provider.channel_id}`;
          const result = state.probes[key];
          const observed = activity?.channels[provider.name];
          const status = !provider.provider_supported ? (isZh ? '无推理适配器' : 'Unsupported provider') : !provider.eligible ? (isZh ? '当前跳过' : 'Currently skipped') : provider.breaker.recovery_probe ? (isZh ? '仅恢复试探' : 'Recovery probe') : (isZh ? '允许尝试' : 'Eligible');
          return <article key={provider.channel_id} className="rounded-lg border border-slate-200 dark:border-slate-700 p-3 space-y-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex flex-wrap items-center gap-2 text-sm"><strong>{provider.name}</strong><span className="ui-chip">{provider.type}</span><span className={`ui-chip ${provider.eligible ? 'text-emerald-600' : 'text-rose-600'}`}>{status} · {provider.breaker.state}</span></div>
              <button type="button" className="ui-btn ui-btn-secondary ui-btn-sm" disabled={Boolean(state.probing) || step.modality !== 'chat' || !provider.provider_supported} title={isZh ? '向此模型发送最多 8 个输出 token 的真实请求；不修改生产熔断状态' : 'Send a real inference capped at 8 output tokens; does not alter circuit state'} onClick={() => state.probe(step.model, provider.channel_id)}>{state.probing === key ? (isZh ? '验证中…' : 'Checking…') : (isZh ? '验证模型' : 'Validate model')}</button>
            </div>
            <p className="ui-meta font-mono break-all">{step.model} → {provider.mapped_model} · {isZh ? '权重' : 'Weight'} {provider.weight} · {(provider.protocols || []).join(' / ') || (isZh ? '默认协议适配' : 'Default adaptation')}</p>
            <p className="ui-meta">{isZh ? '连续失败' : 'Consecutive failures'} {provider.breaker.consecutive_failures} · {isZh ? '恢复探针占用' : 'Canaries in flight'} {provider.breaker.canary_in_flight}{provider.breaker.cooldown_remaining_ms > 0 && ` · ${isZh ? '快照时剩余冷却' : 'Cooldown at snapshot'} ${Math.ceil(provider.breaker.cooldown_remaining_ms / 1000)}s`}</p>
            <p className="ui-meta">{observed ? (isZh ? `本请求模型 24h 在此渠道落盘 ${observed.requests} 次，异常 ${observed.errors} 次` : `${observed.requests} recorded requests, ${observed.errors} failures for this requested model (24h)`) : !activity ? (isZh ? '调用统计不可用' : 'Audit activity unavailable') : (isZh ? '本请求模型 24h 无此渠道的落盘记录' : 'No recorded calls to this channel for the requested model (24h)')}</p>
            {step.modality !== 'chat' && <p className="ui-meta">{isZh ? '此模型请在调试演练中按实际模态验证。' : 'Validate this modality in the playground.'}</p>}
            {result && <p role="status" className={`text-xs ${result.success ? 'text-emerald-600' : 'text-rose-600'}`}>{result.success ? (isZh ? `模型调用已验证 · ${result.latency_ms}ms` : `Inference verified · ${result.latency_ms}ms`) : result.error}{result.checked_at && ` · ${new Date(result.checked_at).toLocaleTimeString()}`}</p>}
          </article>;
        })}
      </div>)}
      <p className="ui-meta px-4 pb-3">{isZh ? '模型验证会产生真实上游调用；结果仅代表该次验证，不会宣称容灾切换已发生。' : 'Validation sends real inference. It verifies that request, not a failover event.'}</p>
    </section>
  );
}
