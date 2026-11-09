import { Check } from 'lucide-react';

export default function NotificationEditorFields({ value, onChange, lang = 'zh', announcement = false }) {
  const isZh = lang === 'zh';
  const prefix = announcement ? 'announcement' : 'message';
  return (
    <>
      <div className="space-y-2">
        <span className="text-xs font-medium text-slate-700 dark:text-slate-300">{isZh ? '重要程度' : 'Priority'}</span>
        <div className="grid grid-cols-3 gap-2">
          {[['info', isZh ? '普通' : 'Info'], ['warning', isZh ? '重要' : 'Important'], ['critical', isZh ? '紧急' : 'Urgent']].map(([id, label]) => (
            <button key={id} type="button" aria-pressed={value.level === id} onClick={() => onChange({ ...value, level: id })}
              className={`px-3 py-2 rounded-lg border text-xs inline-flex items-center justify-center gap-1.5 cursor-pointer ${value.level === id ? 'border-[#355CFF] text-[#355CFF] dark:text-[#91A7FF] bg-[#EEF2FF] dark:bg-[#242D48]' : 'border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0]'}`}>
              {value.level === id && <Check className="w-3 h-3" />}{label}
            </button>
          ))}
        </div>
      </div>
      <div className="space-y-1.5">
        <label htmlFor={`${prefix}-title`} className="text-xs font-medium text-slate-700 dark:text-slate-300">{isZh ? (announcement ? '公告标题' : '消息标题') : 'Title'}</label>
        <input id={`${prefix}-title`} type="text" value={value.title} maxLength={120} onChange={e => onChange({ ...value, title: e.target.value })}
          placeholder={announcement ? (isZh ? '例如：双活集群网络升级与维护预告' : 'e.g. Scheduled Network Upgrade Notice') : (isZh ? '输入消息标题' : 'Enter a message title')} className="ui-field text-sm w-full" />
      </div>
      <div className="space-y-1.5">
        <div className="flex justify-between gap-2"><label htmlFor={`${prefix}-content`} className="text-xs font-medium text-slate-700 dark:text-slate-300">{isZh ? (announcement ? '公告详细内容' : '消息内容') : 'Content'}</label><span className="text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{Array.from(value.content).length} / 10000</span></div>
        <textarea id={`${prefix}-content`} value={value.content} maxLength={10000} rows={7} onChange={e => onChange({ ...value, content: e.target.value })}
          placeholder={isZh ? '输入完整内容，支持换行和分段。' : 'Write the full message. Line breaks and paragraphs are preserved.'} className="ui-field text-sm w-full resize-y leading-6" />
      </div>
    </>
  );
}
