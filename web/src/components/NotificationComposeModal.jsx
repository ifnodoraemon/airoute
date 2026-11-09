import { useEffect, useRef, useState } from 'react';
import { Send, X } from 'lucide-react';
import useModalA11y from '../hooks/useModalA11y';
import useUnsavedGuard from '../hooks/useUnsavedGuard';
import DiscardChangesDialog from './DiscardChangesDialog';
import NotificationEditorFields from './NotificationEditorFields';
import { notificationJSON, notificationRequestID } from '../lib/notifications';

export default function NotificationComposeModal({ onClose, onSent, showToast, lang = 'zh', fetcher }) {
  const isZh = lang === 'zh';
  const [value, setValue] = useState({ recipient_username: '', title: '', content: '', level: 'info' });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const sending = useRef(false);
  const requestKey = useRef(notificationRequestID());
  const controller = useRef(null);
  const dirty = !!(value.recipient_username || value.title || value.content || value.level !== 'info');
  const guard = useUnsavedGuard({ isOpen: true, isDirty: dirty, onClose });
  const dismiss = () => { if (!sending.current) guard.requestClose(); };
  useModalA11y(true, dismiss);
  useEffect(() => () => controller.current?.abort(), []);
  const edit = (next) => { requestKey.current = notificationRequestID(); setValue(next); };
  const submit = async e => {
    e.preventDefault();
    if (sending.current) return;
    if (!value.recipient_username.trim() || !value.title.trim() || !value.content.trim()) {
      setError(isZh ? '请填写收件账号、标题和内容' : 'Recipient, title and content are required');
      return;
    }
    sending.current = true;
    setSubmitting(true);
    setError('');
    const request = new AbortController();
    controller.current = request;
    try {
      await notificationJSON('/api/v1/admin/notifications', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: request.signal,
        body: JSON.stringify({ ...value, recipient_username: value.recipient_username.trim(), title: value.title.trim(), content: value.content.trim(), request_id: requestKey.current }),
      }, fetcher);
      if (request.signal.aborted) return;
      showToast?.(isZh ? '个人消息已发送' : 'Message sent', 'success');
      onSent?.();
      onClose();
    } catch (e) {
      if (!request.signal.aborted) setError(e.message);
    } finally {
      sending.current = false;
      if (!request.signal.aborted) setSubmitting(false);
    }
  };
  return (
    <div className="fixed inset-0 z-50 bg-slate-900/60 backdrop-blur-sm flex items-center justify-center p-4" onClick={e => { if (e.target === e.currentTarget) dismiss(); }}>
      <div role="dialog" aria-modal="true" aria-labelledby="compose-notification-title" className="relative bg-white dark:bg-[#191B1F] rounded-2xl border border-[#E4E6E9] dark:border-[#2C3038] w-full max-w-xl max-h-[90dvh] overflow-y-auto shadow-2xl">
        <div className="px-6 py-4 border-b border-[#E4E6E9] dark:border-[#2C3038] flex items-start justify-between gap-3">
          <div><h3 id="compose-notification-title" className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4] inline-flex items-center gap-2"><Send className="w-4 h-4 text-[#355CFF]" />{isZh ? '发送个人消息' : 'Send a private message'}</h3><p className="text-xs text-[#656B75] dark:text-[#A1A7B0] mt-1.5">{isZh ? '仅收件账号可在“我的消息”中查看。' : 'Only the recipient can view this in My messages.'}</p></div>
          <button type="button" onClick={dismiss} disabled={submitting} aria-label={isZh ? '关闭' : 'Close'} className="p-1 text-[#656B75] cursor-pointer"><X className="w-4 h-4" /></button>
        </div>
        <form onSubmit={submit} className="p-6 space-y-5">
          {error && <p role="alert" className="text-xs text-rose-600 dark:text-rose-400">{error}</p>}
          <fieldset disabled={submitting} className="space-y-5">
            <div className="space-y-1.5"><label htmlFor="notification-recipient" className="text-xs font-medium text-slate-700 dark:text-slate-300">{isZh ? '收件账号' : 'Recipient username'}</label><input id="notification-recipient" autoFocus autoComplete="off" value={value.recipient_username} onChange={e => edit({ ...value, recipient_username: e.target.value })} maxLength={128} className="ui-field text-sm w-full" placeholder={isZh ? '填写平台用户名' : 'Enter the platform username'} /></div>
            <NotificationEditorFields value={value} onChange={edit} lang={lang} />
          </fieldset>
          <div className="flex items-center justify-end gap-2"><button type="button" onClick={dismiss} disabled={submitting} className="ui-btn-secondary ui-btn-sm">{isZh ? '取消' : 'Cancel'}</button><button type="submit" disabled={submitting} className="ui-btn-primary ui-btn-sm">{submitting ? (isZh ? '发送中…' : 'Sending…') : (isZh ? '发送消息' : 'Send message')}</button></div>
        </form>
        <DiscardChangesDialog open={guard.confirming} onKeepEditing={guard.keepEditing} onDiscard={guard.discard} lang={lang} />
      </div>
    </div>
  );
}
