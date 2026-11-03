import { useState } from 'react';

export default function ModelThinkingCapabilities({ channel, profiles = [], onChange, isZh }) {
  const [custom, setCustom] = useState('');
  const config = channel.model_capabilities || {};
  const declared = (channel.models_str || '').split(',').map(value => value.trim()).filter(Boolean);
  const mapped = (channel.mapping_rules || []).map(rule => rule.to?.trim()).filter(Boolean);
  const names = [...new Set([...declared, ...mapped, ...Object.keys(config)])];
  const update = (name, profile) => {
    const next = { ...config };
    if (profile) next[name] = { ...next[name], thinking: { profile } };
    else delete next[name];
    onChange({ ...channel, model_capabilities: next });
  };
  return <details className="border rounded-xl p-3 space-y-3">
    <summary className="font-semibold cursor-pointer">{isZh ? '按上游模型配置思考能力' : 'Thinking capability per upstream model'}</summary>
    <p className="text-slate-500 leading-relaxed">{isZh ? '以下名称必须是映射后的真实上游模型。同一渠道内，引用它的所有路由共用此配置；路由只设置默认值与限制。留空时按协议和官方资料推断，未知能力不自动假定支持。' : 'Use the actual model name after mapping. Every route using this channel/model shares these facts. Routes only own defaults and restrictions. Blank uses documented protocol-specific inference; unknown capabilities stay unknown.'}</p>
    {names.map(name => {
      const thinking = config[name]?.thinking;
      const profile = profiles.find(item => item.id === thinking?.profile);
      return <div key={name} className="space-y-1.5 border-t pt-2">
        <label className="block font-mono break-all">{name}
          <select aria-label={`${isZh ? '思考能力' : 'Thinking capability'} ${name}`} className="ui-field mt-1 font-sans" value={thinking?.profile || ''} onChange={event => update(name, event.target.value)}>
            <option value="">{isZh ? '自动判断 / 未确认' : 'Automatic / unconfirmed'}</option>
            {profiles.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
          </select>
        </label>
        {profile?.efforts?.length > 0 && <div className="flex flex-wrap gap-3 text-slate-500">{profile.efforts.map(effort => <label key={effort} className="flex gap-1 items-center"><input type="checkbox" checked={!thinking.efforts || thinking.efforts.includes(effort)} onChange={event => {
          const current = thinking.efforts || profile.efforts;
          const efforts = event.target.checked ? [...current, effort] : current.filter(value => value !== effort);
          onChange({ ...channel, model_capabilities: { ...config, [name]: { ...config[name], thinking: { ...thinking, efforts } } } });
        }} />{effort}</label>)}</div>}
        {profile?.source_url && <a href={profile.source_url} target="_blank" rel="noreferrer" className="text-indigo-600 hover:underline">{isZh ? '查看参数约定' : 'Parameter documentation'}</a>}
      </div>;
    })}
    <div className="flex gap-2"><input className="ui-field min-w-0" aria-label={isZh ? '其他实际上游模型' : 'Other upstream model'} placeholder={isZh ? '其他实际模型名；* 表示渠道默认' : 'Actual model name; * for channel default'} value={custom} onChange={event => setCustom(event.target.value)} /><button type="button" className="ui-btn-secondary shrink-0" disabled={!custom.trim()} onClick={() => {const name=custom.trim();onChange({...channel,model_capabilities:{...config,[name]:config[name] || {}}});setCustom('');}}>{isZh ? '添加' : 'Add'}</button></div>
  </details>;
}
