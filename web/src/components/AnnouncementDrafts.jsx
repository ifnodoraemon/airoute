import { useEffect, useState } from 'react';
import { notificationDate, notificationJSON } from '../lib/notifications';
import AnnouncementActions from './AnnouncementActions';

export default function AnnouncementDrafts({ lang, refreshKey, onEdit, onChanged, fetcher }) {
  const [data, setData] = useState(null);
  const [page, setPage] = useState(1);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  const isZh = lang === 'zh';
  useEffect(() => {
    const controller = new AbortController();
    setData(null);
    setError('');
    notificationJSON(`/api/v1/admin/announcements?status=draft&page=${page}&page_size=20`, { signal: controller.signal }, fetcher)
      .then(data => {
        if (!Array.isArray(data?.items)) throw new Error('公告数据无效 / Invalid announcements');
        if (controller.signal.aborted) return;
        if (page > 1 && data.items.length === 0) setPage(Math.max(1, Math.ceil(data.total / 20)));
        else setData(data);
      })
      .catch(e => { if (!controller.signal.aborted) setError(e.message); });
    return () => controller.abort();
  }, [page, refreshKey, reload, fetcher]);
  return <div className="space-y-4">
    {error && <p role="alert" className="text-xs text-rose-600">{error}<button type="button" onClick={() => setReload(v => v + 1)} className="ml-3 underline cursor-pointer">{isZh ? '重试' : 'Retry'}</button></p>}
    {!data && !error && <p role="status" className="text-xs text-[#656B75] py-10 text-center">{isZh ? '正在加载草稿…' : 'Loading drafts…'}</p>}
    {data?.items.length === 0 && <p className="ui-card p-12 text-center text-sm text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '暂无公告草稿' : 'No drafts'}</p>}
    {data?.items.map(item => <article key={item.id} className="ui-card p-5">
      <h3 className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{item.title || (isZh ? '未命名公告' : 'Untitled announcement')}</h3>
      <p className="text-[11px] text-[#656B75] dark:text-[#A1A7B0] mt-2">{notificationDate(item.updated_at, lang)}</p>
      <p className="mt-3 text-xs text-[#656B75] dark:text-[#A1A7B0] line-clamp-3 whitespace-pre-wrap break-words">{item.content}</p>
      <AnnouncementActions id={item.id} onEdit={onEdit} onChanged={onChanged} lang={lang} fetcher={fetcher} />
    </article>)}
    {data && (page > 1 || data.total > 20) && <div className="flex justify-end gap-3"><button type="button" disabled={page <= 1} onClick={() => setPage(v => v - 1)} className="ui-btn-secondary ui-btn-sm">{isZh ? '上一页' : 'Previous'}</button><button type="button" disabled={page * 20 >= data.total} onClick={() => setPage(v => v + 1)} className="ui-btn-secondary ui-btn-sm">{isZh ? '下一页' : 'Next'}</button></div>}
  </div>;
}
