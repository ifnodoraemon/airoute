import { useEffect, useRef, useState } from 'react';
import { AlertTriangle } from 'lucide-react';
import { useModalA11y } from '../hooks/useModalA11y';
import { translations } from '../i18n';

// Reusable destructive-action confirmation. Replaces the native window.confirm
// (which is off-brand, suppressible by the browser, and cannot show impact or a
// type-to-confirm guard). For irreversible operations pass requireText so the
// operator must type the resource name before the confirm button enables.
export default function ConfirmDialog({
  open, isZh, title, description, confirmLabel, cancelLabel,
  danger = true, requireText, onConfirm, onCancel,
}) {
  const confirmRef = useRef(null);
  const copy = translations[isZh ? 'zh' : 'en'].common;
  const [typed, setTyped] = useState('');
  useModalA11y(open, onCancel);
  useEffect(() => {
    if (open) {
      setTyped('');
      confirmRef.current?.focus();
    }
  }, [open]);
  if (!open) return null;
  const blocked = Boolean(requireText) && typed !== requireText;
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 backdrop-blur-sm p-4"
      onClick={(e) => { if (e.target === e.currentTarget) onCancel?.(); }}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        className="w-full max-w-sm rounded-2xl border border-[#E4E6E9] dark:border-[#2C3038] bg-white dark:bg-[#191B1F] p-5 shadow-2xl"
      >
        <div className="flex items-start gap-3">
          <span className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border ${danger ? 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border-rose-500/30' : 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/30'}`}>
            <AlertTriangle className="h-4 w-4" />
          </span>
          <div className="min-w-0">
            <h4 id="confirm-dialog-title" className="text-sm font-semibold text-[#202124] dark:text-[#F1F2F4]">{title}</h4>
            {description && <p className="mt-1 text-xs text-[#656B75] dark:text-[#A1A7B0] leading-relaxed">{description}</p>}
          </div>
        </div>
        {requireText && (
          <input
            className="ui-field mt-4"
            aria-label={copy.typeToConfirm.replace('{name}', requireText)}
            placeholder={requireText}
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        )}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" onClick={onCancel} className="ui-btn-secondary">
            {cancelLabel || copy.cancel}
          </button>
          <button type="button" ref={confirmRef} disabled={blocked} onClick={onConfirm} className={danger ? 'ui-btn-danger' : 'ui-btn-primary'}>
            {confirmLabel || copy.confirm}
          </button>
        </div>
      </div>
    </div>
  );
}
