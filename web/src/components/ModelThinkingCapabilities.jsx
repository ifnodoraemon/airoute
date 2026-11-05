import { useState } from 'react';
import { effortName } from '../lib/modelPresentation';

export default function ModelThinkingCapabilities({
  channel,
  profiles = [],
  resolvedCapabilities = {},
  upstreamModels = [],
  onChange,
  isZh,
}) {
  const [selected, setSelected] = useState('');
  const [custom, setCustom] = useState('');
  const config = channel.model_capabilities || {};
  const rules = channel.mapping_rules || [];
  const declared = (channel.models_str || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
  const names = [
    ...new Set([
      ...upstreamModels,
      ...declared.map((name) => rules.find((rule) => rule.from === name)?.to || name),
      ...rules.map((rule) => rule.to?.trim()),
      ...Object.keys(config),
    ]),
  ].filter((name) => name && (!name.includes('*') || name === '*'));
  const name = names.includes(selected) ? selected : names[0];
  const thinking = config[name]?.thinking || config['*']?.thinking || {};
  const resolved = resolvedCapabilities[name];
  const automatic = resolved?.automatic || (resolved?.source === 'catalog' ? resolved : null);
  const profile = profiles.find((item) => item.id === (thinking.profile || automatic?.profile));
  const canDisable = thinking.can_disable ?? profile?.off ?? automatic?.off ?? false;
  const efforts = thinking.efforts ?? profile?.efforts ?? automatic?.efforts ?? [];
  const supported = profile?.efforts || automatic?.efforts || [];
  const update = (patch) =>
    onChange({
      ...channel,
      model_capabilities: {
        ...config,
        [name]: { ...config[name], thinking: { ...thinking, ...patch } },
      },
    });
  const reset = () => {
    const next = { ...config },
      entry = { ...next[name] };
    delete entry.thinking;
    if (Object.keys(entry).length) next[name] = entry;
    else delete next[name];
    onChange({ ...channel, model_capabilities: next });
  };
  return (
    <section
      className="border-t pt-6 space-y-5"
      aria-label={isZh ? '思考能力' : 'Thinking capability'}
    >
      <h4 className="font-semibold">{isZh ? '思考能力' : 'Thinking capability'}</h4>
      {names.length > 0 ? (
        <>
          <label className="block">
            {isZh ? '模型' : 'Model'}
            <select
              aria-label={isZh ? '思考能力模型' : 'Thinking capability model'}
              className="ui-field mt-2"
              value={name}
              onChange={(event) => setSelected(event.target.value)}
            >
              {names.map((value) => (
                <option key={value} value={value}>
                  {value === '*' ? (isZh ? '渠道默认' : 'Channel default') : value}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              aria-label={isZh ? '支持关闭思考' : 'Can disable thinking'}
              checked={canDisable}
              disabled={!profile?.off && !automatic?.off}
              onChange={(event) => update({ can_disable: event.target.checked })}
            />
            {isZh ? '支持关闭思考' : 'Can disable thinking'}
          </label>
          <div className="space-y-3">
            <p>{isZh ? '支持档位' : 'Supported levels'}</p>
            {supported.length > 0 ? (
              <div className="flex flex-wrap gap-x-5 gap-y-3">
                {supported.map((effort) => (
                  <label key={effort} className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      aria-label={`${isZh ? '支持档位' : 'Supported level'} ${effort}`}
                      checked={efforts.includes(effort)}
                      onChange={(event) =>
                        update({
                          efforts: event.target.checked
                            ? [...efforts, effort]
                            : efforts.filter((value) => value !== effort),
                        })
                      }
                    />
                    {effortName(effort, isZh)}
                  </label>
                ))}
              </div>
            ) : (
              <p className="text-slate-400">{isZh ? '模型默认' : 'Model default'}</p>
            )}
          </div>
        </>
      ) : (
        <p className="text-slate-400">{isZh ? '请先添加模型' : 'Add a model first'}</p>
      )}
      <details className="text-xs text-slate-500">
        <summary className="cursor-pointer">{isZh ? '高级' : 'Advanced'}</summary>
        <div className="pt-4 space-y-4">
          {name && (
            <label className="block">
              {isZh ? '参数适配' : 'Parameter adapter'}
              <select
                aria-label={isZh ? '思考参数适配' : 'Thinking parameter adapter'}
                className="ui-field mt-2"
                value={config[name]?.thinking?.profile || ''}
                onChange={(event) => {
                  if (!event.target.value) reset();
                  else
                    update({
                      profile: event.target.value,
                      can_disable: undefined,
                      efforts: undefined,
                    });
                }}
              >
                <option value="">{isZh ? '自动' : 'Automatic'}</option>
                {profiles.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
          )}
          {name && (
            <button type="button" className="text-indigo-600 hover:underline" onClick={reset}>
              {isZh ? '恢复自动识别' : 'Restore automatic recognition'}
            </button>
          )}
          <div className="flex gap-2">
            <input
              className="ui-field min-w-0"
              aria-label={isZh ? '其他实际上游模型' : 'Other upstream model'}
              placeholder={isZh ? '实际模型名，或 *' : 'Actual model name, or *'}
              value={custom}
              onChange={(event) => setCustom(event.target.value)}
            />
            <button
              type="button"
              className="ui-btn-secondary shrink-0"
              disabled={!custom.trim()}
              onClick={() => {
                const added = custom.trim();
                onChange({
                  ...channel,
                  model_capabilities: {
                    ...config,
                    [added]: config[added] || {},
                  },
                });
                setSelected(added);
                setCustom('');
              }}
            >
              {isZh ? '添加' : 'Add'}
            </button>
          </div>
        </div>
      </details>
    </section>
  );
}
