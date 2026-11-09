import { useState } from 'react';
import { Check, ChevronDown, ChevronUp, Megaphone } from 'lucide-react';
import { notificationDate, notificationLevel } from '../lib/notifications';
import AnnouncementActions from './AnnouncementActions';

function AnnouncementCard({ item, lang, onRead, busy, isAdmin, onEdit, onAnnouncementChanged, fetcher }) {
  const [expanded, setExpanded] = useState(false);
  const isZh = lang === 'zh';
  const level = notificationLevel(item.level, lang);
  const toggle = () => {
    if (!expanded) onRead(item);
    setExpanded(value => !value);
  };
  return (
    <article className="ui-card p-5 sm:p-6 min-w-0">
      <div className="flex items-center flex-wrap gap-2 text-[11px] mb-3">
        <span className="text-[#656B75] dark:text-[#A1A7B0]">Airoute · {isZh ? '全站公告' : 'Platform announcement'}</span>
        <span className={`px-2 py-0.5 rounded-md font-medium ${level.className}`}>{level.label}</span>
        {!item.read_at && <span className="text-[#355CFF] dark:text-[#91A7FF] font-medium">{isZh ? '未读' : 'Unread'}</span>}
      </div>
      <h3 className="text-base font-semibold text-[#202124] dark:text-[#F1F2F4] break-words">
        <button type="button" onClick={toggle} aria-expanded={expanded} className="text-left hover:text-[#355CFF] cursor-pointer">{item.title}</button>
      </h3>
      <p className={`text-sm leading-7 text-[#656B75] dark:text-[#A1A7B0] mt-3 whitespace-pre-wrap break-words [overflow-wrap:anywhere] ${expanded ? '' : 'line-clamp-3'}`}>{item.content}</p>
      <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
        <button type="button" onClick={toggle} aria-expanded={expanded} className="inline-flex items-center gap-1 text-xs font-medium text-[#355CFF] dark:text-[#91A7FF] cursor-pointer">
          {expanded ? (isZh ? '收起正文' : 'Collapse') : (isZh ? '阅读全文' : 'Read more')}
          {expanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
        </button>
        {item.read_at ? <span className="text-[11px] text-[#656B75] dark:text-[#A1A7B0] inline-flex items-center gap-1"><Check className="w-3 h-3" />{isZh ? '已读' : 'Read'}</span>
          : <button type="button" disabled={busy} onClick={() => onRead(item)} className="text-xs text-[#656B75] dark:text-[#A1A7B0] hover:text-[#355CFF] disabled:opacity-40 cursor-pointer">{isZh ? '标为已读' : 'Mark as read'}</button>}
      </div>
      {isAdmin && <AnnouncementActions id={item.announcement_id} onEdit={onEdit} onChanged={onAnnouncementChanged} lang={lang} fetcher={fetcher} />}
    </article>
  );
}

export default function AnnouncementTimeline({ items, lang, onRead, busy, isAdmin, onEdit, onAnnouncementChanged, fetcher }) {
  return (
    <ol className="relative space-y-6 max-w-4xl mx-auto" aria-label={lang === 'zh' ? '全站公告时间线' : 'Announcement timeline'}>
      {items.map((item, index) => {
        const date = notificationDate(item.created_at, lang, true);
        const previousDate = index > 0 ? notificationDate(items[index - 1].created_at, lang, true) : '';
        return (
          <li key={item.id} className="relative pl-7 sm:pl-10">
            <span aria-hidden="true" className="absolute left-[7px] top-6 bottom-[-24px] w-px bg-[#E4E6E9] dark:bg-[#2C3038]" />
            <span aria-hidden="true" className={`absolute left-0 top-1 w-4 h-4 rounded-full border-4 border-[#F7F7F5] dark:border-[#111214] ${item.read_at ? 'bg-slate-400' : 'bg-[#355CFF]'}`} />
            {date !== previousDate && <time dateTime={item.created_at} className="block text-xs font-medium text-[#656B75] dark:text-[#A1A7B0] mb-3">{date}</time>}
            {date === previousDate && <div className="mb-3 flex items-center gap-1.5 text-[11px] text-[#656B75] dark:text-[#A1A7B0]"><Megaphone className="w-3 h-3" /><time dateTime={item.created_at}>{notificationDate(item.created_at, lang)}</time></div>}
            <AnnouncementCard item={item} lang={lang} onRead={onRead} busy={busy} isAdmin={isAdmin} onEdit={onEdit} onAnnouncementChanged={onAnnouncementChanged} fetcher={fetcher} />
          </li>
        );
      })}
    </ol>
  );
}
