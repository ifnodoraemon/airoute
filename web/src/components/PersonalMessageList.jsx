import { useState } from 'react';
import { ArrowLeft, Check, Mail, MailOpen } from 'lucide-react';
import { notificationDate, notificationLevel } from '../lib/notifications';

function MessageDetail({ item, lang, onRead, busy }) {
  const isZh = lang === 'zh';
  if (!item) return <div className="p-10 text-center text-sm text-[#656B75] dark:text-[#A1A7B0] flex flex-col items-center justify-center min-h-72"><MailOpen className="w-9 h-9 mb-4 opacity-40" />{isZh ? '选择一条消息，查看完整内容' : 'Select a message to read it'}</div>;
  const level = notificationLevel(item.level, lang);
  return (
    <article className="p-5 sm:p-7 min-w-0" aria-label={isZh ? '消息正文' : 'Message content'}>
      <div className="flex items-center justify-between gap-3 mb-4">
        <span className={`text-[11px] px-2 py-0.5 rounded-md ${level.className}`}>{level.label}</span>
        {item.read_at ? <span className="text-[11px] text-[#656B75] dark:text-[#A1A7B0] flex items-center gap-1"><Check className="w-3 h-3" />{isZh ? '已读' : 'Read'}</span>
          : <button type="button" disabled={busy} onClick={() => onRead(item)} className="text-xs text-[#355CFF] disabled:opacity-40 cursor-pointer">{isZh ? '标为已读' : 'Mark as read'}</button>}
      </div>
      <h3 className="text-lg font-semibold text-[#202124] dark:text-[#F1F2F4] break-words">{item.title}</h3>
      <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0] mt-2">Airoute · <time dateTime={item.created_at}>{notificationDate(item.created_at, lang)}</time></p>
      <p className="mt-6 pt-5 border-t border-[#E4E6E9] dark:border-[#2C3038] text-sm leading-7 text-[#202124] dark:text-[#F1F2F4] whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{item.content}</p>
    </article>
  );
}

export default function PersonalMessageList({ items, lang, onRead, busy }) {
  const [selectedId, setSelectedId] = useState(null);
  const selected = items.find(item => item.id === selectedId);
  return (
    <div className="ui-card grid md:grid-cols-[minmax(230px,0.8fr)_minmax(0,1.6fr)] overflow-hidden">
      <div className={`max-h-[560px] overflow-y-auto border-b md:border-b-0 md:border-r border-[#E4E6E9] dark:border-[#2C3038] ${selected ? 'hidden md:block' : ''}`} role="list" aria-label={lang === 'zh' ? '个人消息列表' : 'Personal messages'}>
        {items.map(item => (
          <div role="listitem" key={item.id}>
            <button type="button" onClick={() => { setSelectedId(item.id); onRead(item); }} aria-pressed={selectedId === item.id}
              className={`w-full text-left px-5 py-4 border-b border-[#E4E6E9] dark:border-[#2C3038] transition-colors cursor-pointer ${selectedId === item.id ? 'bg-[#EEF2FF] dark:bg-[#242D48]' : 'hover:bg-[#F7F7F5] dark:hover:bg-[#22252B]'}`}>
              <span className="flex items-start gap-2.5">
                <Mail className={`w-4 h-4 mt-0.5 shrink-0 ${item.read_at ? 'text-[#A1A7B0]' : 'text-[#355CFF] dark:text-[#91A7FF]'}`} />
                <span className="min-w-0 flex-1">
                  <span className={`block text-sm truncate ${item.read_at ? 'text-[#656B75] dark:text-[#A1A7B0]' : 'font-semibold text-[#202124] dark:text-[#F1F2F4]'}`}>{item.title}</span>
                  <span className="block mt-1 text-xs text-[#656B75] dark:text-[#A1A7B0] line-clamp-2 break-words">{item.content}</span>
                  <time dateTime={item.created_at} className="block mt-2 text-[10px] text-[#656B75] dark:text-[#A1A7B0]">{notificationDate(item.created_at, lang)}</time>
                </span>
                {!item.read_at && <span className="text-[10px] text-[#355CFF] dark:text-[#91A7FF] shrink-0">{lang === 'zh' ? '未读' : 'Unread'}</span>}
              </span>
            </button>
          </div>
        ))}
      </div>
      <div className={`min-w-0 ${selected ? '' : 'hidden md:block'}`}>
        {selected && <button type="button" onClick={() => setSelectedId(null)} className="md:hidden flex items-center gap-1.5 text-xs text-[#355CFF] dark:text-[#91A7FF] mx-5 mt-5 cursor-pointer"><ArrowLeft className="w-3.5 h-3.5" />{lang === 'zh' ? '返回消息列表' : 'Back to messages'}</button>}
        <MessageDetail item={selected} lang={lang} onRead={onRead} busy={busy} />
      </div>
    </div>
  );
}
