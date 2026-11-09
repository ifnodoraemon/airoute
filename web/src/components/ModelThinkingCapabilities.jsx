import { useState } from 'react';
import { effortName } from '../lib/modelPresentation';
import { THINKING_EFFORTS, validThinkingEffort, selectableThinkingEfforts } from '../lib/thinking';
import ModelThinkingProbe from './ModelThinkingProbe';

export default function ModelThinkingCapabilities({
  channel,
  channelId,
  resolvedCapabilities = {},
  upstreamModels = [],
  onChange,
  isZh,
}) {
  const [selected, setSelected] = useState('');
  const [customEffort, setCustomEffort] = useState('');
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
  const resolved = channel.detected_thinking_capabilities?.[name] || resolvedCapabilities[name];
  const canDisable = thinking.can_disable ?? resolved?.off ?? false;
  const efforts = selectableThinkingEfforts(thinking.efforts ?? resolved?.efforts);
  const supported = selectableThinkingEfforts(resolved?.supported_efforts ?? resolved?.efforts);
  const levels = [...new Set([...THINKING_EFFORTS, ...supported, ...efforts])];
  const update = (patch) =>
    onChange({
      ...channel,
      model_capabilities: {
        ...config,
        [name]: { ...config[name], thinking: { ...thinking, ...(thinking.efforts != null ? { efforts } : {}), ...patch } },
      },
    });
  return (
    <section
      className="border-t pt-8 space-y-7"
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
          <label className="flex items-center gap-3 py-1">
            <input
              type="checkbox"
              aria-label={isZh ? '支持关闭思考' : 'Can disable thinking'}
              checked={canDisable}
              onChange={(event) => update({ can_disable: event.target.checked })}
            />
            {isZh ? '支持关闭思考' : 'Can disable thinking'}
          </label>
          <div className="space-y-4">
            <p>{isZh ? '支持档位' : 'Supported levels'}</p>
            {levels.length > 0 ? (
              <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
                {levels.map((effort) => (
                  <label key={effort} className="flex items-center gap-3 py-1" title={effort}>
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
          <details className="text-slate-500">
            <summary className="cursor-pointer">{isZh ? '其他档位' : 'Other levels'}</summary>
            <div className="flex items-center gap-3 mt-4">
              <input className="ui-field" aria-label={isZh ? '自定义思考档位' : 'Custom thinking level'} placeholder={isZh ? '档位名称' : 'Level name'} value={customEffort} onChange={(event) => setCustomEffort(event.target.value)} />
              <button type="button" className="ui-btn-secondary shrink-0" disabled={!validThinkingEffort(customEffort) || efforts.includes(customEffort)} onClick={() => { update({ efforts: [...efforts, customEffort] }); setCustomEffort(''); }}>{isZh ? '添加' : 'Add'}</button>
            </div>
          </details>
          <ModelThinkingProbe key={`${channelId}:${channel.base_url}:${channel.type}:${channel.api_key}:${name}`} channel={channel} channelId={channelId} model={name} thinking={thinking} isZh={isZh} onResult={(result) => {
            if (!result.thinking) return;
            const checks = result.checks || {};
            const nextEfforts = [...new Set([...efforts.filter((value) => checks[value]?.status !== 'unsupported'), ...selectableThinkingEfforts(result.thinking.efforts)])];
            update({ ...(result.thinking.profile ? { profile: result.thinking.profile } : {}), ...(result.thinking.can_disable != null ? { can_disable: result.thinking.can_disable } : {}), efforts: nextEfforts });
          }} />
        </>
      ) : (
        <p className="text-slate-400">{isZh ? '请先添加模型' : 'Add a model first'}</p>
      )}
    </section>
  );
}
