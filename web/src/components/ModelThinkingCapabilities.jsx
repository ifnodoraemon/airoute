import { useState } from 'react';
import ThinkingCapabilitySummary from './ThinkingCapabilitySummary';
import { effortName } from '../lib/modelPresentation';

export default function ModelThinkingCapabilities({
  channel,
  profiles = [],
  resolvedCapabilities = {},
  upstreamModels = [],
  onChange,
  isZh,
}) {
  const [custom, setCustom] = useState('');
  const config = channel.model_capabilities || {};
  const declared = (channel.models_str || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
  const rules = channel.mapping_rules || [];
  const names = [
    ...new Set([
      ...upstreamModels,
      ...declared.map((name) => rules.find((rule) => rule.from === name)?.to || name),
      ...rules.map((rule) => rule.to?.trim()),
      ...Object.keys(config),
    ]),
  ].filter((name) => name && (!name.includes('*') || name === '*'));
  const update = (name, profile) => {
    const next = { ...config };
    if (profile) next[name] = { ...next[name], thinking: { profile } };
    else delete next[name];
    onChange({ ...channel, model_capabilities: next });
  };
  return (
    <details open className="border rounded-xl p-3 space-y-3">
      <summary className="font-semibold cursor-pointer">
        {isZh ? '渠道模型的思考能力' : 'Channel model thinking capabilities'}
      </summary>
      <p className="text-slate-500 leading-relaxed">
        {isZh
          ? '这里登记渠道模型实际支持什么；路由决定默认用什么。所有引用同一渠道模型的路由共用此能力配置。自动识别是按协议资料推断，实际代理存在差异时可手动指定。'
          : 'Record what the upstream supports here. Routes choose defaults. All routes using this channel/model share these facts. Automatic recognition follows documented protocols; override it when a proxy behaves differently.'}
      </p>
      {names.map((name) => {
        const thinking = config[name]?.thinking || config['*']?.thinking;
        const profile = profiles.find((item) => item.id === thinking?.profile);
        const resolved = resolvedCapabilities[name];
        const capability = profile
          ? {
              ...profile,
              known: profile.id !== 'unknown',
              source: 'configured',
              efforts: thinking.efforts || profile.efforts,
            }
          : resolved?.source === 'catalog'
            ? resolved
            : null;
        return (
          <div key={name} className="space-y-2 border-t pt-3">
            <p className="font-mono break-all font-medium">
              {name === '*'
                ? isZh
                  ? '* · 渠道默认能力'
                  : '* · Channel fallback capability'
                : name}
            </p>
            <ThinkingCapabilitySummary capability={capability} lang={isZh ? 'zh' : 'en'} />
            <details className="text-xs">
              <summary className="text-indigo-600 cursor-pointer">
                {isZh ? '调整能力与参数适配' : 'Adjust capability and parameter adapter'}
              </summary>
              <label className="block mt-2">
                {isZh ? '参数适配方案' : 'Parameter adapter'}
                <select
                  aria-label={`${isZh ? '思考能力' : 'Thinking capability'} ${name}`}
                  className="ui-field mt-1"
                  value={config[name]?.thinking?.profile || ''}
                  onChange={(event) => update(name, event.target.value)}
                >
                  <option value="">
                    {isZh ? '自动识别 / 继承渠道默认' : 'Automatic / inherit channel fallback'}
                  </option>
                  {profiles.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.label}
                    </option>
                  ))}
                </select>
              </label>
              {profile?.efforts?.length > 0 && config[name]?.thinking && (
                <div className="mt-2 flex flex-wrap gap-3 text-slate-500">
                  {profile.efforts.map((effort) => (
                    <label key={effort} className="flex gap-1 items-center">
                      <input
                        type="checkbox"
                        checked={!thinking.efforts || thinking.efforts.includes(effort)}
                        onChange={(event) => {
                          const current = thinking.efforts || profile.efforts;
                          const efforts = event.target.checked
                            ? [...current, effort]
                            : current.filter((value) => value !== effort);
                          onChange({
                            ...channel,
                            model_capabilities: {
                              ...config,
                              [name]: { ...config[name], thinking: { ...thinking, efforts } },
                            },
                          });
                        }}
                      />
                      {effortName(effort, isZh)}
                    </label>
                  ))}
                </div>
              )}
              {profile?.source_url && (
                <a
                  href={profile.source_url}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-block mt-2 text-indigo-600 hover:underline"
                >
                  {isZh ? '查看参数约定' : 'Parameter documentation'}
                </a>
              )}
            </details>
          </div>
        );
      })}
      {!names.length && (
        <p className="text-slate-500">
          {isZh
            ? '添加渠道模型后，可查看或指定其思考能力。'
            : 'Add upstream models to inspect or configure their thinking capability.'}
        </p>
      )}
      <div className="flex gap-2">
        <input
          className="ui-field min-w-0"
          aria-label={isZh ? '其他实际上游模型' : 'Other upstream model'}
          placeholder={
            isZh ? '其他实际模型名；* 表示渠道默认' : 'Actual model name; * for channel default'
          }
          value={custom}
          onChange={(event) => setCustom(event.target.value)}
        />
        <button
          type="button"
          className="ui-btn-secondary shrink-0"
          disabled={!custom.trim()}
          onClick={() => {
            const name = custom.trim();
            onChange({ ...channel, model_capabilities: { ...config, [name]: config[name] || {} } });
            setCustom('');
          }}
        >
          {isZh ? '添加' : 'Add'}
        </button>
      </div>
    </details>
  );
}
