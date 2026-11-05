import { effortName } from '../lib/modelPresentation';
import {
  thinkingSelection,
  thinkingFromSelection,
  thinkingOptions,
  thinkingOptionLabel,
} from '../lib/thinking';

export default function RouteThinkingPolicyEditor({
  policy,
  onChange,
  availability,
  error,
  lang = 'zh',
}) {
  const isZh = lang === 'zh';
  const mode = policy.default_mode || 'auto';
  const current = { mode, effort: policy.default_effort };
  const selected = thinkingSelection(current);
  const restricted = policy.allowed_efforts != null;
  const locked = policy.allowed_modes?.length === 1 && policy.allowed_modes[0] === mode;
  const options = thinkingOptions(
    {
      on: availability.on.length > 0,
      off: availability.off.length > 0,
      efforts: availability.efforts.filter(
        (effort) => !restricted || policy.allowed_efforts.includes(effort)
      ),
    },
    current
  );
  const update = (patch) => onChange((previous) => ({ ...previous, ...patch }));
  const range = [...new Set([...availability.efforts, ...(policy.allowed_efforts || [])])];
  return (
    <section className="border-t pt-6 space-y-4">
      <label className="block font-semibold">
        {isZh ? '默认思考' : 'Default thinking'}
        <select
          aria-label={isZh ? '默认思考' : 'Default thinking'}
          className="ui-field mt-2 font-normal"
          value={selected}
          onChange={(event) => {
            const next = thinkingFromSelection(event.target.value);
            update({
              default_mode: next.mode,
              default_effort: next.effort,
              allowed_modes: locked ? [next.mode] : policy.allowed_modes,
            });
          }}
        >
          {!options.includes(selected) && (
            <option value={selected} disabled>
              {thinkingOptionLabel(selected, isZh)} · {isZh ? '不可用' : 'Unavailable'}
            </option>
          )}
          {options.map((option) => (
            <option
              key={option}
              value={option}
              title={availability
                .supporting(option)
                .map((item) => item.channel_name)
                .join(' / ')}
            >
              {thinkingOptionLabel(option, isZh)}
            </option>
          ))}
        </select>
      </label>
      <details className="text-xs text-slate-500">
        <summary className="cursor-pointer">{isZh ? '高级' : 'Advanced'}</summary>
        <div className="pt-4 space-y-4">
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={!!locked}
              onChange={(event) => update({ allowed_modes: event.target.checked ? [mode] : null })}
            />
            {isZh ? '固定思考开关' : 'Lock thinking mode'}
          </label>
          {policy.allowed_modes != null && !locked && (
            <button
              type="button"
              className="text-indigo-600"
              onClick={() => update({ allowed_modes: null })}
            >
              {isZh ? '清除模式限制' : 'Clear mode restrictions'}
            </button>
          )}
          {(availability.efforts.length > 0 || restricted) && (
            <>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  aria-label={isZh ? '限制可选思考强度' : 'Restrict available thinking efforts'}
                  checked={restricted}
                  disabled={!restricted && mode === 'auto'}
                  onChange={(event) =>
                    update({
                      allowed_efforts: event.target.checked ? [...availability.efforts] : null,
                    })
                  }
                />
                {isZh ? '限制可选档位' : 'Restrict levels'}
              </label>
              {restricted && (
                <div className="flex flex-wrap gap-x-5 gap-y-3">
                  {range.map((effort) => (
                    <label key={effort} className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        aria-label={`${isZh ? '允许强度' : 'Allow effort'} ${effort}`}
                        checked={policy.allowed_efforts.includes(effort)}
                        onChange={(event) =>
                          update({
                            allowed_efforts: event.target.checked
                              ? [...policy.allowed_efforts, effort]
                              : policy.allowed_efforts.filter((value) => value !== effort),
                          })
                        }
                      />
                      {effortName(effort, isZh)}
                      {!availability.efforts.includes(effort) &&
                        (isZh ? ' · 不可用' : ' · Unavailable')}
                    </label>
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      </details>
      {error && (
        <p role="alert" className="text-xs text-rose-600 dark:text-rose-400">
          {error}
        </p>
      )}
    </section>
  );
}
