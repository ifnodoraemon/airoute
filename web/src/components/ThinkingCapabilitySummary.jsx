import { effortName } from '../lib/modelPresentation';

export default function ThinkingCapabilitySummary({ capability, lang = 'zh' }) {
  const isZh = lang === 'zh';
  if (!capability?.known)
    return <span className="text-xs text-slate-400">{isZh ? '待确认' : 'Unconfirmed'}</span>;
  return (
    <div className="space-y-2 text-xs font-sans text-slate-500">
      <p>
        {isZh ? '可关闭' : 'Can disable'}：
        {capability.off ? (isZh ? '是' : 'Yes') : isZh ? '否' : 'No'}
      </p>
      <p>
        {isZh ? '档位' : 'Levels'}：
        {capability.efforts?.length
          ? capability.efforts.map((value) => effortName(value, isZh)).join(' / ')
          : isZh
            ? '模型默认'
            : 'Model default'}
      </p>
    </div>
  );
}
