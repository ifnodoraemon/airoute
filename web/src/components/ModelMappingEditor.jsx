import React, { useState } from 'react';
import { ArrowRight, Plus, X, GitMerge, ChevronDown, ChevronUp } from 'lucide-react';

export default function ModelMappingEditor({ mappingStr = '', onChange }) {
  const [isOpen, setIsOpen] = useState(false);

  // Parse mappingStr (e.g. "gpt-4o:deepseek-chat,dept/*:*") into array of objects
  const parseMapping = (str) => {
    if (!str || !str.trim()) return [];
    return str
      .split(',')
      .map(part => {
        const [from, to] = part.split(':').map(s => s.trim());
        return from && to ? { from, to } : null;
      })
      .filter(Boolean);
  };

  const serializeMapping = (list) => {
    return list
      .filter(item => item.from && item.to)
      .map(item => `${item.from.trim()}:${item.to.trim()}`)
      .join(', ');
  };

  const currentList = parseMapping(mappingStr);

  const handleUpdate = (newList) => {
    onChange(serializeMapping(newList));
  };

  const handleAddRow = () => {
    handleUpdate([...currentList, { from: '', to: '' }]);
    if (!isOpen) setIsOpen(true);
  };

  const handleRemoveRow = (index) => {
    const updated = currentList.filter((_, i) => i !== index);
    handleUpdate(updated);
  };

  const handleChangeRow = (index, field, value) => {
    const updated = currentList.map((item, i) => {
      if (i === index) {
        return { ...item, [field]: value };
      }
      return item;
    });
    handleUpdate(updated);
  };

  return (
    <div className="border border-slate-200 dark:border-slate-800 rounded-2xl overflow-hidden bg-white dark:bg-slate-900/40">
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className="w-full p-3 bg-slate-50/70 dark:bg-slate-900/60 flex items-center justify-between text-left hover:bg-slate-100/70 dark:hover:bg-slate-800/50 transition"
      >
        <div className="flex items-center space-x-2">
          <GitMerge className="w-4 h-4 text-emerald-500" />
          <span className="text-xs font-semibold text-slate-800 dark:text-slate-200">
            模型别名映射
          </span>
          {currentList.length > 0 && (
            <span className="text-[11px] px-2 py-0.5 rounded-full bg-emerald-50 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 font-medium border border-emerald-200/80 dark:border-emerald-800/60">
              已配 {currentList.length} 条
            </span>
          )}
        </div>
        <div className="text-slate-400">
          {isOpen ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </div>
      </button>

      {isOpen && (
        <div className="p-3.5 space-y-3 border-t border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#111726]">
          <div className="space-y-2">
            {currentList.map((row, idx) => (
              <div key={idx} className="flex items-center space-x-2">
                <input
                  type="text"
                  placeholder="原模型 (如 gpt-4o)"
                  value={row.from}
                  onChange={(e) => handleChangeRow(idx, 'from', e.target.value)}
                  className="flex-1 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 text-xs font-mono text-slate-800 dark:text-slate-200 focus:outline-none focus:border-indigo-500"
                />
                <ArrowRight className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                <input
                  type="text"
                  placeholder="映射模型 (如 deepseek-chat)"
                  value={row.to}
                  onChange={(e) => handleChangeRow(idx, 'to', e.target.value)}
                  className="flex-1 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-2.5 py-1.5 text-xs font-mono text-slate-800 dark:text-slate-200 focus:outline-none focus:border-indigo-500"
                />
                <button
                  type="button"
                  onClick={() => handleRemoveRow(idx)}
                  className="p-1 text-slate-400 hover:text-rose-500 transition"
                  title="删除该映射"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
          </div>

          <button
            type="button"
            onClick={handleAddRow}
            className="text-xs px-3 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-emerald-50 dark:hover:bg-emerald-950/50 text-slate-700 dark:text-slate-300 hover:text-emerald-600 dark:hover:text-emerald-400 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1"
          >
            <Plus className="w-3 h-3" />
            <span>添加映射规则</span>
          </button>
        </div>
      )}
    </div>
  );
}
