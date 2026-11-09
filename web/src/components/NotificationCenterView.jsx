import { Mail, Megaphone, Plus, Send } from 'lucide-react';
import NotificationFeed from './NotificationFeed';
import AnnouncementDrafts from './AnnouncementDrafts';

export default function NotificationCenterView({ lang = 'zh', kind = 'announcement', onKindChange, counts, isAdmin, onPublish, onSend, refreshKey = 0, onChanged, fetcher, onEdit, onAnnouncementChanged }) {
  const isZh = lang === 'zh';
  const tabs = [
    { id: 'announcement', icon: Megaphone, label: isZh ? '全站公告' : 'Announcements', detail: isZh ? '平台动态与维护通知，按时间回看' : 'Platform updates and maintenance timeline', count: counts?.announcement_unread_count },
    { id: 'message', icon: Mail, label: isZh ? '我的消息' : 'My messages', detail: isZh ? '发送给当前账号的个人通知' : 'Private notifications for your account', count: counts?.message_unread_count },
  ];
  if (isAdmin) tabs.push({ id: 'draft', icon: Megaphone, label: isZh ? '公告草稿' : 'Drafts', detail: isZh ? '未发布的公告，仅管理员可见' : 'Unpublished posts, visible to administrators' });
  return (
    <section className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
        <div><h2 className="text-xl font-semibold text-[#202124] dark:text-[#F1F2F4]">{isZh ? '公告与消息' : 'Announcements & messages'}</h2><p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-2">{isZh ? '关注平台动态，查收与你有关的消息。' : 'Follow platform updates and messages addressed to you.'}</p></div>
        {isAdmin && <div className="flex items-center gap-2">
          <button type="button" onClick={onSend} className="ui-btn-secondary ui-btn-sm"><Send className="w-3.5 h-3.5" />{isZh ? '发送个人消息' : 'Send a message'}</button>
          <button type="button" onClick={onPublish} className="ui-btn-primary ui-btn-sm"><Plus className="w-3.5 h-3.5" />{isZh ? '发布全站公告' : 'Publish announcement'}</button>
        </div>}
      </div>
      <div role="tablist" aria-label={isZh ? '消息分类' : 'Message categories'} className={`grid gap-3 ${isAdmin ? 'sm:grid-cols-3' : 'sm:grid-cols-2'}`}>
        {tabs.map(tab => <button type="button" role="tab" id={`notification-tab-${tab.id}`} aria-controls={`notification-panel-${tab.id}`} aria-selected={kind === tab.id} key={tab.id} onClick={() => onKindChange(tab.id)}
          className={`p-4 rounded-xl border text-left transition-colors cursor-pointer ${kind === tab.id ? 'border-[#355CFF] bg-[#EEF2FF] dark:bg-[#242D48]' : 'border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] hover:border-[#A1A7B0]'}`}>
          <span className="flex items-center gap-2 text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]"><tab.icon className={`w-4 h-4 ${kind === tab.id ? 'text-[#355CFF] dark:text-[#91A7FF]' : 'text-[#656B75]'}`} />{tab.label}{tab.count > 0 && <span className="rounded-full px-2 py-0.5 text-[10px] bg-[#355CFF] text-white">{tab.count > 99 ? '99+' : tab.count}</span>}</span>
          <span className="block text-[11px] mt-2 text-[#656B75] dark:text-[#A1A7B0]">{tab.detail}</span>
        </button>)}
      </div>
      <div role="tabpanel" id={`notification-panel-${kind}`} aria-labelledby={`notification-tab-${kind}`}>
        {kind === 'draft' && isAdmin
          ? <AnnouncementDrafts lang={lang} refreshKey={refreshKey} onEdit={onEdit} onChanged={onAnnouncementChanged} fetcher={fetcher} />
          : <NotificationFeed key={kind} kind={kind} lang={lang} refreshKey={refreshKey} onChanged={onChanged} fetcher={fetcher} isAdmin={isAdmin} onEdit={onEdit} onAnnouncementChanged={onAnnouncementChanged} />}
      </div>
    </section>
  );
}
