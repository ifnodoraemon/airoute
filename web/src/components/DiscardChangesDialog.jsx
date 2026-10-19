import React, { useEffect, useRef } from 'react';
import { AlertTriangle } from 'lucide-react';

/**
 * Confirmation shown when a form with unsaved edits is dismissed.
 * Rendered inside the owning modal so it inherits its stacking context.
 */
export default function DiscardChangesDialog({
  open,
  onKeepEditing,
  onDiscard,
  lang = 'zh',
  title,
  description,
}) {
  const keepRef = useRef(null);

  useEffect(() => {
    if (open) keepRef.current?.focus();
  }, [open]);

  if (!open) return null;
  const isZh = lang === 'zh';

  return (
    <div
      className="absolute inset-0 z-30 flex items-center justify-center bg-slate-900/50 backdrop-blur-sm p-4 animate-in fade-in"
      onClick={(e) => {
        if (e.target === e.currentTarget) onKeepEditing?.();
      }}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="discard-changes-title"
        className="w-full max-w-sm rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-[#0F1420] p-5 shadow-2xl animate-in zoom-in-95 duration-150"
      >
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-amber-100 text-amber-600 dark:bg-amber-950/60 dark:text-amber-400">
            <AlertTriangle className="h-4 w-4" />
          </span>
          <div className="min-w-0">
            <h4 id="discard-changes-title" className="text-sm font-semibold text-slate-900 dark:text-white">
              {title || (isZh ? '放弃未保存的修改？' : 'Discard unsaved changes?')}
            </h4>
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400 leading-relaxed">
              {description ||
                (isZh
                  ? '当前表单还有未保存的内容，关闭后这些修改将会丢失。'
                  : 'This form has unsaved edits. Closing it will discard them.')}
            </p>
          </div>
        </div>

        <div className="mt-5 flex justify-end gap-2">
          <button type="button" ref={keepRef} onClick={onKeepEditing} className="ui-btn-secondary">
            {isZh ? '继续编辑' : 'Keep editing'}
          </button>
          <button type="button" onClick={onDiscard} className="ui-btn-danger">
            {isZh ? '放弃并关闭' : 'Discard & close'}
          </button>
        </div>
      </div>
    </div>
  );
}
