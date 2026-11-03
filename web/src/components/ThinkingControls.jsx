import { thinkingChoices, thinkingControl, minimumThinkingOutput } from '../lib/thinking';

const effortLabels = { minimal: '极少', low: '低', medium: '中', high: '高', xhigh: '很高', max: '最高' };
export default function ThinkingControls({ capability, value, onChange, tools = false, disabled = false, maxTokens, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const choices = thinkingChoices(capability, tools);
  const active = thinkingControl(capability, value);
  const change = patch => onChange({ ...active, ...patch });
  return <div className="space-y-2 text-xs" data-testid="thinking-controls">
    <div className="flex flex-wrap gap-2 items-center">
      <label className="min-w-0 flex-1">{isZh ? '思考模式' : 'Thinking mode'}
        <select aria-label={isZh ? '思考模式' : 'Thinking mode'} className="ui-field mt-1" value={active.mode} disabled={disabled} onChange={event => change({ mode: event.target.value, effort: '' })}>
          <option value="auto">{isZh ? '遵循路由 / 上游默认' : 'Route / upstream default'}</option>
          {(choices.on || active.mode==='on') && <option value="on" disabled={!choices.on}>{isZh ? `开启思考${choices.on ? '' : '（当前不可用）'}` : `On${choices.on ? '' : ' (unavailable)'}`}</option>}
          {(choices.off || active.mode==='off') && <option value="off" disabled={!choices.off}>{isZh ? `关闭思考${choices.off ? '' : '（当前不可用）'}` : `Off${choices.off ? '' : ' (unavailable)'}`}</option>}
        </select>
      </label>
      {active.mode === 'on' && (choices.efforts?.length > 0 || active.effort) && <label className="min-w-0 flex-1">{isZh ? '思考强度' : 'Thinking effort'}
        <select aria-label={isZh ? '思考强度' : 'Thinking effort'} className="ui-field mt-1" value={active.effort || ''} disabled={disabled} onChange={event => change({ effort: event.target.value })}>
          <option value="">{isZh ? '路由 / 模型默认' : 'Route / model default'}</option>
          {active.effort && !choices.efforts?.includes(active.effort) && <option value={active.effort} disabled>{active.effort} ({isZh ? '当前不可用' : 'unavailable'})</option>}
          {(choices.efforts || []).map(effort => <option key={effort} value={effort}>{isZh ? `${effortLabels[effort] || effort} (${effort})` : effort}</option>)}
        </select>
      </label>}
    </div>
    {capability?.summary && <label className="flex items-center gap-2"><input type="checkbox" checked={active.include_summary !== false} disabled={disabled} onChange={event => change({ include_summary: event.target.checked })} />{isZh ? '请求上游返回思考摘要' : 'Request thinking summary'}</label>}
    {((active.mode!=='auto' && !choices[active.mode]) || (active.effort && !choices.efforts?.includes(active.effort))) && <p role="alert" className="text-amber-700 dark:text-amber-300">{isZh ? '原设置已不可用，请选择其他模式或档位；请求不会自动降级。' : 'The selected setting is no longer available. Choose another mode or level; requests will not be downgraded.'}</p>}
    {Number(maxTokens)>0 && minimumThinkingOutput(capability,active)>Number(maxTokens) && <p role="alert" className="text-amber-700 dark:text-amber-300">{isZh ? `此设置在预算渠道至少需要 ${minimumThinkingOutput(capability,active)} 个输出 token，请提高上限或降低强度。` : `Budget-based providers need at least ${minimumThinkingOutput(capability,active)} output tokens. Raise the limit or lower effort.`}</p>}
    <p className="text-slate-500 leading-relaxed">
      {!capability?.known ? (isZh ? '思考能力未知，请在渠道的实际模型配置中确认。' : 'Thinking capability unknown. Configure the actual upstream model on its channel.')
        : !choices.on && !choices.off ? (isZh ? '当前调用方式的开关或档位尚未确认；摘要返回与内部思考独立。' : 'Toggle / effort is unavailable for this call. Summary output is independent of internal thinking.')
        : (isZh ? '只会选择满足设置的渠道；切换渠道不会丢弃你的思考要求。' : 'Only providers satisfying these settings are eligible, including during failover.')}
      {capability?.known && choices.on && !choices.off && (isZh ? ' 当前能力不支持关闭思考。' : ' Disabling thinking is unavailable.')}
      {capability?.source === 'catalog' && (isZh ? ' 能力来自官方资料推断。' : ' Inferred from official documentation.')}
    </p>
  </div>;
}
