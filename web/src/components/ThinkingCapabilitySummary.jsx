import { effortName, thinkingCapabilityText } from '../lib/modelPresentation';

export default function ThinkingCapabilitySummary({ capability, lang = 'zh' }) {
  const isZh = lang === 'zh';
  const source = {
    configured: isZh ? '渠道已配置' : 'Channel configuration',
    catalog: isZh ? '按官方规则推断' : 'Inferred from documentation',
    unknown: isZh ? '尚未确认' : 'Unconfirmed',
  };
  return (
    <div className="space-y-1 text-xs font-sans">
      <p
        className={
          capability?.known
            ? 'text-slate-700 dark:text-slate-200'
            : 'text-amber-700 dark:text-amber-300'
        }
      >
        {thinkingCapabilityText(capability, isZh)}
      </p>
      {capability?.efforts?.length > 0 && (
        <p className="text-slate-500">
          {isZh ? '可用强度：' : 'Effort: '}
          {capability.efforts.map((value) => effortName(value, isZh)).join(' / ')}
        </p>
      )}
      {capability?.output === 'summary' && (
        <p className="text-slate-500">{isZh ? '可请求思考摘要' : 'Thinking summary available'}</p>
      )}
      {capability?.output === 'reasoning' && (
        <p className="text-slate-500">
          {isZh
            ? '可返回思考内容；是否返回由上游决定'
            : 'Thinking text may be returned by the upstream'}
        </p>
      )}
      {capability?.output === 'usage' && (
        <p className="text-slate-500">
          {isZh ? '通过用量判断思考，不提供正文' : 'Reasoning usage available; no thinking text'}
        </p>
      )}
      <p className="text-[11px] text-slate-400">{source[capability?.source] || source.unknown}</p>
    </div>
  );
}
