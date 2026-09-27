import React, { useState } from 'react';
import { Tag, Plus, X, Trash2, List, Grid, Search, Sparkles } from 'lucide-react';

export default function ModelChipManager({ models = [], onChange }) {
  const [inputVal, setInputVal] = useState('');
  const [filterQuery, setFilterQuery] = useState('');
  const [isTextMode, setIsTextMode] = useState(false);

  // Normalize models to array
  const modelList = Array.isArray(models)
    ? models
    : (typeof models === 'string' ? models.split(',').map(s => s.trim()).filter(Boolean) : []);

  const handleAdd = (valToAdd) => {
    const raw = (valToAdd !== undefined ? valToAdd : inputVal).trim();
    if (!raw) return;

    // Support comma or newline separated pasting
    const newItems = raw
      .split(/[\n,]+/)
      .map(s => s.trim())
      .filter(Boolean);

    const merged = Array.from(new Set([...modelList, ...newItems]));
    onChange(merged);
    setInputVal('');
  };

  const handleRemove = (modelToRemove) => {
    const updated = modelList.filter(m => m !== modelToRemove);
    onChange(updated);
  };

  const handleClearAll = () => {
    if (modelList.length === 0) return;
    if (window.confirm(`确定清空已添加的 ${modelList.length} 个模型？`)) {
      onChange([]);
    }
  };

  const handleTextareaChange = (e) => {
    const lines = e.target.value
      .split(/[\n,]+/)
      .map(s => s.trim())
      .filter(Boolean);
    onChange(Array.from(new Set(lines)));
  };

  const filteredList = modelList.filter(m =>
    m.toLowerCase().includes(filterQuery.toLowerCase())
  );

  return (
    <div className="space-y-2.5">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <label className="text-xs font-semibold text-slate-700 dark:text-slate-300 flex items-center space-x-1.5">
            <Tag className="w-3.5 h-3.5 text-indigo-500" />
            <span>支持模型列表</span>
          </label>
          <span className={`text-[11px] px-2 py-0.5 rounded-full font-medium ${
            modelList.length > 0
              ? 'bg-indigo-50 dark:bg-indigo-950/60 text-indigo-600 dark:text-indigo-400 border border-indigo-200/80 dark:border-indigo-800/60'
              : 'bg-slate-100 dark:bg-slate-800 text-slate-500'
          }`}>
            {modelList.length > 0 ? `已配置 ${modelList.length} 个模型` : '暂无模型'}
          </span>
        </div>

        <div className="flex items-center space-x-2">
          {modelList.length > 0 && (
            <button
              type="button"
              onClick={handleClearAll}
              className="text-[11px] text-rose-500 hover:text-rose-700 transition flex items-center space-x-1"
            >
              <Trash2 className="w-3 h-3" />
              <span>清空</span>
            </button>
          )}
          <button
            type="button"
            onClick={() => setIsTextMode(!isTextMode)}
            className="text-[11px] text-indigo-600 dark:text-indigo-400 hover:underline flex items-center space-x-1"
          >
            {isTextMode ? <Grid className="w-3 h-3" /> : <List className="w-3 h-3" />}
            <span>{isTextMode ? '切换标签视图' : '批量文本编辑'}</span>
          </button>
        </div>
      </div>

      {isTextMode ? (
        <div>
          <textarea
            rows={4}
            value={modelList.join('\n')}
            onChange={handleTextareaChange}
            placeholder="每行一个模型名称，或用逗号隔开..."
            className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl p-3 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500 leading-relaxed"
          />
        </div>
      ) : (
        <div className="space-y-2">
          {/* Tag Container */}
          <div className="min-h-[76px] max-h-40 overflow-y-auto p-2.5 rounded-2xl bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex flex-wrap gap-1.5 items-start content-start">
            {modelList.length === 0 ? (
              <div className="w-full py-4 text-center text-xs text-slate-400 flex flex-col items-center justify-center space-y-1">
                <span>暂无模型</span>
              </div>
            ) : (
              filteredList.map((m) => (
                <span
                  key={m}
                  className="group inline-flex items-center space-x-1 px-2.5 py-1 rounded-lg bg-white dark:bg-slate-800 border border-slate-200/90 dark:border-slate-700/80 text-xs font-mono text-slate-800 dark:text-slate-200 shadow-2xs hover:border-indigo-400 transition"
                >
                  <span>{m}</span>
                  <button
                    type="button"
                    onClick={() => handleRemove(m)}
                    className="text-slate-400 hover:text-rose-500 dark:hover:text-rose-400 transition p-0.5 rounded-sm"
                    title={`移除 ${m}`}
                  >
                    <X className="w-3 h-3" />
                  </button>
                </span>
              ))
            )}
          </div>

          {/* Search filter if models count > 6 */}
          {modelList.length > 6 && (
            <div className="relative">
              <Search className="w-3 h-3 text-slate-400 absolute left-2.5 top-2.5" />
              <input
                type="text"
                value={filterQuery}
                onChange={(e) => setFilterQuery(e.target.value)}
                placeholder="在已添加的模型中过滤..."
                className="w-full bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl pl-8 pr-3 py-1.5 text-xs text-slate-800 dark:text-slate-200 focus:outline-none focus:border-indigo-500"
              />
            </div>
          )}

          {/* Manual Add Input */}
          <div className="flex space-x-2 pt-0.5">
            <input
              type="text"
              value={inputVal}
              onChange={(e) => setInputVal(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleAdd();
                }
              }}
              placeholder="模型名称 (回车添加)"
              className="flex-1 bg-slate-50 dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-800 dark:text-slate-100 focus:outline-none focus:border-indigo-500"
            />
            <button
              type="button"
              onClick={() => handleAdd()}
              className="px-3.5 py-2 bg-slate-100 dark:bg-slate-800 hover:bg-indigo-50 dark:hover:bg-indigo-950/60 text-slate-700 dark:text-slate-300 hover:text-indigo-600 dark:hover:text-indigo-300 border border-slate-200 dark:border-slate-700 rounded-xl text-xs font-medium flex items-center space-x-1 transition shrink-0"
            >
              <Plus className="w-3.5 h-3.5" />
              <span>添加</span>
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
