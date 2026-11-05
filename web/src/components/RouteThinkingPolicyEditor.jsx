import ThinkingCapabilitySummary from './ThinkingCapabilitySummary';
import { effortName, thinkingPolicyText } from '../lib/modelPresentation';

export default function RouteThinkingPolicyEditor({
  policy,
  onChange,
  availability,
  error,
  lang = 'zh',
}) {
  const isZh = lang === 'zh';
  const mode = policy.default_mode || 'auto';
  const locked = policy.allowed_modes?.length === 1 && policy.allowed_modes[0] === mode;
  const restricted = policy.allowed_efforts != null;
  const supportedModes = [
    'auto',
    ...(availability.on.length ? ['on'] : []),
    ...(availability.off.length ? ['off'] : []),
  ];
  const modeNames = {
    auto: isZh ? '上游默认' : 'Upstream default',
    on: isZh ? '开启' : 'On',
    off: isZh ? '关闭' : 'Off',
  };
  const update = (patch) => onChange((previous) => ({ ...previous, ...patch }));
  const levels = availability.efforts.filter(
    (effort) => !restricted || policy.allowed_efforts.includes(effort)
  );
  const defaultUnavailable = policy.default_effort && !levels.includes(policy.default_effort);
  const rangeLevels = [...new Set([...availability.efforts, ...(policy.allowed_efforts || [])])];
  const coverage = (effort) => {
    const providers = availability.supporting(effort);
    return {
      text: isZh
        ? `${providers.length}/${availability.total} 个渠道支持`
        : `${providers.length}/${availability.total} channels`,
      title: providers.map((item) => `${item.channel_name} → ${item.mapped_model}`).join('\n'),
    };
  };
  return (
    <fieldset className="border-t pt-3 space-y-3">
      <legend className="font-semibold">{isZh ? '路由思考策略' : 'Route thinking policy'}</legend>
      <p className="text-slate-500">
        {isZh
          ? '档位取当前绑定且已启用的渠道模型所支持的合集。选择某档后，仅支持该档的渠道参与分发，切换渠道时也保留要求。'
          : 'Levels are the union supported by the enabled, bound upstream models. Selecting a level limits dispatch and failover to compatible channels.'}
      </p>
      <p className="text-xs font-medium">{thinkingPolicyText(policy, isZh)}</p>
      {availability.unknown > 0 && (
        <p className="text-xs text-amber-700 dark:text-amber-300">
          {isZh
            ? `${availability.unknown} 个已启用渠道的能力待确认，不参与开关和档位选项的计算。新增或修改映射后，可先按上游默认保存，再查看后端解析结果。`
            : `${availability.unknown} enabled channels have unconfirmed capability and do not contribute toggle or effort options. Save a new mapping with upstream defaults to read its resolved capability.`}
        </p>
      )}
      <div className="grid gap-2 sm:grid-cols-2">
        {availability.bindings.map((item) => (
          <div
            key={item.channel_id}
            className={`rounded-lg border p-3 space-y-2 ${item.enabled ? '' : 'opacity-60'}`}
          >
            <p className="text-xs font-mono break-all">
              {item.channel_name} → {item.mapped_model}
            </p>
            {!item.enabled && (
              <p className="text-xs text-amber-700">
                {isZh
                  ? '渠道未启用，不参与选项计算'
                  : 'Inactive channel; excluded from available options'}
              </p>
            )}
            <ThinkingCapabilitySummary capability={item.thinking} lang={lang} />
          </div>
        ))}
      </div>
      <label className="block">
        {isZh ? '路由默认思考模式' : 'Default thinking mode'}
        <select
          aria-label={isZh ? '路由默认思考模式' : 'Default thinking mode'}
          className="ui-field mt-1"
          value={mode}
          onChange={(event) =>
            update({
              default_mode: event.target.value,
              default_effort: event.target.value === 'on' ? policy.default_effort : '',
              allowed_modes: locked ? [event.target.value] : policy.allowed_modes,
            })
          }
        >
          {!supportedModes.includes(mode) && (
            <option value={mode} disabled>
              {modeNames[mode]} · {isZh ? '当前无渠道支持' : 'Unavailable'}
            </option>
          )}
          {supportedModes.map((value) => (
            <option key={value} value={value}>
              {modeNames[value]}
            </option>
          ))}
        </select>
      </label>
      {mode === 'on' && (availability.efforts.length > 0 || policy.default_effort) && (
        <label className="block">
          {isZh ? '默认强度' : 'Default effort'}
          <select
            aria-label={isZh ? '默认思考强度' : 'Default thinking effort'}
            className="ui-field mt-1"
            value={policy.default_effort || ''}
            onChange={(event) => update({ default_effort: event.target.value })}
          >
            <option value="">
              {isZh
                ? restricted
                  ? '在允许范围内选择模型默认'
                  : '模型默认'
                : restricted
                  ? 'Default within allowed range'
                  : 'Model default'}
            </option>
            {defaultUnavailable && (
              <option value={policy.default_effort} disabled>
                {effortName(policy.default_effort, isZh)} · {isZh ? '当前不可用' : 'Unavailable'}
              </option>
            )}
            {levels.map((effort) => {
              const info = coverage(effort);
              return (
                <option key={effort} value={effort} title={info.title}>
                  {effortName(effort, isZh)} · {info.text}
                </option>
              );
            })}
          </select>
        </label>
      )}
      {mode === 'on' &&
        !availability.efforts.length &&
        !policy.default_effort &&
        availability.on.length > 0 && (
          <p className="text-xs text-slate-500">
            {isZh
              ? '当前上游支持开启思考，但没有已确认的可调档位；使用模型默认强度。'
              : 'Thinking can be enabled, but no adjustable levels are confirmed. The model default applies.'}
          </p>
        )}
      <label className="flex gap-2 items-center">
        <input
          type="checkbox"
          checked={!!locked}
          onChange={(event) => update({ allowed_modes: event.target.checked ? [mode] : null })}
        />
        {isZh ? '只允许默认模式（禁止用户切换）' : 'Allow only the default mode'}
      </label>
      {policy.allowed_modes != null && !locked && (
        <button
          type="button"
          className="text-xs text-indigo-600 hover:underline"
          onClick={() => update({ allowed_modes: null })}
        >
          {isZh ? '取消现有模式限制' : 'Clear current mode restriction'}
        </button>
      )}
      {(availability.efforts.length > 0 || restricted) && (
        <div className="space-y-2">
          <label className="flex gap-2 items-center">
            <input
              type="checkbox"
              aria-label={isZh ? '限制可选思考强度' : 'Restrict available thinking efforts'}
              checked={restricted}
              disabled={!restricted && mode === 'auto'}
              onChange={(event) =>
                update({ allowed_efforts: event.target.checked ? [...availability.efforts] : null })
              }
            />
            {isZh ? '限制用户可选的思考强度' : 'Restrict user-selectable thinking effort'}
          </label>
          {restricted && (
            <div className="flex flex-wrap gap-3">
              {rangeLevels.map((effort) => {
                const info = coverage(effort);
                const supported = availability.efforts.includes(effort);
                return (
                  <label
                    key={effort}
                    className={`flex items-center gap-1 text-xs ${supported ? '' : 'text-amber-700'}`}
                    title={info.title}
                  >
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
                    {effortName(effort, isZh)} ·{' '}
                    {supported ? info.text : isZh ? '无渠道支持，请取消' : 'Unsupported; remove'}
                  </label>
                );
              })}
            </div>
          )}
          <p className="text-[11px] text-slate-500">
            {isZh
              ? mode === 'auto'
                ? '先选择路由默认模式，再设置强度范围。'
                : '取消限制时使用模型默认；启用限制后，只能使用勾选的档位。全部取消将无法显式开启思考。'
              : mode === 'auto'
                ? 'Choose a route default mode before restricting effort.'
                : 'Without a restriction, the model default applies. A restriction permits only checked levels; an empty range prevents explicit thinking-on requests.'}
          </p>
        </div>
      )}
      {error && (
        <p role="alert" className="text-xs text-rose-600 dark:text-rose-400">
          {error}
        </p>
      )}
      <p className="text-slate-500">
        {isZh
          ? '实际能力在“服务商 → 编辑 → 渠道模型的思考能力”维护；路由只能缩小允许范围。'
          : 'Manage actual capabilities in Providers → Edit → Channel model thinking capabilities. Route policy can only narrow that range.'}
      </p>
    </fieldset>
  );
}
