import { effortName } from '../lib/modelPresentation';
import {
  THINKING_EFFORTS,
  selectableThinkingEfforts,
  thinkingSelection,
  thinkingFromSelection,
  thinkingOptions,
  thinkingOptionLabel,
} from '../lib/thinking';

export default function RouteThinkingPolicyEditor({ policy, onChange, availability, error, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const mode = policy.default_mode || 'auto';
  const current = { mode, effort: policy.default_effort };
  const selected = thinkingSelection(current);
  const restricted = policy.allowed_efforts != null;
  const locked = policy.allowed_modes?.length === 1 && policy.allowed_modes[0] === mode;
  const offAllowed = policy.allowed_modes == null || policy.allowed_modes.includes('off');
  const efforts = availability.efforts.filter((effort) => !restricted || policy.allowed_efforts.includes(effort));
  const options = thinkingOptions({ on: availability.on.length > 0, off: availability.off.length > 0 && offAllowed, efforts }, current);
  const range = [...new Set([...THINKING_EFFORTS, ...selectableThinkingEfforts(availability.efforts), ...selectableThinkingEfforts(policy.allowed_efforts)])];
  const update = (patch) => onChange((previous) => ({ ...previous, ...patch }));
  const toggleLevel = (effort, checked) => {
    const previous = policy.allowed_efforts ?? availability.efforts;
    const next = checked ? [...new Set([...previous, effort])] : previous.filter((value) => value !== effort);
    update({
      allowed_efforts: next,
      ...(mode === 'auto' ? {
        default_mode: next.length ? 'on' : (availability.off.length && offAllowed ? 'off' : 'on'),
        default_effort: next.includes('high') ? 'high' : next[0] || '',
      } : {}),
    });
  };
  return (
    <section className="border-t pt-8 space-y-7" aria-label={isZh ? '对外思考能力' : 'Public thinking capabilities'}>
      <h4 className="font-semibold">{isZh ? '思考能力' : 'Thinking capabilities'}</h4>
      <label className="flex items-center gap-3 py-1" title={availability.off.map((item) => item.channel_name).join(' / ')}>
        <input type="checkbox" aria-label={isZh ? '对外支持关闭思考' : 'Allow disabling thinking'} checked={availability.off.length > 0 && offAllowed} disabled={!availability.off.length && !policy.allowed_modes?.includes('off')} onChange={(event) => update({ allowed_modes: event.target.checked ? ['auto', ...(availability.on.length ? ['on'] : []), 'off'] : ['auto', ...(availability.on.length ? ['on'] : [])] })} />
        {isZh ? '支持关闭思考' : 'Can disable thinking'}
      </label>
      <div className="space-y-4">
        <p>{isZh ? '支持档位' : 'Supported levels'}</p>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
          {range.map((effort) => {
            const supported = availability.efforts.includes(effort);
            const checked = restricted ? policy.allowed_efforts.includes(effort) : supported;
            return (
              <label key={effort} className={`flex items-center gap-3 py-1 ${!supported && !checked ? 'text-slate-400' : ''}`} title={supported ? availability.supporting(effort).map((item) => item.channel_name).join(' / ') : (isZh ? '暂无渠道支持' : 'No supporting provider')}>
                <input type="checkbox" aria-label={`${isZh ? '允许强度' : 'Allow effort'} ${effort}`} checked={checked} disabled={!supported && !checked} onChange={(event) => toggleLevel(effort, event.target.checked)} />
                {effortName(effort, isZh)}
              </label>
            );
          })}
        </div>
        {restricted && <button type="button" className="text-indigo-600" onClick={() => update({ allowed_efforts: null })}>{isZh ? '使用渠道全部档位' : 'Use all upstream levels'}</button>}
      </div>
      <label className="block font-semibold">
        {isZh ? '默认思考' : 'Default thinking'}
        <select aria-label={isZh ? '默认思考' : 'Default thinking'} className="ui-field mt-2 font-normal" value={selected} onChange={(event) => {
          const next = thinkingFromSelection(event.target.value);
          update({ default_mode: next.mode, default_effort: next.effort, allowed_modes: locked ? [next.mode] : policy.allowed_modes });
        }}>
          {!options.includes(selected) && <option value={selected} disabled>{thinkingOptionLabel(selected, isZh)} · {isZh ? '不可用' : 'Unavailable'}</option>}
          {options.map((option) => <option key={option} value={option} title={availability.supporting(option).map((item) => item.channel_name).join(' / ')}>{thinkingOptionLabel(option, isZh)}</option>)}
        </select>
      </label>
      <details className="text-xs text-slate-500">
        <summary className="cursor-pointer">{isZh ? '高级' : 'Advanced'}</summary>
        <div className="pt-4 space-y-4">
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={!!locked} onChange={(event) => update({ allowed_modes: event.target.checked ? [mode] : null })} />
            {isZh ? '固定思考开关' : 'Lock thinking mode'}
          </label>
          {policy.allowed_modes != null && !locked && <button type="button" className="text-indigo-600" onClick={() => update({ allowed_modes: null })}>{isZh ? '清除模式限制' : 'Clear mode restrictions'}</button>}
        </div>
      </details>
      {error && <p role="alert" className="text-xs text-rose-600 dark:text-rose-400">{error}</p>}
    </section>
  );
}
