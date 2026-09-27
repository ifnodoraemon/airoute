import React from 'react';
import { CheckCircle2, AlertCircle, Info, X } from 'lucide-react';

export default function Toast({ toast, onClose }) {
  if (!toast || !toast.show) return null;

  const isSuccess = toast.type === 'success';
  const isError = toast.type === 'error';
  const isWarning = toast.type === 'warning';

  return (
    <div className="fixed bottom-6 right-6 z-[9999] flex items-center space-x-3 px-4 py-3 rounded-2xl shadow-2xl border backdrop-blur-xl transition-all duration-300 animate-in slide-in-from-bottom-5 bg-white/95 dark:bg-[#131b2e]/95 border-slate-200 dark:border-slate-700/80 text-slate-800 dark:text-slate-100 max-w-md">
      <div className="shrink-0">
        {isSuccess && <CheckCircle2 className="w-5 h-5 text-emerald-500" />}
        {isError && <AlertCircle className="w-5 h-5 text-rose-500" />}
        {isWarning && <AlertCircle className="w-5 h-5 text-amber-500" />}
        {!isSuccess && !isError && !isWarning && <Info className="w-5 h-5 text-indigo-500" />}
      </div>
      <div className="flex-1 text-xs font-medium leading-relaxed">
        {toast.message}
      </div>
      <button
        onClick={onClose}
        className="p-1 hover:bg-slate-100 dark:hover:bg-slate-800 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 transition"
      >
        <X className="w-3.5 h-3.5" />
      </button>
    </div>
  );
}
