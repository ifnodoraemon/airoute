import { useState } from 'react';
import { notificationJSON } from '../lib/notifications';

export default function AnnouncementActions({ id, onEdit, onChanged, lang, fetcher }) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const isZh = lang === 'zh';
  const remove = async () => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      await notificationJSON(`/api/v1/admin/announcements/${encodeURIComponent(id)}`, { method: 'DELETE' }, fetcher);
      onChanged();
    } catch (e) { setError(e.message); setBusy(false); }
  };
  return <div className="mt-4 pt-3 border-t border-[#E4E6E9] dark:border-[#2C3038] text-xs">
    {error && <p role="alert" className="text-rose-600 mb-2">{error}</p>}
    <div className="flex items-center flex-wrap gap-3">
      {confirming ? <><span className="text-[#656B75] dark:text-[#A1A7B0]">{isZh ? '删除后将从全站移除，确认删除？' : 'Remove this announcement for everyone?'}</span><button type="button" disabled={busy} onClick={remove} className="text-rose-600 cursor-pointer">{isZh ? '确认删除' : 'Delete'}</button><button type="button" disabled={busy} onClick={() => setConfirming(false)} className="text-[#656B75] cursor-pointer">{isZh ? '取消' : 'Cancel'}</button></>
        : <><button type="button" onClick={() => onEdit(id)} className="text-[#355CFF] dark:text-[#91A7FF] cursor-pointer">{isZh ? '编辑公告' : 'Edit'}</button><button type="button" onClick={() => setConfirming(true)} className="text-rose-600 cursor-pointer">{isZh ? '删除公告' : 'Delete'}</button></>}
    </div>
  </div>;
}
