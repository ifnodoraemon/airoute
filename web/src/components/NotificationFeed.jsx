import { CheckCheck, ChevronLeft, ChevronRight, Inbox, RefreshCw } from 'lucide-react';
import useNotificationFeed from '../hooks/useNotificationFeed';
import AnnouncementTimeline from './AnnouncementTimeline';
import PersonalMessageList from './PersonalMessageList';

export default function NotificationFeed({ kind, lang, refreshKey, onChanged, fetcher, isAdmin, onEdit, onAnnouncementChanged }) {
  const isZh = lang === 'zh';
  const feed = useNotificationFeed({ kind, refreshKey, onChanged, fetcher });
  const { data, loading, error, busy, page, unreadOnly } = feed;
  const unreadCount = data?.[kind === 'announcement' ? 'announcement_unread_count' : 'message_unread_count'] || 0;
  const pages = Math.max(1, Math.ceil((data?.total || 0) / 20));
  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <label className="flex items-center gap-2 text-xs text-[#656B75] dark:text-[#A1A7B0] cursor-pointer">
          <input type="checkbox" checked={unreadOnly} disabled={busy} onChange={e => { feed.setUnreadOnly(e.target.checked); feed.setPage(1); }} className="accent-[#355CFF]" />
          {isZh ? '只看未读' : 'Unread only'}
        </label>
        <div className="flex items-center gap-3">
          <button type="button" onClick={feed.reload} disabled={loading || busy} className="ui-btn-secondary ui-btn-sm"><RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />{isZh ? '刷新' : 'Refresh'}</button>
          <button type="button" onClick={feed.markAllRead} disabled={!unreadCount || loading || busy} className="ui-btn-secondary ui-btn-sm"><CheckCheck className="w-3.5 h-3.5" />{isZh ? '本栏全部已读' : 'Mark this tab as read'}</button>
        </div>
      </div>
      {error && <div role="alert" className="p-4 rounded-xl bg-rose-50 dark:bg-rose-950/30 text-rose-700 dark:text-rose-300 text-xs flex flex-wrap justify-between gap-2">{error}<button type="button" onClick={feed.reload} className="underline cursor-pointer">{isZh ? '重试' : 'Retry'}</button></div>}
      {loading && !data && <p role="status" className="py-16 text-center text-sm text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '正在加载…' : 'Loading…'}</p>}
      {!loading && !error && data?.items.length === 0 && <div className="ui-card py-16 px-5 text-center"><Inbox className="w-10 h-10 text-[#A1A7B0] mx-auto mb-4" /><p className="text-sm font-medium text-[#202124] dark:text-[#F1F2F4]">{unreadOnly ? (isZh ? '暂无未读内容' : 'You’re all caught up') : kind === 'announcement' ? (isZh ? '暂无全站公告' : 'No announcements yet') : (isZh ? '暂无个人消息' : 'No personal messages yet')}</p><p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-2">{kind === 'announcement' ? (isZh ? '平台发布的公告会按时间保存在这里。' : 'Published announcements will appear here in order.') : (isZh ? '发送给当前账号的消息会出现在这里。' : 'Messages addressed to your account will appear here.')}</p></div>}
      {data?.items.length > 0 && (kind === 'announcement'
        ? <AnnouncementTimeline items={data.items} lang={lang} onRead={feed.markRead} busy={busy} isAdmin={isAdmin} onEdit={onEdit} onAnnouncementChanged={onAnnouncementChanged} fetcher={fetcher} />
        : <PersonalMessageList key={page} items={data.items} lang={lang} onRead={feed.markRead} busy={busy} />)}
      {data?.total > 0 && <div className="flex items-center justify-between text-xs text-[#656B75] dark:text-[#A1A7B0]">
        <span>{isZh ? `共 ${data.total} 条 · 第 ${page} / ${pages} 页` : `${data.total} items · Page ${page} of ${pages}`}</span>
        <div className="flex items-center gap-2">
          <button type="button" onClick={() => feed.setPage(value => value - 1)} disabled={page <= 1 || loading || busy} aria-label={isZh ? '上一页' : 'Previous page'} className="ui-btn-secondary ui-btn-sm"><ChevronLeft className="w-4 h-4" /></button>
          <button type="button" onClick={() => feed.setPage(value => value + 1)} disabled={page >= pages || loading || busy} aria-label={isZh ? '下一页' : 'Next page'} className="ui-btn-secondary ui-btn-sm"><ChevronRight className="w-4 h-4" /></button>
        </div>
      </div>}
    </div>
  );
}
