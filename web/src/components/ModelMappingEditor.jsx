import React, { useState, useEffect, useRef } from 'react';
import { ArrowRight, Plus, X, GitMerge, ChevronDown, ChevronUp } from 'lucide-react';

let idCounter = 0;
function nextId() {
  idCounter += 1;
  return `rule-${Date.now()}-${idCounter}`;
}

export default function ModelMappingEditor({ mappingStr = '', onChange, lang = 'zh' }) {
  const isZh = lang === 'zh';

  // Parse mappingStr (e.g. "gpt-4o:deepseek-chat, dept/*:*") into array of objects with stable IDs
  const parseMapping = (str) => {
    if (!str || !str.trim()) return [];
    return str
      .split(',')
      .map(part => {
        const colonIdx = part.indexOf(':');
        if (colonIdx === -1) return null;
        const from = part.slice(0, colonIdx).trim();
        const to = part.slice(colonIdx + 1).trim();
        if (!from && !to) return null;
        return {
          id: nextId(),
          from,
          to
        };
      })
      .filter(Boolean);
  };

  const serializeMapping = (list) => {
    return list
      .map(item => ({ from: (item.from || '').trim(), to: (item.to || '').trim() }))
      .filter(item => item.from && item.to)
      .map(item => `${item.from}:${item.to}`)
      .join(', ');
  };

  const [rows, setRows] = useState(() => parseMapping(mappingStr));
  const [isOpen, setIsOpen] = useState(() => Boolean(mappingStr && mappingStr.trim()));
  const lastSerializedRef = useRef(serializeMapping(rows));

  // Sync internal rows when parent passes a new mappingStr that didn't originate from our own onChange
  useEffect(() => {
    const trimmed = (mappingStr || '').trim();
    if (trimmed !== lastSerializedRef.current) {
      lastSerializedRef.current = trimmed;
      const parsed = parseMapping(trimmed);
      setRows(parsed);
      if (parsed.length > 0) {
        setIsOpen(true);
      }
    }
  }, [mappingStr]);

  const updateRowsAndNotify = (newRows) => {
    setRows(newRows);
    const serialized = serializeMapping(newRows);
    lastSerializedRef.current = serialized;
    onChange?.(serialized);
  };

  const handleAddRow = () => {
    const newRows = [...rows, { id: nextId(), from: '', to: '' }];
    updateRowsAndNotify(newRows);
    if (!isOpen) setIsOpen(true);
  };

  const handleRemoveRow = (id) => {
    const newRows = rows.filter(r => r.id !== id);
    updateRowsAndNotify(newRows);
  };

  const handleChangeRow = (id, field, value) => {
    const newRows = rows.map(r => (r.id === id ? { ...r, [field]: value } : r));
    updateRowsAndNotify(newRows);
  };

  const configuredCount = rows.filter(r => r.from.trim() && r.to.trim()).length;

  return (
    <div className="border border-slate-200 dark:border-slate-800 rounded-2xl overflow-hidden bg-white dark:bg-slate-900/40">
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className="w-full p-3 bg-slate-50/70 dark:bg-slate-900/60 flex items-center justify-between text-left hover:bg-slate-100/70 dark:hover:bg-slate-800/50 transition cursor-pointer"
      >
        <div className="flex items-center space-x-2">
          <GitMerge className="w-4 h-4 text-emerald-500" />
          <span className="text-xs font-semibold text-slate-800 dark:text-slate-200">
            {isZh ? '模型别名映射' : 'Model Alias Mapping'}
          </span>
          {configuredCount > 0 && (
            <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-50 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 font-medium border border-emerald-200/80 dark:border-emerald-800/60">
              {isZh ? `已配 ${configuredCount} 条` : `${configuredCount} configured`}
            </span>
          )}
          {rows.length > configuredCount && (
            <span className="text-xs text-amber-600 dark:text-amber-400 font-medium">
              {isZh ? `(${rows.length - configuredCount} 条待补全)` : `(${rows.length - configuredCount} draft)`}
            </span>
          )}
        </div>
        <div className="text-slate-400">
          {isOpen ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </div>
      </button>

      {isOpen && (
        <div className="p-3.5 space-y-3 border-t border-slate-200/80 dark:border-slate-800 bg-white dark:bg-[#111726]">
          {rows.length === 0 ? (
            <p className="ui-meta text-center py-2">
              {isZh ? '暂无映射规则。可将客户端请求模型名（如 gpt-4o）重定向到服务商实际模型名（如 deepseek-chat）' : 'No alias rules configured. Map incoming model names to upstream provider models.'}
            </p>
          ) : (
            <div className="space-y-2">
              {rows.map((row) => (
                <div key={row.id} className="flex items-center space-x-2">
                  <input
                    type="text"
                    placeholder={isZh ? "原模型 (如 gpt-4o)" : "Source Model (e.g. gpt-4o)"}
                    value={row.from}
                    onChange={(e) => handleChangeRow(row.id, 'from', e.target.value)}
                    className="ui-field ui-field-sm font-mono flex-1"
                  />
                  <ArrowRight className="w-3.5 h-3.5 text-slate-400 shrink-0" />
                  <input
                    type="text"
                    placeholder={isZh ? "映射模型 (如 deepseek-chat)" : "Target Model (e.g. deepseek-chat)"}
                    value={row.to}
                    onChange={(e) => handleChangeRow(row.id, 'to', e.target.value)}
                    className="ui-field ui-field-sm font-mono flex-1"
                  />
                  <button
                    type="button"
                    onClick={() => handleRemoveRow(row.id)}
                    className="p-1.5 text-slate-400 hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40 rounded-lg transition cursor-pointer"
                    title={isZh ? "删除该映射" : "Remove mapping"}
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                </div>
              ))}
            </div>
          )}

          <div className="flex items-center justify-between pt-1">
            <button
              type="button"
              onClick={handleAddRow}
              className="text-xs px-3 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-emerald-50 dark:hover:bg-emerald-950/50 text-slate-700 dark:text-slate-300 hover:text-emerald-600 dark:hover:text-emerald-400 border border-slate-200 dark:border-slate-700 transition flex items-center space-x-1 cursor-pointer"
            >
              <Plus className="w-3 h-3" />
              <span>{isZh ? '添加映射规则' : 'Add Mapping Rule'}</span>
            </button>
            <span className="ui-meta-sm">
              {isZh ? '支持 * 通配符，如 dept/*:*' : 'Supports wildcards, e.g. dept/*:*'}
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
