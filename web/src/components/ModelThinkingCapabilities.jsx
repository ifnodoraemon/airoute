import { useState } from 'react';
import { effortName } from '../lib/modelPresentation';

export default function ModelThinkingCapabilities({
  channel,
  resolvedCapabilities = {},
  upstreamModels = [],
  onChange,
  isZh,
}) {
  const [selected, setSelected] = useState('');
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
  const canDisable = thinking.can_disable ?? resolved?.off ?? false;
  const supportsDisable = resolved?.supports_disable ?? resolved?.off ?? false;
  const efforts = thinking.efforts ?? resolved?.efforts ?? [];
  const supported = resolved?.supported_efforts ?? resolved?.efforts ?? [];
  const levels = [...new Set([...supported, ...efforts])];
  const update = (patch) =>
    onChange({
      ...channel,
      model_capabilities: {
        ...config,
        [name]: { ...config[name], thinking: { ...thinking, ...patch } },
      },
    });
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
              disabled={!supportsDisable && !canDisable}
              onChange={(event) => update({ can_disable: event.target.checked })}
            />
            {isZh ? '支持关闭思考' : 'Can disable thinking'}
          </label>
          <div className="space-y-3">
            <p>{isZh ? '支持档位' : 'Supported levels'}</p>
            {levels.length > 0 ? (
              <div className="flex flex-wrap gap-x-5 gap-y-3">
                {levels.map((effort) => (
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
                    {!supported.includes(effort) && (isZh ? ' · 不可用' : ' · Unavailable')}
                  </label>
                ))}
              </div>
            ) : (
              <p className="text-slate-400">
                {resolved?.known
                  ? isZh
                    ? '模型默认'
                    : 'Model default'
                  : isZh
                    ? '待确认'
                    : 'Unconfirmed'}
              </p>
            )}
          </div>
        </>
      ) : (
        <p className="text-slate-400">{isZh ? '请先添加模型' : 'Add a model first'}</p>
      )}
    </section>
  );
}
