import { useEffect, useRef, useState } from 'react';
import { Megaphone, X } from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';
import useUnsavedGuard from '../hooks/useUnsavedGuard';
import DiscardChangesDialog from './DiscardChangesDialog';
import NotificationEditorFields from './NotificationEditorFields';
import { notificationJSON, notificationRequestID } from '../lib/notifications';

const EMPTY = { title: '', content: '', level: 'info', show_banner: false };

export default function AnnouncementModal({ isOpen, announcementId = null, onClose, adminFetch, showToast, lang = 'zh', onPublished }) {
  const isZh = lang === 'zh';
  const [value, setValue] = useState(EMPTY);
  const [initial, setInitial] = useState(EMPTY);
  const [published, setPublished] = useState(false);
  const [loading, setLoading] = useState(!!announcementId);
  const [loadError, setLoadError] = useState('');
  const [saveError, setSaveError] = useState('');
  const [reload, setReload] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const sending = useRef(false);
  const requestKey = useRef(notificationRequestID());
  const submitController = useRef(null);
  const guard = useUnsavedGuard({ isOpen, isDirty: !loading && JSON.stringify(value) !== JSON.stringify(initial), onClose });
  const dismiss = () => { if (!sending.current) guard.requestClose(); };
  useModalA11y(isOpen, dismiss);

  useEffect(() => {
    if (!isOpen) return undefined;
    const controller = new AbortController();
    setLoadError('');
    setSaveError('');
    const load = async () => {
      setLoading(true);
      try {
        const data = await notificationJSON(`/api/v1/admin/announcements/${encodeURIComponent(announcementId)}`, { signal: controller.signal }, adminFetch);
        if (!data || typeof data.title !== 'string' || typeof data.content !== 'string' || typeof data.show_banner !== 'boolean') throw new Error('公告数据无效 / Invalid announcement');
        if (controller.signal.aborted) return;
        const next = { title: data.title, content: data.content, level: data.level, show_banner: data.show_banner };
        setValue(next);
        setInitial(next);
        setPublished(data.status === 'published');
      } catch (e) {
        if (!controller.signal.aborted) setLoadError(e.message);
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    if (announcementId) load();
    else { setValue(EMPTY); setInitial(EMPTY); setLoading(false); setPublished(false); }
    return () => { controller.abort(); submitController.current?.abort(); };
  }, [isOpen, announcementId, adminFetch, reload]);

  const edit = next => { requestKey.current = notificationRequestID(); setValue(next); };
  const submit = async (e, status) => {
    e.preventDefault();
    if (loading || loadError || sending.current) return;
    if (status === 'published' && (!value.title.trim() || !value.content.trim())) {
      setSaveError(isZh ? '发布时标题和内容不可为空' : 'Title and content are required when publishing');
      return;
    }
    sending.current = true;
    setSubmitting(true);
    setSaveError('');
    const request = new AbortController();
    submitController.current = request;
    try {
      const url = '/api/v1/admin/announcements' + (announcementId ? `/${encodeURIComponent(announcementId)}` : '');
      await notificationJSON(url, {
        method: announcementId ? 'PUT' : 'POST', headers: { 'Content-Type': 'application/json' }, signal: request.signal,
        body: JSON.stringify({ ...value, title: value.title.trim(), content: value.content.trim(), status, request_id: requestKey.current }),
      }, adminFetch);
      if (request.signal.aborted) return;
      showToast?.(status === 'published' ? (isZh ? '公告已发布' : 'Announcement published') : (isZh ? '公告已保存为草稿' : 'Draft saved'), 'success');
      onPublished?.();
      onClose();
    } catch (e) {
      if (!request.signal.aborted) setSaveError(e.message);
    } finally {
      sending.current = false;
      if (!request.signal.aborted) setSubmitting(false);
    }
  };

  if (!isOpen) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/60 backdrop-blur-sm" onClick={e => { if (e.target === e.currentTarget) dismiss(); }}>
      <div role="dialog" aria-modal="true" aria-labelledby="announcement-editor-title" className="relative bg-white dark:bg-[#191B1F] rounded-2xl max-w-xl w-full max-h-[90dvh] overflow-y-auto border border-[#E4E6E9] dark:border-[#2C3038] shadow-2xl">
        <div className="px-6 py-4 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-start justify-between gap-3">
          <div><h3 id="announcement-editor-title" className="font-semibold text-sm text-[#202124] dark:text-[#F1F2F4] flex items-center gap-2"><Megaphone className="w-4 h-4 text-[#355CFF]" />{announcementId ? (isZh ? '编辑公告' : 'Edit announcement') : (isZh ? '发布全站公告' : 'Publish announcement')}</h3>
            <p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-2">{isZh ? '每条公告独立保存。发布后进入全站时间线，草稿仅管理员可见。' : 'Posts are managed individually. Published posts appear on the timeline; drafts are visible to administrators.'}</p></div>
          <button type="button" onClick={dismiss} disabled={submitting} aria-label={isZh ? '关闭' : 'Close'} className="p-1 text-[#656B75] cursor-pointer"><X className="w-4 h-4" /></button>
        </div>
        <form onSubmit={e => submit(e, 'published')} className="p-6 space-y-5">
          {loading && <p role="status" className="text-xs text-[#656B75]">{isZh ? '正在加载公告…' : 'Loading announcement…'}</p>}
          {loadError && <div role="alert" className="text-xs text-rose-600 flex justify-between gap-2">{loadError}<button type="button" onClick={() => setReload(v => v + 1)} className="underline cursor-pointer">{isZh ? '重试' : 'Retry'}</button></div>}
          {saveError && <p role="alert" className="text-xs text-rose-600">{saveError}</p>}
          <fieldset disabled={loading || !!loadError || submitting} className="space-y-5">
            <NotificationEditorFields value={value} onChange={edit} lang={lang} announcement />
            <label className="flex items-center gap-2 text-xs text-[#656B75] dark:text-[#A1A7B0] cursor-pointer"><input type="checkbox" checked={value.show_banner} onChange={e => edit({ ...value, show_banner: e.target.checked })} className="accent-[#355CFF]" />{isZh ? '同时在页面顶部显示横幅提醒' : 'Also show a banner at the top of the page'}</label>
            <div className="pt-2 flex items-center justify-end gap-2 flex-wrap">
              <button type="button" onClick={dismiss} className="ui-btn-secondary ui-btn-sm">{isZh ? '取消' : 'Cancel'}</button>
              <button type="button" onClick={e => submit(e, 'draft')} className="ui-btn-secondary ui-btn-sm">{published ? (isZh ? '撤回为草稿' : 'Withdraw to draft') : (isZh ? '保存草稿' : 'Save draft')}</button>
              <button type="submit" className="ui-btn-primary ui-btn-sm">{submitting ? (isZh ? '保存中…' : 'Saving…') : announcementId && published ? (isZh ? '更新已发布公告' : 'Update announcement') : (isZh ? '发布全站公告' : 'Publish announcement')}</button>
            </div>
          </fieldset>
        </form>
        <DiscardChangesDialog open={guard.confirming} onKeepEditing={guard.keepEditing} onDiscard={guard.discard} lang={lang} />
      </div>
    </div>
  );
}
